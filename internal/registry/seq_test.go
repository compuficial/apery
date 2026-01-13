package registry

import (
	"testing"
)

const seqGen = "seq"

func TestSeqGenerator_Config(t *testing.T) {
	RunConfigTests(t, seqGen, []ConfigTestCase{
		// valid configs
		{Name: "default", Config: map[string]any{}, ExpectError: false},
		{Name: "custom start", Config: map[string]any{"start": 10}, ExpectError: false},
		{Name: "custom step", Config: map[string]any{"step": 5}, ExpectError: false},
		{Name: "custom start & step", Config: map[string]any{"start": 10, "step": 5}, ExpectError: false},

		// invalid configs
		{Name: "invalid start type (float)", Config: map[string]any{"start": 1.5}, ExpectError: true},
		{Name: "invalid step type (string)", Config: map[string]any{"step": "10"}, ExpectError: true},
		{Name: "step zero", Config: map[string]any{"step": 0}, ExpectError: true},
	})
}

// SequenceTestCase defines a test case for sequence validation
type SequenceTestCase struct {
	Name     string
	Config   map[string]any
	Expected []int
}

func TestSeqGenerator_Sequence(t *testing.T) {
	tests := []SequenceTestCase{
		{Name: "default 1-5", Config: map[string]any{}, Expected: []int{1, 2, 3, 4, 5}},
		{Name: "start at 10", Config: map[string]any{"start": 10}, Expected: []int{10, 11, 12, 13, 14}},
		{Name: "step by 5", Config: map[string]any{"step": 5}, Expected: []int{1, 6, 11, 16, 21}},
		{Name: "start 10, step 5", Config: map[string]any{"start": 10, "step": 5}, Expected: []int{10, 15, 20, 25, 30}},
		{Name: "start 10, step -1", Config: map[string]any{"start": 10, "step": -1}, Expected: []int{10, 9, 8, 7, 6}},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(seqGen, tt.Config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for i, want := range tt.Expected {
				got, err := gen.Next(nil)
				if err != nil {
					t.Fatalf("generation error: %v", err)
				}

				if got.(int) != want {
					t.Errorf("index %d: got %d, want %d", i, got, want)
				}
			}
		})
	}
}
