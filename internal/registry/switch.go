package registry

import (
	"apery/internal/rng"
	"fmt"
)

// SwitchGenerator dispatches to sub-generators based on a key field's value from the current row.
type SwitchGenerator struct {
	key      string
	cases    map[string]Generator
	fallback Generator // optional
	deps     []string
}

// Next returns an error because switch requires row context.
func (s *SwitchGenerator) Next(_ *rng.Rng) (any, error) {
	return nil, fmt.Errorf("switch: requires row context")
}

// NextWithRow reads the key field, selects the matching case generator, and returns its output.
func (s *SwitchGenerator) NextWithRow(r *rng.Rng, row RowContext) (any, error) {
	keyVal, ok := row.Get(s.key)
	if !ok {
		return nil, fmt.Errorf("switch: key field '%s' not found in row context", s.key)
	}

	keyStr := fmt.Sprint(keyVal)
	gen, matched := s.cases[keyStr]
	if !matched {
		if s.fallback != nil {
			gen = s.fallback
		} else {
			return nil, fmt.Errorf("switch: no case for key value %q and no default", keyStr)
		}
	}

	valueRng := rng.New(rng.Derive(r.GetSeed(), "__value__"))
	if ra, ok := gen.(RowAwareGenerator); ok {
		return ra.NextWithRow(valueRng, row)
	}
	return gen.Next(valueRng)
}

// Dependencies returns the key field plus any transitive dependencies from row-aware sub-generators.
func (s *SwitchGenerator) Dependencies() []string {
	return s.deps
}

func collectSwitchDeps(key string, gens []Generator) []string {
	seen := map[string]bool{key: true}
	deps := []string{key}

	for _, gen := range gens {
		if dd, ok := gen.(DependencyDeclarer); ok {
			for _, dep := range dd.Dependencies() {
				if !seen[dep] {
					deps = append(deps, dep)
					seen[dep] = true
				}
			}
		}
	}
	return deps
}

func instantiateSubGenerator(spec map[string]any, context string) (Generator, error) {
	genName, ok := spec["gen"].(string)
	if !ok || genName == "" {
		return nil, fmt.Errorf("switch: %s missing 'gen'", context)
	}

	genConfig, _ := spec["config"].(map[string]any)
	if genConfig == nil {
		genConfig = map[string]any{}
	}

	gen, err := Get(genName, genConfig)
	if err != nil {
		return nil, fmt.Errorf("switch: %s: %w", context, err)
	}
	return gen, nil
}

func init() {
	MustRegister("switch", func(config map[string]any) (Generator, error) {
		key, ok := config["key"].(string)
		if !ok || key == "" {
			return nil, fmt.Errorf("switch: 'key' must be a non-empty string")
		}

		rawCases, ok := config["cases"]
		if !ok {
			return nil, fmt.Errorf("switch: 'cases' is required")
		}
		casesMap, ok := rawCases.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("switch: 'cases' must be a map, got %T", rawCases)
		}
		if len(casesMap) == 0 {
			return nil, fmt.Errorf("switch: 'cases' cannot be empty")
		}

		cases := make(map[string]Generator, len(casesMap))
		allGens := make([]Generator, 0, len(casesMap)+1)
		for caseName, rawSpec := range casesMap {
			spec, ok := rawSpec.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("switch: case '%s' must be a map, got %T", caseName, rawSpec)
			}
			gen, err := instantiateSubGenerator(spec, fmt.Sprintf("case '%s'", caseName))
			if err != nil {
				return nil, err
			}
			cases[caseName] = gen
			allGens = append(allGens, gen)
		}

		var fallback Generator
		if rawDefault, ok := config["default"]; ok {
			spec, ok := rawDefault.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("switch: 'default' must be a map, got %T", rawDefault)
			}
			gen, err := instantiateSubGenerator(spec, "default")
			if err != nil {
				return nil, err
			}
			fallback = gen
			allGens = append(allGens, gen)
		}

		deps := collectSwitchDeps(key, allGens)

		return &SwitchGenerator{
			key:      key,
			cases:    cases,
			fallback: fallback,
			deps:     deps,
		}, nil
	})
}
