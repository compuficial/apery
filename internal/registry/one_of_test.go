package registry

import (
	"apery/internal/rng"
	"fmt"
	"testing"
)

const oneOfGen = "one_of"

func TestOneOfGenerator_Config(t *testing.T) {
	RunConfigTests(t, oneOfGen, []ConfigTestCase{
		{
			Name: "valid uniform",
			Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "bool"},
					map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 10}},
				},
			},
			ExpectError: false,
		},
		{
			Name: "valid weighted",
			Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "bool"},
					map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 10}},
				},
				"weights": []any{3.0, 1.0},
			},
			ExpectError: false,
		},
		{
			Name: "valid single generator",
			Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "bool"},
				},
			},
			ExpectError: false,
		},
		{
			Name:        "missing generators",
			Config:      map[string]any{},
			ExpectError: true,
		},
		{
			Name:        "empty generators",
			Config:      map[string]any{"generators": []any{}},
			ExpectError: true,
		},
		{
			Name:        "generators not array",
			Config:      map[string]any{"generators": "bool"},
			ExpectError: true,
		},
		{
			Name: "generator entry not map",
			Config: map[string]any{
				"generators": []any{"bool"},
			},
			ExpectError: true,
		},
		{
			Name: "generator entry missing gen",
			Config: map[string]any{
				"generators": []any{
					map[string]any{"config": map[string]any{}},
				},
			},
			ExpectError: true,
		},
		{
			Name: "unknown generator",
			Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "nonexistent"},
				},
			},
			ExpectError: true,
		},
		{
			Name: "invalid sub-generator config",
			Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "int", "config": map[string]any{"min": "bad"}},
				},
			},
			ExpectError: true,
		},
		{
			Name: "weights length mismatch",
			Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "bool"},
					map[string]any{"gen": "bool"},
				},
				"weights": []any{1.0},
			},
			ExpectError: true,
		},
		{
			Name: "weights not array",
			Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "bool"},
				},
				"weights": 1.0,
			},
			ExpectError: true,
		},
		{
			Name: "zero weight",
			Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "bool"},
					map[string]any{"gen": "bool"},
				},
				"weights": []any{1.0, 0.0},
			},
			ExpectError: true,
		},
		{
			Name: "negative weight",
			Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "bool"},
					map[string]any{"gen": "bool"},
				},
				"weights": []any{1.0, -1.0},
			},
			ExpectError: true,
		},
	})
}

func TestOneOfGenerator_Determinism(t *testing.T) {
	configs := []struct {
		name   string
		config map[string]any
	}{
		{
			name: "uniform",
			config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "const", "config": map[string]any{"value": "a"}},
					map[string]any{"gen": "const", "config": map[string]any{"value": "b"}},
					map[string]any{"gen": "const", "config": map[string]any{"value": "c"}},
				},
			},
		},
		{
			name: "weighted",
			config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "const", "config": map[string]any{"value": "a"}},
					map[string]any{"gen": "const", "config": map[string]any{"value": "b"}},
				},
				"weights": []any{9.0, 1.0},
			},
		},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			gen1, err := Get(oneOfGen, tc.config)
			if err != nil {
				t.Fatalf("failed to create generator 1: %v", err)
			}
			gen2, err := Get(oneOfGen, tc.config)
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

func TestOneOfGenerator_UniformDistribution(t *testing.T) {
	config := map[string]any{
		"generators": []any{
			map[string]any{"gen": "const", "config": map[string]any{"value": "a"}},
			map[string]any{"gen": "const", "config": map[string]any{"value": "b"}},
		},
	}

	gen, err := Get(oneOfGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	counts := map[string]int{"a": 0, "b": 0}
	for i := range distributionSamples {
		r := rng.New(rng.SeedFromInt64(int64(i)))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("error at %d: %v", i, err)
		}
		counts[val.(string)]++
	}

	// Expect roughly 50/50
	ratio := float64(counts["a"]) / float64(distributionSamples)
	if ratio < 0.5-distributionTolerance || ratio > 0.5+distributionTolerance {
		t.Errorf("expected ~50%% 'a', got %.2f%% (a=%d, b=%d)", ratio*100, counts["a"], counts["b"])
	}
}

func TestOneOfGenerator_WeightedDistribution(t *testing.T) {
	config := map[string]any{
		"generators": []any{
			map[string]any{"gen": "const", "config": map[string]any{"value": "a"}},
			map[string]any{"gen": "const", "config": map[string]any{"value": "b"}},
		},
		"weights": []any{9.0, 1.0},
	}

	gen, err := Get(oneOfGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	counts := map[string]int{"a": 0, "b": 0}
	for i := range distributionSamples {
		r := rng.New(rng.SeedFromInt64(int64(i)))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("error at %d: %v", i, err)
		}
		counts[val.(string)]++
	}

	// Expect ~90% a, ~10% b
	ratio := float64(counts["a"]) / float64(distributionSamples)
	if ratio < 0.9-distributionTolerance || ratio > 0.9+distributionTolerance {
		t.Errorf("expected ~90%% 'a', got %.2f%% (a=%d, b=%d)", ratio*100, counts["a"], counts["b"])
	}
}

func TestOneOfGenerator_MixedTypes(t *testing.T) {
	config := map[string]any{
		"generators": []any{
			map[string]any{"gen": "bool"},
			map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
			map[string]any{"gen": "const", "config": map[string]any{"value": "hello"}},
		},
	}

	gen, err := Get(oneOfGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	typeSeen := map[string]bool{}
	for i := range distributionSamples {
		r := rng.New(rng.SeedFromInt64(int64(i)))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("error at %d: %v", i, err)
		}
		typeSeen[fmt.Sprintf("%T", val)] = true
	}

	if !typeSeen["bool"] {
		t.Error("never produced a bool value")
	}
	if !typeSeen["int64"] {
		t.Error("never produced an int64 value")
	}
	if !typeSeen["string"] {
		t.Error("never produced a string value")
	}
}
