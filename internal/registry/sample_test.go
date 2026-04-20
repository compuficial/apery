package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"fmt"
	"testing"
)

const sampleGen = "sample"

func TestSampleGenerator_Config(t *testing.T) {
	RunConfigTests(t, sampleGen, []ConfigTestCase{
		{
			Name:        "valid fixed n",
			Config:      map[string]any{"values": []any{"a", "b", "c", "d"}, "n": 2},
			ExpectError: false,
		},
		{
			Name:        "valid n equals values length",
			Config:      map[string]any{"values": []any{"a", "b", "c"}, "n": 3},
			ExpectError: false,
		},
		{
			Name:        "valid n zero",
			Config:      map[string]any{"values": []any{"a", "b"}, "n": 0},
			ExpectError: false,
		},
		{
			Name:        "valid min_n/max_n",
			Config:      map[string]any{"values": []any{"a", "b", "c", "d", "e"}, "min_n": 1, "max_n": 3},
			ExpectError: false,
		},
		{
			Name:        "valid min_n equals max_n",
			Config:      map[string]any{"values": []any{"a", "b", "c"}, "min_n": 2, "max_n": 2},
			ExpectError: false,
		},
		{
			Name:        "n exceeds values",
			Config:      map[string]any{"values": []any{"a", "b"}, "n": 3},
			ExpectError: true,
		},
		{
			Name:        "max_n exceeds values",
			Config:      map[string]any{"values": []any{"a", "b"}, "min_n": 1, "max_n": 3},
			ExpectError: true,
		},
		{
			Name:        "n with min_n",
			Config:      map[string]any{"values": []any{"a", "b", "c"}, "n": 2, "min_n": 1},
			ExpectError: true,
		},
		{
			Name:        "n with max_n",
			Config:      map[string]any{"values": []any{"a", "b", "c"}, "n": 2, "max_n": 3},
			ExpectError: true,
		},
		{
			Name:        "only min_n",
			Config:      map[string]any{"values": []any{"a", "b", "c"}, "min_n": 1},
			ExpectError: true,
		},
		{
			Name:        "only max_n",
			Config:      map[string]any{"values": []any{"a", "b", "c"}, "max_n": 2},
			ExpectError: true,
		},
		{
			Name:        "negative n",
			Config:      map[string]any{"values": []any{"a", "b"}, "n": -1},
			ExpectError: true,
		},
		{
			Name:        "negative min_n",
			Config:      map[string]any{"values": []any{"a", "b", "c"}, "min_n": -1, "max_n": 2},
			ExpectError: true,
		},
		{
			Name:        "max_n less than min_n",
			Config:      map[string]any{"values": []any{"a", "b", "c"}, "min_n": 3, "max_n": 1},
			ExpectError: true,
		},
		{
			Name:        "missing n and min/max",
			Config:      map[string]any{"values": []any{"a", "b", "c"}},
			ExpectError: true,
		},
		{
			Name:        "missing values source",
			Config:      map[string]any{"n": 2},
			ExpectError: true,
		},
		{
			Name:        "empty values",
			Config:      map[string]any{"values": []any{}, "n": 0},
			ExpectError: true,
		},
		{
			Name:        "multiple sources",
			Config:      map[string]any{"values": []any{"a"}, "file": "/tmp/x.txt", "n": 1},
			ExpectError: true,
		},
	})
}

func TestSampleGenerator_Determinism(t *testing.T) {
	configs := []struct {
		name   string
		config map[string]any
	}{
		{
			name:   "fixed n",
			config: map[string]any{"values": []any{"a", "b", "c", "d", "e"}, "n": 3},
		},
		{
			name:   "variable n",
			config: map[string]any{"values": []any{"a", "b", "c", "d", "e"}, "min_n": 1, "max_n": 4},
		},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			gen1, err := Get(sampleGen, tc.config)
			if err != nil {
				t.Fatalf("failed to create generator 1: %v", err)
			}
			gen2, err := Get(sampleGen, tc.config)
			if err != nil {
				t.Fatalf("failed to create generator 2: %v", err)
			}

			for i := range testIterations {
				seed := rng.SeedFromInt64(int64(i))
				v1, err := gen1.Next(rng.New(seed))
				if err != nil {
					t.Fatalf("gen1 error at %d: %v", i, err)
				}
				v2, err := gen2.Next(rng.New(seed))
				if err != nil {
					t.Fatalf("gen2 error at %d: %v", i, err)
				}

				s1 := fmt.Sprintf("%v", v1)
				s2 := fmt.Sprintf("%v", v2)
				if s1 != s2 {
					t.Errorf("determinism failed at %d: %s != %s", i, s1, s2)
				}
			}
		})
	}
}

func TestSampleGenerator_Uniqueness(t *testing.T) {
	config := map[string]any{
		"values": []any{"a", "b", "c", "d", "e", "f", "g", "h"},
		"n":      5,
	}

	gen, err := Get(sampleGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := range distributionSamples {
		r := rng.New(rng.SeedFromInt64(int64(i)))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("error at %d: %v", i, err)
		}

		result := val.([]any)
		if len(result) != 5 {
			t.Fatalf("expected 5 items, got %d at iteration %d", len(result), i)
		}

		seen := make(map[any]bool, len(result))
		for _, item := range result {
			if seen[item] {
				t.Fatalf("duplicate item %v at iteration %d: %v", item, i, result)
			}
			seen[item] = true
		}
	}
}

func TestSampleGenerator_ZeroN(t *testing.T) {
	config := map[string]any{
		"values": []any{"a", "b", "c"},
		"n":      0,
	}

	gen, err := Get(sampleGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := val.([]any)
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d items", len(result))
	}
}

func TestSampleGenerator_AllValues(t *testing.T) {
	values := []any{"a", "b", "c", "d"}
	config := map[string]any{"values": values, "n": 4}

	gen, err := Get(sampleGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := val.([]any)
	if len(result) != 4 {
		t.Fatalf("expected 4 items, got %d", len(result))
	}

	seen := make(map[any]bool, 4)
	for _, item := range result {
		seen[item] = true
	}
	for _, v := range values {
		if !seen[v] {
			t.Errorf("missing value %v in result %v", v, result)
		}
	}
}

func TestSampleGenerator_VariableNRange(t *testing.T) {
	config := map[string]any{
		"values": []any{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
		"min_n":  2,
		"max_n":  6,
	}

	gen, err := Get(sampleGen, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lengths := make(map[int]bool)
	for i := range distributionSamples {
		r := rng.New(rng.SeedFromInt64(int64(i)))
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("error at %d: %v", i, err)
		}

		result := val.([]any)
		l := len(result)
		if l < 2 || l > 6 {
			t.Fatalf("length %d out of range [2, 6] at iteration %d", l, i)
		}
		lengths[l] = true

		// Also verify uniqueness
		seen := make(map[any]bool, l)
		for _, item := range result {
			if seen[item] {
				t.Fatalf("duplicate at iteration %d: %v", i, result)
			}
			seen[item] = true
		}
	}

	if len(lengths) < 2 {
		t.Errorf("expected length variation, only saw %d distinct lengths", len(lengths))
	}
}
