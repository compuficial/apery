package registry

import (
	"apery/internal/rng"
	"fmt"
)

const (
	defaultNormalMu    = 0.0
	defaultNormalSigma = 1.0
)

// NormalFloatGenerator generates random floats following a normal (Gaussian) distribution
type NormalFloatGenerator struct {
	mu    float64
	sigma float64
}

func (g *NormalFloatGenerator) Next(r *rng.Rng) (any, error) {
	return g.mu + g.sigma*r.NormFloat64(), nil
}

func validateNormalFloatConfig(config map[string]any) (float64, float64, error) {
	mu := defaultNormalMu
	sigma := defaultNormalSigma

	// Validate mu parameter
	if val, exists := config["mu"]; exists {
		switch v := val.(type) {
		case float64:
			mu = v
		case int:
			mu = float64(v)
		default:
			return 0, 0, fmt.Errorf("normal_float: 'mu' must be a number, got %T", val)
		}
	}

	// Validate sigma parameter
	if val, exists := config["sigma"]; exists {
		switch v := val.(type) {
		case float64:
			sigma = v
		case int:
			sigma = float64(v)
		default:
			return 0, 0, fmt.Errorf("normal_float: 'sigma' must be a number, got %T", val)
		}
	}

	// Validate sigma >= 0
	if sigma < 0 {
		return 0, 0, fmt.Errorf("normal_float: 'sigma' must be >= 0, got %f", sigma)
	}

	return mu, sigma, nil
}

func init() {
	Register("normal_float", func(config map[string]any) (Generator, error) {
		mu, sigma, err := validateNormalFloatConfig(config)
		if err != nil {
			return nil, err
		}
		return &NormalFloatGenerator{mu: mu, sigma: sigma}, nil
	})
}
