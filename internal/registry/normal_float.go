package registry

import (
	"fmt"
	"github.com/compuficial/apery/internal/rng"
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

// Next returns the next generated float value.
func (g *NormalFloatGenerator) Next(r *rng.Rng) (any, error) {
	val := r.Normal(g.mu, g.sigma)

	if g.hasClamp {
		val = max(g.clampMin, min(g.clampMax, val))
	}
	return val, nil
}

// normalConfig holds the parsed parameters shared by normal_int and normal_float.
type normalConfig struct {
	mu, sigma          float64
	clampMin, clampMax float64
	hasMin, hasMax     bool
}

// parseNormalConfig reads mu, sigma (>= 0), and optional clamp bounds.
func parseNormalConfig(config map[string]any, gen string) (normalConfig, error) {
	nc := normalConfig{mu: defaultNormalMu, sigma: defaultNormalSigma}

	params := []struct {
		key string
		dst *float64
		has *bool
	}{
		{"mu", &nc.mu, nil},
		{"sigma", &nc.sigma, nil},
		{"clamp_min", &nc.clampMin, &nc.hasMin},
		{"clamp_max", &nc.clampMax, &nc.hasMax},
	}
	for _, p := range params {
		val, exists := config[p.key]
		if !exists {
			continue
		}
		v, err := extractFloat(val, p.key, gen)
		if err != nil {
			return normalConfig{}, err
		}
		*p.dst = v
		if p.has != nil {
			*p.has = true
		}
	}

	if nc.sigma < 0 {
		return normalConfig{}, fmt.Errorf("%s: 'sigma' must be >= 0, got %v", gen, nc.sigma)
	}
	if nc.hasMin && nc.hasMax && nc.clampMin > nc.clampMax {
		return normalConfig{}, fmt.Errorf("%s: 'clamp_min' (%v) must be <= 'clamp_max' (%v)", gen, nc.clampMin, nc.clampMax)
	}
	return nc, nil
}

// init registers the normal_float generator.
func init() {
	MustRegister("normal_float", func(config map[string]any) (Generator, error) {
		nc, err := parseNormalConfig(config, "normal_float")
		if err != nil {
			return nil, err
		}

		// A single bound leaves the other side open.
		g := &NormalFloatGenerator{
			mu:       nc.mu,
			sigma:    nc.sigma,
			hasClamp: nc.hasMin || nc.hasMax,
			clampMin: -math.MaxFloat64,
			clampMax: math.MaxFloat64,
		}
		if nc.hasMin {
			g.clampMin = nc.clampMin
		}
		if nc.hasMax {
			g.clampMax = nc.clampMax
		}
		return g, nil
	})
	MustRegisterInfo("normal_float", GeneratorInfo{
		Description: "Normally distributed float (Gaussian)",
		ConfigKeys: []ConfigKey{
			{Name: "mu", Type: "float", Desc: "Mean (default 0.0)", Default: "0.0"},
			{Name: "sigma", Type: "float", Desc: "Standard deviation (default 1.0)", Default: "1.0"},
			{Name: "clamp_min", Type: "float", Desc: "Optional lower clamp"},
			{Name: "clamp_max", Type: "float", Desc: "Optional upper clamp"},
		},
		Example: `- name: score
  gen: normal_float
  config:
    mu: 75.0
    sigma: 10.0`,
	})
}
