package registry

import (
	"apery/internal/rng"
	"testing"
)

// Test constants
const (
	testSeed              = int64(42)
	testIterations        = 100
	distributionSamples   = 10000
	distributionTolerance = 0.02
)

// ConfigTestCase defines a test case for generator configuration
type ConfigTestCase struct {
	Name        string
	Config      map[string]any
	ExpectError bool
}

// RunConfigTests runs configuration validation tests for a generator
func RunConfigTests(t *testing.T, genName string, tests []ConfigTestCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(genName, tt.Config)

			if tt.ExpectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gen == nil {
				t.Errorf("expected generator, got nil")
			}
		})
	}
}

// AssertDeterministic verifies that a generator produces identical output given the same seed
func AssertDeterministic(t *testing.T, genName string, config map[string]any) {
	t.Helper()

	gen1, err := Get(genName, config)
	if err != nil {
		t.Fatalf("failed to create generator 1: %v", err)
	}
	gen2, err := Get(genName, config)
	if err != nil {
		t.Fatalf("failed to create generator 2: %v", err)
	}

	r1 := rng.New(testSeed)
	r2 := rng.New(testSeed)

	for i := range testIterations {
		val1, err := gen1.Next(r1)
		if err != nil {
			t.Fatalf("generation 1 error at %d: %v", i, err)
		}
		val2, err := gen2.Next(r2)
		if err != nil {
			t.Fatalf("generation 2 error at %d: %v", i, err)
		}

		if val1 != val2 {
			t.Errorf("values differ at %d: %v != %v", i, val1, val2)
		}
	}
}

// DeterminismTestCase defines a test case for determinism tests with multiple configs
type DeterminismTestCase struct {
	Name   string
	Config map[string]any
}

// RunDeterminismTests runs determinism tests for multiple configurations
func RunDeterminismTests(t *testing.T, genName string, tests []DeterminismTestCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			AssertDeterministic(t, genName, tt.Config)
		})
	}
}
