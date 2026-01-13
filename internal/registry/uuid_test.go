package registry

import (
	"apery/internal/rng"
	"testing"

	"github.com/google/uuid"
)

const uuidGen = "uuid"

var uuidEmptyConfig = map[string]any{}

func TestUUIDGenerator_Config(t *testing.T) {
	RunConfigTests(t, uuidGen, []ConfigTestCase{
		{Name: "empty config", Config: uuidEmptyConfig, ExpectError: false},
	})
}

func TestUUIDGenerator_Determinism(t *testing.T) {
	AssertDeterministic(t, uuidGen, uuidEmptyConfig)
}

func TestUUIDGenerator_Format(t *testing.T) {
	gen, err := Get(uuidGen, uuidEmptyConfig)
	if err != nil {
		t.Fatalf("failed to create uuid generator: %v", err)
	}

	r := rng.New(testSeed)

	for i := range testIterations {
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("generation error: %v", err)
		}

		uuidStr := val.(string)
		parsed, err := uuid.Parse(uuidStr)
		if err != nil {
			t.Errorf("invalid UUID at %d: %s, error: %v", i, uuidStr, err)
			continue
		}

		if parsed.Version() != 4 {
			t.Errorf("expected version 4, got %d for %s", parsed.Version(), uuidStr)
		}

		if parsed.Variant() != uuid.RFC4122 {
			t.Errorf("expected variant RFC4122, got %v for %s", parsed.Variant(), uuidStr)
		}
	}
}
