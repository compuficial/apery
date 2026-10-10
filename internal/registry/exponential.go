package registry

import (
	"fmt"
	"math"

	"github.com/compuficial/apery/internal/rng"
)

// ExponentialGenerator generates exponentially distributed floats, such as
// the gaps between independent events.
type ExponentialGenerator struct {
	mean     float64
	clampMax float64 // +Inf when unset
}

// Next returns the next generated float value.
func (g *ExponentialGenerator) Next(r *rng.Rng) (any, error) {
	return min(g.clampMax, r.Exponential(g.mean)), nil
}

// init registers the exponential generator.
func init() {
	MustRegister("exponential", func(config map[string]any) (Generator, error) {
		g := &ExponentialGenerator{mean: 1, clampMax: math.Inf(1)}
		if val, ok := config["mean"]; ok {
			mean, err := extractFloat(val, "mean", "exponential")
			if err != nil {
				return nil, err
			}
			g.mean = mean
		}
		if !(g.mean > 0) || math.IsInf(g.mean, 0) {
			return nil, fmt.Errorf("exponential: 'mean' must be a positive number, got %v", g.mean)
		}

		if val, ok := config["clamp_max"]; ok {
			clampMax, err := extractFloat(val, "clamp_max", "exponential")
			if err != nil {
				return nil, err
			}
			if !(clampMax > 0) {
				return nil, fmt.Errorf("exponential: 'clamp_max' must be > 0, got %v", clampMax)
			}
			g.clampMax = clampMax
		}
		return g, nil
	})
	MustRegisterInfo("exponential", GeneratorInfo{
		Description: "Exponentially distributed float >= 0: gaps between independent events (rate = 1/mean)",
		ConfigKeys: []ConfigKey{
			{Name: "mean", Type: "float", Desc: "Average value (> 0)", Default: "1.0"},
			{Name: "clamp_max", Type: "float", Desc: "Optional upper clamp (> 0)"},
		},
		Example: `- name: seconds_to_next_event
  gen: exponential
  config:
    mean: 300
    clamp_max: 3600`,
	})
}
