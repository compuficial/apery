package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"fmt"
)

// ConstGenerator emits a fixed literal value on every row
type ConstGenerator struct {
	value any
}

// Next returns the configured constant value
func (c *ConstGenerator) Next(r *rng.Rng) (any, error) {
	return c.value, nil
}

// validateConstConfig validates and parses config for const generator
func validateConstConfig(config map[string]any) (any, error) {
	value, ok := config["value"]
	if !ok {
		return nil, fmt.Errorf("const: 'value' is required")
	}
	return value, nil
}

// init registers the const generator
func init() {
	MustRegister("const", func(config map[string]any) (Generator, error) {
		value, err := validateConstConfig(config)
		if err != nil {
			return nil, err
		}
		return &ConstGenerator{value: value}, nil
	})
	MustRegisterInfo("const", GeneratorInfo{
		Description: "Fixed literal value on every row",
		ConfigKeys: []ConfigKey{
			{Name: "value", Type: "any", Required: true, Desc: "The constant value to emit"},
		},
		Example: `- name: status
  gen: const
  config:
    value: active`,
	})
}
