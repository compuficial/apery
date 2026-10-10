package registry

import (
	"slices"
	"strings"
	"testing"

	"github.com/compuficial/apery/internal/rng"
)

// nextWithRow builds a row-aware generator and runs it against one row.
func nextWithRow(t *testing.T, gen string, config, row map[string]any) any {
	t.Helper()
	g, err := Get(gen, config)
	if err != nil {
		t.Fatalf("%s: %v", gen, err)
	}
	val, err := g.(RowAwareGenerator).NextWithRow(rng.New(rng.SeedFromInt64(testSeed)), &testRowContext{data: row})
	if err != nil {
		t.Fatalf("%s: %v", gen, err)
	}
	return val
}

// A null input (from null_rate) gives a null from expr and date_offset, as in
// SQL, and empty text from template.
func TestRowAwareNullInputs(t *testing.T) {
	cases := []struct {
		name   string
		gen    string
		config map[string]any
		row    map[string]any
		want   any
	}{
		{"expr null operand", "expr", map[string]any{"expr": "{a} + {b}"}, map[string]any{"a": int64(1), "b": nil}, nil},
		{"expr null before division by zero", "expr", map[string]any{"expr": "({a} * 2) / {b}"}, map[string]any{"a": nil, "b": int64(0)}, nil},
		{"expr without nulls", "expr", map[string]any{"expr": "{a} + {b}"}, map[string]any{"a": int64(1), "b": int64(2)}, int64(3)},
		{"date_offset null base", "date_offset", map[string]any{"base": "{d}", "amount": 1, "unit": "days", "format": "2006-01-02"}, map[string]any{"d": nil}, nil},
		{"date_offset null amount", "date_offset", map[string]any{"base": "2024-01-01", "amount": "{n}", "unit": "days", "format": "2006-01-02"}, map[string]any{"n": nil}, nil},
		{"template null", "template", map[string]any{"tpl": "a{x}b"}, map[string]any{"x": nil}, "ab"},
	}
	for _, c := range cases {
		if got := nextWithRow(t, c.gen, c.config, c.row); got != c.want {
			t.Errorf("%s: got %v (%T), want %v", c.name, got, got, c.want)
		}
	}
}

// A null switch key selects the "null" case, else the default.
func TestSwitchGenerator_NullKey(t *testing.T) {
	constant := func(v string) map[string]any {
		return map[string]any{"gen": "const", "config": map[string]any{"value": v}}
	}
	withNullCase := map[string]any{"key": "k", "cases": map[string]any{"null": constant("none"), "a": constant("A")}, "default": constant("other")}
	withoutNullCase := map[string]any{"key": "k", "cases": map[string]any{"a": constant("A")}, "default": constant("other")}

	if got := nextWithRow(t, switchGen, withNullCase, map[string]any{"k": nil}); got != "none" {
		t.Errorf("null key with a null case = %v, want none", got)
	}
	if got := nextWithRow(t, switchGen, withoutNullCase, map[string]any{"k": nil}); got != "other" {
		t.Errorf("null key without a null case = %v, want other", got)
	}
}

// rel_ref never draws a null: nulls in the target column are not references.
func TestRelRef_SkipsNulls(t *testing.T) {
	ids := []any{nil, int64(1), nil, int64(2), int64(3), nil}
	for _, unique := range []bool{false, true} {
		gen := newBoundRelRef(t, map[string]any{"entity": "User", "field": "id", "unique": unique}, ids)
		draws := drawRows(t, gen, 3)
		if slices.Contains(draws, nil) {
			t.Errorf("unique=%v drew a null: %v", unique, draws)
		}
	}

	gen, err := Get("rel_ref", map[string]any{"entity": "User", "field": "id"})
	if err != nil {
		t.Fatal(err)
	}
	err = gen.(StoreConsumer).BindStore(&testStore{data: map[string][]any{"User.id": {nil, nil}}})
	if err == nil || !strings.Contains(err.Error(), "only nulls") {
		t.Errorf("all-null column: error = %v, want 'only nulls'", err)
	}
}
