package registry

import (
	"apery/internal/rng"
	"testing"

	"github.com/oklog/ulid/v2"
)

const ulidGen = "ulid"

var ulidEmptyConfig = map[string]any{}

func TestULIDGenerator_Config(t *testing.T) {
	RunConfigTests(t, ulidGen, []ConfigTestCase{
		{Name: "empty config", Config: ulidEmptyConfig, ExpectError: false},
	})
}

func TestULIDGenerator_Determinism(t *testing.T) {
	AssertDeterministic(t, ulidGen, ulidEmptyConfig)
}

func TestULIDGenerator_Format(t *testing.T) {
	gen, err := Get(ulidGen, ulidEmptyConfig)
	if err != nil {
		t.Fatalf("failed to create ulid generator: %v", err)
	}

	r := rng.New(testSeed)

	for i := range testIterations {
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("generation error: %v", err)
		}

		ulidStr := val.(string)

		// ULID should be 26 characters
		if len(ulidStr) != 26 {
			t.Errorf("expected 26 characters, got %d for %s", len(ulidStr), ulidStr)
		}

		// Should be parseable as a valid ULID
		parsed, err := ulid.Parse(ulidStr)
		if err != nil {
			t.Errorf("invalid ULID at %d: %s, error: %v", i, ulidStr, err)
			continue
		}

		// Verify it's not zero
		if parsed.Compare(ulid.ULID{}) == 0 {
			t.Errorf("ULID should not be zero: %s", ulidStr)
		}
	}
}

func TestULIDGenerator_Uniqueness(t *testing.T) {
	gen, err := Get(ulidGen, ulidEmptyConfig)
	if err != nil {
		t.Fatalf("failed to create ulid generator: %v", err)
	}

	r := rng.New(testSeed)
	seen := make(map[string]bool)

	for i := range testIterations {
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("generation error: %v", err)
		}

		ulidStr := val.(string)
		if seen[ulidStr] {
			t.Errorf("duplicate ULID at %d: %s", i, ulidStr)
		}
		seen[ulidStr] = true
	}
}
