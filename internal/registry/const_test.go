package registry

import (
	"apery/internal/rng"
	"testing"
)

const constGen = "const"

func TestConstGenerator_Config(t *testing.T) {
	RunConfigTests(t, constGen, []ConfigTestCase{
		{Name: "valid string", Config: map[string]any{"value": "active"}, ExpectError: false},
		{Name: "valid int", Config: map[string]any{"value": 42}, ExpectError: false},
		{Name: "valid float", Config: map[string]any{"value": 3.14}, ExpectError: false},
		{Name: "valid bool", Config: map[string]any{"value": true}, ExpectError: false},
		{Name: "valid null", Config: map[string]any{"value": nil}, ExpectError: false},
		{Name: "missing value", Config: map[string]any{}, ExpectError: true},
	})
}

func TestConstGenerator_Determinism(t *testing.T) {
	RunDeterminismTests(t, constGen, []DeterminismTestCase{
		{Name: "string value", Config: map[string]any{"value": "hello"}},
		{Name: "int value", Config: map[string]any{"value": 99}},
	})
}

func TestConstGenerator_Output(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{"string", "active"},
		{"int", 42},
		{"float", 3.14},
		{"bool", true},
		{"null", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := Get(constGen, map[string]any{"value": tt.value})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for i := range testIterations {
				r := rng.New(rng.SeedFromInt64(int64(i)))
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("unexpected error at %d: %v", i, err)
				}
				if val != tt.value {
					t.Errorf("at %d: expected %v, got %v", i, tt.value, val)
				}
			}
		})
	}
}
