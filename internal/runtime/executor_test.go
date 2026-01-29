package runtime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"apery/internal/plan"
	"apery/internal/registry"
	"apery/internal/rng"
	"apery/internal/writer"
)

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

	entitySeed := rng.Derive(p.Seed, "User[0]")
	fieldSeed := rng.Derive(entitySeed, "score")

	for row := int64(0); row < p.Entities[0].Count; row++ {
		rowSeed := rng.Derive(fieldSeed, strconv.FormatInt(row, 10))
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
