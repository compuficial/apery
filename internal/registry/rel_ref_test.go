package registry

import (
	"apery/internal/rng"
	"testing"
)

// testStore is a minimal ReadOnlyEntityStore for unit tests.
type testStore struct {
	data map[string][]any
}

func (s *testStore) GetColumn(entity, field string) ([]any, bool) {
	vals, ok := s.data[entity+"."+field]
	return vals, ok
}

func newTestRng(seed int64) *rng.Rng {
	return rng.New(rng.SeedFromInt64(seed))
}

func TestRelRefConfig(t *testing.T) {
	RunConfigTests(t, "rel_ref", []ConfigTestCase{
		{Name: "valid minimal", Config: map[string]any{"entity": "User", "field": "id"}, ExpectError: false},
		{Name: "missing entity", Config: map[string]any{"field": "id"}, ExpectError: true},
		{Name: "missing field", Config: map[string]any{"entity": "User"}, ExpectError: true},
		{Name: "valid with distribution", Config: map[string]any{
			"entity": "User", "field": "id", "distribution": "zipf", "s": 2.0,
		}, ExpectError: false},
		{Name: "invalid distribution", Config: map[string]any{
			"entity": "User", "field": "id", "distribution": "poisson",
		}, ExpectError: true},
		{Name: "valid unique", Config: map[string]any{
			"entity": "User", "field": "id", "unique": true,
		}, ExpectError: false},
	})
}

func TestRelRefUniform(t *testing.T) {
	store := &testStore{data: map[string][]any{
		"User.id": {int64(10), int64(20), int64(30)},
	}}
	gen, err := Get("rel_ref", map[string]any{
		"entity": "User", "field": "id", "_store": store,
	})
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}

	r := newTestRng(42)
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}

	// Value must be one of the stored values.
	v := val.(int64)
	if v != 10 && v != 20 && v != 30 {
		t.Fatalf("unexpected value: %v", v)
	}
}

func TestRelRefNextWithoutStore(t *testing.T) {
	gen, err := Get("rel_ref", map[string]any{
		"entity": "User", "field": "id",
	})
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}

	r := newTestRng(42)
	_, err = gen.Next(r)
	if err == nil {
		t.Fatal("expected error when store is nil")
	}
}

func TestRelRefUnique(t *testing.T) {
	store := &testStore{data: map[string][]any{
		"User.id": {int64(1), int64(2), int64(3), int64(4), int64(5)},
	}}
	gen, err := Get("rel_ref", map[string]any{
		"entity": "User", "field": "id", "unique": true, "_store": store,
	})
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}

	r := newTestRng(42)
	seen := make(map[int64]bool)
	for range 5 {
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("Next error: %v", err)
		}
		v := val.(int64)
		if seen[v] {
			t.Fatalf("duplicate value: %v", v)
		}
		seen[v] = true
	}
}

func TestRelRefUniqueReset(t *testing.T) {
	store := &testStore{data: map[string][]any{
		"User.id": {int64(1), int64(2)},
	}}
	gen, err := Get("rel_ref", map[string]any{
		"entity": "User", "field": "id", "unique": true, "_store": store,
	})
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}

	r := newTestRng(42)

	// Draw 2 unique values (exhausts pool).
	for range 2 {
		if _, err := gen.Next(r); err != nil {
			t.Fatalf("Next error: %v", err)
		}
	}

	// Reset and draw again — should work.
	gen.(*RelRefGenerator).Reset()
	for range 2 {
		if _, err := gen.Next(r); err != nil {
			t.Fatalf("Next after Reset error: %v", err)
		}
	}
}

func TestRelRefDeterminism(t *testing.T) {
	store := &testStore{data: map[string][]any{
		"User.id": {int64(1), int64(2), int64(3), int64(4), int64(5)},
	}}
	RunDeterminismTests(t, "rel_ref", []DeterminismTestCase{
		{Name: "uniform", Config: map[string]any{
			"entity": "User", "field": "id", "_store": store,
		}},
		{Name: "zipf", Config: map[string]any{
			"entity": "User", "field": "id", "distribution": "zipf", "s": 2.0, "_store": store,
		}},
	})
}
