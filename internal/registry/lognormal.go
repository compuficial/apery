package registry

import "github.com/compuficial/apery/internal/rng"

// init registers the lognormal generator.
func init() {
	MustRegister("lognormal", normalFloatFactory("lognormal", (*rng.Rng).LogNormal))
	MustRegisterInfo("lognormal", GeneratorInfo{
		Description: "Log-normally distributed float: positive with a long right tail (prices, durations, incomes)",
		ConfigKeys: []ConfigKey{
			{Name: "mu", Type: "float", Desc: "Mean of the underlying normal, in log space (the median is e^mu)", Default: "0.0"},
			{Name: "sigma", Type: "float", Desc: "Standard deviation of the underlying normal, in log space (>= 0)", Default: "1.0"},
			{Name: "clamp_min", Type: "float", Desc: "Optional lower clamp"},
			{Name: "clamp_max", Type: "float", Desc: "Optional upper clamp"},
		},
		Example: `- name: price
  gen: lognormal
  config:
    mu: 3.0      # median about e^3 = 20
    sigma: 0.5
    clamp_max: 500`,
	})
}
