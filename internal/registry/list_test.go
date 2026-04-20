package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"fmt"
	"testing"
)

const listGen = "list"

func TestListGenerator_Config(t *testing.T) {
	RunConfigTests(t, listGen, []ConfigTestCase{
		{
			Name: "valid basic",
			Config: map[string]any{
				"len":  3,
				"item": map[string]any{"gen": "bool"},
			},
			ExpectError: false,
		},
		{
			Name: "valid with item config",
			Config: map[string]any{
				"len":  5,
				"item": map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 10}},
			},
			ExpectError: false,
		},
		{
			Name: "valid zero length",
			Config: map[string]any{
				"len":  0,
				"item": map[string]any{"gen": "bool"},
			},
			ExpectError: false,
		},
		{
			Name: "nested list",
			Config: map[string]any{
				"len": 2,
				"item": map[string]any{
					"gen": "list",
					"config": map[string]any{
						"len":  3,
						"item": map[string]any{"gen": "bool"},
					},
				},
			},
			ExpectError: false,
		},
		{
			Name: "list of objects",
			Config: map[string]any{
				"len": 2,
				"item": map[string]any{
					"gen": "object",
					"config": map[string]any{
						"fields": map[string]any{
							"x": map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 10}},
						},
					},
				},
			},
			ExpectError: false,
		},
		{
			Name:        "missing len and min/max",
			Config:      map[string]any{"item": map[string]any{"gen": "bool"}},
			ExpectError: true,
		},
		// variable-length configs
		{
			Name:        "valid min_len/max_len",
			Config:      map[string]any{"min_len": 1, "max_len": 5, "item": map[string]any{"gen": "bool"}},
			ExpectError: false,
		},
		{
			Name:        "min_len equals max_len",
			Config:      map[string]any{"min_len": 3, "max_len": 3, "item": map[string]any{"gen": "bool"}},
			ExpectError: false,
		},
		{
			Name:        "min_len zero",
			Config:      map[string]any{"min_len": 0, "max_len": 3, "item": map[string]any{"gen": "bool"}},
			ExpectError: false,
		},
		{
			Name:        "len with min_len",
			Config:      map[string]any{"len": 3, "min_len": 1, "item": map[string]any{"gen": "bool"}},
			ExpectError: true,
		},
		{
			Name:        "len with max_len",
			Config:      map[string]any{"len": 3, "max_len": 5, "item": map[string]any{"gen": "bool"}},
			ExpectError: true,
		},
		{
			Name:        "len with both min and max",
			Config:      map[string]any{"len": 3, "min_len": 1, "max_len": 5, "item": map[string]any{"gen": "bool"}},
			ExpectError: true,
		},
		{
			Name:        "only min_len",
			Config:      map[string]any{"min_len": 1, "item": map[string]any{"gen": "bool"}},
			ExpectError: true,
		},
		{
			Name:        "only max_len",
			Config:      map[string]any{"max_len": 5, "item": map[string]any{"gen": "bool"}},
			ExpectError: true,
		},
		{
			Name:        "negative min_len",
			Config:      map[string]any{"min_len": -1, "max_len": 5, "item": map[string]any{"gen": "bool"}},
			ExpectError: true,
		},
		{
			Name:        "max_len less than min_len",
			Config:      map[string]any{"min_len": 5, "max_len": 3, "item": map[string]any{"gen": "bool"}},
			ExpectError: true,
		},
		{
			Name:        "min_len is float",
			Config:      map[string]any{"min_len": 1.5, "max_len": 5, "item": map[string]any{"gen": "bool"}},
			ExpectError: true,
		},
		{
			Name: "negative len",
			Config: map[string]any{
				"len":  -1,
				"item": map[string]any{"gen": "bool"},
			},
			ExpectError: true,
		},
		{
			Name: "len is float",
			Config: map[string]any{
				"len":  3.5,
				"item": map[string]any{"gen": "bool"},
			},
			ExpectError: true,
		},
		{
			Name:        "missing item",
			Config:      map[string]any{"len": 3},
			ExpectError: true,
		},
		{
			Name: "item is string",
			Config: map[string]any{
				"len":  3,
				"item": "bool",
			},
			ExpectError: true,
		},
		{
			Name: "item missing gen",
			Config: map[string]any{
				"len":  3,
				"item": map[string]any{"config": map[string]any{}},
			},
			ExpectError: true,
		},
		{
			Name: "unknown item generator",
			Config: map[string]any{
				"len":  3,
				"item": map[string]any{"gen": "nonexistent"},
			},
			ExpectError: true,
		},
		{
			Name: "invalid item config propagates",
			Config: map[string]any{
				"len":  3,
				"item": map[string]any{"gen": "int", "config": map[string]any{"min": "bad"}},
			},
			ExpectError: true,
		},
	})
}

func TestListGenerator_Determinism(t *testing.T) {
	configs := []struct {
		name   string
		config map[string]any
	}{
		{
			name: "booleans",
			config: map[string]any{
				"len":  5,
				"item": map[string]any{"gen": "bool"},
			},
		},
		{
			name: "integers",
			config: map[string]any{
				"len":  10,
				"item": map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
			},
		},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			gen1, err := Get(listGen, tc.config)
			if err != nil {
				t.Fatalf("failed to create generator 1: %v", err)
			}
			gen2, err := Get(listGen, tc.config)
			if err != nil {
				t.Fatalf("failed to create generator 2: %v", err)
			}

			for i := range testIterations {
				seed := rng.SeedFromInt64(int64(i))
				v1, err := gen1.Next(rng.New(seed))
				if err != nil {
					t.Fatalf("gen1 error at %d: %v", i, err)
				}
				v2, err := gen2.Next(rng.New(seed))
				if err != nil {
					t.Fatalf("gen2 error at %d: %v", i, err)
				}

				s1 := fmt.Sprintf("%v", v1)
				s2 := fmt.Sprintf("%v", v2)
				if s1 != s2 {
					t.Errorf("determinism failed at %d: %s != %s", i, s1, s2)
				}
			}
		})
	}
}

func TestListGenerator_Output(t *testing.T) {
	config := map[string]any{
		"len":  4,
		"item": map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
	}

	gen, err := Get(listGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := val.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", val)
	}

	if len(result) != 4 {
		t.Errorf("expected 4 items, got %d", len(result))
	}

	for i, item := range result {
		v, ok := item.(int64)
		if !ok {
			t.Errorf("item %d: expected int64, got %T", i, item)
		}
		if v < 1 || v > 100 {
			t.Errorf("item %d: %d out of range [1, 100]", i, v)
		}
	}
}

func TestListGenerator_ZeroLength(t *testing.T) {
	config := map[string]any{
		"len":  0,
		"item": map[string]any{"gen": "bool"},
	}

	gen, err := Get(listGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := val.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", val)
	}
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d items", len(result))
	}
}

func TestListGenerator_Nested(t *testing.T) {
	t.Run("list of objects", func(t *testing.T) {
		config := map[string]any{
			"len": 3,
			"item": map[string]any{
				"gen": "object",
				"config": map[string]any{
					"fields": map[string]any{
						"x": map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 10}},
					},
				},
			},
		}

		gen, err := Get(listGen, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		r := rng.New(rng.SeedFromInt64(testSeed))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result, ok := val.([]any)
		if !ok {
			t.Fatalf("expected []any, got %T", val)
		}
		if len(result) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result))
		}

		for i, item := range result {
			obj, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("item %d: expected map[string]any, got %T", i, item)
			}
			if _, ok := obj["x"]; !ok {
				t.Errorf("item %d: missing key 'x'", i)
			}
		}
	})

	t.Run("variable length list of objects", func(t *testing.T) {
		config := map[string]any{
			"min_len": 1,
			"max_len": 5,
			"item": map[string]any{
				"gen": "object",
				"config": map[string]any{
					"fields": map[string]any{
						"x": map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 10}},
					},
				},
			},
		}

		gen, err := Get(listGen, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		r := rng.New(rng.SeedFromInt64(testSeed))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result, ok := val.([]any)
		if !ok {
			t.Fatalf("expected []any, got %T", val)
		}
		if len(result) < 1 || len(result) > 5 {
			t.Fatalf("expected length in [1, 5], got %d", len(result))
		}
	})

	t.Run("list of lists", func(t *testing.T) {
		config := map[string]any{
			"len": 2,
			"item": map[string]any{
				"gen": "list",
				"config": map[string]any{
					"len":  3,
					"item": map[string]any{"gen": "bool"},
				},
			},
		}

		gen, err := Get(listGen, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		r := rng.New(rng.SeedFromInt64(testSeed))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		outer, ok := val.([]any)
		if !ok {
			t.Fatalf("expected []any, got %T", val)
		}
		if len(outer) != 2 {
			t.Fatalf("expected 2 items, got %d", len(outer))
		}

		for i, item := range outer {
			inner, ok := item.([]any)
			if !ok {
				t.Fatalf("item %d: expected []any, got %T", i, item)
			}
			if len(inner) != 3 {
				t.Errorf("item %d: expected 3 items, got %d", i, len(inner))
			}
		}
	})
}

func TestListGenerator_VariableLengthDeterminism(t *testing.T) {
	config := map[string]any{
		"min_len": 1,
		"max_len": 10,
		"item":    map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
	}

	gen1, err := Get(listGen, config)
	if err != nil {
		t.Fatalf("failed to create generator 1: %v", err)
	}
	gen2, err := Get(listGen, config)
	if err != nil {
		t.Fatalf("failed to create generator 2: %v", err)
	}

	for i := range testIterations {
		seed := rng.SeedFromInt64(int64(i))
		v1, err := gen1.Next(rng.New(seed))
		if err != nil {
			t.Fatalf("gen1 error at %d: %v", i, err)
		}
		v2, err := gen2.Next(rng.New(seed))
		if err != nil {
			t.Fatalf("gen2 error at %d: %v", i, err)
		}

		s1 := fmt.Sprintf("%v", v1)
		s2 := fmt.Sprintf("%v", v2)
		if s1 != s2 {
			t.Errorf("determinism failed at %d: %s != %s", i, s1, s2)
		}
	}
}

func TestListGenerator_VariableLengthRange(t *testing.T) {
	config := map[string]any{
		"min_len": 2,
		"max_len": 8,
		"item":    map[string]any{"gen": "bool"},
	}

	gen, err := Get(listGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lengths := make(map[int]bool)
	for i := range distributionSamples {
		r := rng.New(rng.SeedFromInt64(int64(i)))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("error at %d: %v", i, err)
		}

		result := val.([]any)
		l := len(result)
		if l < 2 || l > 8 {
			t.Fatalf("length %d out of range [2, 8] at iteration %d", l, i)
		}
		lengths[l] = true
	}

	// With 10k samples and range [2, 8], we should see variation
	if len(lengths) < 2 {
		t.Errorf("expected length variation, only saw %d distinct lengths", len(lengths))
	}
}
