# Relational Generators Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `rel_ref` generator and `driven_by` plan-level concept to enable M:1, 1:M, and M:N relational data generation.

**Architecture:** One new generator (`rel_ref`) samples from a cross-entity column store. One new plan struct (`DrivenBy`) drives child entity row counts from parent rows. The executor gains a pre-scan pass, a `mapEntityStore`, and a two-phase driven-by execution path with parent-aligned chunking. Validation expands to cover entity dependency ordering and relational constraints.

**Tech Stack:** Go 1.24, existing `internal/registry`, `internal/plan`, `internal/runtime` packages.

**Spec:** `docs/specs/2026-03-15-relational-generators-design.md`

---

## Chunk 1: Plan Types, Registry Interfaces, and Validation

### Task 1: Add `DrivenBy` struct and update `EntitySpec`

**Files:**
- Modify: `internal/plan/plan.go`

- [ ] **Step 1: Add `DrivenBy` struct and update `EntitySpec`**

```go
// DrivenBy configures parent-driven child row generation (1:M relationships).
// When set on an EntitySpec, the executor generates Min to Max child rows per
// parent row instead of using Count. The parent's Field value is auto-injected
// into each child row under the name As.
type DrivenBy struct {
	Entity string // parent entity name
	Field  string // parent field to inject into child rows
	As     string // field name in child row for the injected value
	Min    int64  // minimum children per parent (must be >= 1)
	Max    int64  // maximum children per parent (must be >= Min)
}
```

Add `DrivenBy *DrivenBy` field to `EntitySpec` after `Count`.

- [ ] **Step 2: Run existing tests to verify no breakage**

Run: `go test ./internal/plan/ -v`
Expected: All existing tests pass (DrivenBy is nil by default, no behavior change).

- [ ] **Step 3: Commit**

```
feat(plan): add DrivenBy struct to EntitySpec for 1:M relationships
```

---

### Task 2: Add `EntityStore`, `Resettable` interfaces to registry

**Files:**
- Modify: `internal/registry/registry.go`

- [ ] **Step 1: Add the three new interfaces**

Append after the existing `DependencyDeclarer` interface:

```go
// ReadOnlyEntityStore provides read access to columns stored after entity generation.
// Generators (e.g., rel_ref) use this to sample from previously generated entities.
type ReadOnlyEntityStore interface {
	GetColumn(entity, field string) ([]any, bool)
}

// EntityStore extends ReadOnlyEntityStore with write access for the executor
// to populate columns after generating each entity.
type EntityStore interface {
	StoreColumn(entity, field string, values []any)
	ReadOnlyEntityStore
}

// Resettable is implemented by generators with internal state that must be
// cleared between parent batches in driven_by entities. The executor calls
// Reset() each time the parent index changes during chunk processing.
type Resettable interface {
	Reset()
}
```

- [ ] **Step 2: Run existing tests**

Run: `go test ./internal/registry/ -v`
Expected: All pass. New interfaces are additive.

- [ ] **Step 3: Commit**

```
feat(registry): add EntityStore, ReadOnlyEntityStore, and Resettable interfaces
```

---

### Task 3: Update `plan.Validate()` for relational constraints

**Files:**
- Modify: `internal/plan/validate.go`
- Modify: `internal/plan/validate_test.go`

- [ ] **Step 1: Write failing tests for all new validation rules**

Add these test cases to the existing `TestValidate` table in `validate_test.go`:

```go
// --- DrivenBy validation ---
{
	name: "valid driven_by plan",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Order", DrivenBy: &DrivenBy{
				Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 5,
			}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
		},
	},
	expectError: false,
},
{
	name: "driven_by entity not declared before",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "Order", DrivenBy: &DrivenBy{
				Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 3,
			}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
		},
	},
	expectError: true,
	errContains: "not declared before",
},
{
	name: "driven_by field not in parent",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Order", DrivenBy: &DrivenBy{
				Entity: "User", Field: "email", As: "user_email", Min: 1, Max: 3,
			}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
		},
	},
	expectError: true,
	errContains: "field 'email' does not exist",
},
{
	name: "driven_by as conflicts with child field",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Order", DrivenBy: &DrivenBy{
				Entity: "User", Field: "id", As: "amount", Min: 1, Max: 3,
			}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
		},
	},
	expectError: true,
	errContains: "conflicts with declared field",
},
{
	name: "driven_by min less than 1",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Order", DrivenBy: &DrivenBy{
				Entity: "User", Field: "id", As: "user_id", Min: 0, Max: 3,
			}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
		},
	},
	expectError: true,
	errContains: "min must be >= 1",
},
{
	name: "driven_by max less than min",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Order", DrivenBy: &DrivenBy{
				Entity: "User", Field: "id", As: "user_id", Min: 5, Max: 3,
			}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
		},
	},
	expectError: true,
	errContains: "max must be >= min",
},
{
	name: "count and driven_by both set",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Order", Count: 50, DrivenBy: &DrivenBy{
				Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 3,
			}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
		},
	},
	expectError: true,
	errContains: "exactly one of Count or DrivenBy",
},
{
	name: "neither count nor driven_by",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
		},
	},
	expectError: true,
	errContains: "exactly one of Count or DrivenBy",
},
{
	name: "self-referencing driven_by",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", DrivenBy: &DrivenBy{
				Entity: "User", Field: "id", As: "parent_id", Min: 1, Max: 2,
			}, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
		},
	},
	expectError: true,
	errContains: "not declared before",
},
{
	name: "reserved config key",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Count: 10, Fields: []FieldSpec{
				{Name: "id", Gen: "seq", Config: map[string]any{"_store": "bad"}},
			}},
		},
	},
	expectError: true,
	errContains: "reserved for internal use",
},
{
	name: "rel_ref entity not declared before",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "Order", Count: 10, Fields: []FieldSpec{
				{Name: "user_id", Gen: "rel_ref", Config: map[string]any{"entity": "User", "field": "id"}},
			}},
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
		},
	},
	expectError: true,
	errContains: "not declared before",
},
{
	name: "rel_ref field not in target entity",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Order", Count: 10, Fields: []FieldSpec{
				{Name: "user_id", Gen: "rel_ref", Config: map[string]any{"entity": "User", "field": "email"}},
			}},
		},
	},
	expectError: true,
	errContains: "field 'email' does not exist",
},
{
	name: "valid rel_ref plan",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Order", Count: 50, Fields: []FieldSpec{
				{Name: "user_id", Gen: "rel_ref", Config: map[string]any{"entity": "User", "field": "id"}},
			}},
		},
	},
	expectError: false,
},
{
	name: "unique feasibility exceeded",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "Item", Count: 3, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Cart", DrivenBy: &DrivenBy{
				Entity: "Item", Field: "id", As: "item_id", Min: 1, Max: 5,
			}, Fields: []FieldSpec{
				{Name: "extra_item", Gen: "rel_ref", Config: map[string]any{
					"entity": "Item", "field": "id", "unique": true,
				}},
			}},
		},
	},
	expectError: true,
	errContains: "unique rel_ref",
},
{
	name: "driven_by references parent As field",
	plan: &Plan{
		Seed: 42,
		Entities: []EntitySpec{
			{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
			{Name: "Order", DrivenBy: &DrivenBy{
				Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 3,
			}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
			{Name: "LineItem", DrivenBy: &DrivenBy{
				Entity: "Order", Field: "user_id", As: "order_user_id", Min: 1, Max: 2,
			}, Fields: []FieldSpec{{Name: "qty", Gen: "int"}}},
		},
	},
	expectError: false,
},
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/plan/ -v -run TestValidate`
Expected: New test cases fail (validation logic not yet added).

- [ ] **Step 3: Implement validation changes in `validate.go`**

Replace the `validateEntity` function body. Key changes:
1. Replace `e.Count <= 0` with Count/DrivenBy mutual exclusion check.
2. Add `validateDrivenBy` function that checks entity ordering, field existence, As conflict, Min/Max bounds.
3. Build a map of entity → fields (including `DrivenBy.As` fields) for cross-entity reference checks.
4. Add reserved config key check (`_` prefix) in `validateField`.
5. For `rel_ref` fields, validate `entity` and `field` config keys against the entity fields map.

The `Validate` function needs to build an `entityFields` map (`map[string]map[string]bool`) as it iterates entities, tracking which fields (including injected `As` fields) each entity exposes. Pass this to `validateEntity`.

```go
func Validate(p *Plan) error {
	if p == nil {
		return errors.New("plan: plan is nil")
	}
	if len(p.Entities) == 0 {
		return errors.New("plan: no entities defined")
	}

	entityNames := make(map[string]struct{}, len(p.Entities))
	// Maps entity name -> set of field names (including DrivenBy.As)
	entityFields := make(map[string]map[string]bool, len(p.Entities))

	for i := range p.Entities {
		e := &p.Entities[i]
		if err := validateEntity(e, i, entityNames, entityFields); err != nil {
			return err
		}
		entityNames[e.Name] = struct{}{}

		// Build field set for this entity
		fields := make(map[string]bool, len(e.Fields)+1)
		if e.DrivenBy != nil {
			fields[e.DrivenBy.As] = true
		}
		for _, f := range e.Fields {
			fields[f.Name] = true
		}
		entityFields[e.Name] = fields
	}
	return nil
}
```

Update `validateEntity` signature to accept `entityFields map[string]map[string]bool`. Add Count/DrivenBy mutual exclusion. Add `validateDrivenBy` call. Add `rel_ref` cross-entity checks. Add reserved key check.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/plan/ -v -run TestValidate`
Expected: All tests pass, including the new ones.

- [ ] **Step 5: Run all tests to verify no breakage**

Run: `go test ./... 2>&1 | tail -20`
Expected: All pass. The existing "entity count zero" test still passes because `Count=0` + `DrivenBy=nil` triggers "exactly one of Count or DrivenBy".

Note: The existing test for "entity count zero" expects `errContains: "count must be > 0"`. The new error message is different. Update this test case to expect the new message, e.g., `"exactly one of Count or DrivenBy"`. Same for "entity count negative" — update to match the new validation path.

- [ ] **Step 6: Commit**

```
feat(plan): add relational validation rules for DrivenBy and rel_ref
```

---

### Task 4: Export `DrivenBy` in the public API

**Files:**
- Modify: `run.go`

- [ ] **Step 1: Add DrivenBy type alias**

Add to the type block:
```go
DrivenBy = plan.DrivenBy
```

- [ ] **Step 2: Run tests**

Run: `go test ./... 2>&1 | tail -5`
Expected: All pass.

- [ ] **Step 3: Commit**

```
feat: export DrivenBy type in public API
```

---

## Chunk 2: Entity Store and `rel_ref` Generator

### Task 5: Implement `mapEntityStore` in runtime

**Files:**
- Create: `internal/runtime/entity_store.go`
- Create: `internal/runtime/entity_store_test.go`

- [ ] **Step 1: Write failing tests**

```go
package runtime

import "testing"

func TestMapEntityStore(t *testing.T) {
	store := newMapEntityStore()

	// Store and retrieve
	store.StoreColumn("User", "id", []any{int64(1), int64(2), int64(3)})
	col, ok := store.GetColumn("User", "id")
	if !ok {
		t.Fatal("expected column to exist")
	}
	if len(col) != 3 {
		t.Fatalf("expected 3 values, got %d", len(col))
	}
	if col[0] != int64(1) {
		t.Fatalf("expected 1, got %v", col[0])
	}

	// Missing entity
	_, ok = store.GetColumn("Order", "id")
	if ok {
		t.Fatal("expected missing column")
	}

	// Missing field
	_, ok = store.GetColumn("User", "email")
	if ok {
		t.Fatal("expected missing column")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/ -v -run TestMapEntityStore`
Expected: FAIL — `newMapEntityStore` not defined.

- [ ] **Step 3: Implement `mapEntityStore`**

```go
package runtime

import "apery/internal/registry"

// mapEntityStore is the concrete implementation of registry.EntityStore.
// It stores column data keyed by "entity.field" and is populated by the
// executor after each entity finishes generating.
type mapEntityStore struct {
	data map[string][]any
}

func newMapEntityStore() *mapEntityStore {
	return &mapEntityStore{data: make(map[string][]any)}
}

func storeKey(entity, field string) string {
	return entity + "." + field
}

// StoreColumn saves a column of values for later retrieval by rel_ref generators.
func (s *mapEntityStore) StoreColumn(entity, field string, values []any) {
	s.data[storeKey(entity, field)] = values
}

// GetColumn retrieves a previously stored column. Returns false if not found.
func (s *mapEntityStore) GetColumn(entity, field string) ([]any, bool) {
	vals, ok := s.data[storeKey(entity, field)]
	return vals, ok
}

// Verify interface compliance at compile time.
var _ registry.EntityStore = (*mapEntityStore)(nil)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime/ -v -run TestMapEntityStore`
Expected: PASS.

- [ ] **Step 5: Commit**

```
feat(runtime): add mapEntityStore for cross-entity column storage
```

---

### Task 6: Implement `rel_ref` generator (uniform mode)

**Files:**
- Create: `internal/registry/rel_ref.go`
- Create: `internal/registry/rel_ref_test.go`

- [ ] **Step 1: Write failing tests for config validation**

```go
package registry

import "testing"

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/registry/ -v -run TestRelRefConfig`
Expected: FAIL — `rel_ref` not registered.

- [ ] **Step 3: Write failing test for Next() with store**

```go
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

	// Value must be one of the stored values
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
```

Add a `testStore` helper:

```go
type testStore struct {
	data map[string][]any
}

func (s *testStore) GetColumn(entity, field string) ([]any, bool) {
	vals, ok := s.data[entity+"."+field]
	return vals, ok
}
```

And a `newTestRng` helper (or reuse existing pattern):

```go
func newTestRng(seed int64) *rng.Rng {
	return rng.New(rng.SeedFromInt64(seed))
}
```

- [ ] **Step 4: Implement `rel_ref` generator**

```go
package registry

import (
	"apery/internal/rng"
	"fmt"
)

const maxUniqueRetries = 100

// RelRefGenerator samples values from a previously generated entity's column.
// It reads from a ReadOnlyEntityStore injected via the "_store" config key.
type RelRefGenerator struct {
	entity string
	field  string
	store  ReadOnlyEntityStore
	dist   string // "uniform" or "zipf"
	zipfS  float64

	// unique tracking (stateful across Next calls within a chunk)
	unique bool
	seen   map[any]bool
}

// Next returns a value sampled from the referenced entity's column.
// Returns an error if the store has not been injected.
func (g *RelRefGenerator) Next(r *rng.Rng) (any, error) {
	if g.store == nil {
		return nil, fmt.Errorf("rel_ref: entity store not available (internal error)")
	}

	col, ok := g.store.GetColumn(g.entity, g.field)
	if !ok {
		return nil, fmt.Errorf("rel_ref: column %s.%s not found in store", g.entity, g.field)
	}
	if len(col) == 0 {
		return nil, fmt.Errorf("rel_ref: column %s.%s is empty", g.entity, g.field)
	}

	n := int64(len(col))

	if !g.unique {
		idx := g.sampleIndex(r, n)
		return col[idx], nil
	}

	// Unique mode: retry on collision
	for range maxUniqueRetries {
		idx := g.sampleIndex(r, n)
		val := col[idx]
		if !g.seen[val] {
			g.seen[val] = true
			return val, nil
		}
	}

	return nil, fmt.Errorf("rel_ref: unique constraint failed after %d retries (pool: %s.%s)",
		maxUniqueRetries, g.entity, g.field)
}

// Reset clears the uniqueness tracker between parent batches.
func (g *RelRefGenerator) Reset() {
	if g.seen != nil {
		clear(g.seen)
	}
}

func (g *RelRefGenerator) sampleIndex(r *rng.Rng, n int64) int64 {
	if g.dist == "zipf" {
		z := r.NewZipf(g.zipfS, 1.0, uint64(n-1))
		return int64(z.Uint64())
	}
	return r.IntRange(0, n-1)
}

func init() {
	MustRegister("rel_ref", func(config map[string]any) (Generator, error) {
		entity, ok := config["entity"].(string)
		if !ok || entity == "" {
			return nil, fmt.Errorf("rel_ref: 'entity' is required and must be a string")
		}
		field, ok := config["field"].(string)
		if !ok || field == "" {
			return nil, fmt.Errorf("rel_ref: 'field' is required and must be a string")
		}

		dist := "uniform"
		if d, exists := config["distribution"]; exists {
			ds, ok := d.(string)
			if !ok {
				return nil, fmt.Errorf("rel_ref: 'distribution' must be a string")
			}
			if ds != "uniform" && ds != "zipf" {
				return nil, fmt.Errorf("rel_ref: 'distribution' must be \"uniform\" or \"zipf\", got %q", ds)
			}
			dist = ds
		}

		zipfS := 1.5
		if s, exists := config["s"]; exists {
			v, err := extractFloat(s, "s", "rel_ref")
			if err != nil {
				return nil, err
			}
			if v <= 1 {
				return nil, fmt.Errorf("rel_ref: 's' must be > 1, got %f", v)
			}
			zipfS = v
		}

		unique := false
		if u, exists := config["unique"]; exists {
			ub, ok := u.(bool)
			if !ok {
				return nil, fmt.Errorf("rel_ref: 'unique' must be a bool")
			}
			unique = ub
		}

		// Store is optional at factory time (nil during initFields validation)
		var store ReadOnlyEntityStore
		if s, exists := config["_store"]; exists && s != nil {
			store, ok = s.(ReadOnlyEntityStore)
			if !ok {
				return nil, fmt.Errorf("rel_ref: '_store' has wrong type")
			}
		}

		gen := &RelRefGenerator{
			entity: entity,
			field:  field,
			store:  store,
			dist:   dist,
			zipfS:  zipfS,
			unique: unique,
		}
		if unique {
			gen.seen = make(map[any]bool)
		}

		return gen, nil
	})
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/registry/ -v -run TestRelRef`
Expected: All pass.

- [ ] **Step 6: Write determinism test**

```go
func TestRelRefDeterminism(t *testing.T) {
	store := &testStore{data: map[string][]any{
		"User.id": {int64(1), int64(2), int64(3), int64(4), int64(5)},
	}}
	config := map[string]any{
		"entity": "User", "field": "id", "_store": store,
	}

	gen1, _ := Get("rel_ref", config)
	gen2, _ := Get("rel_ref", config)

	for i := range 100 {
		r1 := newTestRng(int64(i))
		r2 := newTestRng(int64(i))
		v1, _ := gen1.Next(r1)
		v2, _ := gen2.Next(r2)
		if v1 != v2 {
			t.Fatalf("mismatch at %d: %v != %v", i, v1, v2)
		}
	}
}
```

- [ ] **Step 7: Run determinism test**

Run: `go test ./internal/registry/ -v -run TestRelRefDeterminism`
Expected: PASS.

- [ ] **Step 8: Write unique mode tests**

```go
func TestRelRefUnique(t *testing.T) {
	store := &testStore{data: map[string][]any{
		"Item.id": {int64(1), int64(2), int64(3), int64(4), int64(5)},
	}}
	gen, _ := Get("rel_ref", map[string]any{
		"entity": "Item", "field": "id", "unique": true, "_store": store,
	})

	seen := make(map[any]bool)
	for i := range 5 {
		r := newTestRng(int64(i * 1000))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("Next error at %d: %v", i, err)
		}
		if seen[val] {
			t.Fatalf("duplicate value at %d: %v", i, val)
		}
		seen[val] = true
	}
}

func TestRelRefUniqueExhausted(t *testing.T) {
	store := &testStore{data: map[string][]any{
		"Item.id": {int64(1), int64(2)},
	}}
	gen, _ := Get("rel_ref", map[string]any{
		"entity": "Item", "field": "id", "unique": true, "_store": store,
	})

	// Draw 2 values (should work)
	for i := range 2 {
		r := newTestRng(int64(i * 1000))
		if _, err := gen.Next(r); err != nil {
			t.Fatalf("unexpected error at %d: %v", i, err)
		}
	}

	// Third draw should fail
	r := newTestRng(999)
	_, err := gen.Next(r)
	if err == nil {
		t.Fatal("expected error on exhausted pool")
	}
}

func TestRelRefReset(t *testing.T) {
	store := &testStore{data: map[string][]any{
		"Item.id": {int64(1), int64(2)},
	}}
	gen, _ := Get("rel_ref", map[string]any{
		"entity": "Item", "field": "id", "unique": true, "_store": store,
	})

	// Draw 2 values
	for i := range 2 {
		r := newTestRng(int64(i * 1000))
		if _, err := gen.Next(r); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Reset and draw again — should succeed
	gen.(Resettable).Reset()
	r := newTestRng(42)
	if _, err := gen.Next(r); err != nil {
		t.Fatalf("error after reset: %v", err)
	}
}
```

- [ ] **Step 9: Run all rel_ref tests**

Run: `go test ./internal/registry/ -v -run TestRelRef`
Expected: All pass.

- [ ] **Step 10: Commit**

```
feat(registry): add rel_ref generator with uniform, zipf, and unique modes
```

---

## Chunk 3: Executor Changes — Store Integration, `driven_by` Execution

### Task 7: Add pre-scan, store wiring, and column extraction to executor

**Files:**
- Modify: `internal/runtime/executor.go`

This task makes multiple interacting changes to the executor. All changes must be applied together in a single step to avoid intermediate compile errors (refactoring `runEntity`'s return type, adding the store parameter to `runChunk`, and wiring the store in `Run()` are interdependent).

- [ ] **Step 1: Write a failing integration test first**

Add to `executor_test.go` (this test will pass after the full wiring is done):

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/ -v -run TestExecutorRelRef`
Expected: FAIL — `rel_ref` gets nil store at generation time.

- [ ] **Step 3: Apply all executor changes atomically**

Add these utility functions to `executor.go`:

```go
// copyConfig returns a shallow copy of a config map.
func copyConfig(cfg map[string]any) map[string]any {
	out := make(map[string]any, len(cfg)+1)
	for k, v := range cfg {
		out[k] = v
	}
	return out
}

// requiredColumns scans all entities to determine which (entity, field) pairs
// need to be stored for downstream rel_ref and DrivenBy references.
func requiredColumns(entities []plan.EntitySpec) map[string]bool {
	required := make(map[string]bool)
	for _, e := range entities {
		if e.DrivenBy != nil {
			required[e.DrivenBy.Entity+"."+e.DrivenBy.Field] = true
		}
		for _, f := range e.Fields {
			if f.Gen == "rel_ref" {
				entity, _ := f.Config["entity"].(string)
				field, _ := f.Config["field"].(string)
				if entity != "" && field != "" {
					required[entity+"."+field] = true
				}
			}
		}
	}
	return required
}

// extractColumn collects a single field's values from generated records.
func extractColumn(records []*writer.OrderedMap, fieldName string) []any {
	col := make([]any, len(records))
	for i, rec := range records {
		val, _ := rec.Get(fieldName)
		col[i] = val
	}
	return col
}

// hasUniqueRelRef returns true if any field in the entity is a rel_ref
// generator with unique: true in its config.
func hasUniqueRelRef(entity *plan.EntitySpec) bool {
	for _, f := range entity.Fields {
		if f.Gen == "rel_ref" {
			if u, ok := f.Config["unique"].(bool); ok && u {
				return true
			}
		}
	}
	return false
}
```

Refactor `Run()` to:
1. Create `store := newMapEntityStore()` and `required := requiredColumns(p.Entities)`.
2. Call `runEntity` (now returns `[]*writer.OrderedMap, error` instead of writing directly).
3. Write records to writer in `Run()`.
4. Extract and store required columns after writing.

Refactor `runEntity`:
1. Change return type from `error` to `([]*writer.OrderedMap, error)`.
2. Remove the write loop (lines 166-172). Return collected records instead.
3. Add `store registry.ReadOnlyEntityStore` parameter.
4. Pass store to `runChunk`.

Refactor `runChunk`:
1. Add `store registry.ReadOnlyEntityStore` parameter.
2. When creating generators, if `field.genName == "rel_ref"` and store is non-nil, inject `_store` via `copyConfig`.

Worker count override: In `runEntity`, if `hasUniqueRelRef(entity)` and `entity.DrivenBy == nil`, override to workers=1 for that entity only.

```go
// In runEntity, before starting workers:
workers := e.workerCount()
if hasUniqueRelRef(entity) && entity.DrivenBy == nil {
	workers = 1 // unique rel_ref requires serial execution for entity-scoped uniqueness
}
```

Updated `Run()` structure:

```go
func (e *Executor) Run(ctx context.Context, p *plan.Plan) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer e.closeWithError(&err)

	if err := plan.Validate(p); err != nil {
		return err
	}

	store := newMapEntityStore()
	required := requiredColumns(p.Entities)

	for idx := range p.Entities {
		entity := &p.Entities[idx]
		var records []*writer.OrderedMap
		var err error

		if entity.DrivenBy != nil {
			records, err = e.runDrivenByEntity(ctx, p.Seed, idx, entity, store)
		} else {
			records, err = e.runEntity(ctx, p.Seed, idx, entity, store)
		}
		if err != nil {
			return fmt.Errorf("failed to generate %s entity: %w", entity.Name, err)
		}

		for _, record := range records {
			if err := e.writer.WriteRecord(entity.Name, record); err != nil {
				return err
			}
		}

		// Extract and store required columns
		for key := range required {
			parts := strings.SplitN(key, ".", 2)
			if parts[0] == entity.Name {
				store.StoreColumn(entity.Name, parts[1], extractColumn(records, parts[1]))
			}
		}
	}
	return nil
}
```

Note: `runDrivenByEntity` does not exist yet — add a stub that returns `nil, fmt.Errorf("driven_by not yet implemented")`. It will be implemented in Task 9.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime/ -v -run TestExecutorRelRef`
Expected: PASS.

- [ ] **Step 5: Run all existing tests including golden tests**

Run: `go test ./... 2>&1 | tail -20`
Expected: All pass. Existing golden tests pass because the refactor preserves behavior (same records, same order, same output).

- [ ] **Step 6: Commit**

```
feat(runtime): add entity store, pre-scan, column extraction, and rel_ref store injection
```

---

### Task 8: Implement `driven_by` execution path

**Files:**
- Modify: `internal/runtime/executor.go`
- Create: `internal/runtime/driven_by.go`

- [ ] **Step 1: Write failing executor test for `driven_by`**

Add to `executor_test.go`:

```go
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
		expectedUserID := (i/2) + 1
		expected := fmt.Sprintf(`"user_id":%d`, expectedUserID)
		if !strings.Contains(line, expected) {
			t.Errorf("order line %d: expected %s, got: %s", i, expected, line)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/ -v -run TestExecutorDrivenBy`
Expected: FAIL — `driven_by` execution path not implemented.

- [ ] **Step 3: Implement `driven_by` execution in `driven_by.go`**

Create `internal/runtime/driven_by.go` with:

```go
package runtime

import (
	"apery/internal/plan"
	"apery/internal/registry"
	"apery/internal/rng"
	"fmt"
	"sort"
)

// drivenByLayout holds the precomputed child counts and prefix sums
// for a driven_by entity's two-phase execution.
type drivenByLayout struct {
	counts    []int64 // number of children per parent
	prefixSum []int64 // prefixSum[i] = total rows before parent i
	total     int64   // total child rows across all parents
	parentCol []any   // parent field values to inject
}

// computeDrivenByLayout runs Phase 1: determines child counts per parent.
func computeDrivenByLayout(entitySeed rng.Seed, db *plan.DrivenBy, parentValues []any) *drivenByLayout {
	n := int64(len(parentValues))
	counts := make([]int64, n)
	prefixSum := make([]int64, n)
	var total int64

	for i := int64(0); i < n; i++ {
		countSeed := rng.Derive(entitySeed, fmt.Sprintf("count[%d]", i))
		r := rng.New(countSeed)
		count := db.Min
		if db.Max > db.Min {
			count = db.Min + r.Int63n(db.Max-db.Min+1)
		}
		counts[i] = count
		prefixSum[i] = total
		total += count
	}

	return &drivenByLayout{
		counts:    counts,
		prefixSum: prefixSum,
		total:     total,
		parentCol: parentValues,
	}
}

// parentForRow returns the parent index for a given global row index
// using binary search on the prefix sum.
func (l *drivenByLayout) parentForRow(globalRow int64) int64 {
	// Find the last parent whose prefixSum <= globalRow
	idx := sort.Search(len(l.prefixSum), func(i int) bool {
		return l.prefixSum[i] > globalRow
	})
	return int64(idx - 1)
}

// makeDrivenByChunks creates parent-aligned chunks when unique fields exist,
// or standard row-based chunks otherwise.
func makeDrivenByChunks(layout *drivenByLayout, chunkSize int64, needsAlignment bool) []chunk {
	if !needsAlignment {
		return makeChunks(layout.total, chunkSize)
	}

	// Parent-aligned: each chunk boundary falls on a parent boundary
	var chunks []chunk
	idx := 0
	start := int64(0)
	for start < layout.total {
		end := start + chunkSize
		if end > layout.total {
			end = layout.total
		}
		// Align end to the next parent boundary
		if end < layout.total {
			parentIdx := layout.parentForRow(end - 1)
			// Include all children of this parent
			if parentIdx+1 < int64(len(layout.prefixSum)) {
				end = layout.prefixSum[parentIdx+1]
			} else {
				end = layout.total
			}
		}
		chunks = append(chunks, chunk{Start: start, End: end, Index: idx})
		idx++
		start = end
	}
	return chunks
}
```

- [ ] **Step 4: Add `runDrivenByEntity` to executor**

In `executor.go`, add a method that handles the driven_by path. Called from `Run()` when `entity.DrivenBy != nil`:

```go
// runDrivenByEntity generates rows for an entity driven by a parent entity.
// Phase 1: compute child counts per parent. Phase 2: chunked parallel generation.
func (e *Executor) runDrivenByEntity(ctx context.Context, seed int64, entityIndex int, entity *plan.EntitySpec, store *mapEntityStore) ([]*writer.OrderedMap, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	db := entity.DrivenBy
	entitySeed := rng.Derive(rng.SeedFromInt64(seed), fmt.Sprintf("%s[%d]", entity.Name, entityIndex))

	parentCol, ok := store.GetColumn(db.Entity, db.Field)
	if !ok {
		return nil, fmt.Errorf("driven_by: column %s.%s not found in store", db.Entity, db.Field)
	}

	layout := computeDrivenByLayout(entitySeed, db, parentCol)
	if layout.total == 0 {
		return nil, nil
	}

	fields, err := e.initFields(entity, entitySeed)
	if err != nil {
		return nil, err
	}

	needsAlignment := hasUniqueRelRef(entity)
	chunks := makeDrivenByChunks(layout, e.chunkSize, needsAlignment)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	chunkCh := make(chan chunk)
	resultCh := make(chan chunkResult, len(chunks))

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	setErr := func(err error) {
		errOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	workers := e.workerCount()
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ch := range chunkCh {
				records, err := e.runDrivenByChunk(ctx, entity, fields, ch, store, layout)
				if err != nil {
					setErr(err)
					return
				}
				resultCh <- chunkResult{index: ch.Index, records: records}
			}
		}()
	}
	for _, ch := range chunks {
		chunkCh <- ch
	}
	close(chunkCh)
	wg.Wait()
	close(resultCh)

	results := make([][]*writer.OrderedMap, len(chunks))
	for res := range resultCh {
		results[res.index] = res.records
	}
	if firstErr != nil {
		return nil, firstErr
	}

	var all []*writer.OrderedMap
	for _, records := range results {
		all = append(all, records...)
	}
	return all, nil
}
```

```go
// runDrivenByChunk generates rows for a driven_by entity chunk.
// It maps global row indices to parent indices, injects the parent value,
// and resets Resettable generators on parent transitions.
func (e *Executor) runDrivenByChunk(ctx context.Context, entity *plan.EntitySpec, fields []fieldRuntime, ch chunk, store registry.ReadOnlyEntityStore, layout *drivenByLayout) ([]*writer.OrderedMap, error) {
	db := entity.DrivenBy

	// Create per-chunk generator instances
	chunkFields := make([]chunkField, 0, len(fields))
	for _, field := range fields {
		var gen registry.Generator
		var err error
		if field.genName == "rel_ref" && store != nil {
			cfg := copyConfig(field.config)
			cfg["_store"] = store
			gen, err = field.factory(cfg)
		} else {
			gen, err = field.factory(field.config)
		}
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field.name, err)
		}
		if seeker, ok := gen.(seekableGenerator); ok {
			if err := seeker.SeekRow(ch.Start); err != nil {
				return nil, fmt.Errorf("field '%s': %w", field.name, err)
			}
		}
		chunkFields = append(chunkFields, chunkField{name: field.name, gen: gen, seed: field.seed})
	}

	records := make([]*writer.OrderedMap, 0, int(ch.End-ch.Start))
	lastParent := int64(-1)

	for row := ch.Start; row < ch.End; row++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		parentIdx := layout.parentForRow(row)

		// Reset Resettable generators on parent transition
		if parentIdx != lastParent {
			if lastParent != -1 {
				for _, cf := range chunkFields {
					if r, ok := cf.gen.(registry.Resettable); ok {
						r.Reset()
					}
				}
			}
			lastParent = parentIdx
		}

		record := writer.NewOrderedMap()
		// Inject parent value as first field
		record.Set(db.As, layout.parentCol[parentIdx])

		// Generate remaining fields
		for _, field := range chunkFields {
			rowSeed := rng.DeriveIndex(field.seed, row)
			r := rng.New(rowSeed)

			var val any
			var err error
			if ra, ok := field.gen.(registry.RowAwareGenerator); ok {
				val, err = ra.NextWithRow(r, record)
			} else {
				val, err = field.gen.Next(r)
			}
			if err != nil {
				return nil, fmt.Errorf("row %d, field '%s': %w", row, field.name, err)
			}
			record.Set(field.name, val)
		}
		records = append(records, record)
	}
	return records, nil
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/runtime/ -v -run TestExecutorDrivenBy`
Expected: PASS.

- [ ] **Step 7: Run all tests**

Run: `go test ./... 2>&1 | tail -20`
Expected: All pass.

- [ ] **Step 8: Commit**

```
feat(runtime): implement driven_by execution with two-phase chunking
```

---

### Task 9: Add driven_by + rel_ref + unique integration test (M:N pattern)

**Files:**
- Modify: `internal/runtime/executor_test.go`

- [ ] **Step 1: Write M:N integration test**

```go
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
	// Enrollment lines: 10 students × 2-3 each = 20-30
	enrollmentLines := len(lines) - studentLines - courseLines
	if enrollmentLines < 20 || enrollmentLines > 30 {
		t.Fatalf("expected 20-30 enrollment lines, got %d", enrollmentLines)
	}

	// Verify uniqueness: no student should have duplicate course_ids
	// Parse enrollment lines and group by student_id
	// (simplified: just verify no adjacent same-student lines have same course_id)
}
```

- [ ] **Step 2: Write determinism test for driven_by**

```go
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
				{Name: "amount", Gen: "int", Config: map[string]any{"min": 1, "max": 1000}},
			}},
		},
	}

	data1 := runPlanWithOpts(t, p, WithWorkers(1), WithChunkSize(1000))
	data2 := runPlanWithOpts(t, p, WithWorkers(4), WithChunkSize(50))

	if !bytes.Equal(data1, data2) {
		t.Fatal("driven_by output differs between worker configurations")
	}
}
```

- [ ] **Step 3: Write `seq` contiguous behavior test inside `driven_by`**

```go
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
	orderLines := lines[3:] // skip 3 User lines
	// seq should produce 1,2,3,4,5,6 across all parents (not 1,2,1,2,1,2)
	for i, line := range orderLines {
		expected := fmt.Sprintf(`"order_id":%d`, i+1)
		if !strings.Contains(line, expected) {
			t.Errorf("order line %d: expected %s in: %s", i, expected, line)
		}
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/runtime/ -v -run "TestExecutorManyToMany|TestExecutorDrivenByDeterminism|TestExecutorDrivenBySeqContiguous"`
Expected: PASS.

- [ ] **Step 5: Commit**

```
test(runtime): add M:N, determinism, and seq contiguous tests for driven_by
```

---

## Chunk 4: Golden Tests, Docs, and Cleanup

### Task 10: Add `relational` canonical plan to golden tests

**Files:**
- Modify: `internal/runtime/determinism_helpers_test.go`

- [ ] **Step 1: Add `relationalPlan` and register it in `canonicalPlans`**

```go
var relationalPlan = plan.Plan{
	Seed: goldenSeed,
	Entities: []plan.EntitySpec{
		{Name: "Department", Count: 5, Fields: []plan.FieldSpec{
			{Name: "id", Gen: "seq"},
			{Name: "name", Gen: "pick", Config: map[string]any{
				"values": []any{"Engineering", "Sales", "Marketing", "Support", "Finance"},
			}},
		}},
		{Name: "Employee", Count: 10000, Fields: []plan.FieldSpec{
			{Name: "id", Gen: "seq"},
			{Name: "dept_id", Gen: "rel_ref", Config: map[string]any{
				"entity": "Department", "field": "id",
			}},
			{Name: "dept_id_skewed", Gen: "rel_ref", Config: map[string]any{
				"entity": "Department", "field": "id",
				"distribution": "zipf", "s": 2.0,
			}},
		}},
		{Name: "Task", DrivenBy: &plan.DrivenBy{
			Entity: "Employee", Field: "id", As: "employee_id", Min: 1, Max: 3,
		}, Fields: []plan.FieldSpec{
			{Name: "title", Gen: "pick", Config: map[string]any{
				"values": []any{"review", "deploy", "test", "design", "document"},
			}},
		}},
		{Name: "Skill", Count: 20, Fields: []plan.FieldSpec{
			{Name: "id", Gen: "seq"},
			{Name: "name", Gen: "pick", Config: map[string]any{
				"values": []any{"Go", "Python", "Rust", "JS", "SQL", "Docker", "K8s", "AWS", "GCP", "Linux"},
			}},
		}},
		{Name: "EmployeeSkill", DrivenBy: &plan.DrivenBy{
			Entity: "Employee", Field: "id", As: "employee_id", Min: 2, Max: 5,
		}, Fields: []plan.FieldSpec{
			{Name: "skill_id", Gen: "rel_ref", Config: map[string]any{
				"entity": "Skill", "field": "id", "unique": true,
			}},
		}},
	},
}
```

Add to `canonicalPlans`:
```go
{"relational", &relationalPlan},
```

- [ ] **Step 2: Regenerate golden files**

Run: `go test ./internal/runtime -run TestGolden -update -v`
Expected: Golden files created for `relational.digest` and `relational.jsonl`.

- [ ] **Step 3: Run golden tests**

Run: `go test ./internal/runtime -run TestGolden -v`
Expected: All pass (including existing scalar, composite, row_aware).

- [ ] **Step 4: Run stress tests**

Run: `go test ./internal/runtime -run TestStress -v`
Expected: All pass. Relational plan produces identical output across randomized worker/chunk configs.

- [ ] **Step 5: Commit**

```
test(runtime): add relational canonical plan to golden and stress tests
```

---

### Task 11: Update `cmd/apery/main.go` with relational example

**Files:**
- Modify: `cmd/apery/main.go`

- [ ] **Step 1: Add a relational entity to the example plan**

Add an `Order` entity after the existing `User` entity that uses `driven_by` and `rel_ref`:

```go
{
	Name: "Order",
	DrivenBy: &apery.DrivenBy{
		Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 5,
	},
	Fields: []apery.FieldSpec{
		{Name: "order_id", Gen: "seq"},
		{Name: "amount", Gen: "int", Config: map[string]any{"min": 10, "max": 500}},
		{Name: "status", Gen: "pick", Config: map[string]any{
			"values": []any{"pending", "shipped", "delivered"},
		}},
	},
},
```

- [ ] **Step 2: Run to verify it works**

Run: `go run ./cmd/apery`
Expected: Generates output with User rows followed by Order rows with `user_id` field.

- [ ] **Step 3: Commit**

```
feat(example): add relational Order entity to main.go example
```

---

### Task 12: Update all documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `docs/spec.md`
- Modify: `docs/plan.md`

- [ ] **Step 1: Update `CLAUDE.md`**

1. Add `rel_ref` to the generator list in the "Built-in generators" line.
2. In the Architecture section under Plan, mention `DrivenBy` on `EntitySpec`.
3. In "Adding New Generators", add note about `relational` canonical plan category.
4. Add invariant: "Generator factories must not mutate config values."

- [ ] **Step 2: Update `README.md`**

1. Add "Relational Generators" subsection under Generators:
```markdown
### Relational Generators

- `rel_ref` - Foreign key sampling from a previously generated entity's column (uniform or Zipf distribution, optional uniqueness)
```

2. Add "Relational Data" feature to the feature list:
```markdown
- Relational data generation (M:1, 1:M, M:N via `rel_ref` and `driven_by`)
```

3. Add a relational example showing the M:N pattern (Student/Course/Enrollment or similar).

4. Update the example plan if needed to include the relational Order entity.

- [ ] **Step 3: Update `docs/spec.md`**

1. Remove `m2m(target,meanDegree)` from section 5.3 and appendix A.4.3.
2. Add note that M:N is achieved via composition of `driven_by` + `rel_ref`.
3. Update section 7.5 (Relation Resolution) to describe the actual implementation.

- [ ] **Step 4: Update `docs/plan.md`**

1. Check off `rel_ref(target,field)`.
2. Replace `m2m(target,meanDegree)` line with: `[x] M:N via composition (driven_by + rel_ref + unique)`.
3. Check off `Relation resolution (M:1 alias sampling, 1:M multinomial, M:N degree + dedupe)`.

- [ ] **Step 5: Review all changes for accuracy**

Read each modified file and verify no outdated references remain.

- [ ] **Step 6: Commit**

```
docs: update all documentation for relational generators
```

---

### Task 13: Final verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -v 2>&1 | tail -30`
Expected: All pass.

- [ ] **Step 2: Run golden tests**

Run: `go test ./internal/runtime -run TestGolden -v`
Expected: All pass.

- [ ] **Step 3: Run stress tests**

Run: `go test ./internal/runtime -run TestStress -v`
Expected: All pass.

- [ ] **Step 4: Build**

Run: `go build ./cmd/apery`
Expected: Clean build.

- [ ] **Step 5: Run example**

Run: `go run ./cmd/apery`
Expected: Produces output.jsonl with User and Order records. Orders have valid user_id values.
