package registry

import (
	"fmt"
	"github.com/compuficial/apery/internal/rng"
)

// SampleGenerator selects N unique items without replacement from a value set
type SampleGenerator struct {
	values  []any
	minN    int64
	maxN    int64
	indices []int    // identity permutation, restored after every draw
	swaps   []int    // swap partner per position, used to undo a draw
	scratch *rng.Rng // reseeded per draw; instances are chunk-local
}

// Next selects N unique values using a partial Fisher-Yates shuffle over a
// reusable index array. Undoing the swaps afterwards keeps each row O(N)
// instead of O(len(values)).
func (s *SampleGenerator) Next(r *rng.Rng) (any, error) {
	seed := r.GetSeed()
	n := s.minN
	if s.maxN > s.minN {
		s.scratch.Reseed(rng.Derive(seed, "__n__"))
		n = s.scratch.IntRange(s.minN, s.maxN)
	}

	s.scratch.Reseed(rng.Derive(seed, "__shuffle__"))
	result := make([]any, n)
	for i := range int(n) {
		j := i + int(s.scratch.IntRange(0, int64(len(s.indices)-1-i)))
		s.indices[i], s.indices[j] = s.indices[j], s.indices[i]
		s.swaps[i] = j
		result[i] = s.values[s.indices[i]]
	}

	for i := int(n) - 1; i >= 0; i-- {
		j := s.swaps[i]
		s.indices[i], s.indices[j] = s.indices[j], s.indices[i]
	}

	return result, nil
}

func validateSampleConfig(config map[string]any) ([]any, int64, int64, error) {
	// Load values using same source options as pick
	values, _, err := loadValues(config, "sample")
	if err != nil {
		return nil, 0, 0, err
	}

	// Parse count: n or min_n/max_n
	_, hasN := config["n"]
	_, hasMinN := config["min_n"]
	_, hasMaxN := config["max_n"]

	var minN, maxN int64

	if hasN && (hasMinN || hasMaxN) {
		return nil, 0, 0, fmt.Errorf("sample: cannot specify 'n' together with 'min_n'/'max_n'")
	}

	if hasN {
		n, err := extractInt(config["n"], "n", "sample")
		if err != nil {
			return nil, 0, 0, err
		}
		if n < 0 {
			return nil, 0, 0, fmt.Errorf("sample: 'n' must be >= 0, got %d", n)
		}
		if int64(len(values)) < n {
			return nil, 0, 0, fmt.Errorf("sample: 'n' (%d) exceeds available values (%d)", n, len(values))
		}
		minN = n
		maxN = n
	} else if hasMinN || hasMaxN {
		if !hasMinN || !hasMaxN {
			return nil, 0, 0, fmt.Errorf("sample: must specify both 'min_n' and 'max_n'")
		}
		minN, err = extractInt(config["min_n"], "min_n", "sample")
		if err != nil {
			return nil, 0, 0, err
		}
		maxN, err = extractInt(config["max_n"], "max_n", "sample")
		if err != nil {
			return nil, 0, 0, err
		}
		if minN < 0 {
			return nil, 0, 0, fmt.Errorf("sample: 'min_n' must be >= 0, got %d", minN)
		}
		if maxN < minN {
			return nil, 0, 0, fmt.Errorf("sample: 'max_n' (%d) must be >= 'min_n' (%d)", maxN, minN)
		}
		if int64(len(values)) < maxN {
			return nil, 0, 0, fmt.Errorf("sample: 'max_n' (%d) exceeds available values (%d)", maxN, len(values))
		}
	} else {
		return nil, 0, 0, fmt.Errorf("sample: must specify 'n' or both 'min_n' and 'max_n'")
	}

	return values, minN, maxN, nil
}

func init() {
	MustRegister("sample", func(config map[string]any) (Generator, error) {
		values, minN, maxN, err := validateSampleConfig(config)
		if err != nil {
			return nil, err
		}
		indices := make([]int, len(values))
		for i := range indices {
			indices[i] = i
		}
		return &SampleGenerator{
			values:  values,
			minN:    minN,
			maxN:    maxN,
			indices: indices,
			swaps:   make([]int, maxN),
			scratch: rng.New(0),
		}, nil
	})
	MustRegisterInfo("sample", GeneratorInfo{
		Description: "Select N unique items without replacement",
		ConfigKeys: []ConfigKey{
			{Name: "values", Type: "[]any", Desc: "Inline value list (mutually exclusive with file/url)"},
			{Name: "file", Type: "string", Desc: "Path to newline-delimited file (relative paths resolve against the plan file)"},
			{Name: "url", Type: "string", Desc: "URL to fetch values from once per run (requires allowlist; output is only as stable as the remote content)"},
			{Name: "allowlist", Type: "[]any", Desc: "Allowed hosts, as host or host:port, checked on every redirect (required with url)"},
			{Name: "n", Type: "int", Desc: "Fixed sample size (mutually exclusive with min_n/max_n)"},
			{Name: "min_n", Type: "int", Desc: "Minimum sample size for variable mode"},
			{Name: "max_n", Type: "int", Desc: "Maximum sample size for variable mode"},
		},
		Example: `- name: skills
  gen: sample
  config:
    values: [Go, Python, Rust, TypeScript]
    min_n: 2
    max_n: 4`,
	})
}
