package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"math"
	"testing"
)

const normalFloatGen = "normal_float"

func TestNormalFloatGenerator_Config(t *testing.T) {
	RunConfigTests(t, normalFloatGen, []ConfigTestCase{
		// valid configs
		{Name: "default", Config: map[string]any{}, ExpectError: false},
		{Name: "mu only", Config: map[string]any{"mu": 10.0}, ExpectError: false},
		{Name: "sigma only", Config: map[string]any{"sigma": 2.5}, ExpectError: false},
		{Name: "mu and sigma", Config: map[string]any{"mu": 100.0, "sigma": 15.0}, ExpectError: false},
		{Name: "mu as int", Config: map[string]any{"mu": 50}, ExpectError: false},
		{Name: "sigma as int", Config: map[string]any{"sigma": 10}, ExpectError: false},
		{Name: "negative mu", Config: map[string]any{"mu": -50.0}, ExpectError: false},
		{Name: "sigma zero", Config: map[string]any{"sigma": 0.0}, ExpectError: false},
		{Name: "clamp_min only", Config: map[string]any{"clamp_min": 0.0}, ExpectError: false},
		{Name: "clamp_max only", Config: map[string]any{"clamp_max": 1.0}, ExpectError: false},
		{Name: "both clamps", Config: map[string]any{"clamp_min": 0.0, "clamp_max": 1.0}, ExpectError: false},
		{Name: "clamps as int", Config: map[string]any{"clamp_min": 0, "clamp_max": 100}, ExpectError: false},
		{Name: "equal clamps", Config: map[string]any{"clamp_min": 0.5, "clamp_max": 0.5}, ExpectError: false},

		// invalid configs
		{Name: "negative sigma", Config: map[string]any{"sigma": -1.0}, ExpectError: true},
		{Name: "invalid mu type", Config: map[string]any{"mu": "10"}, ExpectError: true},
		{Name: "invalid sigma type", Config: map[string]any{"sigma": "2.5"}, ExpectError: true},
		{Name: "invalid clamp_min type", Config: map[string]any{"clamp_min": "0"}, ExpectError: true},
		{Name: "invalid clamp_max type", Config: map[string]any{"clamp_max": "1"}, ExpectError: true},
		{Name: "clamp_min > clamp_max", Config: map[string]any{"clamp_min": 1.0, "clamp_max": 0.0}, ExpectError: true},
	})
}

func TestNormalFloatGenerator_Determinism(t *testing.T) {
	RunDeterminismTests(t, normalFloatGen, []DeterminismTestCase{
		{Name: "default", Config: map[string]any{}},
		{Name: "mu only", Config: map[string]any{"mu": 10.0}},
		{Name: "sigma only", Config: map[string]any{"sigma": 2.5}},
		{Name: "mu and sigma", Config: map[string]any{"mu": 100.0, "sigma": 15.0}},
		{Name: "sigma zero", Config: map[string]any{"sigma": 0.0}},
		{Name: "with clamps", Config: map[string]any{"mu": 0.5, "sigma": 0.3, "clamp_min": 0.0, "clamp_max": 1.0}},
	})
}

func TestNormalFloatGenerator_Distribution(t *testing.T) {
	tests := []struct {
		Name      string
		Config    map[string]any
		ExpMu     float64
		ExpSigma  float64
		Tolerance float64
	}{
		{Name: "standard normal", Config: map[string]any{}, ExpMu: 0.0, ExpSigma: 1.0, Tolerance: 0.05},
		{Name: "mu=100 sigma=15", Config: map[string]any{"mu": 100.0, "sigma": 15.0}, ExpMu: 100.0, ExpSigma: 15.0, Tolerance: 0.05},
		{Name: "negative mu", Config: map[string]any{"mu": -50.0, "sigma": 5.0}, ExpMu: -50.0, ExpSigma: 5.0, Tolerance: 0.05},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(normalFloatGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			r := rng.New(rng.SeedFromInt64(testSeed))
			samples := make([]float64, distributionSamples)

			for i := range distributionSamples {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}
				samples[i] = val.(float64)
			}

			// Calculate sample mean
			sum := 0.0
			for _, v := range samples {
				sum += v
			}
			mean := sum / float64(len(samples))

			// Calculate sample standard deviation
			sumSquares := 0.0
			for _, v := range samples {
				diff := v - mean
				sumSquares += diff * diff
			}
			stddev := math.Sqrt(sumSquares / float64(len(samples)))

			// Verify mean is within tolerance
			if math.Abs(mean-tt.ExpMu) > tt.ExpSigma*tt.Tolerance+tt.Tolerance {
				t.Errorf("mean %f differs from expected %f by more than tolerance", mean, tt.ExpMu)
			}

			// Verify stddev is within tolerance
			if math.Abs(stddev-tt.ExpSigma) > tt.ExpSigma*tt.Tolerance+tt.Tolerance {
				t.Errorf("stddev %f differs from expected %f by more than tolerance", stddev, tt.ExpSigma)
			}
		})
	}
}

func TestNormalFloatGenerator_SigmaZero(t *testing.T) {
	// When sigma is 0, all values should equal mu
	gen, err := Get(normalFloatGen, map[string]any{"mu": 42.0, "sigma": 0.0})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))

	for i := range testIterations {
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("generation error at %d: %v", i, err)
		}
		if val.(float64) != 42.0 {
			t.Errorf("expected 42.0 when sigma=0, got %f", val.(float64))
		}
	}
}

func TestNormalFloatGenerator_Clamping(t *testing.T) {
	tests := []struct {
		Name     string
		Config   map[string]any
		ClampMin float64
		ClampMax float64
	}{
		{
			Name:     "clamp both bounds",
			Config:   map[string]any{"mu": 0.5, "sigma": 1.0, "clamp_min": 0.0, "clamp_max": 1.0},
			ClampMin: 0.0,
			ClampMax: 1.0,
		},
		{
			Name:     "clamp min only",
			Config:   map[string]any{"mu": 0.0, "sigma": 1.0, "clamp_min": 0.0},
			ClampMin: 0.0,
			ClampMax: math.MaxFloat64,
		},
		{
			Name:     "clamp max only",
			Config:   map[string]any{"mu": 100.0, "sigma": 50.0, "clamp_max": 100.0},
			ClampMin: -math.MaxFloat64,
			ClampMax: 100.0,
		},
		{
			Name:     "tight clamp",
			Config:   map[string]any{"mu": 0.0, "sigma": 100.0, "clamp_min": 0.5, "clamp_max": 0.5},
			ClampMin: 0.5,
			ClampMax: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(normalFloatGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			r := rng.New(rng.SeedFromInt64(testSeed))

			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}
				v := val.(float64)
				if v < tt.ClampMin || v > tt.ClampMax {
					t.Errorf("value %f outside clamp bounds [%f, %f]", v, tt.ClampMin, tt.ClampMax)
				}
			}
		})
	}
}

func TestNormalFloatGenerator_OutputType(t *testing.T) {
	gen, err := Get(normalFloatGen, map[string]any{"mu": 100.0, "sigma": 15.0})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("generation error: %v", err)
	}

	if _, ok := val.(float64); !ok {
		t.Errorf("expected float64, got %T", val)
	}
}
