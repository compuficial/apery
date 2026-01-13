package registry

import (
	"apery/internal/rng"
	"testing"
)

const pickGen = "pick"

func TestPickGenerator_Config(t *testing.T) {
	RunConfigTests(t, pickGen, []ConfigTestCase{
		// valid configs
		{Name: "two strings", Config: map[string]any{"values": []any{"A", "B"}}, ExpectError: false},
		{Name: "four strings", Config: map[string]any{"values": []any{"A", "B", "C", "D"}}, ExpectError: false},
		{Name: "single value", Config: map[string]any{"values": []any{"A"}}, ExpectError: false},
		{Name: "integers", Config: map[string]any{"values": []any{1, 2, 3, 4}}, ExpectError: false},
		{Name: "mixed types", Config: map[string]any{"values": []any{"A", 1, 5.5, true}}, ExpectError: false},

		// invalid configs
		{Name: "missing values", Config: map[string]any{}, ExpectError: true},
		{Name: "empty array", Config: map[string]any{"values": []any{}}, ExpectError: true},
		{Name: "values not array", Config: map[string]any{"values": "not-array"}, ExpectError: true},
	})
}

func TestPickGenerator_Determinism(t *testing.T) {
	RunDeterminismTests(t, pickGen, []DeterminismTestCase{
		{Name: "two values", Config: map[string]any{"values": []any{"a", "b"}}},
		{Name: "five values", Config: map[string]any{"values": []any{1, 2, 3, 4, 5}}},
	})
}

func TestPickGenerator_ValuesOnly(t *testing.T) {
	tests := []struct {
		Name   string
		Values []any
	}{
		{Name: "strings", Values: []any{"a", "b", "c"}},
		{Name: "integers", Values: []any{1, 2, 3, 4, 5}},
		{Name: "mixed", Values: []any{"x", 42, true}},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(pickGen, map[string]any{"values": tt.Values})
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			// Build set of valid values
			validSet := make(map[any]bool)
			for _, v := range tt.Values {
				validSet[v] = true
			}

			r := rng.New(testSeed)

			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}

				if !validSet[val] {
					t.Errorf("got %v which is not in values at index %d", val, i)
				}
			}
		})
	}
}
