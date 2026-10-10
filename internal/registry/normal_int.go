package registry

import (
	"math"

	"github.com/compuficial/apery/internal/rng"
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
	val := r.Normal(g.mu, g.sigma)
	rounded := int64(math.Round(val))

	if g.hasClamp {
		rounded = max(g.clampMin, min(g.clampMax, rounded))
	}
	return rounded, nil
}

// init registers the normal_int generator.
func init() {
	MustRegister("normal_int", func(config map[string]any) (Generator, error) {
		nc, err := parseNormalConfig(config, "normal_int")
		if err != nil {
			return nil, err
		}

		// A single bound leaves the other side open.
		g := &NormalIntGenerator{
			mu:       nc.mu,
			sigma:    nc.sigma,
			hasClamp: nc.hasMin || nc.hasMax,
			clampMin: math.MinInt64,
			clampMax: math.MaxInt64,
		}
		if nc.hasMin {
			g.clampMin = int64(nc.clampMin)
		}
		if nc.hasMax {
			g.clampMax = int64(nc.clampMax)
		}
		return g, nil
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
