# Row-Context Generators Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `template` and `switch` generators that can read other fields in the current row, plus the infrastructure to support them.

**Architecture:** Opt-in `RowAwareGenerator` interface (keeps existing `Generator` untouched). Executor type-checks at dispatch time and validates field ordering at init time. `OrderedMap` satisfies `RowContext` via structural typing — zero changes needed.

**Tech Stack:** Go 1.24, existing `apery` module. No new dependencies.

**Spec:** `docs/specs/2026-03-12-row-context-generators-design.md`

---

## Chunk 1: Interfaces and Executor Infrastructure

### Task 1: Add interfaces to registry

**Files:**
- Modify: `internal/registry/registry.go`

- [ ] **Step 1: Add `RowContext`, `RowAwareGenerator`, and `DependencyDeclarer` interfaces**

Append to `internal/registry/registry.go` after the existing `FactoryFor` function:

```go
// RowContext provides read access to already-generated field values in the current row.
type RowContext interface {
	Get(fieldName string) (any, bool)
}

// RowAwareGenerator is a generator that needs access to other fields in the current row.
// The executor calls NextWithRow instead of Next when this interface is satisfied.
// Next() should return an error as a safety net for incorrect usage (e.g., nesting inside object/list).
type RowAwareGenerator interface {
	Generator
	NextWithRow(r *rng.Rng, row RowContext) (any, error)
}

// DependencyDeclarer is implemented by generators that reference other fields in the row.
// The executor validates that all declared dependencies appear earlier in the field list.
type DependencyDeclarer interface {
	Dependencies() []string
}
```

- [ ] **Step 2: Verify all existing tests still pass**

Run: `go test ./...`
Expected: All tests PASS (no existing code changed, only new interfaces added)

- [ ] **Step 3: Commit**

```bash
git add internal/registry/registry.go
git commit -m "feat: add RowContext, RowAwareGenerator, DependencyDeclarer interfaces"
```

---

### Task 2: Add row-aware dispatch to executor

**Files:**
- Modify: `internal/runtime/executor.go`

- [ ] **Step 1: Update the `runChunk` row loop to dispatch via `RowAwareGenerator` when available**

In `runChunk`, replace the field loop body (lines 246-253):

```go
// before:
		for _, field := range chunkFields {
			rowSeed := rng.DeriveIndex(field.seed, row)
			val, err := field.gen.Next(rng.New(rowSeed))
			if err != nil {
				return nil, fmt.Errorf("row %d, field '%s': %w", row, field.name, err)
			}
			record.Set(field.name, val)
		}
```

```go
// after:
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
```

Note: `record` is `*writer.OrderedMap` which already satisfies `registry.RowContext` via structural typing (`Get(string) (any, bool)`). No cast or adapter needed.

- [ ] **Step 2: Verify all existing tests still pass**

Run: `go test ./...`
Expected: All tests PASS (behavior unchanged for non-RowAwareGenerator fields)

- [ ] **Step 3: Commit**

```bash
git add internal/runtime/executor.go
git commit -m "feat: executor dispatches via RowAwareGenerator when available"
```

---

### Task 3: Add dependency validation to executor

**Files:**
- Modify: `internal/runtime/executor.go`
- Modify: `internal/runtime/executor_test.go`

- [ ] **Step 1: Write failing test for dependency validation**

Add to `executor_test.go`. This test needs a stub `RowAwareGenerator` that declares dependencies. Create a minimal test helper at the top of the test file:

```go
// stubRowAwareGen is a test-only generator that implements RowAwareGenerator and DependencyDeclarer.
type stubRowAwareGen struct {
	deps []string
}

func (s *stubRowAwareGen) Next(r *rng.Rng) (any, error) {
	return nil, fmt.Errorf("stubRowAwareGen: requires row context")
}

func (s *stubRowAwareGen) NextWithRow(r *rng.Rng, row registry.RowContext) (any, error) {
	v, ok := row.Get(s.deps[0])
	if !ok {
		return nil, fmt.Errorf("missing dep %s", s.deps[0])
	}
	return fmt.Sprintf("got:%v", v), nil
}

func (s *stubRowAwareGen) Dependencies() []string {
	return s.deps
}
```

Then register a test generator in an `init()` or `TestMain` and write the actual test:

```go
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
		var buf bytes.Buffer
		w := writer.NewJSONLWriter(&buf)
		e := New(w)
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
		var buf bytes.Buffer
		w := writer.NewJSONLWriter(&buf)
		e := New(w)
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
		var buf bytes.Buffer
		w := writer.NewJSONLWriter(&buf)
		e := New(w)
		err := e.Run(context.Background(), p)
		if err == nil {
			t.Fatal("expected error for nonexistent dependency")
		}
	})
}
```

Note: check existing imports in `executor_test.go` — you may need to add `"bytes"`, `"strings"`, `"fmt"`, `"apery/internal/registry"`, and `"apery/internal/writer"`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runtime/ -run TestExecutorDependencyValidation -v`
Expected: FAIL — `initFields` doesn't check dependencies yet

- [ ] **Step 3: Implement dependency validation in `initFields`**

In `executor.go`, modify `initFields` to add dependency checking after the existing validation loop. The existing code already creates a throwaway generator at line 187 (`factory(field.Config)`). Restructure `initFields` to also check `DependencyDeclarer`:

```go
func (e *Executor) initFields(entity *plan.EntitySpec, entitySeed rng.Seed) ([]fieldRuntime, error) {
	fields := make([]fieldRuntime, 0, len(entity.Fields))
	knownFields := make(map[string]bool)

	for _, field := range entity.Fields {
		factory, err := registry.FactoryFor(field.Gen)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field.Name, err)
		}

		gen, err := factory(field.Config)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field.Name, err)
		}

		// Validate dependency ordering for row-aware generators
		if dd, ok := gen.(registry.DependencyDeclarer); ok {
			for _, dep := range dd.Dependencies() {
				if !knownFields[dep] {
					return nil, fmt.Errorf("field '%s' references '%s', which must be declared before it", field.Name, dep)
				}
			}
		}

		fieldSeed := rng.Derive(entitySeed, field.Name)
		knownFields[field.Name] = true
		fields = append(fields, fieldRuntime{
			name:    field.Name,
			genName: field.Gen,
			config:  field.Config,
			factory: factory,
			seed:    fieldSeed,
		})
		e.logf("%s -> %s (seed: %d)", field.Name, field.Gen, fieldSeed)
	}

	return fields, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/runtime/ -run TestExecutorDependencyValidation -v`
Expected: All 3 subtests PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`
Expected: All tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/executor.go internal/runtime/executor_test.go
git commit -m "feat: executor validates dependency ordering for row-aware generators"
```

---

### Task 4: Add end-to-end executor test for row-aware dispatch

**Files:**
- Modify: `internal/runtime/executor_test.go`

- [ ] **Step 1: Write test verifying row-aware generators receive correct row context**

```go
func TestExecutorRowAwareDispatch(t *testing.T) {
	p := &plan.Plan{
		Seed: 42,
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

	var buf bytes.Buffer
	w := writer.NewJSONLWriter(&buf)
	e := New(w, WithWorkers(1))
	err := e.Run(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	// Each line should have a "label" field that starts with "got:"
	for i, line := range lines {
		if !strings.Contains(line, `"label":"got:`) {
			t.Errorf("line %d: expected label with row context value, got: %s", i, line)
		}
	}
}

func TestExecutorRowAwareDeterminism(t *testing.T) {
	p := &plan.Plan{
		Seed: 99,
		Entities: []plan.EntitySpec{
			{
				Name:  "Test",
				Count: 100,
				Fields: []plan.FieldSpec{
					{Name: "status", Gen: "bool"},
					{Name: "label", Gen: "__test_row_aware", Config: map[string]any{"deps": []any{"status"}}},
				},
			},
		},
	}

	run := func(workers int, chunkSize int64) string {
		var buf bytes.Buffer
		w := writer.NewJSONLWriter(&buf)
		e := New(w, WithWorkers(workers), WithChunkSize(chunkSize))
		if err := e.Run(context.Background(), p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return buf.String()
	}

	baseline := run(1, 100)
	if run(4, 25) != baseline {
		t.Error("output differs with 4 workers, chunk 25")
	}
	if run(8, 10) != baseline {
		t.Error("output differs with 8 workers, chunk 10")
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/runtime/ -run "TestExecutorRowAware" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/runtime/executor_test.go
git commit -m "test: add executor integration tests for row-aware dispatch and determinism"
```

---

## Chunk 2: Template Generator

### Task 5: Implement template parser and generator

**Files:**
- Create: `internal/registry/template.go`
- Create: `internal/registry/template_test.go`

- [ ] **Step 1: Write failing config validation tests**

Create `internal/registry/template_test.go`:

```go
package registry

import (
	"testing"
)

const templateGen = "template"

func TestTemplateGenerator_Config(t *testing.T) {
	RunConfigTests(t, templateGen, []ConfigTestCase{
		{
			Name:        "valid simple",
			Config:      map[string]any{"tpl": "Hello, {name}!"},
			ExpectError: false,
		},
		{
			Name:        "valid multiple refs",
			Config:      map[string]any{"tpl": "{first} {last}"},
			ExpectError: false,
		},
		{
			Name:        "valid no refs (literal only)",
			Config:      map[string]any{"tpl": "Hello world"},
			ExpectError: false,
		},
		{
			Name:        "valid escaped braces",
			Config:      map[string]any{"tpl": "use {{braces}} here"},
			ExpectError: false,
		},
		{
			Name:        "valid mixed refs and escapes",
			Config:      map[string]any{"tpl": "{name} owns {{company}}"},
			ExpectError: false,
		},
		{
			Name:        "missing tpl",
			Config:      map[string]any{},
			ExpectError: true,
		},
		{
			Name:        "tpl not string",
			Config:      map[string]any{"tpl": 123},
			ExpectError: true,
		},
		{
			Name:        "empty placeholder",
			Config:      map[string]any{"tpl": "hello {}"},
			ExpectError: true,
		},
		{
			Name:        "unclosed brace",
			Config:      map[string]any{"tpl": "hello {name"},
			ExpectError: true,
		},
		{
			Name:        "nested braces",
			Config:      map[string]any{"tpl": "hello {a{b}}"},
			ExpectError: true,
		},
		{
			Name:        "unmatched closing brace",
			Config:      map[string]any{"tpl": "hello }"},
			ExpectError: true,
		},
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/registry/ -run TestTemplateGenerator_Config -v`
Expected: FAIL — generator "template" not found

- [ ] **Step 3: Implement template generator**

Create `internal/registry/template.go`:

```go
package registry

import (
	"apery/internal/rng"
	"fmt"
	"strings"
)

// TemplateGenerator performs string interpolation using field values from the current row.
type TemplateGenerator struct {
	parts []templatePart
	deps  []string
}

type templatePart struct {
	literal bool
	value   string
}

// Next returns an error because template requires row context.
func (tg *TemplateGenerator) Next(_ *rng.Rng) (any, error) {
	return nil, fmt.Errorf("template: requires row context")
}

// NextWithRow performs string interpolation using values from the row context.
func (tg *TemplateGenerator) NextWithRow(_ *rng.Rng, row RowContext) (any, error) {
	var b strings.Builder
	for _, part := range tg.parts {
		if part.literal {
			b.WriteString(part.value)
			continue
		}
		v, ok := row.Get(part.value)
		if !ok {
			return nil, fmt.Errorf("template: field '%s' not found in row context", part.value)
		}
		b.WriteString(fmt.Sprint(v))
	}
	return b.String(), nil
}

// Dependencies returns the field names this template references.
func (tg *TemplateGenerator) Dependencies() []string {
	return tg.deps
}

// parseTemplate parses a template string into literal and reference parts.
// Supports {field_name} for references and {{ / }} for escaped braces.
func parseTemplate(tpl string) ([]templatePart, []string, error) {
	var parts []templatePart
	var deps []string
	seen := make(map[string]bool)

	i := 0
	var literal strings.Builder

	for i < len(tpl) {
		ch := tpl[i]

		if ch == '{' {
			if i+1 < len(tpl) && tpl[i+1] == '{' {
				// Escaped {{
				literal.WriteByte('{')
				i += 2
				continue
			}

			// Flush literal
			if literal.Len() > 0 {
				parts = append(parts, templatePart{literal: true, value: literal.String()})
				literal.Reset()
			}

			// Find closing brace
			end := strings.IndexByte(tpl[i+1:], '}')
			if end == -1 {
				return nil, nil, fmt.Errorf("template: unclosed '{' at position %d", i)
			}

			fieldName := tpl[i+1 : i+1+end]

			// Check for nested braces
			if strings.ContainsAny(fieldName, "{}") {
				return nil, nil, fmt.Errorf("template: nested braces not allowed at position %d", i)
			}

			if fieldName == "" {
				return nil, nil, fmt.Errorf("template: empty placeholder '{}' at position %d", i)
			}

			parts = append(parts, templatePart{literal: false, value: fieldName})
			if !seen[fieldName] {
				deps = append(deps, fieldName)
				seen[fieldName] = true
			}

			i += 1 + end + 1 // skip { + fieldName + }
			continue
		}

		if ch == '}' {
			if i+1 < len(tpl) && tpl[i+1] == '}' {
				// Escaped }}
				literal.WriteByte('}')
				i += 2
				continue
			}
			return nil, nil, fmt.Errorf("template: unmatched '}' at position %d", i)
		}

		literal.WriteByte(ch)
		i++
	}

	// Flush remaining literal
	if literal.Len() > 0 {
		parts = append(parts, templatePart{literal: true, value: literal.String()})
	}

	return parts, deps, nil
}

func init() {
	MustRegister("template", func(config map[string]any) (Generator, error) {
		tpl, ok := config["tpl"].(string)
		if !ok {
			return nil, fmt.Errorf("template: 'tpl' must be a string")
		}

		parts, deps, err := parseTemplate(tpl)
		if err != nil {
			return nil, err
		}

		return &TemplateGenerator{parts: parts, deps: deps}, nil
	})
}
```

- [ ] **Step 4: Run config tests**

Run: `go test ./internal/registry/ -run TestTemplateGenerator_Config -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/registry/template.go internal/registry/template_test.go
git commit -m "feat: add template generator with config validation"
```

---

### Task 6: Add template output and determinism tests

**Files:**
- Modify: `internal/registry/template_test.go`

- [ ] **Step 1: Add output correctness and edge case tests**

Append to `template_test.go`:

```go
func TestTemplateGenerator_Output(t *testing.T) {
	// template requires row context, so we test via NextWithRow directly
	gen, err := Get(templateGen, map[string]any{"tpl": "Hello, {name}! Age: {age}"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ra, ok := gen.(RowAwareGenerator)
	if !ok {
		t.Fatal("expected RowAwareGenerator")
	}

	row := &testRowContext{data: map[string]any{"name": "Alice", "age": int64(30)}}
	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := ra.NextWithRow(r, row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Hello, Alice! Age: 30"
	if val != expected {
		t.Errorf("expected %q, got %q", expected, val)
	}
}

func TestTemplateGenerator_EscapedBraces(t *testing.T) {
	gen, err := Get(templateGen, map[string]any{"tpl": "use {{braces}} and {name}"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ra := gen.(RowAwareGenerator)
	row := &testRowContext{data: map[string]any{"name": "test"}}
	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := ra.NextWithRow(r, row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "use {braces} and test"
	if val != expected {
		t.Errorf("expected %q, got %q", expected, val)
	}
}

func TestTemplateGenerator_MissingField(t *testing.T) {
	gen, err := Get(templateGen, map[string]any{"tpl": "Hello, {missing}!"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ra := gen.(RowAwareGenerator)
	row := &testRowContext{data: map[string]any{"name": "Alice"}}
	r := rng.New(rng.SeedFromInt64(testSeed))
	_, err = ra.NextWithRow(r, row)
	if err == nil {
		t.Fatal("expected error for missing field")
	}
}

func TestTemplateGenerator_NextWithoutRowContext(t *testing.T) {
	gen, err := Get(templateGen, map[string]any{"tpl": "Hello, {name}!"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	_, err = gen.Next(r)
	if err == nil {
		t.Fatal("expected error when calling Next without row context")
	}
}

func TestTemplateGenerator_Dependencies(t *testing.T) {
	gen, err := Get(templateGen, map[string]any{"tpl": "{a} and {b} and {a}"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dd, ok := gen.(DependencyDeclarer)
	if !ok {
		t.Fatal("expected DependencyDeclarer")
	}

	deps := dd.Dependencies()
	if len(deps) != 2 || deps[0] != "a" || deps[1] != "b" {
		t.Errorf("expected [a b], got %v", deps)
	}
}

func TestTemplateGenerator_Determinism(t *testing.T) {
	config := map[string]any{"tpl": "{x} = {y}"}
	gen1, _ := Get(templateGen, config)
	gen2, _ := Get(templateGen, config)

	ra1 := gen1.(RowAwareGenerator)
	ra2 := gen2.(RowAwareGenerator)

	for i := range testIterations {
		row := &testRowContext{data: map[string]any{"x": int64(i), "y": int64(i * 2)}}
		seed := rng.SeedFromInt64(int64(i))

		v1, _ := ra1.NextWithRow(rng.New(seed), row)
		v2, _ := ra2.NextWithRow(rng.New(seed), row)

		if v1 != v2 {
			t.Errorf("determinism failed at %d: %v != %v", i, v1, v2)
		}
	}
}
```

Also add a `testRowContext` helper near the top of the test file (this will be reused by switch tests):

```go
// testRowContext is a stub RowContext for unit testing row-aware generators.
type testRowContext struct {
	data map[string]any
}

func (r *testRowContext) Get(fieldName string) (any, bool) {
	v, ok := r.data[fieldName]
	return v, ok
}
```

- [ ] **Step 2: Run all template tests**

Run: `go test ./internal/registry/ -run TestTemplateGenerator -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/registry/template_test.go
git commit -m "test: add template output, determinism, and edge case tests"
```

---

## Chunk 3: Switch Generator

### Task 7: Implement switch generator

**Files:**
- Create: `internal/registry/switch.go`
- Create: `internal/registry/switch_test.go`

- [ ] **Step 1: Write failing config validation tests**

Create `internal/registry/switch_test.go`:

```go
package registry

import (
	"apery/internal/rng"
	"fmt"
	"testing"
)

const switchGen = "switch"

func TestSwitchGenerator_Config(t *testing.T) {
	RunConfigTests(t, switchGen, []ConfigTestCase{
		{
			Name: "valid with default",
			Config: map[string]any{
				"key": "status",
				"cases": map[string]any{
					"active":   map[string]any{"gen": "const", "config": map[string]any{"value": "yes"}},
					"inactive": map[string]any{"gen": "const", "config": map[string]any{"value": "no"}},
				},
				"default": map[string]any{"gen": "const", "config": map[string]any{"value": "unknown"}},
			},
			ExpectError: false,
		},
		{
			Name: "valid without default",
			Config: map[string]any{
				"key": "status",
				"cases": map[string]any{
					"active": map[string]any{"gen": "const", "config": map[string]any{"value": "yes"}},
				},
			},
			ExpectError: false,
		},
		{
			Name:        "missing key",
			Config:      map[string]any{"cases": map[string]any{"a": map[string]any{"gen": "bool"}}},
			ExpectError: true,
		},
		{
			Name:        "key not string",
			Config:      map[string]any{"key": 123, "cases": map[string]any{"a": map[string]any{"gen": "bool"}}},
			ExpectError: true,
		},
		{
			Name:        "missing cases",
			Config:      map[string]any{"key": "status"},
			ExpectError: true,
		},
		{
			Name:        "empty cases",
			Config:      map[string]any{"key": "status", "cases": map[string]any{}},
			ExpectError: true,
		},
		{
			Name: "cases not map",
			Config: map[string]any{"key": "status", "cases": "bad"},
			ExpectError: true,
		},
		{
			Name: "case value not map",
			Config: map[string]any{
				"key":   "status",
				"cases": map[string]any{"active": "bad"},
			},
			ExpectError: true,
		},
		{
			Name: "case missing gen",
			Config: map[string]any{
				"key":   "status",
				"cases": map[string]any{"active": map[string]any{"config": map[string]any{}}},
			},
			ExpectError: true,
		},
		{
			Name: "unknown case generator",
			Config: map[string]any{
				"key":   "status",
				"cases": map[string]any{"active": map[string]any{"gen": "nonexistent"}},
			},
			ExpectError: true,
		},
		{
			Name: "invalid case config propagates",
			Config: map[string]any{
				"key":   "status",
				"cases": map[string]any{"active": map[string]any{"gen": "int", "config": map[string]any{"min": "bad"}}},
			},
			ExpectError: true,
		},
		{
			Name: "default not map",
			Config: map[string]any{
				"key":     "status",
				"cases":   map[string]any{"active": map[string]any{"gen": "bool"}},
				"default": "bad",
			},
			ExpectError: true,
		},
		{
			Name: "default missing gen",
			Config: map[string]any{
				"key":     "status",
				"cases":   map[string]any{"active": map[string]any{"gen": "bool"}},
				"default": map[string]any{"config": map[string]any{}},
			},
			ExpectError: true,
		},
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/registry/ -run TestSwitchGenerator_Config -v`
Expected: FAIL — generator "switch" not found

- [ ] **Step 3: Implement switch generator**

Create `internal/registry/switch.go`:

```go
package registry

import (
	"apery/internal/rng"
	"fmt"
)

// SwitchGenerator dispatches to sub-generators based on a key field's value from the current row.
type SwitchGenerator struct {
	key      string
	cases    map[string]Generator
	fallback Generator // optional
	deps     []string
}

// Next returns an error because switch requires row context.
func (s *SwitchGenerator) Next(_ *rng.Rng) (any, error) {
	return nil, fmt.Errorf("switch: requires row context")
}

// NextWithRow reads the key field, selects the matching case generator, and returns its output.
func (s *SwitchGenerator) NextWithRow(r *rng.Rng, row RowContext) (any, error) {
	keyVal, ok := row.Get(s.key)
	if !ok {
		return nil, fmt.Errorf("switch: key field '%s' not found in row context", s.key)
	}

	keyStr := fmt.Sprint(keyVal)
	gen, matched := s.cases[keyStr]
	if !matched {
		if s.fallback != nil {
			gen = s.fallback
		} else {
			return nil, fmt.Errorf("switch: no case for key value %q and no default", keyStr)
		}
	}

	valueRng := rng.New(rng.Derive(r.GetSeed(), "__value__"))
	if ra, ok := gen.(RowAwareGenerator); ok {
		return ra.NextWithRow(valueRng, row)
	}
	return gen.Next(valueRng)
}

// Dependencies returns the key field plus any transitive dependencies from row-aware sub-generators.
func (s *SwitchGenerator) Dependencies() []string {
	return s.deps
}

func collectSwitchDeps(key string, gens []Generator) []string {
	seen := map[string]bool{key: true}
	deps := []string{key}

	for _, gen := range gens {
		if dd, ok := gen.(DependencyDeclarer); ok {
			for _, dep := range dd.Dependencies() {
				if !seen[dep] {
					deps = append(deps, dep)
					seen[dep] = true
				}
			}
		}
	}
	return deps
}

func instantiateSubGenerator(spec map[string]any, context string) (Generator, error) {
	genName, ok := spec["gen"].(string)
	if !ok || genName == "" {
		return nil, fmt.Errorf("switch: %s missing 'gen'", context)
	}

	genConfig, _ := spec["config"].(map[string]any)
	if genConfig == nil {
		genConfig = map[string]any{}
	}

	gen, err := Get(genName, genConfig)
	if err != nil {
		return nil, fmt.Errorf("switch: %s: %w", context, err)
	}
	return gen, nil
}

func init() {
	MustRegister("switch", func(config map[string]any) (Generator, error) {
		key, ok := config["key"].(string)
		if !ok || key == "" {
			return nil, fmt.Errorf("switch: 'key' must be a non-empty string")
		}

		rawCases, ok := config["cases"]
		if !ok {
			return nil, fmt.Errorf("switch: 'cases' is required")
		}
		casesMap, ok := rawCases.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("switch: 'cases' must be a map, got %T", rawCases)
		}
		if len(casesMap) == 0 {
			return nil, fmt.Errorf("switch: 'cases' cannot be empty")
		}

		cases := make(map[string]Generator, len(casesMap))
		allGens := make([]Generator, 0, len(casesMap)+1)
		for caseName, rawSpec := range casesMap {
			spec, ok := rawSpec.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("switch: case '%s' must be a map, got %T", caseName, rawSpec)
			}
			gen, err := instantiateSubGenerator(spec, fmt.Sprintf("case '%s'", caseName))
			if err != nil {
				return nil, err
			}
			cases[caseName] = gen
			allGens = append(allGens, gen)
		}

		var fallback Generator
		if rawDefault, ok := config["default"]; ok {
			spec, ok := rawDefault.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("switch: 'default' must be a map, got %T", rawDefault)
			}
			gen, err := instantiateSubGenerator(spec, "default")
			if err != nil {
				return nil, err
			}
			fallback = gen
			allGens = append(allGens, gen)
		}

		deps := collectSwitchDeps(key, allGens)

		return &SwitchGenerator{
			key:      key,
			cases:    cases,
			fallback: fallback,
			deps:     deps,
		}, nil
	})
}
```

- [ ] **Step 4: Run config tests**

Run: `go test ./internal/registry/ -run TestSwitchGenerator_Config -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/registry/switch.go internal/registry/switch_test.go
git commit -m "feat: add switch generator with config validation"
```

---

### Task 8: Add switch output, determinism, and edge case tests

**Files:**
- Modify: `internal/registry/switch_test.go`

- [ ] **Step 1: Add output, determinism, and edge case tests**

Append to `switch_test.go`:

```go
func TestSwitchGenerator_Output(t *testing.T) {
	gen, err := Get(switchGen, map[string]any{
		"key": "status",
		"cases": map[string]any{
			"active":   map[string]any{"gen": "const", "config": map[string]any{"value": "welcome"}},
			"inactive": map[string]any{"gen": "const", "config": map[string]any{"value": "goodbye"}},
		},
		"default": map[string]any{"gen": "const", "config": map[string]any{"value": "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ra := gen.(RowAwareGenerator)
	r := rng.New(rng.SeedFromInt64(testSeed))

	t.Run("matches active", func(t *testing.T) {
		row := &testRowContext{data: map[string]any{"status": "active"}}
		val, err := ra.NextWithRow(r, row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "welcome" {
			t.Errorf("expected 'welcome', got %v", val)
		}
	})

	t.Run("matches inactive", func(t *testing.T) {
		row := &testRowContext{data: map[string]any{"status": "inactive"}}
		val, err := ra.NextWithRow(r, row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "goodbye" {
			t.Errorf("expected 'goodbye', got %v", val)
		}
	})

	t.Run("falls through to default", func(t *testing.T) {
		row := &testRowContext{data: map[string]any{"status": "unknown"}}
		val, err := ra.NextWithRow(r, row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "hello" {
			t.Errorf("expected 'hello', got %v", val)
		}
	})
}

func TestSwitchGenerator_NoMatchNoDefault(t *testing.T) {
	gen, err := Get(switchGen, map[string]any{
		"key": "status",
		"cases": map[string]any{
			"active": map[string]any{"gen": "const", "config": map[string]any{"value": "yes"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ra := gen.(RowAwareGenerator)
	row := &testRowContext{data: map[string]any{"status": "unknown"}}
	r := rng.New(rng.SeedFromInt64(testSeed))
	_, err = ra.NextWithRow(r, row)
	if err == nil {
		t.Fatal("expected error for no matching case and no default")
	}
}

func TestSwitchGenerator_MissingKeyField(t *testing.T) {
	gen, err := Get(switchGen, map[string]any{
		"key": "status",
		"cases": map[string]any{
			"active": map[string]any{"gen": "const", "config": map[string]any{"value": "yes"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ra := gen.(RowAwareGenerator)
	row := &testRowContext{data: map[string]any{"other": "value"}}
	r := rng.New(rng.SeedFromInt64(testSeed))
	_, err = ra.NextWithRow(r, row)
	if err == nil {
		t.Fatal("expected error for missing key field")
	}
}

func TestSwitchGenerator_IntKeyCoercion(t *testing.T) {
	gen, err := Get(switchGen, map[string]any{
		"key": "code",
		"cases": map[string]any{
			"1": map[string]any{"gen": "const", "config": map[string]any{"value": "one"}},
			"2": map[string]any{"gen": "const", "config": map[string]any{"value": "two"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ra := gen.(RowAwareGenerator)
	row := &testRowContext{data: map[string]any{"code": int64(1)}}
	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := ra.NextWithRow(r, row)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "one" {
		t.Errorf("expected 'one', got %v", val)
	}
}

func TestSwitchGenerator_NextWithoutRowContext(t *testing.T) {
	gen, err := Get(switchGen, map[string]any{
		"key":   "status",
		"cases": map[string]any{"a": map[string]any{"gen": "bool"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	_, err = gen.Next(r)
	if err == nil {
		t.Fatal("expected error when calling Next without row context")
	}
}

func TestSwitchGenerator_Dependencies(t *testing.T) {
	gen, err := Get(switchGen, map[string]any{
		"key": "status",
		"cases": map[string]any{
			"active": map[string]any{"gen": "bool"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dd := gen.(DependencyDeclarer)
	deps := dd.Dependencies()
	if len(deps) != 1 || deps[0] != "status" {
		t.Errorf("expected [status], got %v", deps)
	}
}

func TestSwitchGenerator_TransitiveDependencies(t *testing.T) {
	gen, err := Get(switchGen, map[string]any{
		"key": "status",
		"cases": map[string]any{
			"active": map[string]any{"gen": "template", "config": map[string]any{"tpl": "Hi {name}"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dd := gen.(DependencyDeclarer)
	deps := dd.Dependencies()
	// Should include both "status" (key) and "name" (from nested template)
	depsSet := make(map[string]bool)
	for _, d := range deps {
		depsSet[d] = true
	}
	if !depsSet["status"] || !depsSet["name"] {
		t.Errorf("expected deps to include 'status' and 'name', got %v", deps)
	}
}

func TestSwitchGenerator_Determinism(t *testing.T) {
	config := map[string]any{
		"key": "status",
		"cases": map[string]any{
			"true":  map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
			"false": map[string]any{"gen": "int", "config": map[string]any{"min": 200, "max": 300}},
		},
	}

	gen1, _ := Get(switchGen, config)
	gen2, _ := Get(switchGen, config)

	ra1 := gen1.(RowAwareGenerator)
	ra2 := gen2.(RowAwareGenerator)

	for i := range testIterations {
		status := i%2 == 0
		row := &testRowContext{data: map[string]any{"status": status}}
		seed := rng.SeedFromInt64(int64(i))

		v1, _ := ra1.NextWithRow(rng.New(seed), row)
		v2, _ := ra2.NextWithRow(rng.New(seed), row)

		s1 := fmt.Sprintf("%v", v1)
		s2 := fmt.Sprintf("%v", v2)
		if s1 != s2 {
			t.Errorf("determinism failed at %d: %s != %s", i, s1, s2)
		}
	}
}

func TestSwitchGenerator_RandomCaseOutput(t *testing.T) {
	config := map[string]any{
		"key": "status",
		"cases": map[string]any{
			"true":  map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
			"false": map[string]any{"gen": "int", "config": map[string]any{"min": 200, "max": 300}},
		},
	}

	gen, err := Get(switchGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ra := gen.(RowAwareGenerator)

	for i := range distributionSamples {
		status := i%2 == 0
		row := &testRowContext{data: map[string]any{"status": status}}
		seed := rng.SeedFromInt64(int64(i))
		val, err := ra.NextWithRow(rng.New(seed), row)
		if err != nil {
			t.Fatalf("error at %d: %v", i, err)
		}

		v := val.(int64)
		if status {
			if v < 1 || v > 100 {
				t.Errorf("true case: %d out of range [1, 100] at %d", v, i)
			}
		} else {
			if v < 200 || v > 300 {
				t.Errorf("false case: %d out of range [200, 300] at %d", v, i)
			}
		}
	}
}
```

- [ ] **Step 2: Run all switch tests**

Run: `go test ./internal/registry/ -run TestSwitchGenerator -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/registry/switch_test.go
git commit -m "test: add switch output, determinism, dependencies, and edge case tests"
```

---

## Chunk 4: Integration, Docs, and Spot-Check

### Task 9: Update main.go with template and switch examples

**Files:**
- Modify: `cmd/apery/main.go`

- [ ] **Step 1: Add `template` and `switch` fields to the example plan**

Add these fields after the existing `contact_method` field in the plan. Since `template` depends on `department` (declared earlier) and `switch` depends on `is_active` (declared earlier), ordering is valid:

```go
{Name: "greeting", Gen: "template", Config: map[string]any{
	"tpl": "Welcome, employee #{id} from {department}!",
}},
{Name: "access_level", Gen: "switch", Config: map[string]any{
	"key": "department",
	"cases": map[string]any{
		"engineering": map[string]any{"gen": "const", "config": map[string]any{"value": "full"}},
		"sales":       map[string]any{"gen": "const", "config": map[string]any{"value": "read-only"}},
		"marketing":   map[string]any{"gen": "const", "config": map[string]any{"value": "read-only"}},
		"support":     map[string]any{"gen": "const", "config": map[string]any{"value": "limited"}},
	},
	"default": map[string]any{"gen": "const", "config": map[string]any{"value": "standard"}},
}},
```

- [ ] **Step 2: Build and run, spot-check output**

Run: `go build ./cmd/apery && go run ./cmd/apery && head -3 output.jsonl`
Expected: Each row has `greeting` with interpolated values and `access_level` matching the department.

- [ ] **Step 3: Commit**

```bash
git add cmd/apery/main.go
git commit -m "feat: add template and switch examples to main.go for spot-checking"
```

---

### Task 10: Update docs

**Files:**
- Modify: `docs/plan.md`
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update plan.md**

Check off `template` and `switch` in the composite generators section:

```
- [x] template(tpl) — string interpolation with row field references
- [x] switch(key,cases)
```

- [ ] **Step 2: Update CLAUDE.md**

In the built-in generators list, add `template`, `switch` to the end.

- [ ] **Step 3: Update README.md**

In the composite generators section, add:

```
- `template` - String interpolation with `{field_name}` placeholders from the current row
- `switch` - Dispatch to sub-generator based on another field's value, with optional default
```

- [ ] **Step 4: Run full test suite**

Run: `go test ./...`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add docs/plan.md CLAUDE.md README.md
git commit -m "docs: add template and switch to generator lists and check off plan items"
```
