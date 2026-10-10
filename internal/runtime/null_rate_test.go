package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"

	"github.com/compuficial/apery/internal/plan"
)

// decodeRows parses JSONL output into one map per record.
func decodeRows(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var rows []map[string]any
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		var row map[string]any
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatalf("decode %q: %v", sc.Text(), err)
		}
		rows = append(rows, row)
	}
	return rows
}

// nullablePlan has a nullable column x, a template that reads it, and plain
// columns on either side.
func nullablePlan(rate float64) *plan.Plan {
	return singleEntity(10_000,
		plan.FieldSpec{Name: "id", Gen: "seq"},
		plan.FieldSpec{Name: "x", Gen: "int", Config: map[string]any{"min": 0, "max": 1000}, NullRate: rate},
		plan.FieldSpec{Name: "seq_n", Gen: "seq", NullRate: rate},
		plan.FieldSpec{Name: "label", Gen: "template", Config: map[string]any{"tpl": "x={x}"}},
		plan.FieldSpec{Name: "y", Gen: "float"},
	)
}

// null_rate only blanks values: rows that aren't null match the plan without
// it, other columns are untouched, and the rate holds.
func TestNullRate_OnlyBlanksValues(t *testing.T) {
	base := decodeRows(t, runPlanWithOpts(t, nullablePlan(0)))
	nullable := decodeRows(t, runPlanWithOpts(t, nullablePlan(0.3)))

	nulls := 0
	for i, row := range nullable {
		want := base[i]
		for _, col := range []string{"id", "y"} {
			if row[col] != want[col] {
				t.Fatalf("row %d: %s = %v, want %v", i, col, row[col], want[col])
			}
		}
		for _, col := range []string{"x", "seq_n"} {
			if row[col] != nil && row[col] != want[col] {
				t.Fatalf("row %d: %s = %v, want %v or null", i, col, row[col], want[col])
			}
		}
		if row["x"] == nil {
			nulls++
			if row["label"] != "x=" {
				t.Fatalf("row %d: label = %v, want x= for a null x", i, row["label"])
			}
		}
	}
	if rate := float64(nulls) / float64(len(nullable)); rate < 0.28 || rate > 0.32 {
		t.Errorf("null rate = %.3f, want about 0.3", rate)
	}
}

// Null decisions depend only on the row, not on chunking or workers.
func TestNullRate_ChunkIndependent(t *testing.T) {
	want := runPlanWithOpts(t, nullablePlan(0.3), WithWorkers(1), WithChunkSize(10_000))
	for _, opts := range [][]Option{
		{WithWorkers(4), WithChunkSize(7)},
		{WithWorkers(8), WithChunkSize(1000)},
	} {
		if got := runPlanWithOpts(t, nullablePlan(0.3), opts...); !bytes.Equal(got, want) {
			t.Fatal("output changed with chunking")
		}
	}
}

func TestNullRate_One(t *testing.T) {
	for i, row := range decodeRows(t, runPlanWithOpts(t, nullablePlan(1))) {
		if row["x"] != nil || row["seq_n"] != nil {
			t.Fatalf("row %d: %v, want x and seq_n null", i, row)
		}
	}
}

// Downstream entities see nulls: rel_ref skips them, expr propagates them,
// and driven_by exposes them.
func TestNullRate_DownstreamEntities(t *testing.T) {
	p := &plan.Plan{Seed: 7, Entities: []plan.EntitySpec{
		{Name: "User", Count: 200, Fields: []plan.FieldSpec{
			{Name: "id", Gen: "seq"},
			{Name: "ref", Gen: "seq", Config: map[string]any{"start": 1000}, NullRate: 0.5},
			{Name: "score", Gen: "int", Config: map[string]any{"min": 1, "max": 9}, NullRate: 0.4},
		}},
		{Name: "Pick", Count: 500, Fields: []plan.FieldSpec{
			{Name: "ref", Gen: "rel_ref", Config: map[string]any{"entity": "User", "field": "ref"}},
		}},
		{Name: "Unique", Count: 50, Fields: []plan.FieldSpec{
			{Name: "ref", Gen: "rel_ref", Config: map[string]any{"entity": "User", "field": "ref", "unique": true}},
		}},
		{Name: "Order", DrivenBy: &plan.DrivenBy{Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 1,
			Expose: []plan.ParentField{{Field: "score"}}},
			Fields: []plan.FieldSpec{{Name: "double", Gen: "expr", Config: map[string]any{"expr": "{score} * 2"}}}},
	}}

	users := map[any]bool{}
	seen := map[any]bool{}
	for _, row := range decodeRows(t, runPlanWithOpts(t, p)) {
		switch row["_entity"] {
		case "User":
			users[row["ref"]] = true
		case "Pick", "Unique":
			if row["ref"] == nil || !users[row["ref"]] {
				t.Fatalf("%s drew %v, want a non-null User.ref", row["_entity"], row["ref"])
			}
			if row["_entity"] == "Unique" {
				if seen[row["ref"]] {
					t.Fatalf("Unique repeated %v", row["ref"])
				}
				seen[row["ref"]] = true
			}
		case "Order":
			if (row["score"] == nil) != (row["double"] == nil) {
				t.Fatalf("Order %v: double must be null exactly when score is", row)
			}
		}
	}
}
