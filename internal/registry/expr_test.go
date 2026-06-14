package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"testing"
)

const exprGen = "expr"

func TestExprGenerator_Config(t *testing.T) {
	RunConfigTests(t, exprGen, []ConfigTestCase{
		{Name: "field over literal", Config: map[string]any{"expr": "{total} / 12"}, ExpectError: false},
		{Name: "two fields", Config: map[string]any{"expr": "{amount} * {fx_rate}"}, ExpectError: false},
		{Name: "parentheses", Config: map[string]any{"expr": "({a} + {b}) * 2"}, ExpectError: false},
		{Name: "unary minus", Config: map[string]any{"expr": "-{x}"}, ExpectError: false},
		{Name: "literal only", Config: map[string]any{"expr": "1 + 2 * 3"}, ExpectError: false},
		{Name: "decimal literal", Config: map[string]any{"expr": "{x} * 0.5"}, ExpectError: false},
		{Name: "field named like a keyword", Config: map[string]any{"expr": "{type} * 2"}, ExpectError: false},
		{Name: "missing expr", Config: map[string]any{}, ExpectError: true},
		{Name: "bare identifier (no braces)", Config: map[string]any{"expr": "total / 12"}, ExpectError: true},
		{Name: "expr not string", Config: map[string]any{"expr": 42}, ExpectError: true},
		{Name: "empty expr", Config: map[string]any{"expr": "   "}, ExpectError: true},
		{Name: "unclosed brace", Config: map[string]any{"expr": "{x"}, ExpectError: true},
		{Name: "empty field ref", Config: map[string]any{"expr": "{}"}, ExpectError: true},
		{Name: "trailing operator", Config: map[string]any{"expr": "{x} +"}, ExpectError: true},
		{Name: "unbalanced paren", Config: map[string]any{"expr": "({x} + 1"}, ExpectError: true},
		{Name: "double operator", Config: map[string]any{"expr": "{x} * * {y}"}, ExpectError: true},
		{Name: "bad character", Config: map[string]any{"expr": "{x} % 2"}, ExpectError: true},
		{Name: "bad number", Config: map[string]any{"expr": "1.2.3"}, ExpectError: true},
	})
}

func evalExpr(t *testing.T, expr string, row map[string]any) any {
	t.Helper()
	gen, err := Get(exprGen, map[string]any{"expr": expr})
	if err != nil {
		t.Fatalf("compile %q: %v", expr, err)
	}
	ra, ok := gen.(RowAwareGenerator)
	if !ok {
		t.Fatal("expr must be a RowAwareGenerator")
	}
	val, err := ra.NextWithRow(rng.New(rng.SeedFromInt64(testSeed)), &testRowContext{data: row})
	if err != nil {
		t.Fatalf("eval %q: %v", expr, err)
	}
	return val
}

func TestExprGenerator_Output(t *testing.T) {
	tests := []struct {
		name string
		expr string
		row  map[string]any
		want any
	}{
		{"whole division stays int", "{total} / 12", map[string]any{"total": int64(12000)}, int64(1000)},
		{"fractional division is float", "{total} / 12", map[string]any{"total": int64(10000)}, 10000.0 / 12.0},
		{"non-integer division stays float", "{x} / 4", map[string]any{"x": int64(10)}, 2.5},
		{"multiply ints", "{a} * {b}", map[string]any{"a": int64(6), "b": int64(7)}, int64(42)},
		{"float multiply stays float", "{x} * 1.5", map[string]any{"x": int64(3)}, 4.5},
		{"precedence", "1 + 2 * 3", map[string]any{}, int64(7)},
		{"parentheses override", "(1 + 2) * 3", map[string]any{}, int64(9)},
		{"unary minus", "-{x} + 5", map[string]any{"x": int64(2)}, int64(3)},
		{"subtraction", "{a} - {b}", map[string]any{"a": int64(3), "b": int64(10)}, int64(-7)},
		{"float field input", "{p} * 2", map[string]any{"p": float64(2.5)}, int64(5)},
		{"keyword field name", "{type} * 2", map[string]any{"type": int64(3)}, int64(6)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evalExpr(t, tt.expr, tt.row)
			if got != tt.want {
				t.Errorf("expr %q = %#v (%T), want %#v (%T)", tt.expr, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestExprGenerator_RuntimeErrors(t *testing.T) {
	tests := []struct {
		name string
		expr string
		row  map[string]any
	}{
		{"missing field", "{nope} + 1", map[string]any{}},
		{"non-numeric field", "{name} * 2", map[string]any{"name": "Alice"}},
		{"division by zero", "{x} / 0", map[string]any{"x": int64(5)}},
		{"division by zero field", "{x} / {y}", map[string]any{"x": int64(5), "y": int64(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := Get(exprGen, map[string]any{"expr": tt.expr})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			ra := gen.(RowAwareGenerator)
			_, err = ra.NextWithRow(rng.New(rng.SeedFromInt64(testSeed)), &testRowContext{data: tt.row})
			if err == nil {
				t.Errorf("expr %q: expected runtime error, got none", tt.expr)
			}
		})
	}
}

func TestExprGenerator_Dependencies(t *testing.T) {
	gen, err := Get(exprGen, map[string]any{"expr": "{a} + {b} * {a} - {c}"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	dd, ok := gen.(DependencyDeclarer)
	if !ok {
		t.Fatal("expected DependencyDeclarer")
	}
	deps := dd.Dependencies()
	want := []string{"a", "b", "c"}
	if len(deps) != len(want) {
		t.Fatalf("deps = %v, want %v", deps, want)
	}
	for i := range want {
		if deps[i] != want[i] {
			t.Errorf("deps = %v, want %v", deps, want)
		}
	}
}

func TestExprGenerator_NextWithoutRowContext(t *testing.T) {
	gen, err := Get(exprGen, map[string]any{"expr": "{x} + 1"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := gen.Next(rng.New(rng.SeedFromInt64(testSeed))); err == nil {
		t.Fatal("expected error when calling Next without row context")
	}
}

func TestExprGenerator_Determinism(t *testing.T) {
	config := map[string]any{"expr": "({x} + {y}) / 2"}
	gen1, _ := Get(exprGen, config)
	gen2, _ := Get(exprGen, config)
	ra1 := gen1.(RowAwareGenerator)
	ra2 := gen2.(RowAwareGenerator)

	for i := range testIterations {
		row := &testRowContext{data: map[string]any{"x": int64(i), "y": int64(i * 3)}}
		v1, _ := ra1.NextWithRow(rng.New(rng.SeedFromInt64(int64(i))), row)
		v2, _ := ra2.NextWithRow(rng.New(rng.SeedFromInt64(int64(i))), row)
		if v1 != v2 {
			t.Errorf("determinism failed at %d: %v != %v", i, v1, v2)
		}
	}
}
