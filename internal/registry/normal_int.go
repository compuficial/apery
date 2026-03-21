package registry

import (
	"apery/internal/rng"
	"fmt"
	"math"
)

// NormalIntGenerator generates random int64 values following a normal (Gaussian) distribution
type NormalIntGenerator struct {
	mu       float64
	sigma    float64
	hasClamp bool
	clampMin int64
	clampMax int64
}

// Next returns the next generated int value.
func (g *NormalIntGenerator) Next(r *rng.Rng) (any, error) {
	val := g.mu + g.sigma*r.NormFloat64()
	rounded := int64(math.Round(val))

	if g.hasClamp {
		rounded = max(g.clampMin, min(g.clampMax, rounded))
	}
	return rounded, nil
}

// validateNormalIntConfig validates and parses config for normal_int generator.
func validateNormalIntConfig(config map[string]any) (float64, float64, bool, int64, int64, error) {
	mu := defaultNormalMu
	sigma := defaultNormalSigma
	hasClamp := false
	var clampMin, clampMax int64

	// Validate mu parameter
	if val, exists := config["mu"]; exists {
		v, err := extractFloat(val, "mu", "normal_int")
		if err != nil {
			return 0, 0, false, 0, 0, err
		}
		mu = v
	}

	// Validate sigma parameter
	if val, exists := config["sigma"]; exists {
		v, err := extractFloat(val, "sigma", "normal_int")
		if err != nil {
			return 0, 0, false, 0, 0, err
		}
		sigma = v
	}

	// Validate sigma >= 0
	if sigma < 0 {
		return 0, 0, false, 0, 0, fmt.Errorf("normal_int: 'sigma' must be >= 0, got %f", sigma)
	}

	// Validate clamp_min parameter (accepts float or int, converts to int64)
	hasClampMin := false
	if val, exists := config["clamp_min"]; exists {
		hasClampMin = true
		v, err := extractFloat(val, "clamp_min", "normal_int")
		if err != nil {
			return 0, 0, false, 0, 0, err
		}
		clampMin = int64(v)
	}

	// Validate clamp_max parameter (accepts float or int, converts to int64)
	hasClampMax := false
	if val, exists := config["clamp_max"]; exists {
		hasClampMax = true
		v, err := extractFloat(val, "clamp_max", "normal_int")
		if err != nil {
			return 0, 0, false, 0, 0, err
		}
		clampMax = int64(v)
	}

	// Enable clamping if either bound is set
	hasClamp = hasClampMin || hasClampMax

	// Set defaults for missing bounds when clamping is enabled
	if hasClamp {
		if !hasClampMin {
			clampMin = math.MinInt64
		}
		if !hasClampMax {
			clampMax = math.MaxInt64
		}
	}

	// Validate clamp_min <= clamp_max
	if hasClampMin && hasClampMax && clampMin > clampMax {
		return 0, 0, false, 0, 0, fmt.Errorf("normal_int: 'clamp_min' (%d) must be <= 'clamp_max' (%d)", clampMin, clampMax)
	}

	return mu, sigma, hasClamp, clampMin, clampMax, nil
}

// init registers the normal_int generator.
func init() {
	MustRegister("normal_int", func(config map[string]any) (Generator, error) {
		mu, sigma, hasClamp, clampMin, clampMax, err := validateNormalIntConfig(config)
		if err != nil {
			return nil, err
		}
		return &NormalIntGenerator{
			mu:       mu,
			sigma:    sigma,
			hasClamp: hasClamp,
			clampMin: clampMin,
			clampMax: clampMax,
		}, nil
	})
	MustRegisterInfo("normal_int", GeneratorInfo{
		Description: "Normally distributed integer with optional clamping",
		ConfigKeys: []ConfigKey{
			{Name: "mu", Type: "float", Desc: "Mean (default 0.0)", Default: "0.0"},
			{Name: "sigma", Type: "float", Desc: "Standard deviation (default 1.0)", Default: "1.0"},
			{Name: "clamp_min", Type: "int", Desc: "Optional lower clamp"},
			{Name: "clamp_max", Type: "int", Desc: "Optional upper clamp"},
		},
		Example: `- name: age
  gen: normal_int
  config:
    mu: 35
    sigma: 10
    clamp_min: 18
    clamp_max: 80`,
	})
}
