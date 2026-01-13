package registry

import (
	"apery/internal/rng"
	"testing"
)

const floatGen = "float"

func TestFloatGenerator_Config(t *testing.T) {
	RunConfigTests(t, floatGen, []ConfigTestCase{
		// valid configs
		{Name: "default", Config: map[string]any{}, ExpectError: false},
		{Name: "min", Config: map[string]any{"min": 10.5}, ExpectError: false},
		{Name: "max", Config: map[string]any{"max": 20.6}, ExpectError: false},
		{Name: "min and max", Config: map[string]any{"min": 10.1, "max": 20}, ExpectError: false},
		{Name: "min equals max", Config: map[string]any{"min": 10.4, "max": 10.7}, ExpectError: false},
		{Name: "negative range", Config: map[string]any{"min": -100.3, "max": -10.2}, ExpectError: false},

		// invalid configs
		{Name: "min > max", Config: map[string]any{"min": 20.53, "max": 10.666}, ExpectError: true},
		{Name: "invalid max type (string)", Config: map[string]any{"max": "20.5555"}, ExpectError: true},
	})
}

func TestFloatGenerator_Determinism(t *testing.T) {
	RunDeterminismTests(t, floatGen, []DeterminismTestCase{
		{Name: "default", Config: map[string]any{}},
		{Name: "min", Config: map[string]any{"min": 10.1}},
		{Name: "max", Config: map[string]any{"max": 20.77}},
		{Name: "min and max", Config: map[string]any{"min": 10.1, "max": 20.239}},
		{Name: "min equals max", Config: map[string]any{"min": 10.5, "max": 10.5}},
		{Name: "negative range", Config: map[string]any{"min": -100.5, "max": -10.5}},
	})
}

func TestFloatGenerator_Range(t *testing.T) {
	tests := []struct {
		Name   string
		Config map[string]any
		Min    float64
		Max    float64
	}{
		{Name: "0-100", Config: map[string]any{}, Min: 0, Max: 100},
		{Name: "10-20", Config: map[string]any{"min": 10, "max": 20}, Min: 10, Max: 20},
		{Name: "negative range", Config: map[string]any{"min": -50, "max": -10}, Min: -50, Max: -10},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(floatGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			r := rng.New(testSeed)

			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}

				v := val.(float64)
				if v < tt.Min || v > tt.Max {
					t.Errorf("value %f out of range [%f, %f] at index %d", v, tt.Min, tt.Max, i)
				}
			}
		})
	}
}
