package registry

import (
	"fmt"
	"math"

	"github.com/compuficial/apery/internal/rng"
)

// roundedLogNormal emits a lognormal draw, after clamping, as the nearest
// whole number: amounts such as prices in cents.
type roundedLogNormal struct {
	floats *NormalFloatGenerator
}

// Next returns the next generated int64 value.
func (g *roundedLogNormal) Next(r *rng.Rng) (any, error) {
	v, _ := g.floats.Next(r)
	f := math.Round(v.(float64))
	// int64 of a larger float differs by CPU, so it is an error instead.
	if !(f < 1<<63) {
		return nil, fmt.Errorf("lognormal: %g is too large to round to an integer; lower 'mu' or set 'clamp_max'", f)
	}
	return int64(f), nil
}

// newLogNormal builds the float generator, wrapped to round when asked.
func newLogNormal(config map[string]any) (Generator, error) {
	gen, err := normalFloatFactory("lognormal", (*rng.Rng).LogNormal)(config)
	if err != nil {
		return nil, err
	}

	val, exists := config["round"]
	if !exists {
		return gen, nil
	}
	round, ok := val.(bool)
	if !ok {
		return nil, fmt.Errorf("lognormal: 'round' must be true or false, got %v", val)
	}
	if !round {
		return gen, nil
	}
	return &roundedLogNormal{floats: gen.(*NormalFloatGenerator)}, nil
}

// init registers the lognormal generator.
func init() {
	MustRegister("lognormal", newLogNormal)
	MustRegisterInfo("lognormal", GeneratorInfo{
		Description: "Log-normally distributed float, or whole number with round: positive with a long right tail (prices, durations, incomes)",
		ConfigKeys: []ConfigKey{
			{Name: "mu", Type: "float", Desc: "Mean of the underlying normal, in log space (the median is e^mu)", Default: "0.0"},
			{Name: "sigma", Type: "float", Desc: "Standard deviation of the underlying normal, in log space (>= 0)", Default: "1.0"},
			{Name: "clamp_min", Type: "float", Desc: "Optional lower clamp"},
			{Name: "clamp_max", Type: "float", Desc: "Optional upper clamp"},
			{Name: "round", Type: "bool", Desc: "Emit the nearest whole number (int64), after clamping: prices in cents", Default: "false"},
		},
		Example: `- name: price
  gen: lognormal
  config:
    mu: 3.0      # median about e^3 = 20
    sigma: 0.5
    clamp_max: 500`,
	})
}
