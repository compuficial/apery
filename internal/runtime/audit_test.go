package runtime

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/registry"
	"github.com/compuficial/apery/internal/rng"
	"github.com/compuficial/apery/internal/writer"
)

// generated counts values produced by the __test_counter generator.
var generated atomic.Int64

type counterGen struct{}

func (counterGen) Next(_ *rng.Rng) (any, error) { return generated.Add(1), nil }

func init() {
	registry.MustRegister("__test_counter", func(map[string]any) (registry.Generator, error) {
		return counterGen{}, nil
	})
}

// runWithTimeout fails the test instead of hanging when Run deadlocks.
func runWithTimeout(t *testing.T, ctx context.Context, e *Executor, p *plan.Plan) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx, p) }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return within 10s (deadlock)")
		return nil
	}
}

func singleEntity(count int64, fields ...plan.FieldSpec) *plan.Plan {
	return &plan.Plan{Seed: 1, Entities: []plan.EntitySpec{{Name: "U", Count: count, Fields: fields}}}
}

// A failing row with more chunks than workers used to block the dispatcher
// forever once every worker had exited.
func TestRun_ErrorWithMoreChunksThanWorkers(t *testing.T) {
	p := singleEntity(10,
		plan.FieldSpec{Name: "z", Gen: "int", Config: map[string]any{"min": 0, "max": 0}},
		plan.FieldSpec{Name: "q", Gen: "expr", Config: map[string]any{"expr": "1 / {z}"}},
	)
	for _, workers := range []int{1, 2} {
		err := runWithTimeout(t, context.Background(), New(&stubWriter{}, WithWorkers(workers), WithChunkSize(2)), p)
		if err == nil || !strings.Contains(err.Error(), "division by zero") {
			t.Fatalf("workers=%d: error = %v, want division by zero", workers, err)
		}
	}
}

// Cancelling mid-run (Ctrl+C) must stop generation promptly.
func TestRun_CancelMidRun(t *testing.T) {
	p := singleEntity(50_000_000, plan.FieldSpec{Name: "id", Gen: "seq"})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	err := runWithTimeout(t, ctx, New(&stubWriter{}, WithWorkers(2), WithChunkSize(1000)), p)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// Generator problems anywhere in the plan fail before any record is written.
func TestRun_ValidatesGeneratorsBeforeWriting(t *testing.T) {
	p := &plan.Plan{Seed: 1, Entities: []plan.EntitySpec{
		{Name: "A", Count: 5, Fields: []plan.FieldSpec{{Name: "id", Gen: "seq"}}},
		{Name: "B", Count: 5, Fields: []plan.FieldSpec{{Name: "x", Gen: "nosuchgen"}}},
	}}
	w := &recordWriter{}
	err := New(w).Run(context.Background(), p)
	if _, ok := errors.AsType[*plan.ValidationError](err); !ok {
		t.Fatalf("error %v is not a *plan.ValidationError", err)
	}
	if len(w.records) != 0 {
		t.Fatalf("%d records written before validation failed", len(w.records))
	}
}

// Validate builds every generator and reports each problem with a code.
func TestValidate_GeneratorIssues(t *testing.T) {
	p := &plan.Plan{Seed: 1, Entities: []plan.EntitySpec{{Name: "U", Count: 1, Fields: []plan.FieldSpec{
		{Name: "a", Gen: "uuidd"},
		{Name: "b", Gen: "int", Config: map[string]any{"min": 10, "max": 1}},
		{Name: "c", Gen: "int", Config: map[string]any{"mn": 1}},
		{Name: "d", Gen: "template", Config: map[string]any{"tpl": "{e}"}},
		{Name: "e", Gen: "seq"},
		{Name: "f", Gen: "object", Config: map[string]any{"fields": map[string]any{"n": map[string]any{"gen": "seq"}}}},
	}}}}

	var verr *plan.ValidationError
	if err := Validate(p); !errors.As(err, &verr) {
		t.Fatalf("error %v is not a *plan.ValidationError", err)
	}

	want := []struct{ field, code, hint string }{
		{"a", "unknown_generator", `did you mean "uuid"`},
		{"b", "invalid_config", "apery describe generator int"},
		{"c", "unknown_config_key", "valid keys: min, max"},
		{"d", "dependency_order", ""},
		{"f", "invalid_config", ""},
	}
	if len(verr.Issues) != len(want) {
		t.Fatalf("issues = %+v, want %d", verr.Issues, len(want))
	}
	if !strings.Contains(verr.Issues[2].Message, `did you mean "min"`) {
		t.Errorf("unknown key message %q lacks suggestion", verr.Issues[2].Message)
	}
	for i, w := range want {
		got := verr.Issues[i]
		if got.Field != w.field || got.Code != w.code || !strings.Contains(got.Hint, w.hint) {
			t.Errorf("issue %d = %+v, want field %s code %s hint containing %q", i, got, w.field, w.code, w.hint)
		}
	}
}

// firstWriteWriter records how many values had been generated when the
// first record arrived.
type firstWriteWriter struct {
	generatedAtFirstWrite int64
}

func (w *firstWriteWriter) WriteRecord(string, *writer.OrderedMap) error {
	if w.generatedAtFirstWrite == 0 {
		w.generatedAtFirstWrite = generated.Load()
	}
	return nil
}

func (w *firstWriteWriter) Close() error { return nil }

// Records stream out while the entity is still generating, so memory is
// bounded by chunks in flight rather than row count.
func TestRun_StreamsChunksInFlight(t *testing.T) {
	const workers, chunkSize, rows = 2, 100, 200_000
	generated.Store(0)
	w := &firstWriteWriter{}
	p := singleEntity(rows, plan.FieldSpec{Name: "n", Gen: "__test_counter"})
	if err := New(w, WithWorkers(workers), WithChunkSize(chunkSize)).Run(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	// Validation builds the generator once (no values); after that at most
	// the in-flight window can be generated before the first write.
	limit := int64(3 * workers * chunkSize)
	if w.generatedAtFirstWrite > limit {
		t.Fatalf("%d values generated before the first write, want <= %d", w.generatedAtFirstWrite, limit)
	}
}

// Standalone unique rel_ref is one batch: unique across the whole entity
// and identical for every chunk size and worker count.
func TestRun_StandaloneUniqueAcrossChunks(t *testing.T) {
	p := &plan.Plan{Seed: 1, Entities: []plan.EntitySpec{
		{Name: "Tag", Count: 10, Fields: []plan.FieldSpec{{Name: "id", Gen: "seq"}}},
		{Name: "X", Count: 10, Fields: []plan.FieldSpec{{Name: "tag_id", Gen: "rel_ref",
			Config: map[string]any{"entity": "Tag", "field": "id", "unique": true}}}},
	}}

	var baseline []any
	for _, chunk := range []int64{1, 3, 100} {
		for _, workers := range []int{1, 4} {
			w := &recordWriter{}
			if err := New(w, WithChunkSize(chunk), WithWorkers(workers)).Run(context.Background(), p); err != nil {
				t.Fatal(err)
			}
			var got []any
			seen := map[any]bool{}
			for _, r := range w.records[10:] {
				v, _ := r.Get("tag_id")
				if seen[v] {
					t.Fatalf("chunk=%d workers=%d: duplicate %v", chunk, workers, v)
				}
				seen[v] = true
				got = append(got, v)
			}
			if baseline == nil {
				baseline = got
				continue
			}
			for i := range got {
				if got[i] != baseline[i] {
					t.Fatalf("chunk=%d workers=%d: row %d = %v, want %v", chunk, workers, i, got[i], baseline[i])
				}
			}
		}
	}
}

// WithRowLimit caps every entity, including driven_by children.
func TestRun_RowLimit(t *testing.T) {
	p := &plan.Plan{Seed: 1, Entities: []plan.EntitySpec{
		{Name: "User", Count: 100, Fields: []plan.FieldSpec{{Name: "id", Gen: "seq"}}},
		{Name: "Order", DrivenBy: &plan.DrivenBy{Entity: "User", Field: "id", As: "user_id", Min: 2, Max: 5},
			Fields: []plan.FieldSpec{{Name: "n", Gen: "int"}}},
	}}
	w := &recordWriter{}
	if err := New(w, WithRowLimit(3)).Run(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if len(w.records) != 6 {
		t.Fatalf("records = %d, want 3 users + 3 orders", len(w.records))
	}
	for _, r := range w.records[3:] {
		if uid, _ := r.Get("user_id"); uid.(int64) > 3 {
			t.Fatalf("order references user %v outside the capped parents", uid)
		}
	}
}

// Output failures are marked so callers can tell them from generation errors.
func TestRun_WriteErrorsAreMarked(t *testing.T) {
	err := New(&stubWriter{writeErr: errors.New("disk full")}).Run(context.Background(), singleEntity(1, plan.FieldSpec{Name: "id", Gen: "seq"}))
	if !errors.Is(err, ErrWrite) {
		t.Fatalf("error = %v, want ErrWrite", err)
	}
	err = New(&stubWriter{closeErr: errors.New("flush failed")}).Run(context.Background(), singleEntity(1, plan.FieldSpec{Name: "id", Gen: "seq"}))
	if !errors.Is(err, ErrWrite) {
		t.Fatalf("close error = %v, want ErrWrite", err)
	}
}
