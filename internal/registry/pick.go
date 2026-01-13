package registry

import (
	"apery/internal/rng"
	"fmt"
)

// PickGenerator randomly selects values from a configured list
type PickGenerator struct {
	values []any
}

// Next returns a random value from the list
func (p *PickGenerator) Next(r *rng.Rng) (any, error) {
	ran := r.Intn(len(p.values))
	return p.values[ran], nil
}

// validatePickConfig validates and parses config for pick generator
func validatePickConfig(config map[string]any) ([]any, error) {
	// Check if values exists
	val, exists := config["values"]
	if !exists {
		return nil, fmt.Errorf("pick: 'values' is required (example: {\"values\": [\"option1\", \"option2\"]})")
	}

	// Validate type
	values, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("pick: 'values' must be an array, got %T (use: [\"a\", \"b\"], not \"a\")", val)
	}

	// Validate not empty
	if len(values) == 0 {
		return nil, fmt.Errorf("pick: 'values' cannot be empty (must have at least one value to pick from)")
	}

	return values, nil
}

func init() {
	Register("pick", func(config map[string]any) (Generator, error) {
		values, err := validatePickConfig(config)
		if err != nil {
			return nil, err
		}
		return &PickGenerator{values: values}, nil
	})
}
