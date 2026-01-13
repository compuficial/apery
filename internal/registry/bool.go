package registry

import (
	"apery/internal/rng"
	"fmt"
)

// BoolGenerator generates boolean values with configurable probability
type BoolGenerator struct {
	probability float64
}

// Next returns true with probability p, false otherwise
func (b *BoolGenerator) Next(r *rng.Rng) (any, error) {
	return r.Float64() <= b.probability, nil
}

// validateBoolConfig validates and parses config for bool generator
func validateBoolConfig(config map[string]any) (float64, error) {
	// Default to 50/50 if not specified
	val, exists := config["probability"]
	if !exists {
		return 0.5, nil
	}

	// Validate type
	probability, ok := val.(float64)
	if !ok {
		return 0, fmt.Errorf("bool: 'probability' must be a float64, got %T (use: 0.7, not 7 or \"0.7\")", val)
	}

	// Validate range
	if probability < 0 || probability > 1 {
		return 0, fmt.Errorf("bool: 'probability' must be between 0.0 and 1.0, got %f (use: 0.7 for 70%% true)", probability)
	}

	return probability, nil
}

func init() {
	Register("bool", func(config map[string]any) (Generator, error) {
		probability, err := validateBoolConfig(config)
		if err != nil {
			return nil, err
		}
		return &BoolGenerator{probability: probability}, nil
	})
}
