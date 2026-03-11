package registry

import (
	"apery/internal/rng"
	"fmt"
	"testing"
)

const objectGen = "object"

func TestObjectGenerator_Config(t *testing.T) {
	RunConfigTests(t, objectGen, []ConfigTestCase{
		{
			Name: "single field",
			Config: map[string]any{
				"fields": map[string]any{
					"name": map[string]any{"gen": "bool"},
				},
			},
			ExpectError: false,
		},
		{
			Name: "multiple fields",
			Config: map[string]any{
				"fields": map[string]any{
					"name":   map[string]any{"gen": "bool"},
					"age":    map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
					"active": map[string]any{"gen": "bool", "config": map[string]any{"probability": 0.8}},
				},
			},
			ExpectError: false,
		},
		{
			Name: "nested object",
			Config: map[string]any{
				"fields": map[string]any{
					"profile": map[string]any{
						"gen": "object",
						"config": map[string]any{
							"fields": map[string]any{
								"active": map[string]any{"gen": "bool"},
							},
						},
					},
				},
			},
			ExpectError: false,
		},
		{
			Name:        "missing fields",
			Config:      map[string]any{},
			ExpectError: true,
		},
		{
			Name:        "fields is array",
			Config:      map[string]any{"fields": []any{"a", "b"}},
			ExpectError: true,
		},
		{
			Name:        "fields is empty map",
			Config:      map[string]any{"fields": map[string]any{}},
			ExpectError: true,
		},
		{
			Name: "field spec is string",
			Config: map[string]any{
				"fields": map[string]any{"name": "bool"},
			},
			ExpectError: true,
		},
		{
			Name: "field spec missing gen",
			Config: map[string]any{
				"fields": map[string]any{
					"name": map[string]any{"config": map[string]any{}},
				},
			},
			ExpectError: true,
		},
		{
			Name: "field spec gen is not string",
			Config: map[string]any{
				"fields": map[string]any{
					"name": map[string]any{"gen": 123},
				},
			},
			ExpectError: true,
		},
		{
			Name: "unknown generator",
			Config: map[string]any{
				"fields": map[string]any{
					"name": map[string]any{"gen": "nonexistent"},
				},
			},
			ExpectError: true,
		},
		{
			Name: "invalid sub-generator config propagates",
			Config: map[string]any{
				"fields": map[string]any{
					"age": map[string]any{"gen": "int", "config": map[string]any{"min": "bad"}},
				},
			},
			ExpectError: true,
		},
		{
			Name: "field config is not map",
			Config: map[string]any{
				"fields": map[string]any{
					"name": map[string]any{"gen": "bool", "config": "bad"},
				},
			},
			ExpectError: false, // config fails type assertion, falls back to empty map
		},
	})
}

func TestObjectGenerator_Determinism(t *testing.T) {
	configs := []struct {
		name   string
		config map[string]any
	}{
		{
			name: "single field",
			config: map[string]any{
				"fields": map[string]any{
					"active": map[string]any{"gen": "bool"},
				},
			},
		},
		{
			name: "multiple fields",
			config: map[string]any{
				"fields": map[string]any{
					"a": map[string]any{"gen": "bool"},
					"b": map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
					"c": map[string]any{"gen": "float", "config": map[string]any{"min": 0.0, "max": 1.0}},
				},
			},
		},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			gen1, err := Get(objectGen, tc.config)
			if err != nil {
				t.Fatalf("failed to create generator 1: %v", err)
			}
			gen2, err := Get(objectGen, tc.config)
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

func TestObjectGenerator_Output(t *testing.T) {
	config := map[string]any{
		"fields": map[string]any{
			"flag":  map[string]any{"gen": "bool"},
			"count": map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
		},
	}

	gen, err := Get(objectGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", val)
	}

	if _, ok := result["flag"]; !ok {
		t.Errorf("missing key 'flag'")
	}
	if _, ok := result["count"]; !ok {
		t.Errorf("missing key 'count'")
	}
	if len(result) != 2 {
		t.Errorf("expected 2 keys, got %d", len(result))
	}
}

func TestObjectGenerator_Nested(t *testing.T) {
	t.Run("2 levels deep", func(t *testing.T) {
		config := map[string]any{
			"fields": map[string]any{
				"inner": map[string]any{
					"gen": "object",
					"config": map[string]any{
						"fields": map[string]any{
							"val": map[string]any{"gen": "bool"},
						},
					},
				},
			},
		}

		gen, err := Get(objectGen, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		r := rng.New(rng.SeedFromInt64(testSeed))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		outer, ok := val.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any, got %T", val)
		}

		inner, ok := outer["inner"].(map[string]any)
		if !ok {
			t.Fatalf("expected inner to be map[string]any, got %T", outer["inner"])
		}

		if _, ok := inner["val"]; !ok {
			t.Errorf("missing key 'val' in inner object")
		}
	})

	t.Run("3 levels deep", func(t *testing.T) {
		config := map[string]any{
			"fields": map[string]any{
				"level1": map[string]any{
					"gen": "object",
					"config": map[string]any{
						"fields": map[string]any{
							"level2": map[string]any{
								"gen": "object",
								"config": map[string]any{
									"fields": map[string]any{
										"val": map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 10}},
									},
								},
							},
						},
					},
				},
			},
		}

		gen, err := Get(objectGen, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		r := rng.New(rng.SeedFromInt64(testSeed))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		l0, ok := val.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any at level 0, got %T", val)
		}

		l1, ok := l0["level1"].(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any at level 1, got %T", l0["level1"])
		}

		l2, ok := l1["level2"].(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any at level 2, got %T", l1["level2"])
		}

		if _, ok := l2["val"]; !ok {
			t.Errorf("missing key 'val' at level 2")
		}
	})

	t.Run("nested determinism", func(t *testing.T) {
		config := map[string]any{
			"fields": map[string]any{
				"inner": map[string]any{
					"gen": "object",
					"config": map[string]any{
						"fields": map[string]any{
							"a": map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 1000}},
							"b": map[string]any{"gen": "bool"},
						},
					},
				},
			},
		}

		gen1, _ := Get(objectGen, config)
		gen2, _ := Get(objectGen, config)

		for i := range testIterations {
			seed := rng.SeedFromInt64(int64(i))
			v1, _ := gen1.Next(rng.New(seed))
			v2, _ := gen2.Next(rng.New(seed))

			s1 := fmt.Sprintf("%v", v1)
			s2 := fmt.Sprintf("%v", v2)
			if s1 != s2 {
				t.Errorf("nested determinism failed at iteration %d: %s != %s", i, s1, s2)
			}
		}
	})
}
