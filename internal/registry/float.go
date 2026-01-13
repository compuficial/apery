package registry

import (
	"apery/internal/rng"
	"fmt"
)

const (
	defaultFloatMin = 0.0
	defaultFloatMax = 100.0
)

// FloatGenerator generates random floats within a specified range
type FloatGenerator struct {
	min float64
	max float64
}

func (i *FloatGenerator) Next(r *rng.Rng) (any, error) {
	return r.FloatRange(i.min, i.max), nil
}

func validateFloatConfig(config map[string]any) (float64, float64, error) {
	minVal := defaultFloatMin
	maxVal := defaultFloatMax

	// Validate min parameter
	if val, exists := config["min"]; exists {
		switch v := val.(type) {
		case float64:
			minVal = v
		case int:
			minVal = float64(v)
		default:
			return 0, 0, fmt.Errorf("float: 'min' must be a number, got %T (use: 0.5 or 0)", val)
		}
	}

	// Validate max parameter
	if val, exists := config["max"]; exists {
		switch v := val.(type) {
		case float64:
			maxVal = v
		case int:
			maxVal = float64(v)
		default:
			return 0, 0, fmt.Errorf("float: 'max' must be a number, got %T (use: 100.5 or 100)", val)
		}
	}

	// Validate min <= max
	if minVal > maxVal {
		return 0, 0, fmt.Errorf("float: 'min' (%f) cannot be greater than 'max' (%f)", minVal, maxVal)
	}

	return minVal, maxVal, nil
}

func init() {
	Register("float", func(config map[string]any) (Generator, error) {
		min, max, err := validateFloatConfig(config)
		if err != nil {
			return nil, err
		}
		return &FloatGenerator{min: min, max: max}, nil
	})
}
