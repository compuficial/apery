package registry

import (
	"apery/internal/rng"
	"fmt"
)

const (
	defaultIntMin int64 = 0
	defaultIntMax int64 = 100
)

// IntGenerator generates random integers within a specified range
type IntGenerator struct {
	min int64
	max int64
}

// Next returns the next generated int value.
func (i *IntGenerator) Next(r *rng.Rng) (any, error) {
	return r.IntRange(i.min, i.max), nil
}

// validateIntConfig validates and parses config for int generator.
func validateIntConfig(config map[string]any) (int64, int64, error) {
	minVal := int64(defaultIntMin)
	maxVal := int64(defaultIntMax)

	// Validate min parameter
	if val, exists := config["min"]; exists {
		v, err := extractInt(val, "min", "int")
		if err != nil {
			return 0, 0, err
		}
		minVal = v
	}

	// Validate max parameter
	if val, exists := config["max"]; exists {
		v, err := extractInt(val, "max", "int")
		if err != nil {
			return 0, 0, err
		}
		maxVal = v
	}

	// Validate min <= max
	if minVal > maxVal {
		return 0, 0, fmt.Errorf("int: 'min' (%d) cannot be greater than 'max' (%d)", minVal, maxVal)
	}

	return minVal, maxVal, nil
}

// init registers the int generator.
func init() {
	MustRegister("int", func(config map[string]any) (Generator, error) {
		min, max, err := validateIntConfig(config)
		if err != nil {
			return nil, err
		}
		return &IntGenerator{min: min, max: max}, nil
	})
}
