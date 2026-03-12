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
			Name:        "cases not map",
			Config:      map[string]any{"key": "status", "cases": "bad"},
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
