package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"math"
	"testing"
)

const boolGen = "bool"

func TestBoolGenerator_Config(t *testing.T) {
	RunConfigTests(t, boolGen, []ConfigTestCase{
		// valid configs
		{Name: "default probability 0.5", Config: map[string]any{}, ExpectError: false},
		{Name: "probability 0.0", Config: map[string]any{"probability": 0.0}, ExpectError: false},
		{Name: "probability 0.5", Config: map[string]any{"probability": 0.5}, ExpectError: false},
		{Name: "probability 0.7", Config: map[string]any{"probability": 0.7}, ExpectError: false},
		{Name: "probability 1.0", Config: map[string]any{"probability": 1.0}, ExpectError: false},
		{Name: "probability 1 as int", Config: map[string]any{"probability": 1}, ExpectError: false},
		{Name: "probability 0 as int", Config: map[string]any{"probability": 0}, ExpectError: false},

		// invalid configs
		{Name: "invalid negative probability", Config: map[string]any{"probability": -0.1}, ExpectError: true},
		{Name: "invalid probability > 1", Config: map[string]any{"probability": 1.5}, ExpectError: true},
		{Name: "invalid type string", Config: map[string]any{"probability": "0.7"}, ExpectError: true},
	})
}

func TestBoolGenerator_Determinism(t *testing.T) {
	RunDeterminismTests(t, boolGen, []DeterminismTestCase{
		{Name: "default probability", Config: map[string]any{}},
		{Name: "probability 0.0", Config: map[string]any{"probability": 0.0}},
		{Name: "probability 0.5", Config: map[string]any{"probability": 0.5}},
		{Name: "probability 0.7", Config: map[string]any{"probability": 0.7}},
		{Name: "probability 1.0", Config: map[string]any{"probability": 1.0}},
	})
}

// DistributionTestCase defines a test case for probability distribution
type DistributionTestCase struct {
	Name        string
	Config      map[string]any
	Probability float64
}

func TestBoolGenerator_Distribution(t *testing.T) {
	tests := []DistributionTestCase{
		{Name: "default probability 0.5", Config: map[string]any{}, Probability: 0.5},
		{Name: "probability 0.0 always false", Config: map[string]any{"probability": 0.0}, Probability: 0.0},
		{Name: "probability 0.5", Config: map[string]any{"probability": 0.5}, Probability: 0.5},
		{Name: "probability 0.7", Config: map[string]any{"probability": 0.7}, Probability: 0.7},
		{Name: "probability 1.0 always true", Config: map[string]any{"probability": 1.0}, Probability: 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(boolGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			r := rng.New(rng.SeedFromInt64(testSeed))
			trueCount := 0

			for i := range distributionSamples {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}
				if val.(bool) {
					trueCount++
				}
			}

			actualProb := float64(trueCount) / distributionSamples

			// Special cases: 0.0 and 1.0 should be exact
			if tt.Probability == 0.0 {
				if trueCount != 0 {
					t.Errorf("probability 0.0 should always return false, got %d true out of %d", trueCount, distributionSamples)
				}
				return
			}
			if tt.Probability == 1.0 {
				if trueCount != distributionSamples {
					t.Errorf("probability 1.0 should always return true, got %d true out of %d", trueCount, distributionSamples)
				}
				return
			}

			// For other probabilities, check within tolerance
			if math.Abs(actualProb-tt.Probability) > distributionTolerance {
				t.Errorf("distribution mismatch: expected ~%.2f, got %.3f (true count: %d/%d)",
					tt.Probability, actualProb, trueCount, distributionSamples)
			}
		})
	}
}
