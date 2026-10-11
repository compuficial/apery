package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/compuficial/apery/internal/plan"
)

// sequentialPlan has time_seq with exponential gaps, whose values build on
// the row before, in a standalone entity and in a driven_by entity beside
// unique draws, which reset per parent. Purchase reads the standalone one.
func sequentialPlan() *plan.Plan {
	arrivals := plan.FieldSpec{Name: "at", Gen: "time_seq", Config: map[string]any{
		"start": "2024-01-01T00:00:00Z", "step": "1m", "gaps": "exponential", "format": time.RFC3339Nano,
	}}
	return &plan.Plan{
		Seed: 7,
		Entities: []plan.EntitySpec{
			{Name: "Customer", Count: 40, Fields: []plan.FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Product", Count: 10, Fields: []plan.FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Visit", Count: 200, Fields: []plan.FieldSpec{arrivals}},
			{Name: "Bookmark", DrivenBy: &plan.DrivenBy{
				Entity: "Customer", Field: "id", As: "customer_id", Min: 1, Max: 6,
			}, Fields: []plan.FieldSpec{
				{Name: "product_id", Gen: "rel_ref", Config: map[string]any{
					"entity": "Product", "field": "id", "unique": true,
				}},
				arrivals,
			}},
			{Name: "Purchase", Count: 50, Fields: []plan.FieldSpec{
				{Name: "visited_at", Gen: "rel_ref", Config: map[string]any{"entity": "Visit", "field": "at"}},
			}},
		},
	}
}

// Any chunking gives the output of one chunk on one worker.
func TestSequential_ChunkingIndependent(t *testing.T) {
	p := sequentialPlan()
	want := runPlanWithOpts(t, p, WithWorkers(1), WithChunkSize(10_000))
	for _, chunkSize := range []int64{1, 7, 64} {
		if got := runPlanWithOpts(t, p, WithWorkers(4), WithChunkSize(chunkSize)); !bytes.Equal(got, want) {
			path := dumpOutput(fmt.Sprintf("sequential.chunk%d", chunkSize), got)
			t.Fatalf("chunk size %d: output differs from one chunk; see %s", chunkSize, path)
		}
	}
}

// Writing only Purchase still generates its dependency Visit in order.
func TestSequential_AsDependency(t *testing.T) {
	p := sequentialPlan()
	want := linesOf(t, runPlanWithOpts(t, p), []string{"Purchase"})
	if got := runPlanWithOpts(t, p, WithEntities("Purchase"), WithWorkers(4), WithChunkSize(7)); !bytes.Equal(got, want) {
		t.Fatalf("Purchase alone: %d bytes, want %d matching the full run", len(got), len(want))
	}
}

// Each entity is one timeline from start, across driven_by parents too, and
// each parent's unique draws stay distinct.
func TestSequential_OneTimelinePerEntity(t *testing.T) {
	out := runPlanWithOpts(t, sequentialPlan(), WithWorkers(4), WithChunkSize(7))

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	last := map[string]time.Time{}
	drawn := map[string]bool{} // "customer/product" pairs
	for line := range bytes.Lines(out) {
		var rec struct {
			Entity   string  `json:"_entity"`
			At       string  `json:"at"`
			Customer float64 `json:"customer_id"`
			Product  float64 `json:"product_id"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		if rec.At == "" {
			continue
		}

		at, err := time.Parse(time.RFC3339Nano, rec.At)
		if err != nil {
			t.Fatal(err)
		}
		prev, seen := last[rec.Entity]
		switch {
		case !seen && !at.Equal(start):
			t.Errorf("%s starts at %v, want %v", rec.Entity, at, start)
		case seen && at.Before(prev):
			t.Errorf("%s: %v comes after %v", rec.Entity, at, prev)
		}
		last[rec.Entity] = at

		if rec.Entity == "Bookmark" {
			pair := fmt.Sprintf("%v/%v", rec.Customer, rec.Product)
			if drawn[pair] {
				t.Errorf("customer %v drew product %v twice", rec.Customer, rec.Product)
			}
			drawn[pair] = true
		}
	}
	if len(last) != 2 {
		t.Fatalf("timelines for %v, want Visit and Bookmark", last)
	}
}
