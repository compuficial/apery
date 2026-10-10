package registry

import (
	"strings"
	"testing"

	"github.com/compuficial/apery/internal/rng"
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

// newBoundRelRef builds a rel_ref and binds it to a store holding User.id.
func newBoundRelRef(t *testing.T, config map[string]any, ids []any) Generator {
	t.Helper()
	gen, err := Get("rel_ref", config)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	store := &testStore{data: map[string][]any{"User.id": ids}}
	if err := gen.(StoreConsumer).BindStore(store); err != nil {
		t.Fatalf("BindStore: %v", err)
	}
	return gen
}

// drawRows draws n values, one fresh row RNG per draw, as the executor does.
func drawRows(t *testing.T, gen Generator, n int) []any {
	t.Helper()
	out := make([]any, 0, n)
	for i := range n {
		val, err := gen.Next(rng.New(rng.DeriveIndex(rng.SeedFromInt64(testSeed), int64(i))))
		if err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
		out = append(out, val)
	}
	return out
}

func intIDs(n int) []any {
	ids := make([]any, n)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	return ids
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

func TestRelRefStoreRefs(t *testing.T) {
	gen, err := Get("rel_ref", map[string]any{"entity": "User", "field": "id"})
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	refs := gen.(StoreConsumer).StoreRefs()
	if len(refs) != 1 || refs[0] != (ColumnRef{Entity: "User", Field: "id"}) {
		t.Fatalf("StoreRefs = %v, want [User.id]", refs)
	}
}

func TestRelRefUniform(t *testing.T) {
	gen := newBoundRelRef(t, map[string]any{"entity": "User", "field": "id"}, []any{int64(10), int64(20), int64(30)})
	for _, val := range drawRows(t, gen, 50) {
		v := val.(int64)
		if v != 10 && v != 20 && v != 30 {
			t.Fatalf("unexpected value: %v", v)
		}
	}
}

func TestRelRefNextWithoutStore(t *testing.T) {
	gen, err := Get("rel_ref", map[string]any{"entity": "User", "field": "id"})
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	if _, err := gen.Next(newTestRng(42)); err == nil {
		t.Fatal("expected error when store is not bound")
	}
}

func TestRelRefBindStoreErrors(t *testing.T) {
	gen, err := Get("rel_ref", map[string]any{"entity": "User", "field": "id"})
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	consumer := gen.(StoreConsumer)
	if err := consumer.BindStore(&testStore{data: map[string][]any{}}); err == nil {
		t.Fatal("expected error for missing column")
	}
	if err := consumer.BindStore(&testStore{data: map[string][]any{"User.id": {}}}); err == nil {
		t.Fatal("expected error for empty column")
	}
}

// Drawing every value of the pool must always succeed: no retry budget.
func TestRelRefUniqueDrawsWholePool(t *testing.T) {
	for _, dist := range []string{"uniform", "zipf"} {
		t.Run(dist, func(t *testing.T) {
			const n = 1000
			gen := newBoundRelRef(t, map[string]any{
				"entity": "User", "field": "id", "unique": true, "distribution": dist,
			}, intIDs(n))

			seen := make(map[any]bool, n)
			for _, v := range drawRows(t, gen, n) {
				if seen[v] {
					t.Fatalf("duplicate value %v", v)
				}
				seen[v] = true
			}

			_, err := gen.Next(newTestRng(1))
			if err == nil || !strings.Contains(err.Error(), "exhausted") {
				t.Fatalf("expected exhaustion error after %d draws, got %v", n, err)
			}
		})
	}
}

// Uniqueness is by value: duplicate pool values count once.
func TestRelRefUniqueDedupesPool(t *testing.T) {
	gen := newBoundRelRef(t, map[string]any{"entity": "User", "field": "id", "unique": true},
		[]any{"a", "b", "a", "b", "a"})
	got := drawRows(t, gen, 2)
	if got[0] == got[1] {
		t.Fatalf("duplicate value %v", got[0])
	}
	if _, err := gen.Next(newTestRng(1)); err == nil {
		t.Fatal("expected exhaustion after 2 distinct values")
	}
}

// Maps and lists can't be compared for uniqueness; binding must say so.
func TestRelRefUniqueRejectsNonScalarPool(t *testing.T) {
	gen, err := Get("rel_ref", map[string]any{"entity": "User", "field": "id", "unique": true})
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	store := &testStore{data: map[string][]any{"User.id": {map[string]any{"a": 1}}}}
	if err := gen.(StoreConsumer).BindStore(store); err == nil {
		t.Fatal("expected error for non-scalar pool values")
	}
}

func TestRelRefUniqueReset(t *testing.T) {
	gen := newBoundRelRef(t, map[string]any{"entity": "User", "field": "id", "unique": true}, intIDs(2))
	drawRows(t, gen, 2)
	gen.(Resettable).Reset()
	drawRows(t, gen, 2)
}

// Only unique rel_ref carries per-batch state.
func TestRelRefResettableOnlyWhenUnique(t *testing.T) {
	plain, _ := Get("rel_ref", map[string]any{"entity": "User", "field": "id"})
	if _, ok := plain.(Resettable); ok {
		t.Fatal("non-unique rel_ref should not be Resettable")
	}
	unique, _ := Get("rel_ref", map[string]any{"entity": "User", "field": "id", "unique": true})
	if _, ok := unique.(Resettable); !ok {
		t.Fatal("unique rel_ref should be Resettable")
	}
}

func TestRelRefDeterminism(t *testing.T) {
	configs := map[string]map[string]any{
		"uniform":        {"entity": "User", "field": "id"},
		"zipf":           {"entity": "User", "field": "id", "distribution": "zipf", "s": 2.0},
		"unique uniform": {"entity": "User", "field": "id", "unique": true},
		"unique zipf":    {"entity": "User", "field": "id", "unique": true, "distribution": "zipf"},
	}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			a := drawRows(t, newBoundRelRef(t, cfg, intIDs(50)), 40)
			b := drawRows(t, newBoundRelRef(t, cfg, intIDs(50)), 40)
			for i := range a {
				if a[i] != b[i] {
					t.Fatalf("draw %d differs: %v != %v", i, a[i], b[i])
				}
			}
		})
	}
}
