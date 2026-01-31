package registry

import (
	"apery/internal/rng"
	"fmt"
)

// SeqGenerator generates sequential integers starting from 1
type SeqGenerator struct {
	current int64
	step    int64
}

// Next returns the next integer in the sequence
func (s *SeqGenerator) Next(r *rng.Rng) (any, error) {
	s.current += s.step
	return s.current, nil
}

// validateSeqConfig validates and parses config for seq generator
func validateSeqConfig(config map[string]any) (start, step int64, err error) {
	start, step = 1, 1

	// Validate start parameter
	if val, exists := config["start"]; exists {
		v, err := extractInt(val, "start", "seq")
		if err != nil {
			return 0, 0, err
		}
		start = v
	}

	// Validate step parameter
	if val, exists := config["step"]; exists {
		v, err := extractInt(val, "step", "seq")
		if err != nil {
			return 0, 0, err
		}
		if v == 0 {
			return 0, 0, fmt.Errorf("seq: 'step' cannot be 0 (sequence would not progress)")
		}
		step = v
	}

	return start, step, nil
}

// init registers the seq generator.
func init() {
	MustRegister("seq", func(config map[string]any) (Generator, error) {
		start, step, err := validateSeqConfig(config)
		if err != nil {
			return nil, err
		}
		return &SeqGenerator{current: start - step, step: step}, nil
	})
}
