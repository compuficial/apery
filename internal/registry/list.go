package registry

import (
	"apery/internal/rng"
	"fmt"
)

// ListGenerator generates a list of items from a single sub-generator
type ListGenerator struct {
	minLen int64
	maxLen int64
	item   Generator
}

// Next generates a slice of items using the sub-generator
func (l *ListGenerator) Next(r *rng.Rng) (any, error) {
	length := l.minLen
	if l.maxLen > l.minLen {
		lenSeed := rng.Derive(r.GetSeed(), "__len__")
		length = rng.New(lenSeed).IntRange(l.minLen, l.maxLen)
	}

	result := make([]any, length)
	for i := range length {
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
func validateListConfig(config map[string]any) (int64, int64, Generator, error) {
	_, hasLen := config["len"]
	_, hasMinLen := config["min_len"]
	_, hasMaxLen := config["max_len"]

	var minLen, maxLen int64

	if hasLen && (hasMinLen || hasMaxLen) {
		return 0, 0, nil, fmt.Errorf("list: cannot specify 'len' together with 'min_len'/'max_len'")
	}

	if hasLen {
		length, err := extractInt(config["len"], "len", "list")
		if err != nil {
			return 0, 0, nil, err
		}
		if length < 0 {
			return 0, 0, nil, fmt.Errorf("list: 'len' must be >= 0, got %d", length)
		}
		minLen = length
		maxLen = length
	} else if hasMinLen || hasMaxLen {
		if !hasMinLen || !hasMaxLen {
			return 0, 0, nil, fmt.Errorf("list: must specify both 'min_len' and 'max_len'")
		}
		var err error
		minLen, err = extractInt(config["min_len"], "min_len", "list")
		if err != nil {
			return 0, 0, nil, err
		}
		maxLen, err = extractInt(config["max_len"], "max_len", "list")
		if err != nil {
			return 0, 0, nil, err
		}
		if minLen < 0 {
			return 0, 0, nil, fmt.Errorf("list: 'min_len' must be >= 0, got %d", minLen)
		}
		if maxLen < minLen {
			return 0, 0, nil, fmt.Errorf("list: 'max_len' (%d) must be >= 'min_len' (%d)", maxLen, minLen)
		}
	} else {
		return 0, 0, nil, fmt.Errorf("list: must specify 'len' or both 'min_len' and 'max_len'")
	}

	// Extract and validate item generator spec
	rawItem, ok := config["item"]
	if !ok {
		return 0, 0, nil, fmt.Errorf("list: 'item' is required")
	}
	spec, ok := rawItem.(map[string]any)
	if !ok {
		return 0, 0, nil, fmt.Errorf("list: 'item' must be a map, got %T", rawItem)
	}

	genName, ok := spec["gen"].(string)
	if !ok || genName == "" {
		return 0, 0, nil, fmt.Errorf("list: 'item' missing 'gen'")
	}

	genConfig, _ := spec["config"].(map[string]any)
	if genConfig == nil {
		genConfig = map[string]any{}
	}

	gen, err := Get(genName, genConfig)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("list: item: %w", err)
	}

	return minLen, maxLen, gen, nil
}

// init registers the list generator
func init() {
	MustRegister("list", func(config map[string]any) (Generator, error) {
		minLen, maxLen, item, err := validateListConfig(config)
		if err != nil {
			return nil, err
		}

		return &ListGenerator{minLen: minLen, maxLen: maxLen, item: item}, nil
	})
}
