package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"apery/internal/plan"
	"apery/internal/registry"
	"apery/internal/rng"
	"apery/internal/writer"
)

// stubRowAwareGen is a test-only generator that implements RowAwareGenerator and DependencyDeclarer.
type stubRowAwareGen struct {
	deps []string
}

func (s *stubRowAwareGen) Next(_ *rng.Rng) (any, error) {
	return nil, fmt.Errorf("stubRowAwareGen: requires row context")
}

func (s *stubRowAwareGen) NextWithRow(_ *rng.Rng, row registry.RowContext) (any, error) {
	v, ok := row.Get(s.deps[0])
	if !ok {
		return nil, fmt.Errorf("missing dep %s", s.deps[0])
	}
	return fmt.Sprintf("got:%v", v), nil
}

func (s *stubRowAwareGen) Dependencies() []string {
	return s.deps
}

func init() {
	registry.MustRegister("__test_row_aware", func(config map[string]any) (registry.Generator, error) {
		deps, _ := config["deps"].([]any)
		strs := make([]string, len(deps))
		for i, d := range deps {
			strs[i] = d.(string)
		}
		return &stubRowAwareGen{deps: strs}, nil
	})
}

type valueWriter struct {
	values []int64
}

func (w *valueWriter) WriteRecord(entity string, record *writer.OrderedMap) error {
	val, ok := record.Get("score")
	if !ok {
		return fmt.Errorf("missing field: score")
	}
	ival, ok := val.(int64)
	if !ok {
		return fmt.Errorf("score: expected int64, got %T", val)
	}
	w.values = append(w.values, ival)
	return nil
}

func (w *valueWriter) Close() error {
	return nil
}

type dualFieldWriter struct {
	ids    []int64
	scores []int64
}

func (w *dualFieldWriter) WriteRecord(entity string, record *writer.OrderedMap) error {
	idVal, ok := record.Get("id")
	if !ok {
		return fmt.Errorf("missing field: id")
	}
	scoreVal, ok := record.Get("score")
	if !ok {
		return fmt.Errorf("missing field: score")
	}
	id, ok := idVal.(int64)
	if !ok {
		return fmt.Errorf("id: expected int64, got %T", idVal)
	}
	score, ok := scoreVal.(int64)
	if !ok {
		return fmt.Errorf("score: expected int64, got %T", scoreVal)
	}
	w.ids = append(w.ids, id)
	w.scores = append(w.scores, score)
	return nil
}

func (w *dualFieldWriter) Close() error {
	return nil
}

type stubWriter struct {
	writeErr error
	closeErr error
	closed   bool
}

func (w *stubWriter) WriteRecord(entity string, record *writer.OrderedMap) error {
	return w.writeErr
}

func (w *stubWriter) Close() error {
	w.closed = true
	return w.closeErr
}

type stubLogger struct {
	calls int
}

func (l *stubLogger) Printf(format string, args ...any) {
	l.calls++
}

func TestExecutorRowSeedDeterminism(t *testing.T) {
	cfg := map[string]any{"min": 1, "max": 10}
	p := plan.Plan{
		Seed: 42,
		Entities: []plan.EntitySpec{
			{
				Name:  "User",
				Count: 5,
				Fields: []plan.FieldSpec{
					{Name: "score", Gen: "int", Config: cfg},
				},
			},
		},
	}

	recorder := &valueWriter{}
	executor := New(recorder)
	if err := executor.Run(context.Background(), &p); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(recorder.values) != int(p.Entities[0].Count) {
		t.Fatalf("expected %d values, got %d", p.Entities[0].Count, len(recorder.values))
	}

	gen, err := registry.Get("int", cfg)
	if err != nil {
		t.Fatalf("get generator: %v", err)
	}

	entitySeed := rng.Derive(rng.SeedFromInt64(p.Seed), "User[0]")
	fieldSeed := rng.Derive(entitySeed, "score")

	for row := int64(0); row < p.Entities[0].Count; row++ {
		rowSeed := rng.DeriveIndex(fieldSeed, row)
		val, err := gen.Next(rng.New(rowSeed))
		if err != nil {
			t.Fatalf("row %d: %v", row, err)
		}
		expected, ok := val.(int64)
		if !ok {
			t.Fatalf("row %d: expected int64, got %T", row, val)
		}
		if recorder.values[row] != expected {
			t.Fatalf("row %d: expected %d, got %d", row, expected, recorder.values[row])
		}
	}
}

func TestExecutorContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := &stubWriter{}
	executor := New(w)
	p := &plan.Plan{
		Seed: 1,
		Entities: []plan.EntitySpec{
			{
				Name:  "User",
				Count: 1,
				Fields: []plan.FieldSpec{
					{Name: "id", Gen: "seq"},
				},
			},
		},
	}

	err := executor.Run(ctx, p)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if !w.closed {
		t.Fatal("expected writer to be closed")
	}
}

func TestExecutorErrorJoinOnClose(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("close failed")
	w := &stubWriter{writeErr: writeErr, closeErr: closeErr}
	executor := New(w)
	p := &plan.Plan{
		Seed: 1,
		Entities: []plan.EntitySpec{
			{
				Name:  "User",
				Count: 1,
				Fields: []plan.FieldSpec{
					{Name: "id", Gen: "seq"},
				},
			},
		},
	}

	err := executor.Run(context.Background(), p)
	if !errors.Is(err, writeErr) {
		t.Fatalf("expected write error, got %v", err)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected close error, got %v", err)
	}
}

func TestExecutorLogger(t *testing.T) {
	logger := &stubLogger{}
	w := &stubWriter{}
	executor := New(w, WithLogger(logger))
	p := &plan.Plan{
		Seed: 1,
		Entities: []plan.EntitySpec{
			{
				Name:  "User",
				Count: 1,
				Fields: []plan.FieldSpec{
					{Name: "id", Gen: "seq"},
				},
			},
		},
	}

	if err := executor.Run(context.Background(), p); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if logger.calls == 0 {
		t.Fatal("expected logger to be called")
	}
}

func TestExecutorChunkDeterminism(t *testing.T) {
	p := plan.Plan{
		Seed: 7,
		Entities: []plan.EntitySpec{
			{
				Name:  "Event",
				Count: 50,
				Fields: []plan.FieldSpec{
					{Name: "id", Gen: "seq", Config: map[string]any{"start": 1, "step": 1}},
					{Name: "score", Gen: "int", Config: map[string]any{"min": 1, "max": 10}},
				},
			},
		},
	}

	run := func(workers int, chunkSize int64) ([]int64, []int64, error) {
		w := &dualFieldWriter{}
		executor := New(w, WithWorkers(workers), WithChunkSize(chunkSize))
		if err := executor.Run(context.Background(), &p); err != nil {
			return nil, nil, err
		}
		return w.ids, w.scores, nil
	}

	idA, scoreA, err := run(1, 10)
	if err != nil {
		t.Fatalf("run A: %v", err)
	}
	idB, scoreB, err := run(4, 7)
	if err != nil {
		t.Fatalf("run B: %v", err)
	}

	if len(idA) != len(idB) || len(scoreA) != len(scoreB) {
		t.Fatalf("mismatched lengths: ids %d/%d scores %d/%d", len(idA), len(idB), len(scoreA), len(scoreB))
	}
	for i := range idA {
		if idA[i] != idB[i] {
			t.Fatalf("seq mismatch at %d: %d != %d", i, idA[i], idB[i])
		}
		if scoreA[i] != scoreB[i] {
			t.Fatalf("score mismatch at %d: %d != %d", i, scoreA[i], scoreB[i])
		}
	}

	for i := range idA {
		expected := int64(i + 1)
		if idA[i] != expected {
			t.Fatalf("seq order mismatch at %d: expected %d, got %d", i, expected, idA[i])
		}
	}
}

func TestExecutorDependencyValidation(t *testing.T) {
	t.Run("valid ordering", func(t *testing.T) {
		p := &plan.Plan{
			Seed: 1,
			Entities: []plan.EntitySpec{
				{
					Name:  "Test",
					Count: 1,
					Fields: []plan.FieldSpec{
						{Name: "status", Gen: "bool"},
						{Name: "label", Gen: "__test_row_aware", Config: map[string]any{"deps": []any{"status"}}},
					},
				},
			},
		}
		e := New(&stubWriter{})
		err := e.Run(context.Background(), p)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("invalid ordering", func(t *testing.T) {
		p := &plan.Plan{
			Seed: 1,
			Entities: []plan.EntitySpec{
				{
					Name:  "Test",
					Count: 1,
					Fields: []plan.FieldSpec{
						{Name: "label", Gen: "__test_row_aware", Config: map[string]any{"deps": []any{"status"}}},
						{Name: "status", Gen: "bool"},
					},
				},
			},
		}
		e := New(&stubWriter{})
		err := e.Run(context.Background(), p)
		if err == nil {
			t.Fatal("expected error for invalid field ordering")
		}
		if !strings.Contains(err.Error(), "must be declared before") {
			t.Fatalf("expected ordering error, got: %v", err)
		}
	})

	t.Run("dependency on nonexistent field", func(t *testing.T) {
		p := &plan.Plan{
			Seed: 1,
			Entities: []plan.EntitySpec{
				{
					Name:  "Test",
					Count: 1,
					Fields: []plan.FieldSpec{
						{Name: "label", Gen: "__test_row_aware", Config: map[string]any{"deps": []any{"nonexistent"}}},
					},
				},
			},
		}
		e := New(&stubWriter{})
		err := e.Run(context.Background(), p)
		if err == nil {
			t.Fatal("expected error for nonexistent dependency")
		}
	})
}

type recordWriter struct {
	records []*writer.OrderedMap
}

func (w *recordWriter) WriteRecord(entity string, record *writer.OrderedMap) error {
	w.records = append(w.records, record)
	return nil
}

func (w *recordWriter) Close() error {
	return nil
}

func TestExecutorRowAwareDispatch(t *testing.T) {
	p := &plan.Plan{
		Seed: 99,
		Entities: []plan.EntitySpec{
			{
				Name:  "Test",
				Count: 5,
				Fields: []plan.FieldSpec{
					{Name: "status", Gen: "bool"},
					{Name: "label", Gen: "__test_row_aware", Config: map[string]any{"deps": []any{"status"}}},
				},
			},
		},
	}

	w := &recordWriter{}
	e := New(w)
	if err := e.Run(context.Background(), p); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(w.records) != 5 {
		t.Fatalf("expected 5 records, got %d", len(w.records))
	}

	for i, rec := range w.records {
		label, ok := rec.Get("label")
		if !ok {
			t.Fatalf("row %d: missing label", i)
		}
		s, ok := label.(string)
		if !ok {
			t.Fatalf("row %d: label is %T, want string", i, label)
		}
		if !strings.HasPrefix(s, "got:") {
			t.Fatalf("row %d: label = %q, want prefix 'got:'", i, s)
		}
	}
}

func TestExecutorRowAwareDeterminism(t *testing.T) {
	p := plan.Plan{
		Seed: 77,
		Entities: []plan.EntitySpec{
			{
				Name:  "Test",
				Count: 20,
				Fields: []plan.FieldSpec{
					{Name: "status", Gen: "bool"},
					{Name: "label", Gen: "__test_row_aware", Config: map[string]any{"deps": []any{"status"}}},
				},
			},
		},
	}

	run := func(workers int, chunkSize int64) ([]string, error) {
		w := &recordWriter{}
		e := New(w, WithWorkers(workers), WithChunkSize(chunkSize))
		if err := e.Run(context.Background(), &p); err != nil {
			return nil, err
		}
		labels := make([]string, len(w.records))
		for i, rec := range w.records {
			v, _ := rec.Get("label")
			labels[i] = fmt.Sprintf("%v", v)
		}
		return labels, nil
	}

	a, err := run(1, 5)
	if err != nil {
		t.Fatalf("run A: %v", err)
	}
	b, err := run(4, 3)
	if err != nil {
		t.Fatalf("run B: %v", err)
	}

	if len(a) != len(b) {
		t.Fatalf("length mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("row %d: %q != %q", i, a[i], b[i])
		}
	}
}
