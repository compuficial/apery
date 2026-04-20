package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"fmt"
)

// Default values for Zipf parameters
const (
	defaultZipfS    = 1.1
	defaultZipfV    = 1.0
	defaultZipfImax = uint64(100)
)

// ZipfGenerator generates random int64 values following a Zipf distribution.
// Useful for modeling "long tail" patterns like word frequencies or popularity rankings.
type ZipfGenerator struct {
	s    float64
	v    float64
	imax uint64
}

// Next returns the next Zipf-distributed value as int64
func (g *ZipfGenerator) Next(r *rng.Rng) (any, error) {
	z := r.NewZipf(g.s, g.v, g.imax)
	return int64(z.Uint64()), nil
}

// validateZipfConfig validates and parses config for zipf generator
func validateZipfConfig(config map[string]any) (float64, float64, uint64, error) {
	s := defaultZipfS
	v := defaultZipfV
	imax := defaultZipfImax

	// Validate s parameter (skewness)
	if val, exists := config["s"]; exists {
		extracted, err := extractFloat(val, "s", "zipf")
		if err != nil {
			return 0, 0, 0, err
		}
		s = extracted
	}

	// s must be > 1 (mathematical requirement for Zipf distribution)
	if s <= 1 {
		return 0, 0, 0, fmt.Errorf("zipf: 's' must be > 1, got %f", s)
	}

	// Validate v parameter (offset)
	if val, exists := config["v"]; exists {
		extracted, err := extractFloat(val, "v", "zipf")
		if err != nil {
			return 0, 0, 0, err
		}
		v = extracted
	}

	// v must be >= 1
	if v < 1 {
		return 0, 0, 0, fmt.Errorf("zipf: 'v' must be >= 1, got %f", v)
	}

	// Validate imax parameter (maximum value) - must be a positive integer
	if val, exists := config["imax"]; exists {
		extracted, err := extractUint(val, "imax", "zipf")
		if err != nil {
			return 0, 0, 0, err
		}
		if extracted < 1 {
			return 0, 0, 0, fmt.Errorf("zipf: 'imax' must be >= 1, got %d", extracted)
		}
		imax = extracted
	}

	return s, v, imax, nil
}

// init registers the zipf generator.
func init() {
	MustRegister("zipf", func(config map[string]any) (Generator, error) {
		s, v, imax, err := validateZipfConfig(config)
		if err != nil {
			return nil, err
		}
		return &ZipfGenerator{
			s:    s,
			v:    v,
			imax: imax,
		}, nil
	})
	MustRegisterInfo("zipf", GeneratorInfo{
		Description: "Zipf-distributed integer for long-tail patterns",
		ConfigKeys: []ConfigKey{
			{Name: "s", Type: "float", Desc: "Skewness parameter (default 1.1)", Default: "1.1"},
			{Name: "v", Type: "float", Desc: "Offset parameter (default 1.0)", Default: "1.0"},
			{Name: "imax", Type: "int", Desc: "Maximum value (default 100)", Default: "100"},
		},
		Example: `- name: popularity
  gen: zipf
  config:
    s: 1.5
    imax: 1000`,
	})
}
