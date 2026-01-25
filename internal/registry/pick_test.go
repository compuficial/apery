package registry

import (
	"apery/internal/rng"
	"path/filepath"
	"testing"
)

const pickGen = "pick"

func TestPickGenerator_Config(t *testing.T) {
	testdataDir := filepath.Join("testdata")

	RunConfigTests(t, pickGen, []ConfigTestCase{
		// valid configs - inline values
		{Name: "two strings", Config: map[string]any{"values": []any{"A", "B"}}, ExpectError: false},
		{Name: "four strings", Config: map[string]any{"values": []any{"A", "B", "C", "D"}}, ExpectError: false},
		{Name: "single value", Config: map[string]any{"values": []any{"A"}}, ExpectError: false},
		{Name: "integers", Config: map[string]any{"values": []any{1, 2, 3, 4}}, ExpectError: false},
		{Name: "mixed types", Config: map[string]any{"values": []any{"A", 1, 5.5, true}}, ExpectError: false},

		// valid configs - file
		{Name: "valid file", Config: map[string]any{"file": filepath.Join(testdataDir, "pick_values.txt")}, ExpectError: false},
		{Name: "file with whitespace", Config: map[string]any{"file": filepath.Join(testdataDir, "pick_whitespace.txt")}, ExpectError: false},

		// invalid configs - missing source
		{Name: "missing values and file", Config: map[string]any{}, ExpectError: true},

		// invalid configs - inline values
		{Name: "empty array", Config: map[string]any{"values": []any{}}, ExpectError: true},
		{Name: "values not array", Config: map[string]any{"values": "not-array"}, ExpectError: true},

		// invalid configs - file
		{Name: "file not found", Config: map[string]any{"file": "/nonexistent/path.txt"}, ExpectError: true},
		{Name: "empty file", Config: map[string]any{"file": filepath.Join(testdataDir, "pick_empty.txt")}, ExpectError: true},
		{Name: "file not a string", Config: map[string]any{"file": 123}, ExpectError: true},

		// invalid configs - both specified
		{Name: "both values and file", Config: map[string]any{"values": []any{"a"}, "file": "path.txt"}, ExpectError: true},
	})
}

func TestPickGenerator_Determinism(t *testing.T) {
	testdataDir := filepath.Join("testdata")

	RunDeterminismTests(t, pickGen, []DeterminismTestCase{
		{Name: "two values", Config: map[string]any{"values": []any{"a", "b"}}},
		{Name: "five values", Config: map[string]any{"values": []any{1, 2, 3, 4, 5}}},
		{Name: "file source", Config: map[string]any{"file": filepath.Join(testdataDir, "pick_values.txt")}},
	})
}

func TestPickGenerator_FileLoading(t *testing.T) {
	testdataDir := filepath.Join("testdata")

	t.Run("values loaded correctly", func(t *testing.T) {
		gen, err := Get(pickGen, map[string]any{"file": filepath.Join(testdataDir, "pick_values.txt")})
		if err != nil {
			t.Fatalf("failed to create generator: %v", err)
		}

		validSet := map[any]bool{"red": true, "green": true, "blue": true}
		r := rng.New(testSeed)

		for i := range testIterations {
			val, err := gen.Next(r)
			if err != nil {
				t.Fatalf("generation error at %d: %v", i, err)
			}
			if !validSet[val] {
				t.Errorf("got %v which is not in expected values at index %d", val, i)
			}
		}
	})

	t.Run("whitespace trimmed and empty lines skipped", func(t *testing.T) {
		gen, err := Get(pickGen, map[string]any{"file": filepath.Join(testdataDir, "pick_whitespace.txt")})
		if err != nil {
			t.Fatalf("failed to create generator: %v", err)
		}

		// Expected values after trimming whitespace and skipping empty lines
		validSet := map[any]bool{"apple": true, "banana": true, "cherry": true}
		r := rng.New(testSeed)

		for i := range testIterations {
			val, err := gen.Next(r)
			if err != nil {
				t.Fatalf("generation error at %d: %v", i, err)
			}
			if !validSet[val] {
				t.Errorf("got %q which is not in expected values at index %d (whitespace not trimmed?)", val, i)
			}
		}
	})
}

func TestPickGenerator_ValuesOnly(t *testing.T) {
	tests := []struct {
		Name   string
		Values []any
	}{
		{Name: "strings", Values: []any{"a", "b", "c"}},
		{Name: "integers", Values: []any{1, 2, 3, 4, 5}},
		{Name: "mixed", Values: []any{"x", 42, true}},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(pickGen, map[string]any{"values": tt.Values})
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			// Build set of valid values
			validSet := make(map[any]bool)
			for _, v := range tt.Values {
				validSet[v] = true
			}

			r := rng.New(testSeed)

			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}

				if !validSet[val] {
					t.Errorf("got %v which is not in values at index %d", val, i)
				}
			}
		})
	}
}
