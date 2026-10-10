package registry

import (
	"fmt"
	"github.com/compuficial/apery/internal/rng"
	"sort"
)

// ObjectGenerator generates composite objects from other generators with configurable fields
type ObjectGenerator struct {
	fields  []objectField
	scratch *rng.Rng // reseeded per sub-field; instances are chunk-local
}

type objectField struct {
	name string
	gen  Generator
}

// Next generates a map with each field produced by its sub-generator
func (o *ObjectGenerator) Next(r *rng.Rng) (any, error) {
	result := make(map[string]any, len(o.fields))
	seed := r.GetSeed()
	for _, field := range o.fields {
		o.scratch.Reseed(rng.Derive(seed, field.name))
		val, err := field.gen.Next(o.scratch)
		if err != nil {
			return nil, fmt.Errorf("object: field '%s': %w", field.name, err)
		}
		result[field.name] = val
	}
	return result, nil
}

// validateObjectConfig validates config and instantiates sub-generators, sorted by field name
func validateObjectConfig(config map[string]any) ([]objectField, error) {
	rawFields, ok := config["fields"]
	if !ok {
		return nil, fmt.Errorf("object: 'fields' is required")
	}

	fieldsMap, ok := rawFields.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("object: 'fields' must be a map, got %T", rawFields)
	}

	if len(fieldsMap) == 0 {
		return nil, fmt.Errorf("object: 'fields' cannot be empty")
	}

	// Collect and sort field names for deterministic ordering
	names := make([]string, 0, len(fieldsMap))
	for name := range fieldsMap {
		names = append(names, name)
	}
	sort.Strings(names)

	fields := make([]objectField, 0, len(names))
	for _, name := range names {
		where := fmt.Sprintf("object: field '%s'", name)
		gen, err := buildSubGenerator(fieldsMap[name], config, where, nestPure)
		if err != nil {
			return nil, err
		}
		fields = append(fields, objectField{name: name, gen: gen})
	}

	return fields, nil
}

// init registers the object generator
func init() {
	MustRegister("object", func(config map[string]any) (Generator, error) {
		fields, err := validateObjectConfig(config)
		if err != nil {
			return nil, err
		}

		return &ObjectGenerator{fields: fields, scratch: rng.New(0)}, nil
	})
	MustRegisterInfo("object", GeneratorInfo{
		Description: "Nested object with named sub-fields",
		ConfigKeys: []ConfigKey{
			{Name: "fields", Type: "map[string]any", Required: true, Desc: "Map of field names to sub-generator specs ({gen, config, null_rate})"},
		},
		Example: `- name: address
  gen: object
  config:
    fields:
      city:
        gen: pick
        config:
          values: [NYC, LA, Chicago]
      zip:
        gen: int
        config:
          min: 10000
          max: 99999`,
	})
}
