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
		v, err := extractFloat(val, "min", "float")
		if err != nil {
			return 0, 0, err
		}
		minVal = v
	}

	// Validate max parameter
	if val, exists := config["max"]; exists {
		v, err := extractFloat(val, "max", "float")
		if err != nil {
			return 0, 0, err
		}
		maxVal = v
	}

	// Validate min <= max
	if minVal > maxVal {
		return 0, 0, fmt.Errorf("float: 'min' (%f) cannot be greater than 'max' (%f)", minVal, maxVal)
	}

	return minVal, maxVal, nil
}

func init() {
	MustRegister("float", func(config map[string]any) (Generator, error) {
		min, max, err := validateFloatConfig(config)
		if err != nil {
			return nil, err
		}
		return &FloatGenerator{min: min, max: max}, nil
	})
}
