package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"fmt"
)

// OneOfGenerator randomly selects one of several sub-generators per invocation
type OneOfGenerator struct {
	gens    []Generator
	weights []float64 // nil = uniform; non-nil = precomputed CDF
}

// Next picks a generator (uniformly or weighted) and returns its output
func (o *OneOfGenerator) Next(r *rng.Rng) (any, error) {
	selectRng := rng.New(rng.Derive(r.GetSeed(), "__select__"))

	var idx int
	if o.weights == nil {
		idx = selectRng.Intn(len(o.gens))
	} else {
		f := selectRng.Float64()
		idx = len(o.gens) - 1
		for i, threshold := range o.weights {
			if f < threshold {
				idx = i
				break
			}
		}
	}

	valueRng := rng.New(rng.Derive(r.GetSeed(), "__value__"))
	return o.gens[idx].Next(valueRng)
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
		spec, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("one_of: generators[%d] must be a map, got %T", i, raw)
		}

		genName, ok := spec["gen"].(string)
		if !ok || genName == "" {
			return nil, nil, fmt.Errorf("one_of: generators[%d] missing 'gen'", i)
		}

		genConfig, _ := spec["config"].(map[string]any)
		if genConfig == nil {
			genConfig = map[string]any{}
		}

		gen, err := Get(genName, genConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("one_of: generators[%d]: %w", i, err)
		}
		gens[i] = gen
	}

	// Handle optional weights
	weightsVal, hasWeights := config["weights"]
	if hasWeights {
		cdf, err := validateOneOfWeights(weightsVal, len(gens))
		if err != nil {
			return nil, nil, err
		}
		return gens, cdf, nil
	}

	return gens, nil, nil
}

func validateOneOfWeights(weightsVal any, numGens int) ([]float64, error) {
	raw, ok := weightsVal.([]any)
	if !ok {
		return nil, fmt.Errorf("one_of: 'weights' must be an array, got %T", weightsVal)
	}
	if len(raw) != numGens {
		return nil, fmt.Errorf("one_of: 'weights' length (%d) must match 'generators' length (%d)", len(raw), numGens)
	}

	weights := make([]float64, len(raw))
	var total float64
	for i, w := range raw {
		v, err := extractFloat(w, "weights", "one_of")
		if err != nil {
			return nil, err
		}
		if v <= 0 {
			return nil, fmt.Errorf("one_of: 'weights[%d]' must be > 0, got %v", i, v)
		}
		weights[i] = v
		total += v
	}

	// Build CDF
	cdf := make([]float64, len(weights))
	cumulative := 0.0
	for i, w := range weights {
		cumulative += w / total
		cdf[i] = cumulative
	}
	cdf[len(cdf)-1] = 1.0

	return cdf, nil
}

func init() {
	MustRegister("one_of", func(config map[string]any) (Generator, error) {
		gens, weights, err := validateOneOfConfig(config)
		if err != nil {
			return nil, err
		}
		return &OneOfGenerator{gens: gens, weights: weights}, nil
	})
	MustRegisterInfo("one_of", GeneratorInfo{
		Description: "Randomly dispatch to one of several generators",
		ConfigKeys: []ConfigKey{
			{Name: "generators", Type: "[]any", Required: true, Desc: "Array of generator specs ({gen, config})"},
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
