package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"fmt"
)

// SampleGenerator selects N unique items without replacement from a value set
type SampleGenerator struct {
	values []any
	minN   int64
	maxN   int64
}

// Next selects N unique values using a partial Fisher-Yates shuffle
func (s *SampleGenerator) Next(r *rng.Rng) (any, error) {
	n := s.minN
	if s.maxN > s.minN {
		nSeed := rng.Derive(r.GetSeed(), "__n__")
		n = rng.New(nSeed).IntRange(s.minN, s.maxN)
	}

	// Partial Fisher-Yates: shuffle first n elements of index array
	indices := make([]int, len(s.values))
	for i := range indices {
		indices[i] = i
	}

	shuffleRng := rng.New(rng.Derive(r.GetSeed(), "__shuffle__"))
	result := make([]any, n)
	for i := range n {
		j := int(int64(i) + shuffleRng.IntRange(0, int64(len(indices)-1-int(i))))
		indices[i], indices[j] = indices[j], indices[i]
		result[i] = s.values[indices[i]]
	}

	return result, nil
}

func validateSampleConfig(config map[string]any) ([]any, int64, int64, error) {
	// Load values using same source options as pick
	values, err := loadSampleValues(config)
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

func loadSampleValues(config map[string]any) ([]any, error) {
	hasValues := config["values"] != nil
	hasFile := config["file"] != nil
	hasURL := config["url"] != nil

	sourceCount := 0
	if hasValues {
		sourceCount++
	}
	if hasFile {
		sourceCount++
	}
	if hasURL {
		sourceCount++
	}

	if sourceCount == 0 {
		return nil, fmt.Errorf("sample: must specify 'values', 'file', or 'url'")
	}
	if sourceCount > 1 {
		return nil, fmt.Errorf("sample: specify only one of 'values', 'file', or 'url'")
	}

	switch {
	case hasValues:
		return validatePickValues(config["values"])
	case hasFile:
		return loadPickFile(config["file"])
	default:
		return loadPickURL(config["url"], config["allowlist"])
	}
}

func init() {
	MustRegister("sample", func(config map[string]any) (Generator, error) {
		values, minN, maxN, err := validateSampleConfig(config)
		if err != nil {
			return nil, err
		}
		return &SampleGenerator{values: values, minN: minN, maxN: maxN}, nil
	})
	MustRegisterInfo("sample", GeneratorInfo{
		Description: "Select N unique items without replacement",
		ConfigKeys: []ConfigKey{
			{Name: "values", Type: "[]any", Desc: "Inline value list (mutually exclusive with file/url)"},
			{Name: "file", Type: "string", Desc: "Path to newline-delimited file"},
			{Name: "url", Type: "string", Desc: "URL to fetch values from (requires allowlist)"},
			{Name: "allowlist", Type: "[]any", Desc: "Allowed URL prefixes (required with url)"},
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
