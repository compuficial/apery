package registry

import (
	"fmt"

	"github.com/compuficial/apery/internal/rng"
)

// maxPoissonLambda keeps counts exact: past 2^53, float64 can't hold every
// integer.
const maxPoissonLambda = 1e15

// PoissonGenerator generates Poisson-distributed counts: how many events
// happen in a period when they occur independently at a steady rate.
type PoissonGenerator struct {
	lambda float64
}

// Next returns the next generated count as int64.
func (g *PoissonGenerator) Next(r *rng.Rng) (any, error) {
	return r.Poisson(g.lambda), nil
}

// init registers the poisson generator.
func init() {
	MustRegister("poisson", func(config map[string]any) (Generator, error) {
		g := &PoissonGenerator{lambda: 1}
		if val, ok := config["lambda"]; ok {
			lambda, err := extractFloat(val, "lambda", "poisson")
			if err != nil {
				return nil, err
			}
			g.lambda = lambda
		}
		if !(g.lambda > 0 && g.lambda <= maxPoissonLambda) {
			return nil, fmt.Errorf("poisson: 'lambda' must be > 0 and <= %g, got %v", float64(maxPoissonLambda), g.lambda)
		}
		return g, nil
	})
	MustRegisterInfo("poisson", GeneratorInfo{
		Description: "Poisson-distributed integer count: events per period at a steady average rate",
		ConfigKeys: []ConfigKey{
			{Name: "lambda", Type: "float", Desc: "Average count (> 0); also the variance", Default: "1.0"},
		},
		Example: `- name: support_tickets_per_day
  gen: poisson
  config:
    lambda: 4.5`,
	})
}
