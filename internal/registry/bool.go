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
	probability, err := extractFloat(val, "probability", "bool")
	if err != nil {
		return 0, err
	}

	// Validate range
	if probability < 0 || probability > 1 {
		return 0, fmt.Errorf("bool: 'probability' must be between 0.0 and 1.0, got %f (use: 0.7 for 70%% true)", probability)
	}

	return probability, nil
}

// init registers the bool generator.
func init() {
	MustRegister("bool", func(config map[string]any) (Generator, error) {
		probability, err := validateBoolConfig(config)
		if err != nil {
			return nil, err
		}
		return &BoolGenerator{probability: probability}, nil
	})
	MustRegisterInfo("bool", GeneratorInfo{
		Description: "Boolean with configurable probability",
		ConfigKeys: []ConfigKey{
			{Name: "probability", Type: "float", Desc: "Probability of true (default 0.5)", Default: "0.5"},
		},
		Example: `- name: active
  gen: bool
  config:
    probability: 0.8`,
	})
}
