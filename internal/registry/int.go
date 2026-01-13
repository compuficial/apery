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

func (i *IntGenerator) Next(r *rng.Rng) (any, error) {
	return r.IntRange(i.min, i.max), nil
}

func validateIntConfig(config map[string]any) (int64, int64, error) {
	minVal := int64(defaultIntMin)
	maxVal := int64(defaultIntMax)

	// Validate min parameter
	if val, exists := config["min"]; exists {
		m, ok := val.(int)
		if !ok {
			return 0, 0, fmt.Errorf("int: 'min' must be an integer, got %T (use: 10, not 10.0)", val)
		}
		minVal = int64(m)
	}

	// Validate max parameter
	if val, exists := config["max"]; exists {
		m, ok := val.(int)
		if !ok {
			return 0, 0, fmt.Errorf("int: 'max' must be an integer, got %T (use: 100, not 100.0)", val)
		}
		maxVal = int64(m)
	}

	// Validate min <= max
	if minVal > maxVal {
		return 0, 0, fmt.Errorf("int: 'min' (%d) cannot be greater than 'max' (%d)", minVal, maxVal)
	}

	return minVal, maxVal, nil
}

func init() {
	Register("int", func(config map[string]any) (Generator, error) {
		min, max, err := validateIntConfig(config)
		if err != nil {
			return nil, err
		}
		return &IntGenerator{min: min, max: max}, nil
	})
}
