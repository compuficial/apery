package registry

import (
	"fmt"
	"github.com/compuficial/apery/internal/rng"
)

// OneOfGenerator randomly selects one of several sub-generators per invocation
type OneOfGenerator struct {
	gens    []Generator
	weights []float64 // nil = uniform; non-nil = precomputed CDF
	scratch *rng.Rng  // reseeded per draw; instances are chunk-local
}

// Next picks a generator (uniformly or weighted) and returns its output
func (o *OneOfGenerator) Next(r *rng.Rng) (any, error) {
	seed := r.GetSeed()
	o.scratch.Reseed(rng.Derive(seed, "__select__"))

	var idx int
	if o.weights == nil {
		idx = o.scratch.Intn(len(o.gens))
	} else {
		idx = cdfIndex(o.weights, o.scratch.Float64())
	}

	o.scratch.Reseed(rng.Derive(seed, "__value__"))
	return o.gens[idx].Next(o.scratch)
}

func validateOneOfConfig(config map[string]any) ([]Generator, []float64, error) {
	rawGens, ok := config["generators"]
	if !ok {
		return nil, nil, fmt.Errorf("one_of: 'generators' is required")
	}

	genSpecs, ok := rawGens.([]any)
	if !ok {
		return nil, nil, fmt.Errorf("one_of: 'generators' must be an array, got %T", rawGens)
	}

	if len(genSpecs) == 0 {
		return nil, nil, fmt.Errorf("one_of: 'generators' cannot be empty")
	}

	gens := make([]Generator, len(genSpecs))
	for i, raw := range genSpecs {
		gen, err := buildSubGenerator(raw, config, fmt.Sprintf("one_of: generators[%d]", i), nestPure)
		if err != nil {
			return nil, nil, err
		}
		gens[i] = gen
	}

	// Handle optional weights
	weightsVal, hasWeights := config["weights"]
	if hasWeights {
		cdf, err := buildCDF(weightsVal, len(gens), "one_of", "generators")
		if err != nil {
			return nil, nil, err
		}
		return gens, cdf, nil
	}

	return gens, nil, nil
}

func init() {
	MustRegister("one_of", func(config map[string]any) (Generator, error) {
		gens, weights, err := validateOneOfConfig(config)
		if err != nil {
			return nil, err
		}
		return &OneOfGenerator{gens: gens, weights: weights, scratch: rng.New(0)}, nil
	})
	MustRegisterInfo("one_of", GeneratorInfo{
		Description: "Randomly dispatch to one of several generators",
		ConfigKeys: []ConfigKey{
			{Name: "generators", Type: "[]any", Required: true, Desc: "Array of generator specs ({gen, config, null_rate})"},
			{Name: "weights", Type: "[]any", Desc: "Optional weights for non-uniform selection"},
		},
		Example: `- name: contact
  gen: one_of
  config:
    generators:
      - gen: regex
        config:
          pattern: "[a-z]+@example.com"
      - gen: regex
        config:
          pattern: "\\+1-\\d{3}-\\d{4}"
    weights: [7, 3]`,
	})
}
