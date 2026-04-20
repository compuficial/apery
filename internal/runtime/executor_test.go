package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/registry"
	"github.com/compuficial/apery/internal/rng"
	"github.com/compuficial/apery/internal/writer"
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

type countHandler struct {
	calls int
}

func (h *countHandler) Enabled(_ context.Context, _ slog.Level) bool  { return true }
func (h *countHandler) Handle(_ context.Context, _ slog.Record) error { h.calls++; return nil }
func (h *countHandler) WithAttrs(_ []slog.Attr) slog.Handler          { return h }
func (h *countHandler) WithGroup(_ string) slog.Handler               { return h }

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
	h := &countHandler{}
	logger := slog.New(h)
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
	if h.calls == 0 {
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

func TestExecutorRelRef(t *testing.T) {
	var buf bytes.Buffer
	w := writer.NewJSONLWriterFromWriter(&buf)

	p := &plan.Plan{
		Seed: 99,
		Entities: []plan.EntitySpec{
			{Name: "User", Count: 5, Fields: []plan.FieldSpec{
				{Name: "id", Gen: "seq"},
			}},
			{Name: "Order", Count: 10, Fields: []plan.FieldSpec{
				{Name: "user_id", Gen: "rel_ref", Config: map[string]any{
					"entity": "User", "field": "id",
				}},
			}},
		},
	}

	e := New(w, WithWorkers(1), WithChunkSize(100))
	if err := e.Run(t.Context(), p); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// 5 User rows + 10 Order rows = 15 lines
	if len(lines) != 15 {
		t.Fatalf("expected 15 lines, got %d", len(lines))
	}
}

func TestExecutorDrivenBy(t *testing.T) {
	var buf bytes.Buffer
	w := writer.NewJSONLWriterFromWriter(&buf)

	p := &plan.Plan{
		Seed: 42,
		Entities: []plan.EntitySpec{
			{Name: "User", Count: 3, Fields: []plan.FieldSpec{
				{Name: "id", Gen: "seq"},
			}},
			{Name: "Order", DrivenBy: &plan.DrivenBy{
				Entity: "User", Field: "id", As: "user_id", Min: 2, Max: 2,
			}, Fields: []plan.FieldSpec{
				{Name: "amount", Gen: "int", Config: map[string]any{"min": 1, "max": 100}},
			}},
		},
	}

	e := New(w, WithWorkers(1), WithChunkSize(100))
	if err := e.Run(t.Context(), p); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// 3 Users + 6 Orders (3 parents × 2 children each) = 9 lines
	if len(lines) != 9 {
		t.Fatalf("expected 9 lines, got %d", len(lines))
	}

	// Check that Order lines have user_id field with values 1, 1, 2, 2, 3, 3
	orderLines := lines[3:]
	for i, line := range orderLines {
		expectedUserID := (i / 2) + 1
		expected := fmt.Sprintf(`"user_id":%d`, expectedUserID)
		if !strings.Contains(line, expected) {
			t.Errorf("order line %d: expected %s, got: %s", i, expected, line)
		}
	}
}

func TestExecutorManyToMany(t *testing.T) {
	var buf bytes.Buffer
	w := writer.NewJSONLWriterFromWriter(&buf)

	p := &plan.Plan{
		Seed: 42,
		Entities: []plan.EntitySpec{
			{Name: "Student", Count: 10, Fields: []plan.FieldSpec{
				{Name: "id", Gen: "seq"},
			}},
			{Name: "Course", Count: 5, Fields: []plan.FieldSpec{
				{Name: "id", Gen: "seq"},
			}},
			{Name: "Enrollment", DrivenBy: &plan.DrivenBy{
				Entity: "Student", Field: "id", As: "student_id", Min: 2, Max: 3,
			}, Fields: []plan.FieldSpec{
				{Name: "course_id", Gen: "rel_ref", Config: map[string]any{
					"entity": "Course", "field": "id", "unique": true,
				}},
			}},
		},
	}

	e := New(w, WithWorkers(4), WithChunkSize(5))
	if err := e.Run(t.Context(), p); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	studentLines := 10
	courseLines := 5
	enrollmentLines := len(lines) - studentLines - courseLines
	if enrollmentLines < 20 || enrollmentLines > 30 {
		t.Fatalf("expected 20-30 enrollment lines, got %d", enrollmentLines)
	}
}

func TestExecutorDrivenByDeterminism(t *testing.T) {
	p := &plan.Plan{
		Seed: 42,
		Entities: []plan.EntitySpec{
			{Name: "User", Count: 100, Fields: []plan.FieldSpec{
				{Name: "id", Gen: "seq"},
			}},
			{Name: "Order", DrivenBy: &plan.DrivenBy{
				Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 5,
			}, Fields: []plan.FieldSpec{
				{Name: "amount", Gen: "int", Config: map[string]any{"min": 1, "max": 100}},
			}},
		},
	}

	data1 := runPlanWithOpts(t, p, WithWorkers(1), WithChunkSize(1000))
	data2 := runPlanWithOpts(t, p, WithWorkers(4), WithChunkSize(50))

	if !bytes.Equal(data1, data2) {
		t.Fatal("driven_by output differs between worker configurations")
	}
}

func TestExecutorDrivenBySeqContiguous(t *testing.T) {
	var buf bytes.Buffer
	w := writer.NewJSONLWriterFromWriter(&buf)

	p := &plan.Plan{
		Seed: 42,
		Entities: []plan.EntitySpec{
			{Name: "User", Count: 3, Fields: []plan.FieldSpec{
				{Name: "id", Gen: "seq"},
			}},
			{Name: "Order", DrivenBy: &plan.DrivenBy{
				Entity: "User", Field: "id", As: "user_id", Min: 2, Max: 2,
			}, Fields: []plan.FieldSpec{
				{Name: "order_id", Gen: "seq"},
			}},
		},
	}

	e := New(w, WithWorkers(1), WithChunkSize(100))
	if err := e.Run(t.Context(), p); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	orderLines := lines[3:] // Skip 3 User lines
	if len(orderLines) != 6 {
		t.Fatalf("expected 6 order lines, got %d", len(orderLines))
	}

	// seq should produce contiguous IDs: 1, 2, 3, 4, 5, 6
	for i, line := range orderLines {
		expected := fmt.Sprintf(`"order_id":%d`, i+1)
		if !strings.Contains(line, expected) {
			t.Errorf("order line %d: expected %s, got: %s", i, expected, line)
		}
	}
}
