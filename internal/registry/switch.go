package registry

import (
	"fmt"
	"sort"

	"github.com/compuficial/apery/internal/rng"
)

// SwitchGenerator dispatches to sub-generators based on a key field's value from the current row.
type SwitchGenerator struct {
	key      string
	cases    map[string]Generator
	fallback Generator // optional
	deps     []string
	scratch  *rng.Rng // reseeded per row; instances are chunk-local
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

	keyStr := "null" // a null key selects the "null" case, if any
	if keyVal != nil {
		keyStr = fmt.Sprint(keyVal)
	}
	gen, matched := s.cases[keyStr]
	if !matched {
		if s.fallback != nil {
			gen = s.fallback
		} else {
			return nil, fmt.Errorf("switch: no case for key value %q and no default", keyStr)
		}
	}

	s.scratch.Reseed(rng.Derive(r.GetSeed(), "__value__"))
	if ra, ok := gen.(RowAwareGenerator); ok {
		return ra.NextWithRow(s.scratch, row)
	}
	return gen.Next(s.scratch)
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
		caseNames := make([]string, 0, len(casesMap))
		for caseName := range casesMap {
			caseNames = append(caseNames, caseName)
		}
		sort.Strings(caseNames) // deterministic error reporting and dependency order
		for _, caseName := range caseNames {
			where := fmt.Sprintf("switch: case '%s'", caseName)
			gen, err := buildSubGenerator(casesMap[caseName], config, where, nestRowAware)
			if err != nil {
				return nil, err
			}
			cases[caseName] = gen
			allGens = append(allGens, gen)
		}

		var fallback Generator
		if rawDefault, ok := config["default"]; ok {
			gen, err := buildSubGenerator(rawDefault, config, "switch: default", nestRowAware)
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
			scratch:  rng.New(0),
		}, nil
	})
	MustRegisterInfo("switch", GeneratorInfo{
		Description: "Conditional dispatch based on another field's value",
		ConfigKeys: []ConfigKey{
			{Name: "key", Type: "string", Required: true, Desc: "Row field name to switch on"},
			{Name: "cases", Type: "map[string]any", Required: true, Desc: "Map of field values to generator specs"},
			{Name: "default", Type: "map[string]any", Desc: "Fallback generator spec if no case matches"},
		},
		Example: `- name: access_level
  gen: switch
  config:
    key: department
    cases:
      engineering:
        gen: const
        config:
          value: full
      sales:
        gen: const
        config:
          value: read-only
    default:
      gen: const
      config:
        value: standard`,
	})
}
