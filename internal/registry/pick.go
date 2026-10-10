package registry

import (
	"fmt"

	"github.com/compuficial/apery/internal/rng"
)

// PickGenerator randomly selects values from a configured list
type PickGenerator struct {
	values  []any
	weights []float64 // nil = uniform; non-nil = precomputed CDF
}

// Next returns a random value from the list, using weighted selection if configured
func (p *PickGenerator) Next(r *rng.Rng) (any, error) {
	if p.weights == nil {
		return p.values[r.Intn(len(p.values))], nil
	}
	return p.values[cdfIndex(p.weights, r.Float64())], nil
}

// validatePickConfig validates and parses config for pick generator
func validatePickConfig(config map[string]any) ([]any, []float64, error) {
	values, isInline, err := loadValues(config, "pick")
	if err != nil {
		return nil, nil, err
	}

	// Handle optional weights (inline values only)
	weightsVal, hasWeights := config["weights"]
	if !hasWeights {
		return values, nil, nil
	}
	if !isInline {
		return nil, nil, fmt.Errorf("pick: 'weights' can only be used with inline 'values'")
	}
	cdf, err := buildCDF(weightsVal, len(values), "pick", "values")
	if err != nil {
		return nil, nil, err
	}
	return values, cdf, nil
}

// init registers the pick generator.
func init() {
	MustRegister("pick", func(config map[string]any) (Generator, error) {
		values, weights, err := validatePickConfig(config)
		if err != nil {
			return nil, err
		}
		return &PickGenerator{values: values, weights: weights}, nil
	})
	MustRegisterInfo("pick", GeneratorInfo{
		Description: "Random selection from a value list, file, or URL",
		ConfigKeys: []ConfigKey{
			{Name: "values", Type: "[]any", Desc: "Inline value list (mutually exclusive with file/url)"},
			{Name: "file", Type: "string", Desc: "Path to newline-delimited file (relative paths resolve against the plan file)"},
			{Name: "url", Type: "string", Desc: "URL to fetch values from once per run (requires allowlist; output is only as stable as the remote content)"},
			{Name: "allowlist", Type: "[]any", Desc: "Allowed hosts, as host or host:port, checked on every redirect (required with url)"},
			{Name: "weights", Type: "[]any", Desc: "Optional weights for non-uniform selection (inline values only)"},
		},
		Example: `- name: department
  gen: pick
  config:
    values: [engineering, sales, marketing]
    weights: [50, 30, 20]`,
	})
}
