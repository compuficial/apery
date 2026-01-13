package registry

import (
	"apery/internal/rng"
	"fmt"
)

// SeqGenerator generates sequential integers starting from 1
type SeqGenerator struct {
	current int
	step    int
}

// Next returns the next integer in the sequence
func (s *SeqGenerator) Next(r *rng.Rng) (any, error) {
	s.current += s.step
	return s.current, nil
}

// validateSeqConfig validates and parses config for seq generator
func validateSeqConfig(config map[string]any) (start, step int, err error) {
	start, step = 1, 1

	// Validate start parameter
	if val, exists := config["start"]; exists {
		s, ok := val.(int)
		if !ok {
			return 0, 0, fmt.Errorf("seq: 'start' must be an integer, got %T (use: 100, not 100.0)", val)
		}
		start = s
	}

	// Validate step parameter
	if val, exists := config["step"]; exists {
		st, ok := val.(int)
		if !ok {
			return 0, 0, fmt.Errorf("seq: 'step' must be an integer, got %T (use: 5, not 5.0)", val)
		}
		if st == 0 {
			return 0, 0, fmt.Errorf("seq: 'step' cannot be 0 (sequence would not progress)")
		}
		step = st
	}

	return start, step, nil
}

func init() {
	Register("seq", func(config map[string]any) (Generator, error) {
		start, step, err := validateSeqConfig(config)
		if err != nil {
			return nil, err
		}
		return &SeqGenerator{current: start - step, step: step}, nil
	})
}
