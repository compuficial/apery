package registry

import (
	"apery/internal/rng"
	"testing"
)

const templateGen = "template"

// testRowContext is a stub RowContext for unit testing row-aware generators.
type testRowContext struct {
	data map[string]any
}

func (r *testRowContext) Get(fieldName string) (any, bool) {
	v, ok := r.data[fieldName]
	return v, ok
}

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

func TestTemplateGenerator_Output(t *testing.T) {
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
