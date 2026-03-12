# Row-Context Generators Design

**Date:** 2026-03-12
**Scope:** `template` and `switch` generators, `RowAwareGenerator` interface, executor row-context support
**Deferred:** `pipe` (needs expression engine), `when`/`map`/`expr` (need expression engine), `rel_ref`/`m2m` (need relation resolution)

---

## Problem

The current `Generator` interface (`Next(*rng.Rng) (any, error)`) has no access to other fields in the current row. This prevents generators that depend on sibling field values, such as string interpolation (`template`) and conditional dispatch (`switch`).

## Design Decisions

1. **Opt-in second interface** over widening `Generator` — avoids touching all 18 existing generators.
2. **Explicit field ordering** over topological sort — users declare dependent fields after their dependencies; validated at init time.
3. **`pipe` deferred** — awkward without an expression engine; `template` + `switch` cover most practical use cases.
4. **Top-level fields only** — `template` and `switch` are only valid as top-level entity fields, not as sub-generators inside `object`/`list`. Those composite generators call `Next()`, not `NextWithRow()`, so row context would not be forwarded. This restriction will be revisited when composite generators gain row-context forwarding.

---

## 1. New Interfaces

Three new interfaces in `internal/registry/registry.go`:

### RowContext

```go
// RowContext provides read access to already-generated field values in the current row.
type RowContext interface {
    Get(fieldName string) (any, bool)
}
```

Read-only access to the in-progress row record. The executor passes the `*writer.OrderedMap` it's already building, which satisfies this interface via Go's structural typing (it already has a matching `Get` method).

### RowAwareGenerator

```go
// RowAwareGenerator is a generator that needs access to other fields in the current row.
type RowAwareGenerator interface {
    Generator
    NextWithRow(r *rng.Rng, row RowContext) (any, error)
}
```

Embeds `Generator` so it remains a valid `Generator`. The `Next()` method on implementations should return an error like `"template: requires row context"` as a safety net, but the executor will always call `NextWithRow` when the interface is satisfied.

### DependencyDeclarer

```go
// DependencyDeclarer is implemented by generators that reference other fields in the row.
type DependencyDeclarer interface {
    Dependencies() []string
}
```

Returns the field names this generator reads from the row. Used by the executor at init time to validate field ordering.

---

## 2. Executor Changes

Two changes to `internal/runtime/executor.go`:

### 2.1 Row Loop Dispatch

In `runChunk`, the per-field generation changes from always calling `Next()` to checking for `RowAwareGenerator`:

```go
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

    record.Set(field.name, val)
}
```

The type assertion costs a few nanoseconds per field per row (pointer comparison on type descriptor) — negligible versus generation work.

Note: `runChunk` creates fresh generator instances per chunk via `field.factory(field.config)`. The type assertion works because the factory returns a concrete type that implements `RowAwareGenerator`, and Go's interface assertion on the `Generator` interface value will correctly detect the wider interface.

### 2.2 Dependency Validation

In `initFields`, after instantiating all generators, validate that any generator implementing `DependencyDeclarer` only references fields that appear earlier in the field list:

```go
knownFields := make(map[string]bool)
for _, field := range fields {
    // initFields already creates a throwaway generator instance for config validation.
    // We reuse that pattern to also check dependency ordering.
    gen, _ := field.factory(field.config)
    if dd, ok := gen.(registry.DependencyDeclarer); ok {
        for _, dep := range dd.Dependencies() {
            if !knownFields[dep] {
                return fmt.Errorf("field '%s' references '%s', which must be declared before it", field.name, dep)
            }
        }
    }
    knownFields[field.name] = true
}
```

This runs once at plan init time, not per row. The generator instance created here is for validation only and is discarded — real per-chunk generator instances are created in `runChunk`.

---

## 3. `template` Generator

**File:** `internal/registry/template.go`
**Restriction:** Top-level entity fields only. Cannot be used as a sub-generator inside `object` or `list` (those call `Next()`, not `NextWithRow()`). The factory should validate this is not possible by design — since `object`/`list` call `registry.Get()` which returns a `Generator`, and the executor is the only caller that checks for `RowAwareGenerator`, nesting will silently fall through to the `Next()` error path at runtime. This is acceptable — the error message clearly states "requires row context".

### Config

```json
{
    "gen": "template",
    "config": {
        "tpl": "Hello, {first_name}! You are {age} years old."
    }
}
```

### Behavior

**Factory time:**
- Parse the template string into alternating literal and reference segments
- Extract all field name references (text between `{` and `}`)
- Validate: no empty placeholders `{}`, no unclosed `{`, no nested `{a{b}}`
- Store parsed parts and dependency list

**Generation time:**
- Iterate pre-parsed parts, replacing references with `fmt.Sprint(row.Get(fieldName))`
- If a referenced field is missing from the row context, return an error
- If a referenced field has a nil value, `fmt.Sprint(nil)` produces `"<nil>"` — this is acceptable since nil values are an explicit choice by the plan author
- Does not consume the RNG — output is purely a function of row context values

### Escape Sequences

- `{{` produces a literal `{`
- `}}` produces a literal `}`

### Determinism

Determinism is inherited from the referenced fields. The template generator itself introduces no randomness — same row context values always produce the same string output.

### Struct

```go
type TemplateGenerator struct {
    parts []templatePart  // pre-parsed segments
    deps  []string        // referenced field names
}

type templatePart struct {
    literal bool
    value   string  // literal text or field name
}
```

### Interfaces Implemented

- `Generator` — `Next()` returns error "requires row context"
- `RowAwareGenerator` — `NextWithRow()` performs interpolation
- `DependencyDeclarer` — `Dependencies()` returns referenced field names

---

## 4. `switch` Generator

**File:** `internal/registry/switch.go`
**Restriction:** Same as `template` — top-level entity fields only. If nested inside `object`/`list`, `Next()` will return an error "requires row context".

### Config

```json
{
    "gen": "switch",
    "config": {
        "key": "status",
        "cases": {
            "active": {"gen": "const", "config": {"value": "Welcome back!"}},
            "inactive": {"gen": "const", "config": {"value": "We miss you!"}}
        },
        "default": {"gen": "const", "config": {"value": "Hello!"}}
    }
}
```

### Behavior

**Factory time:**
- Validate `key` (required string — the field name to read)
- Validate and instantiate all case sub-generators (keyed by string)
- Validate and instantiate optional `default` sub-generator
- Error if `cases` is empty

**Generation time:**
- Read the key field's value from row context via `fmt.Sprint(row.Get(key))` (converts any type to string for matching)
- Look up the matching case in the map
- If no match and default exists, use default
- If no match and no default, return an error
- Pass a derived RNG (`rng.Derive(r.GetSeed(), "__value__")`) to the chosen sub-generator
- If the chosen sub-generator implements `RowAwareGenerator`, call `NextWithRow` forwarding the row context; otherwise call `Next`

### Determinism

1. **Selection:** same row context = same key value = same case chosen
2. **Value generation:** `rng.Derive(r.GetSeed(), "__value__")` produces a deterministic child seed from the row seed. All cases receive the same derived seed regardless of which case is selected — the selection path does not affect the seed, only which generator consumes it.
3. **Nested row-aware generators:** receive the same deterministic row context and derived seed

### Struct

```go
type SwitchGenerator struct {
    key      string
    cases    map[string]Generator
    fallback Generator  // optional, nil if no default
    deps     []string   // [key] + transitive deps from row-aware sub-generators
}
```

### Dependencies

`Dependencies()` returns the union of `[key]` plus any dependencies declared by sub-generators (cases and default) that implement `DependencyDeclarer`. This ensures transitive dependencies are caught by the executor's ordering validation. For example, if a case contains a `template` referencing field `name`, then `name` must also be declared before the `switch` field.

### Interfaces Implemented

- `Generator` — `Next()` returns error "requires row context"
- `RowAwareGenerator` — `NextWithRow()` performs key lookup and dispatch
- `DependencyDeclarer` — `Dependencies()` returns `[key]` plus transitive sub-generator dependencies

---

## 5. `OrderedMap` as `RowContext`

No code changes needed. `writer.OrderedMap` already has `Get(key string) (any, bool)` which satisfies the `registry.RowContext` interface via Go's structural typing. The executor passes the in-progress `record` directly to `NextWithRow`.

---

## 6. Testing Strategy

### Unit Tests (per generator)

- **Config validation:** valid configs, missing required fields, invalid types, empty cases
- **Determinism:** same seed + same row context = identical output
- **Output correctness:** template interpolation with various types, switch case selection, default fallback, missing field errors
- **Edge cases:** escaped braces in template, no-match-no-default in switch, nested row-aware sub-generators in switch

### Executor Integration Tests

- **Dependency validation:** fields referencing undeclared or later-declared fields produce clear errors
- **End-to-end:** plan with mix of regular and row-aware generators produces correct output
- **Determinism:** same plan + seed = identical output with row-aware generators

### No Changes to Existing Tests

Existing generator tests and executor tests should continue to pass unchanged since the `Generator` interface is not modified.

---

## 7. Files Changed

| File | Change |
|------|--------|
| `internal/registry/registry.go` | Add `RowContext`, `RowAwareGenerator`, `DependencyDeclarer` interfaces |
| `internal/runtime/executor.go` | Type-check dispatch in row loop, dependency validation in `initFields` |
| `internal/registry/template.go` | New file |
| `internal/registry/template_test.go` | New file |
| `internal/registry/switch.go` | New file |
| `internal/registry/switch_test.go` | New file |
| `internal/runtime/executor_test.go` | New integration tests for row-aware generators |
| `cmd/apery/main.go` | Add `template` and `switch` usage to example plan for spot-checking output |
| `docs/plan.md` | Check off `template` and `switch` |
| `CLAUDE.md` | Add `template`, `switch` to generator list |
| `README.md` | Add `template`, `switch` to composite generators section |
