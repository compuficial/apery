package registry

import (
	"apery/internal/rng"
	"fmt"
)

// ListGenerator generates a list of N items from a single sub-generator
type ListGenerator struct {
	len  int64
	item Generator
}

// Next generates a slice of items using the sub-generator
func (l *ListGenerator) Next(r *rng.Rng) (any, error) {
	result := make([]any, l.len)
	for i := range l.len {
		childSeed := rng.DeriveIndex(r.GetSeed(), i)
		val, err := l.item.Next(rng.New(childSeed))
		if err != nil {
			return nil, fmt.Errorf("list: item '%d': %w", i, err)
		}
		result[i] = val
	}
	return result, nil
}

// validateListConfig validates config and instantiates the item sub-generator
func validateListConfig(config map[string]any) (int64, Generator, error) {
	// Extract and validate len
	rawLen, ok := config["len"]
	if !ok {
		return 0, nil, fmt.Errorf("list: 'len' is required")
	}
	length, err := extractInt(rawLen, "len", "list")
	if err != nil {
		return 0, nil, err
	}
	if length < 0 {
		return 0, nil, fmt.Errorf("list: 'len' must be >= 0, got %d", length)
	}

	// Extract and validate item generator spec
	rawItem, ok := config["item"]
	if !ok {
		return 0, nil, fmt.Errorf("list: 'item' is required")
	}
	spec, ok := rawItem.(map[string]any)
	if !ok {
		return 0, nil, fmt.Errorf("list: 'item' must be a map, got %T", rawItem)
	}

	genName, ok := spec["gen"].(string)
	if !ok || genName == "" {
		return 0, nil, fmt.Errorf("list: 'item' missing 'gen'")
	}

	genConfig, _ := spec["config"].(map[string]any)
	if genConfig == nil {
		genConfig = map[string]any{}
	}

	gen, err := Get(genName, genConfig)
	if err != nil {
		return 0, nil, fmt.Errorf("list: item: %w", err)
	}

	return length, gen, nil
}

// init registers the list generator
func init() {
	MustRegister("list", func(config map[string]any) (Generator, error) {
		len, item, err := validateListConfig(config)
		if err != nil {
			return nil, err
		}

		return &ListGenerator{len: len, item: item}, nil
	})
}
