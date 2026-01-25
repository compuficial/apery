package registry

import (
	"apery/internal/rng"
	"testing"
)

const intGen = "int"

func TestIntGenerator_Config(t *testing.T) {
	RunConfigTests(t, intGen, []ConfigTestCase{
		// valid configs
		{Name: "default", Config: map[string]any{}, ExpectError: false},
		{Name: "min", Config: map[string]any{"min": 10}, ExpectError: false},
		{Name: "max", Config: map[string]any{"max": 20}, ExpectError: false},
		{Name: "min and max", Config: map[string]any{"min": 10, "max": 20}, ExpectError: false},
		{Name: "min equals max", Config: map[string]any{"min": 10, "max": 10}, ExpectError: false},
		{Name: "negative range", Config: map[string]any{"min": -100, "max": -10}, ExpectError: false},

		// invalid configs
		{Name: "min > max", Config: map[string]any{"min": 20, "max": 10}, ExpectError: true},
		{Name: "invalid min type (float)", Config: map[string]any{"min": 10.5}, ExpectError: true},
		{Name: "invalid max type (string)", Config: map[string]any{"max": "20"}, ExpectError: true},
	})
}

func TestIntGenerator_Determinism(t *testing.T) {
	RunDeterminismTests(t, intGen, []DeterminismTestCase{
		{Name: "default", Config: map[string]any{}},
		{Name: "min", Config: map[string]any{"min": 10}},
		{Name: "max", Config: map[string]any{"max": 20}},
		{Name: "min and max", Config: map[string]any{"min": 10, "max": 20}},
		{Name: "min equals max", Config: map[string]any{"min": 10, "max": 10}},
		{Name: "negative range", Config: map[string]any{"min": -100, "max": -10}},
	})
}

func TestIntGenerator_Range(t *testing.T) {
	tests := []struct {
		Name   string
		Config map[string]any
		Min    int64
		Max    int64
	}{
		{Name: "0-100", Config: map[string]any{}, Min: 0, Max: 100},
		{Name: "10-20", Config: map[string]any{"min": 10, "max": 20}, Min: 10, Max: 20},
		{Name: "negative range", Config: map[string]any{"min": -50, "max": -10}, Min: -50, Max: -10},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(intGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			r := rng.New(testSeed)

			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}

				v := val.(int64)
				if v < tt.Min || v > tt.Max {
					t.Errorf("value %d out of range [%d, %d] at index %d", v, tt.Min, tt.Max, i)
				}
			}
		})
	}
}

func TestIntGenerator_OutputType(t *testing.T) {
	gen, err := Get(intGen, map[string]any{"min": 10, "max": 20})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(testSeed)
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("generation error: %v", err)
	}

	if _, ok := val.(int64); !ok {
		t.Errorf("expected int64, got %T", val)
	}
}
