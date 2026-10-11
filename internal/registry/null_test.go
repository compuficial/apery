package registry

import (
	"fmt"
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
		{"expr null function argument", "expr", map[string]any{"expr": "round({a} / 12)"}, map[string]any{"a": nil}, nil},
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

// intChild is a composite child spec for an int, with null_rate when rate
// is non-nil.
func intChild(rate any) map[string]any {
	spec := map[string]any{"gen": "int", "config": map[string]any{"min": 0, "max": 1000}}
	if rate != nil {
		spec["null_rate"] = rate
	}
	return spec
}

// leafValues flattens a composite value into the values to compare.
func leafValues(v any) []any {
	switch v := v.(type) {
	case map[string]any:
		return []any{v["a"], v["b"]}
	case []any:
		return v
	default:
		return []any{v}
	}
}

// A composite child takes null_rate too. Only that child's values become
// null, at about the rate, and the rest match the spec without it.
func TestCompositeChildNullRate(t *testing.T) {
	configs := func(rate any) map[string]map[string]any {
		return map[string]map[string]any{
			"object": {"fields": map[string]any{"a": intChild(rate), "b": intChild(nil)}},
			"list":   {"item": intChild(rate), "min_len": 3, "max_len": 3},
			"one_of": {"generators": []any{intChild(rate)}},
		}
	}
	plain, nullable := configs(nil), configs(0.3)

	for gen := range plain {
		g0, err := Get(gen, plain[gen])
		if err != nil {
			t.Fatal(err)
		}
		g1, err := Get(gen, nullable[gen])
		if err != nil {
			t.Fatal(err)
		}

		nulls, total := 0, 0
		for i := range int64(5000) {
			seed := rng.DeriveIndex(rng.SeedFromInt64(testSeed), i)
			v0, err0 := g0.Next(rng.New(seed))
			v1, err1 := g1.Next(rng.New(seed))
			if err0 != nil || err1 != nil {
				t.Fatalf("%s row %d: %v, %v", gen, i, err0, err1)
			}
			want, got := leafValues(v0), leafValues(v1)
			for j := range got {
				nullable := gen != "object" || j == 0 // object's b has no null_rate
				switch {
				case got[j] == nil && nullable:
					nulls++
				case got[j] != want[j]:
					t.Fatalf("%s row %d: %v, want %v (or null)", gen, i, got, want)
				}
				if nullable {
					total++
				}
			}
		}
		if rate := float64(nulls) / float64(total); rate < 0.28 || rate > 0.32 {
			t.Errorf("%s: null rate %.3f, want about 0.3", gen, rate)
		}
	}
}

// A nullable switch case stays row-aware and keeps its dependencies.
func TestSwitchCaseNullRate(t *testing.T) {
	config := map[string]any{"key": "k", "cases": map[string]any{
		"a": map[string]any{"gen": "template", "config": map[string]any{"tpl": "x={x}"}, "null_rate": 0.5},
	}}
	g, err := Get(switchGen, config)
	if err != nil {
		t.Fatal(err)
	}
	if deps := g.(DependencyDeclarer).Dependencies(); !slices.Contains(deps, "x") {
		t.Errorf("dependencies = %v, want x", deps)
	}

	nulls := 0
	for i := range int64(1000) {
		row := &testRowContext{data: map[string]any{"k": "a", "x": i}}
		v, err := g.(RowAwareGenerator).NextWithRow(rng.New(rng.DeriveIndex(rng.SeedFromInt64(testSeed), i)), row)
		switch {
		case err != nil:
			t.Fatal(err)
		case v == nil:
			nulls++
		case v != fmt.Sprintf("x=%d", i):
			t.Fatalf("row %d = %v", i, v)
		}
	}
	if nulls < 450 || nulls > 550 {
		t.Errorf("%d nulls in 1000, want about 500", nulls)
	}
}

func TestCompositeChildNullRate_Invalid(t *testing.T) {
	cases := map[string]map[string]any{
		"must be between 0 and 1":  {"fields": map[string]any{"a": intChild(1.5)}},
		"'null_rate' must be":      {"fields": map[string]any{"a": intChild("often")}},
		`did you mean "null_rate"`: {"fields": map[string]any{"a": map[string]any{"gen": "int", "nul_rate": 0.5}}},
	}
	for want, config := range cases {
		if _, err := Get("object", config); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want %q", err, want)
		}
	}
}
