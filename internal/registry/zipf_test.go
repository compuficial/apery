package registry

import (
	"apery/internal/rng"
	"testing"
)

const zipfGen = "zipf"

func TestZipfGenerator_Config(t *testing.T) {
	RunConfigTests(t, zipfGen, []ConfigTestCase{
		// valid configs
		{Name: "default", Config: map[string]any{}, ExpectError: false},
		{Name: "s only", Config: map[string]any{"s": 1.5}, ExpectError: false},
		{Name: "v only", Config: map[string]any{"v": 2.0}, ExpectError: false},
		{Name: "imax only", Config: map[string]any{"imax": 50}, ExpectError: false},
		{Name: "all params", Config: map[string]any{"s": 2.0, "v": 1.5, "imax": 200}, ExpectError: false},
		{Name: "s as int", Config: map[string]any{"s": 2}, ExpectError: false},
		{Name: "v as int", Config: map[string]any{"v": 1}, ExpectError: false},
		{Name: "s barely valid", Config: map[string]any{"s": 1.001}, ExpectError: false},
		{Name: "v exactly 1", Config: map[string]any{"v": 1.0}, ExpectError: false},
		{Name: "imax 1", Config: map[string]any{"imax": 1}, ExpectError: false},
		{Name: "large imax", Config: map[string]any{"imax": 1000000}, ExpectError: false},

		// invalid configs
		{Name: "s = 1", Config: map[string]any{"s": 1.0}, ExpectError: true},
		{Name: "s < 1", Config: map[string]any{"s": 0.5}, ExpectError: true},
		{Name: "s negative", Config: map[string]any{"s": -1.0}, ExpectError: true},
		{Name: "v < 1", Config: map[string]any{"v": 0.5}, ExpectError: true},
		{Name: "v zero", Config: map[string]any{"v": 0.0}, ExpectError: true},
		{Name: "v negative", Config: map[string]any{"v": -1.0}, ExpectError: true},
		{Name: "imax zero", Config: map[string]any{"imax": 0}, ExpectError: true},
		{Name: "imax negative", Config: map[string]any{"imax": -1}, ExpectError: true},
		{Name: "invalid s type", Config: map[string]any{"s": "1.5"}, ExpectError: true},
		{Name: "invalid v type", Config: map[string]any{"v": "1.0"}, ExpectError: true},
		{Name: "invalid imax type string", Config: map[string]any{"imax": "100"}, ExpectError: true},
		{Name: "invalid imax type float", Config: map[string]any{"imax": 100.0}, ExpectError: true},
	})
}

func TestZipfGenerator_Determinism(t *testing.T) {
	RunDeterminismTests(t, zipfGen, []DeterminismTestCase{
		{Name: "default", Config: map[string]any{}},
		{Name: "s=1.5", Config: map[string]any{"s": 1.5}},
		{Name: "v=2.0", Config: map[string]any{"v": 2.0}},
		{Name: "imax=50", Config: map[string]any{"imax": 50}},
		{Name: "all params", Config: map[string]any{"s": 2.0, "v": 1.5, "imax": 200}},
		{Name: "high skew", Config: map[string]any{"s": 3.0, "imax": 1000}},
	})
}

func TestZipfGenerator_Distribution(t *testing.T) {
	// Zipf distribution should produce lower values more frequently
	// Higher s = more concentration on low values
	tests := []struct {
		Name              string
		Config            map[string]any
		Imax              uint64
		ExpectLowHeavy    bool   // expect lower values to be more frequent
		LowThresholdRatio float64 // ratio of imax to consider "low"
	}{
		{
			Name:              "default skew",
			Config:            map[string]any{"s": 1.1, "imax": 100},
			Imax:              100,
			ExpectLowHeavy:    true,
			LowThresholdRatio: 0.1, // values <= 10 should be majority
		},
		{
			Name:              "high skew",
			Config:            map[string]any{"s": 2.0, "imax": 100},
			Imax:              100,
			ExpectLowHeavy:    true,
			LowThresholdRatio: 0.05, // values <= 5 should be majority with higher skew
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(zipfGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			r := rng.New(testSeed)
			lowCount := 0
			lowThreshold := int64(float64(tt.Imax) * tt.LowThresholdRatio)

			for i := range distributionSamples {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}
				v := val.(int64)
				if v <= lowThreshold {
					lowCount++
				}
			}

			lowRatio := float64(lowCount) / float64(distributionSamples)

			// For Zipf distribution, low values should appear much more frequently
			// than their proportion of the range
			if tt.ExpectLowHeavy && lowRatio < 0.3 {
				t.Errorf("expected low values to be frequent (got %.2f%% <= %d), Zipf distribution may be incorrect",
					lowRatio*100, lowThreshold)
			}
		})
	}
}

func TestZipfGenerator_Range(t *testing.T) {
	tests := []struct {
		Name   string
		Config map[string]any
		Imax   uint64
	}{
		{Name: "small range", Config: map[string]any{"imax": 10}, Imax: 10},
		{Name: "default range", Config: map[string]any{}, Imax: 100},
		{Name: "large range", Config: map[string]any{"imax": 1000}, Imax: 1000},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(zipfGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			r := rng.New(testSeed)

			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}
				v := val.(int64)
				if v < 0 || v > int64(tt.Imax) {
					t.Errorf("value %d outside range [0, %d]", v, tt.Imax)
				}
			}
		})
	}
}

func TestZipfGenerator_ImaxOne(t *testing.T) {
	// When imax = 1, all values should be 0 or 1
	gen, err := Get(zipfGen, map[string]any{"imax": 1})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(testSeed)

	for i := range testIterations {
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("generation error at %d: %v", i, err)
		}
		v := val.(int64)
		if v < 0 || v > 1 {
			t.Errorf("expected value in [0, 1] when imax=1, got %d", v)
		}
	}
}

func TestZipfGenerator_OutputType(t *testing.T) {
	gen, err := Get(zipfGen, map[string]any{"s": 1.5, "imax": 100})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(testSeed)
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("generation error: %v", err)
	}

	if _, ok := val.(int64); !ok {
		t.Errorf("expected int64, got %T", val)
	}
}

func TestZipfGenerator_SkewnessEffect(t *testing.T) {
	// Higher s should result in more concentration on value 0
	configs := []struct {
		S         float64
		MinZeros  int // minimum expected zeros out of distributionSamples
	}{
		{S: 1.1, MinZeros: 100},   // low skew: fewer zeros
		{S: 2.0, MinZeros: 500},   // medium skew: more zeros
		{S: 3.0, MinZeros: 1000},  // high skew: most zeros
	}

	for i := 0; i < len(configs)-1; i++ {
		low := configs[i]
		high := configs[i+1]

		genLow, _ := Get(zipfGen, map[string]any{"s": low.S, "imax": 100})
		genHigh, _ := Get(zipfGen, map[string]any{"s": high.S, "imax": 100})

		rLow := rng.New(testSeed)
		rHigh := rng.New(testSeed)

		zerosLow := 0
		zerosHigh := 0

		for j := range distributionSamples {
			valLow, _ := genLow.Next(rLow)
			valHigh, _ := genHigh.Next(rHigh)
			if valLow.(int64) == 0 {
				zerosLow++
			}
			if valHigh.(int64) == 0 {
				zerosHigh++
			}
			_ = j
		}

		if zerosHigh <= zerosLow {
			t.Errorf("s=%.1f produced %d zeros, but s=%.1f produced %d zeros (expected more with higher s)",
				high.S, zerosHigh, low.S, zerosLow)
		}
	}
}
