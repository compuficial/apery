package registry

import (
	"apery/internal/rng"
	"math"
	"testing"
)

const normalIntGen = "normal_int"

func TestNormalIntGenerator_Config(t *testing.T) {
	RunConfigTests(t, normalIntGen, []ConfigTestCase{
		// valid configs
		{Name: "default", Config: map[string]any{}, ExpectError: false},
		{Name: "mu only", Config: map[string]any{"mu": 10.0}, ExpectError: false},
		{Name: "sigma only", Config: map[string]any{"sigma": 2.5}, ExpectError: false},
		{Name: "mu and sigma", Config: map[string]any{"mu": 100.0, "sigma": 15.0}, ExpectError: false},
		{Name: "mu as int", Config: map[string]any{"mu": 50}, ExpectError: false},
		{Name: "sigma as int", Config: map[string]any{"sigma": 10}, ExpectError: false},
		{Name: "negative mu", Config: map[string]any{"mu": -50.0}, ExpectError: false},
		{Name: "sigma zero", Config: map[string]any{"sigma": 0.0}, ExpectError: false},
		{Name: "clamp_min only", Config: map[string]any{"clamp_min": 0}, ExpectError: false},
		{Name: "clamp_max only", Config: map[string]any{"clamp_max": 100}, ExpectError: false},
		{Name: "both clamps", Config: map[string]any{"clamp_min": 0, "clamp_max": 100}, ExpectError: false},
		{Name: "clamps as float", Config: map[string]any{"clamp_min": 0.0, "clamp_max": 100.0}, ExpectError: false},
		{Name: "equal clamps", Config: map[string]any{"clamp_min": 50, "clamp_max": 50}, ExpectError: false},

		// invalid configs
		{Name: "negative sigma", Config: map[string]any{"sigma": -1.0}, ExpectError: true},
		{Name: "invalid mu type", Config: map[string]any{"mu": "10"}, ExpectError: true},
		{Name: "invalid sigma type", Config: map[string]any{"sigma": "2.5"}, ExpectError: true},
		{Name: "invalid clamp_min type", Config: map[string]any{"clamp_min": "0"}, ExpectError: true},
		{Name: "invalid clamp_max type", Config: map[string]any{"clamp_max": "100"}, ExpectError: true},
		{Name: "clamp_min > clamp_max", Config: map[string]any{"clamp_min": 100, "clamp_max": 0}, ExpectError: true},
	})
}

func TestNormalIntGenerator_Determinism(t *testing.T) {
	RunDeterminismTests(t, normalIntGen, []DeterminismTestCase{
		{Name: "default", Config: map[string]any{}},
		{Name: "mu only", Config: map[string]any{"mu": 10.0}},
		{Name: "sigma only", Config: map[string]any{"sigma": 2.5}},
		{Name: "mu and sigma", Config: map[string]any{"mu": 100.0, "sigma": 15.0}},
		{Name: "sigma zero", Config: map[string]any{"sigma": 0.0}},
		{Name: "with clamps", Config: map[string]any{"mu": 50.0, "sigma": 20.0, "clamp_min": 0, "clamp_max": 100}},
	})
}

func TestNormalIntGenerator_Distribution(t *testing.T) {
	tests := []struct {
		Name      string
		Config    map[string]any
		ExpMu     float64
		ExpSigma  float64
		Tolerance float64
	}{
		{Name: "standard normal", Config: map[string]any{}, ExpMu: 0.0, ExpSigma: 1.0, Tolerance: 0.1},
		{Name: "mu=100 sigma=15", Config: map[string]any{"mu": 100.0, "sigma": 15.0}, ExpMu: 100.0, ExpSigma: 15.0, Tolerance: 0.05},
		{Name: "negative mu", Config: map[string]any{"mu": -50.0, "sigma": 5.0}, ExpMu: -50.0, ExpSigma: 5.0, Tolerance: 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(normalIntGen, tt.Config)
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
				samples[i] = float64(val.(int64))
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

			// Verify mean is within tolerance (rounding adds some noise, so we use a slightly larger tolerance)
			if math.Abs(mean-tt.ExpMu) > tt.ExpSigma*tt.Tolerance+tt.Tolerance+0.5 {
				t.Errorf("mean %f differs from expected %f by more than tolerance", mean, tt.ExpMu)
			}

			// Verify stddev is within tolerance
			if math.Abs(stddev-tt.ExpSigma) > tt.ExpSigma*tt.Tolerance+tt.Tolerance+0.5 {
				t.Errorf("stddev %f differs from expected %f by more than tolerance", stddev, tt.ExpSigma)
			}
		})
	}
}

func TestNormalIntGenerator_SigmaZero(t *testing.T) {
	// When sigma is 0, all values should equal round(mu)
	gen, err := Get(normalIntGen, map[string]any{"mu": 42.7, "sigma": 0.0})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))

	for i := range testIterations {
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("generation error at %d: %v", i, err)
		}
		if val.(int64) != 43 {
			t.Errorf("expected 43 when sigma=0 and mu=42.7, got %d", val.(int64))
		}
	}
}

func TestNormalIntGenerator_Clamping(t *testing.T) {
	tests := []struct {
		Name     string
		Config   map[string]any
		ClampMin int64
		ClampMax int64
	}{
		{
			Name:     "clamp both bounds",
			Config:   map[string]any{"mu": 50.0, "sigma": 100.0, "clamp_min": 0, "clamp_max": 100},
			ClampMin: 0,
			ClampMax: 100,
		},
		{
			Name:     "clamp min only",
			Config:   map[string]any{"mu": 0.0, "sigma": 50.0, "clamp_min": 0},
			ClampMin: 0,
			ClampMax: math.MaxInt64,
		},
		{
			Name:     "clamp max only",
			Config:   map[string]any{"mu": 100.0, "sigma": 50.0, "clamp_max": 100},
			ClampMin: math.MinInt64,
			ClampMax: 100,
		},
		{
			Name:     "tight clamp",
			Config:   map[string]any{"mu": 0.0, "sigma": 100.0, "clamp_min": 50, "clamp_max": 50},
			ClampMin: 50,
			ClampMax: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(normalIntGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			r := rng.New(rng.SeedFromInt64(testSeed))

			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}
				v := val.(int64)
				if v < tt.ClampMin || v > tt.ClampMax {
					t.Errorf("value %d outside clamp bounds [%d, %d]", v, tt.ClampMin, tt.ClampMax)
				}
			}
		})
	}
}

func TestNormalIntGenerator_OutputType(t *testing.T) {
	gen, err := Get(normalIntGen, map[string]any{"mu": 100.0, "sigma": 15.0})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("generation error: %v", err)
	}

	if _, ok := val.(int64); !ok {
		t.Errorf("expected int64, got %T", val)
	}
}
