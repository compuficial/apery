# Relational Generators Design Spec

**Date:** 2026-03-15
**Status:** Draft

## Overview

Add relational data generation to Apery through two primitives:

1. **`rel_ref` generator** — M:1 foreign key sampling from a previously generated entity's column
2. **`driven_by` plan-level concept** — 1:M parent-driven child row generation

M:N relationships are expressed as composition: a junction entity uses `driven_by` (1:M from left) + `rel_ref` with `unique: true` (M:1 to right). No dedicated `m2m` generator is needed.

## Design Decisions

- **Explicit entity ordering** — Entities generate in declared order. If entity B references entity A, A must appear first. Validation fails with a clear error otherwise. Consistent with existing field dependency ordering.
- **Column store, not row store** — After generating an entity, the executor stores only columns referenced by later `rel_ref` fields or `DrivenBy` specs. O(rows × referenced_columns) memory. Referenced columns are determined by a pre-scan pass in the executor before generation begins.
- **Composition over special-casing** — M:N falls out of `driven_by` + `rel_ref` + `unique`. No new generator type needed.
- **Self-referencing entities are not supported** — An entity cannot reference itself via `rel_ref` or `driven_by`. The "must appear earlier" validation rule naturally prevents this. This is an intentional non-goal; recursive/hierarchical data generation (e.g., org charts, comment trees) is out of scope.
- **Reserved config key prefix** — Config keys starting with `_` are reserved for executor-internal use. Plan validation rejects user configs containing `_`-prefixed keys.
- **Generator factories must not mutate config values** — Shallow config copies share slice/map references. Factories must treat config values as read-only. This is an invariant that applies to all generators (not just `rel_ref`) and should be documented in the "Adding New Generators" section of `CLAUDE.md`.
- **`DrivenBy.Min >= 1`** — `driven_by` always produces at least one child per parent. Optional relationships (some parents have zero children) should use `rel_ref` on the child entity with a static `Count` instead. This avoids complexity in the prefix-sum/chunking model around zero-length parent batches.

## `rel_ref` Generator

### Config

| Key            | Type     | Required | Default     | Description |
|----------------|----------|----------|-------------|-------------|
| `entity`       | string   | yes      |             | Source entity name |
| `field`        | string   | yes      |             | Source field name to sample from |
| `distribution` | string   | no       | `"uniform"` | Sampling distribution: `"uniform"` or `"zipf"` |
| `s`            | float64  | no       | `1.5`       | Zipf `s` parameter (only when distribution is `"zipf"`) |
| `unique`       | bool     | no       | `false`     | No duplicate values within uniqueness scope (see below) |

### Behavior

- Reads from the cross-entity column store, not the current row. Implements `Generator`, not `RowAwareGenerator`.
- **Uniform mode**: Selects from the stored column with equal probability per value.
- **Zipf mode**: Selects with Zipf-distributed skew. Higher-ranked values (earlier in generation order) are selected more frequently. Uses the same `zipf` distribution logic as the existing `zipf` generator.

### Factory and Store Injection

The `rel_ref` factory reads the store from the internal config key `"_store"`. The factory tolerates a nil/missing store — it stores a nil reference and defers the actual store lookup to `Next()` time. This is necessary because `initFields` in the executor calls the factory for dependency validation before the store is populated. The generator returns an error from `Next()` if the store is still nil at generation time.

In `runChunk`, the executor injects the populated store via `"_store"` before calling the factory, so all per-chunk generator instances have a valid store reference.

### Unique Mode

When `unique: true`, uniqueness is enforced **by value** — no duplicate output values within the scope, even if multiple array indices in the source column contain the same value. The feasibility pool size is the count of *distinct* values in the source column, not the total column length.

Scoping rules:

- **Inside a `driven_by` entity**: Uniqueness is scoped to the current parent's batch of children. The executor resets the uniqueness tracker between parent rows via the `Resettable` interface (see below). All children of a single parent are guaranteed to land in the same chunk (see Chunked Parallel Execution), so the tracker is chunk-local with no cross-worker contention.
- **Outside `driven_by` (standalone entity)**: Uniqueness is scoped to the entire entity. This requires single-threaded execution (workers=1) for that entity. The executor detects this at init time and overrides the worker count for entities that have `unique: true` on any `rel_ref` field without a `driven_by` parent. This is a deliberate trade-off: global uniqueness across parallel chunks is fundamentally incompatible with determinism, so we enforce serial execution.
- **Statefulness**: `rel_ref` with `unique: true` maintains an internal set of seen values across `Next()` calls within a chunk. Determinism relies on rows within a chunk being processed sequentially (the `runChunk` row loop is sequential).
- **Collision handling**: On collision, resamples up to 100 times. Returns an error if retries are exhausted.
- **Error message**: `field 'course_id' in entity 'Enrollment': rel_ref unique constraint failed: requested 60 unique values from pool of 50 distinct values`

### `Resettable` Interface

Defined in `internal/registry/registry.go` alongside `Generator`, `RowAwareGenerator`, and `DependencyDeclarer`.

```go
// Resettable is implemented by generators with internal state that must be
// cleared between parent batches in driven_by entities.
type Resettable interface {
    Reset()
}
```

The executor calls `Reset()` on generators implementing this interface each time the parent index changes within the `runChunk` row loop for `driven_by` entities. `rel_ref` with `unique: true` implements `Resettable` to clear its seen-values set.

### Determinism

`rel_ref` uses the standard seed derivation hierarchy: root → entity → field → row. The column store is populated in entity generation order, which is deterministic. Zipf ranking is based on array index (insertion order), which is deterministic.

### Example

```go
{Name: "user_id", Gen: "rel_ref", Config: map[string]any{
    "entity": "User",
    "field":  "id",
    "distribution": "zipf",
    "s": 1.5,
}}
```

## `driven_by` on EntitySpec

### Plan Structure

```go
type DrivenBy struct {
    Entity string // parent entity name
    Field  string // parent field to inject into child rows
    As     string // field name in child row for the injected value
    Min    int64  // minimum children per parent
    Max    int64  // maximum children per parent
}

type EntitySpec struct {
    Name     string
    Count    int64     // used when DrivenBy is nil
    DrivenBy *DrivenBy // used for 1:M parent-driven generation
    Fields   []FieldSpec
}
```

When `DrivenBy` is set, `Count` must be 0 (its zero value). The total row count is dynamic: sum of `rand_uniform(Min, Max)` across all parent rows.

### Behavior

- The executor iterates each parent row in order.
- For each parent, it generates `Min` to `Max` child rows (uniform random, deterministic per parent via seed derivation).
- Each child row has the parent's referenced field value auto-injected as the **first field** (named by `DrivenBy.As`), before any declared fields. In the `runChunk` row loop, this is inserted into the `OrderedMap` via `record.Set(drivenBy.As, parentValue)` before the field generation loop. This ensures it appears first in JSONL/CSV output.
- Child row fields are then generated normally, including any `rel_ref` fields.
- The child count per parent is derived from: `rng.Derive(entitySeed, fmt.Sprintf("count[%d]", parentIndex))`.
- The `As`-injected field is a first-class field: it is stored in the column store if referenced by downstream entities, and it can be referenced by `rel_ref` or `DrivenBy` from later entities.

### `seq` Generator Behavior

The `seq` generator inside a `driven_by` entity produces a contiguous global sequence across all parents (1, 2, 3, ..., N), not resetting per parent. This is consistent with `seq`'s existing behavior of using `SeekRow` with the global row index.

### Validation

- `DrivenBy.Entity` must reference an entity declared earlier in the plan.
- `DrivenBy.Field` must exist in the referenced entity's field list (including the `As`-injected field of that entity's own `DrivenBy`, if applicable).
- `DrivenBy.As` must not conflict with any field name in the child entity's `Fields`.
- `DrivenBy.Min` must be >= 1. A minimum of 0 children per parent is not supported — use `rel_ref` without `driven_by` for optional relationships.
- `DrivenBy.Max` must be >= `DrivenBy.Min`.
- `Count` and `DrivenBy` are mutually exclusive. Validation replaces the existing unconditional `Count > 0` check with: if `DrivenBy` is nil, `Count` must be > 0; if `DrivenBy` is set, `Count` must be 0.

### Determinism

Child count per parent uses a dedicated seed derived from the entity seed + parent index. This keeps the child count independent of child row content. Child rows use the standard hierarchy but with a composite index: `rng.DeriveIndex(fieldSeed, globalRowIndex)` where `globalRowIndex` is the absolute position across all parents (not relative to the current parent). This preserves the chunked parallel execution model.

### Seed Derivation

The entity seed follows the existing convention: `rng.Derive(rng.SeedFromInt64(planSeed), fmt.Sprintf("%s[%d]", entity.Name, entityIndex))` where `entityIndex` is the entity's position in the plan's `Entities` slice.

```
Root Seed (plan.Seed)
  └─> Parent Entity Seed: Derive(root, "User[0]")
  └─> Child Entity Seed: Derive(root, "Order[1]")
      ├─> Count Seed: Derive(entitySeed, "count[parentIdx]")
      │   └─> determines how many children this parent gets
      └─> Field Seeds: Derive(entitySeed, fieldName)
          └─> Row Seeds: DeriveIndex(fieldSeed, globalRowIndex)
```

## Cross-Entity Column Store

### Interface

Both interfaces are defined in `internal/registry/registry.go`. This avoids import cycles: `registry` defines them, generators in `registry` accept `ReadOnlyEntityStore`, and `runtime` (which already imports `registry`) uses the full `EntityStore` to populate it.

```go
// EntityStore is used by the executor to populate the store after entity generation.
type EntityStore interface {
    StoreColumn(entity, field string, values []any)
    ReadOnlyEntityStore
}

// ReadOnlyEntityStore is the read-only view passed to generators.
type ReadOnlyEntityStore interface {
    GetColumn(entity, field string) ([]any, bool)
}
```

Generators receive `ReadOnlyEntityStore` — they cannot mutate the store. The executor holds the full `EntityStore` to populate it. The concrete implementation lives in `internal/runtime` (e.g., a `mapEntityStore` struct).

### Determining Referenced Columns

The executor performs a pre-scan pass over all entities before generation begins to determine which (entity, field) pairs need storage. This scan inspects:

1. All `rel_ref` field configs for their `entity` and `field` values.
2. All `DrivenBy` specs for their `Entity` and `Field` values.

This is a separate pass from `plan.Validate()` — validation checks structural correctness, the pre-scan determines storage requirements. The pre-scan runs after validation succeeds.

### Lifecycle

1. **Pre-scan**: Executor scans all entities to build a set of required (entity, field) pairs.
2. **Entity generation**: After an entity finishes generating, the executor extracts required columns from the generated records and stores them. The `DrivenBy.As`-injected field is included if referenced downstream.
3. **Downstream generation**: `rel_ref` generators receive the store via the `"_store"` config key injected by the executor in `runChunk`.
4. **Cleanup**: The store is discarded when `Run()` completes.

### Passing the Store to Generators

The executor injects `"_store"` into a shallow copy of the config map before calling the `rel_ref` factory in `runChunk`. Shallow copy is sufficient because config values are primitives, slices, and maps that are not mutated by generators.

```go
// copyConfig returns a shallow copy of a config map.
func copyConfig(cfg map[string]any) map[string]any {
    out := make(map[string]any, len(cfg)+1)
    for k, v := range cfg {
        out[k] = v
    }
    return out
}

// In runChunk, before calling factory:
if field.genName == "rel_ref" {
    cfg := copyConfig(field.config)
    cfg["_store"] = store
    gen, err = field.factory(cfg)
} else {
    gen, err = field.factory(field.config)
}
```

During `initFields`, the factory is called without `"_store"` for dependency validation. The `rel_ref` factory tolerates this and stores a nil reference. The throwaway generator from `initFields` is never used for generation.

## M:N via Composition

A many-to-many relationship between Students and Courses:

```go
Plan{
    Seed: 42,
    Entities: []EntitySpec{
        {
            Name:  "Student",
            Count: 1000,
            Fields: []FieldSpec{
                {Name: "id", Gen: "seq"},
                {Name: "name", Gen: "pick", Config: map[string]any{"values": []any{"Alice", "Bob", "Carol"}}},
            },
        },
        {
            Name:  "Course",
            Count: 50,
            Fields: []FieldSpec{
                {Name: "id", Gen: "seq"},
                {Name: "title", Gen: "pick", Config: map[string]any{"values": []any{"Math", "Physics", "History"}}},
            },
        },
        {
            Name: "Enrollment",
            DrivenBy: &DrivenBy{
                Entity: "Student",
                Field:  "id",
                As:     "student_id",
                Min:    2,
                Max:    5,
            },
            Fields: []FieldSpec{
                {Name: "course_id", Gen: "rel_ref", Config: map[string]any{
                    "entity": "Course",
                    "field":  "id",
                    "unique": true,
                }},
                {Name: "enrolled_at", Gen: "time", Config: map[string]any{
                    "start": "2025-01-01T00:00:00Z",
                    "end":   "2025-12-31T23:59:59Z",
                }},
            },
        },
    },
}
```

This generates:
- 1000 Students
- 50 Courses
- ~3500 Enrollments (1000 students × avg 3.5 enrollments each)
- Each student's enrollments reference unique courses (no duplicate enrollments)

## Chunked Parallel Execution

### `rel_ref` entities (no `driven_by`)

No change to the existing chunking model. Each chunk generates rows independently. The column store is read-only during child entity generation.

**Exception**: If any `rel_ref` field has `unique: true`, the entity runs single-threaded (workers=1) to maintain a consistent uniqueness tracker. The executor detects this during `initFields` and overrides the worker count for that entity only.

### `driven_by` entities

Parent-driven entities cannot use the standard chunking model directly because row count is dynamic (depends on per-parent child counts). Instead:

1. **Phase 1 — Count generation**: Single-threaded pass over all parent rows to determine child counts. This produces a `[]int64` of counts and a prefix sum for global row index mapping. Uses dedicated count seeds. This is lightweight (one RNG call per parent row).
2. **Phase 2 — Chunked generation**: With total row count known, standard chunking applies. Each chunk maps global row indices back to (parentIndex, localChildIndex) via binary search on the prefix sum. The parent's field value is injected based on this mapping.

**Parent-aligned chunk boundaries**: When any `rel_ref` field has `unique: true`, chunk boundaries are aligned to parent boundaries — a parent's children are never split across chunks. This ensures the uniqueness tracker is chunk-local with no cross-worker contention. The alignment is computed from the prefix sum: each chunk starts at a parent boundary and includes all children of its last parent.

When no `unique` fields exist, standard row-based chunking applies (no alignment needed).

**Resetting uniqueness trackers**: Within a chunk processing a `driven_by` entity, the executor tracks the current parent index. When the parent index changes (detected via the prefix sum mapping), the executor calls `Reset()` on all generators implementing `Resettable`. This clears the `rel_ref` uniqueness tracker for the new parent's batch.

## Validation Changes

`plan.Validate()` gains:

1. **Entity dependency ordering** — If `DrivenBy` or any field's `rel_ref` references entity X, X must appear earlier. This requires validation to parse `rel_ref` configs to extract `entity` values.
2. **`DrivenBy` field existence** — `DrivenBy.Field` must exist in the target entity's fields (including its `DrivenBy.As` field if it has one).
3. **`DrivenBy.As` conflict check** — Must not collide with child entity field names.
4. **`Count`/`DrivenBy` mutual exclusion** — Replaces the existing `Count > 0` check. If `DrivenBy` is nil, `Count > 0` is required. If `DrivenBy` is set, `Count` must be 0.
5. **`rel_ref` entity/field existence** — Referenced entity and field must exist earlier in the plan.
6. **`unique` feasibility (static check)** — When `unique: true` and the entity has `DrivenBy`, error at validation time if `DrivenBy.Max` exceeds the referenced entity's `Count` (for entities with a static `Count`). This catches provably impossible configurations. For referenced entities that are themselves `driven_by` (dynamic row count), static checking is not feasible — the runtime collision-retry error handles exhaustion at generation time.
7. **Self-reference prohibition** — An entity cannot reference itself in `rel_ref` or `DrivenBy`. This is implied by the "must appear earlier" rule but validated explicitly with a clear error message.
8. **Reserved config keys** — Reject any user-provided config key starting with `_`.

## Error Messages

- `entity 'Order' references 'User' via driven_by, but 'User' is not declared before it`
- `entity 'Order' driven_by field 'user_id' conflicts with declared field 'user_id'`
- `field 'user_id' in entity 'Order': rel_ref references entity 'User' field 'id', but 'User' is not declared before 'Order'`
- `field 'course_id' in entity 'Enrollment': rel_ref unique constraint failed: requested 60 unique values from pool of 50 distinct values`
- `entity 'Order': exactly one of Count or DrivenBy must be set`
- `entity 'User': cannot reference itself via rel_ref or driven_by`
- `entity 'Enrollment': driven_by min must be >= 1, got 0`
- `entity 'Enrollment': unique rel_ref 'course_id' requests up to 10 unique values but 'Course' has only 5 distinct values`
- `field 'status' in entity 'Order': config key '_store' is reserved for internal use`

## Golden Test Integration

`rel_ref` requires multi-entity plans with a column store, which does not fit into the existing canonical plan categories (scalar, composite, row-aware). A new canonical plan category `relational` should be added to `internal/runtime/determinism_helpers_test.go` and included in the `canonicalPlans` slice. Both `TestGolden` and `TestStress` will pick it up automatically.

The relational plan should exercise:
- `rel_ref` with uniform distribution
- `rel_ref` with zipf distribution
- `rel_ref` with `unique: true`
- `driven_by` with a parent entity
- M:N junction pattern (driven_by + unique rel_ref)

## Documentation Updates

After implementation, the following must be updated:

- **`CLAUDE.md`**: Add `rel_ref` to the generator list. Document `DrivenBy` on `EntitySpec`. Update the "Adding New Generators" workflow to mention the new `relational` canonical plan.
- **`README.md`**: Add a "Relational Generators" section under Generators. Add a relational example (e.g., Users + Orders or the Student/Course/Enrollment M:N example). Update the feature list to mention relational integrity.
- **`docs/spec.md`**: Remove `m2m` from the generator list and add a note that M:N is achieved via composition. Update the relation resolution section (7.5) to reflect the actual implementation.
- **`docs/plan.md`**: Check off `rel_ref(target,field)`. Replace `m2m(target,meanDegree)` with the composition approach. Check off relation resolution.
- **`internal/plan/plan.go`**: Package-level and type-level godoc for `DrivenBy`.
- **`internal/registry/rel_ref.go`**: Factory function docs, config parameter docs, `Resettable` implementation docs.
- **`internal/runtime/executor.go`**: Godoc for `EntityStore`, `ReadOnlyEntityStore`, `copyConfig`, the driven-by execution path, parent-aligned chunking, and the `Resettable` reset loop.
- **All new and modified files**: Function-level and package-level comments suitable for `go doc` generation.
