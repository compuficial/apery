package registry

import (
	"math"
	"testing"

	"github.com/compuficial/apery/internal/rng"
)

func TestLogNormalGenerator_Config(t *testing.T) {
	RunConfigTests(t, "lognormal", []ConfigTestCase{
		{Name: "defaults", Config: map[string]any{}},
		{Name: "mu and sigma", Config: map[string]any{"mu": 3, "sigma": 0.5}},
		{Name: "clamps", Config: map[string]any{"clamp_min": 1, "clamp_max": 100}},
		{Name: "negative sigma", Config: map[string]any{"sigma": -1}, ExpectError: true},
		{Name: "clamps reversed", Config: map[string]any{"clamp_min": 10, "clamp_max": 1}, ExpectError: true},
		{Name: "sigma not a number", Config: map[string]any{"sigma": "wide"}, ExpectError: true},
		{Name: "unknown key", Config: map[string]any{"median": 20}, ExpectError: true},
	})
}

func TestExponentialGenerator_Config(t *testing.T) {
	RunConfigTests(t, "exponential", []ConfigTestCase{
		{Name: "defaults", Config: map[string]any{}},
		{Name: "mean", Config: map[string]any{"mean": 300}},
		{Name: "clamp_max", Config: map[string]any{"mean": 300, "clamp_max": 3600}},
		{Name: "zero mean", Config: map[string]any{"mean": 0}, ExpectError: true},
		{Name: "negative mean", Config: map[string]any{"mean": -5}, ExpectError: true},
		{Name: "infinite mean", Config: map[string]any{"mean": math.Inf(1)}, ExpectError: true},
		{Name: "zero clamp_max", Config: map[string]any{"clamp_max": 0}, ExpectError: true},
		{Name: "rate is not a key", Config: map[string]any{"rate": 2}, ExpectError: true},
	})
}

func TestPoissonGenerator_Config(t *testing.T) {
	RunConfigTests(t, "poisson", []ConfigTestCase{
		{Name: "defaults", Config: map[string]any{}},
		{Name: "lambda", Config: map[string]any{"lambda": 4.5}},
		{Name: "integer lambda", Config: map[string]any{"lambda": 50}},
		{Name: "largest lambda", Config: map[string]any{"lambda": 1e15}},
		{Name: "zero lambda", Config: map[string]any{"lambda": 0}, ExpectError: true},
		{Name: "negative lambda", Config: map[string]any{"lambda": -1}, ExpectError: true},
		{Name: "lambda too large", Config: map[string]any{"lambda": 1e16}, ExpectError: true},
		{Name: "NaN lambda", Config: map[string]any{"lambda": math.NaN()}, ExpectError: true},
	})
}

func TestDistributionGenerators_Determinism(t *testing.T) {
	RunDeterminismTests(t, "lognormal", []DeterminismTestCase{
		{Name: "default", Config: map[string]any{}},
		{Name: "clamped", Config: map[string]any{"mu": 3, "sigma": 2, "clamp_max": 50}},
	})
	RunDeterminismTests(t, "exponential", []DeterminismTestCase{
		{Name: "default", Config: map[string]any{}},
		{Name: "clamped", Config: map[string]any{"mean": 10, "clamp_max": 5}},
	})
	RunDeterminismTests(t, "poisson", []DeterminismTestCase{
		{Name: "multiply", Config: map[string]any{"lambda": 2}},
		{Name: "ptrs", Config: map[string]any{"lambda": 200}},
	})
}

// Each generator emits its documented type, inside its range and clamps.
func TestDistributionGenerators_Output(t *testing.T) {
	cases := []struct {
		gen    string
		config map[string]any
		check  func(v any) bool
	}{
		{"lognormal", map[string]any{"mu": 3, "sigma": 2, "clamp_max": 50}, func(v any) bool {
			f, ok := v.(float64)
			return ok && f > 0 && f <= 50
		}},
		{"lognormal", map[string]any{"mu": 0, "sigma": 1, "clamp_min": 2}, func(v any) bool {
			f, ok := v.(float64)
			return ok && f >= 2
		}},
		{"exponential", map[string]any{"mean": 10, "clamp_max": 5}, func(v any) bool {
			f, ok := v.(float64)
			return ok && f >= 0 && f <= 5
		}},
		{"poisson", map[string]any{"lambda": 4.5}, func(v any) bool {
			k, ok := v.(int64)
			return ok && k >= 0
		}},
	}
	for _, c := range cases {
		gen, err := Get(c.gen, c.config)
		if err != nil {
			t.Fatalf("%s: %v", c.gen, err)
		}
		for i := range int64(5000) {
			v, err := gen.Next(rng.New(rng.DeriveIndex(rng.SeedFromInt64(testSeed), i)))
			if err != nil || !c.check(v) {
				t.Fatalf("%s %v: draw %d = %v (%T), err %v", c.gen, c.config, i, v, v, err)
			}
		}
	}
}
