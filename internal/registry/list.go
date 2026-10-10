package registry

import (
	"fmt"
	"github.com/compuficial/apery/internal/rng"
)

// ListGenerator generates a list of items from a single sub-generator
type ListGenerator struct {
	minLen  int64
	maxLen  int64
	item    Generator
	scratch *rng.Rng // reseeded per draw; instances are chunk-local
}

// Next generates a slice of items using the sub-generator
func (l *ListGenerator) Next(r *rng.Rng) (any, error) {
	seed := r.GetSeed()
	length := l.minLen
	if l.maxLen > l.minLen {
		l.scratch.Reseed(rng.Derive(seed, "__len__"))
		length = l.scratch.IntRange(l.minLen, l.maxLen)
	}

	result := make([]any, length)
	for i := range length {
		l.scratch.Reseed(rng.DeriveIndex(seed, i))
		val, err := l.item.Next(l.scratch)
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

	rawItem, ok := config["item"]
	if !ok {
		return 0, 0, nil, fmt.Errorf("list: 'item' is required")
	}
	gen, err := buildSubGenerator(rawItem, config, "list: item", nestPure)
	if err != nil {
		return 0, 0, nil, err
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

		return &ListGenerator{minLen: minLen, maxLen: maxLen, item: item, scratch: rng.New(0)}, nil
	})
	MustRegisterInfo("list", GeneratorInfo{
		Description: "Array of items from a single generator",
		ConfigKeys: []ConfigKey{
			{Name: "item", Type: "map[string]any", Required: true, Desc: "Sub-generator spec ({gen, config, null_rate})"},
			{Name: "len", Type: "int", Desc: "Fixed length (mutually exclusive with min_len/max_len)"},
			{Name: "min_len", Type: "int", Desc: "Minimum length for variable-length mode"},
			{Name: "max_len", Type: "int", Desc: "Maximum length for variable-length mode"},
		},
		Example: `- name: tags
  gen: list
  config:
    min_len: 1
    max_len: 4
    item:
      gen: pick
      config:
        values: [admin, beta, vip]`,
	})
}
