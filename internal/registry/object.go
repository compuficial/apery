package registry

import (
	"apery/internal/rng"
	"fmt"
	"sort"
)

// ObjectGenerator generates composite objects from other generators with configurable fields
type ObjectGenerator struct {
	fields []objectField
}

type objectField struct {
	name string
	gen  Generator
}

// Next generates a map with each field produced by its sub-generator
func (o *ObjectGenerator) Next(r *rng.Rng) (any, error) {
	result := make(map[string]any, len(o.fields))
	for _, field := range o.fields {
		childSeed := rng.Derive(r.GetSeed(), field.name)
		val, err := field.gen.Next(rng.New(childSeed))
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
		spec, ok := fieldsMap[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("object: field '%s' must be a map, got %T", name, fieldsMap[name])
		}

		genName, ok := spec["gen"].(string)
		if !ok || genName == "" {
			return nil, fmt.Errorf("object: field '%s' missing 'gen'", name)
		}

		genConfig, _ := spec["config"].(map[string]any)
		if genConfig == nil {
			genConfig = map[string]any{}
		}

		gen, err := Get(genName, genConfig)
		if err != nil {
			return nil, fmt.Errorf("object: field '%s': %w", name, err)
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

		return &ObjectGenerator{fields: fields}, nil
	})
}
