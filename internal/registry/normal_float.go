package registry

import (
	"apery/internal/rng"
	"fmt"
	"math"
)

const (
	defaultNormalMu    = 0.0
	defaultNormalSigma = 1.0
)

// NormalFloatGenerator generates random floats following a normal (Gaussian) distribution
type NormalFloatGenerator struct {
	mu       float64
	sigma    float64
	hasClamp bool
	clampMin float64
	clampMax float64
}

func (g *NormalFloatGenerator) Next(r *rng.Rng) (any, error) {
	val := g.mu + g.sigma*r.NormFloat64()

	if g.hasClamp {
		val = max(g.clampMin, min(g.clampMax, val))
	}
	return val, nil
}

func validateNormalFloatConfig(config map[string]any) (float64, float64, bool, float64, float64, error) {
	mu := defaultNormalMu
	sigma := defaultNormalSigma
	hasClamp := false
	var clampMin, clampMax float64

	// Validate mu parameter
	if val, exists := config["mu"]; exists {
		v, err := extractFloat(val, "mu", "normal_float")
		if err != nil {
			return 0, 0, false, 0, 0, err
		}
		mu = v
	}

	// Validate sigma parameter
	if val, exists := config["sigma"]; exists {
		v, err := extractFloat(val, "sigma", "normal_float")
		if err != nil {
			return 0, 0, false, 0, 0, err
		}
		sigma = v
	}

	// Validate sigma >= 0
	if sigma < 0 {
		return 0, 0, false, 0, 0, fmt.Errorf("normal_float: 'sigma' must be >= 0, got %f", sigma)
	}

	// Validate clamp_min parameter
	hasClampMin := false
	if val, exists := config["clamp_min"]; exists {
		hasClampMin = true
		v, err := extractFloat(val, "clamp_min", "normal_float")
		if err != nil {
			return 0, 0, false, 0, 0, err
		}
		clampMin = v
	}

	// Validate clamp_max parameter
	hasClampMax := false
	if val, exists := config["clamp_max"]; exists {
		hasClampMax = true
		v, err := extractFloat(val, "clamp_max", "normal_float")
		if err != nil {
			return 0, 0, false, 0, 0, err
		}
		clampMax = v
	}

	// Enable clamping if either bound is set
	hasClamp = hasClampMin || hasClampMax

	// Set defaults for missing bounds when clamping is enabled
	if hasClamp {
		if !hasClampMin {
			clampMin = -math.MaxFloat64
		}
		if !hasClampMax {
			clampMax = math.MaxFloat64
		}
	}

	// Validate clamp_min <= clamp_max
	if hasClampMin && hasClampMax && clampMin > clampMax {
		return 0, 0, false, 0, 0, fmt.Errorf("normal_float: 'clamp_min' (%f) must be <= 'clamp_max' (%f)", clampMin, clampMax)
	}

	return mu, sigma, hasClamp, clampMin, clampMax, nil
}

func init() {
	MustRegister("normal_float", func(config map[string]any) (Generator, error) {
		mu, sigma, hasClamp, clampMin, clampMax, err := validateNormalFloatConfig(config)
		if err != nil {
			return nil, err
		}
		return &NormalFloatGenerator{
			mu:       mu,
			sigma:    sigma,
			hasClamp: hasClamp,
			clampMin: clampMin,
			clampMax: clampMax,
		}, nil
	})
}
