package registry

import (
	"apery/internal/rng"
	"strings"
	"testing"
	"time"
)

const timeGen = "time"

func TestTimeGenerator_Config(t *testing.T) {
	RunConfigTests(t, timeGen, []ConfigTestCase{
		// valid configs
		{Name: "default", Config: map[string]any{}, ExpectError: false},
		{Name: "start", Config: map[string]any{"start": "2026-01-01T00:00:00Z"}, ExpectError: false},
		{Name: "end", Config: map[string]any{"end": "2025-12-01T00:00:00Z"}, ExpectError: false},
		{Name: "start & end", Config: map[string]any{"start": "2024-12-01T00:00:00Z", "end": "2030-01-01T00:00:00Z"}, ExpectError: false},
		{Name: "format", Config: map[string]any{"format": "2006-01-02"}, ExpectError: false},
		{Name: "timezone", Config: map[string]any{"tz": "America/New_York"}, ExpectError: false},
		{Name: "all", Config: map[string]any{
			"start":  "2024-12-01",
			"end":    "2030-01-01",
			"format": "2006-01-02",
			"tz":     "America/New_York",
		}, ExpectError: false},

		// invalid configs
		{Name: "invalid timezone", Config: map[string]any{"tz": "Invalid/Timezone"}, ExpectError: true},
		{Name: "start after end", Config: map[string]any{
			"start": "2030-01-01T00:00:00Z",
			"end":   "2024-01-01T00:00:00Z",
		}, ExpectError: true},
		{Name: "start equals end", Config: map[string]any{
			"start": "2024-01-01T00:00:00Z",
			"end":   "2024-01-01T00:00:00Z",
		}, ExpectError: true},
		{Name: "invalid start format", Config: map[string]any{"start": "not-a-date"}, ExpectError: true},
		{Name: "invalid end format", Config: map[string]any{"end": "not-a-date"}, ExpectError: true},
		{Name: "start not string", Config: map[string]any{"start": 12345}, ExpectError: true},
		{Name: "end not string", Config: map[string]any{"end": 12345}, ExpectError: true},
	})
}

func TestTimeGenerator_Determinism(t *testing.T) {
	RunDeterminismTests(t, timeGen, []DeterminismTestCase{
		{Name: "default", Config: map[string]any{}},
		{Name: "with start", Config: map[string]any{"start": "2024-01-01T00:00:00Z"}},
		{Name: "with end", Config: map[string]any{"end": "2025-12-31T23:59:59Z"}},
		{Name: "with range", Config: map[string]any{
			"start": "2024-01-01T00:00:00Z",
			"end":   "2024-12-31T23:59:59Z",
		}},
		{Name: "with format", Config: map[string]any{"format": "2006-01-02"}},
		{Name: "with timezone", Config: map[string]any{"tz": "America/New_York"}},
	})
}

func TestTimeGenerator_Range(t *testing.T) {
	tests := []struct {
		Name   string
		Config map[string]any
		Start  string
		End    string
		Format string
	}{
		{
			Name:   "default range",
			Config: map[string]any{},
			Start:  "2020-01-01T00:00:00Z",
			End:    "2030-12-31T23:59:59Z",
			Format: time.RFC3339,
		},
		{
			Name: "custom range",
			Config: map[string]any{
				"start": "2024-01-01T00:00:00Z",
				"end":   "2024-12-31T23:59:59Z",
			},
			Start:  "2024-01-01T00:00:00Z",
			End:    "2024-12-31T23:59:59Z",
			Format: time.RFC3339,
		},
		{
			Name: "custom format and range",
			Config: map[string]any{
				"start":  "2024-06-01",
				"end":    "2024-06-30",
				"format": "2006-01-02",
			},
			Start:  "2024-06-01",
			End:    "2024-06-30",
			Format: "2006-01-02",
		},
		{
			Name: "narrow range (1 hour)",
			Config: map[string]any{
				"start": "2024-01-01T00:00:00Z",
				"end":   "2024-01-01T01:00:00Z",
			},
			Start:  "2024-01-01T00:00:00Z",
			End:    "2024-01-01T01:00:00Z",
			Format: time.RFC3339,
		},
		{
			Name: "very large range",
			Config: map[string]any{
				"start": "1970-01-01T00:00:00Z",
				"end":   "2099-12-31T23:59:59Z",
			},
			Start:  "1970-01-01T00:00:00Z",
			End:    "2099-12-31T23:59:59Z",
			Format: time.RFC3339,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(timeGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			start, err := time.Parse(tt.Format, tt.Start)
			if err != nil {
				t.Fatalf("failed to parse start time: %v", err)
			}

			end, err := time.Parse(tt.Format, tt.End)
			if err != nil {
				t.Fatalf("failed to parse end time: %v", err)
			}

			r := rng.New(testSeed)

			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}

				timeStr := val.(string)
				generated, err := time.Parse(tt.Format, timeStr)
				if err != nil {
					t.Fatalf("failed to parse generated time %q at %d: %v", timeStr, i, err)
				}

				if generated.Before(start) || generated.After(end) {
					t.Errorf("time %q out of range [%s, %s] at index %d",
						timeStr, tt.Start, tt.End, i)
				}
			}
		})
	}
}

func TestTimeGenerator_Format(t *testing.T) {
	tests := []struct {
		Name   string
		Config map[string]any
		Format string
	}{
		{
			Name:   "default RFC3339",
			Config: map[string]any{},
			Format: time.RFC3339,
		},
		{
			Name:   "date only",
			Config: map[string]any{"format": "2006-01-02"},
			Format: "2006-01-02",
		},
		{
			Name:   "time only",
			Config: map[string]any{"format": "15:04:05"},
			Format: "15:04:05",
		},
		{
			Name:   "custom format",
			Config: map[string]any{"format": "Jan 02, 2006 at 3:04 PM"},
			Format: "Jan 02, 2006 at 3:04 PM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(timeGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			r := rng.New(testSeed)

			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}

				timeStr := val.(string)
				// Verify it can be parsed with the expected format
				_, err = time.Parse(tt.Format, timeStr)
				if err != nil {
					t.Errorf("failed to parse generated time %q with format %q at %d: %v",
						timeStr, tt.Format, i, err)
				}
			}
		})
	}
}

func TestTimeGenerator_Timezone(t *testing.T) {
	tests := []struct {
		Name     string
		Config   map[string]any
		Timezone string
	}{
		{
			Name:     "default UTC",
			Config:   map[string]any{},
			Timezone: "UTC",
		},
		{
			Name:     "America/New_York",
			Config:   map[string]any{"tz": "America/New_York"},
			Timezone: "America/New_York",
		},
		{
			Name:     "Europe/London",
			Config:   map[string]any{"tz": "Europe/London"},
			Timezone: "Europe/London",
		},
		{
			Name:     "Asia/Tokyo",
			Config:   map[string]any{"tz": "Asia/Tokyo"},
			Timezone: "Asia/Tokyo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(timeGen, tt.Config)
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			loc, err := time.LoadLocation(tt.Timezone)
			if err != nil {
				t.Fatalf("failed to load timezone: %v", err)
			}

			r := rng.New(testSeed)

			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}

				timeStr := val.(string)
				generated, err := time.Parse(time.RFC3339, timeStr)
				if err != nil {
					t.Fatalf("failed to parse generated time %q at %d: %v", timeStr, i, err)
				}

				// Verify timezone by checking the offset matches what we expect
				// Parse the same timestamp in the expected timezone
				_, generatedOffset := generated.Zone()
				_, expectedOffset := generated.In(loc).Zone()

				if generatedOffset != expectedOffset {
					t.Errorf("timezone offset mismatch at %d: expected %d, got %d for time %q",
						i, expectedOffset, generatedOffset, timeStr)
				}

				// For UTC specifically, verify it uses Z notation
				if tt.Timezone == "UTC" {
					if !strings.HasSuffix(timeStr, "Z") {
						t.Errorf("expected UTC time to end with 'Z', got %q at index %d", timeStr, i)
					}
				}
			}
		})
	}
}

func TestTimeGenerator_EdgeCases(t *testing.T) {
	tests := []struct {
		Name        string
		Config      map[string]any
		ExpectError bool
		ErrorMsg    string
	}{
		{
			Name: "format mismatch - date-only start with RFC3339 format",
			Config: map[string]any{
				"start": "2024-01-01",
			},
			ExpectError: true,
			ErrorMsg:    "parsing time",
		},
		{
			Name: "format mismatch - RFC3339 start with date-only format",
			Config: map[string]any{
				"start":  "2024-01-01T00:00:00Z",
				"format": "2006-01-02",
			},
			ExpectError: true,
			ErrorMsg:    "parsing time",
		},
		{
			Name: "consistent format - date-only",
			Config: map[string]any{
				"start":  "2024-01-01",
				"end":    "2024-12-31",
				"format": "2006-01-02",
			},
			ExpectError: false,
		},
		{
			Name: "consistent format - RFC3339",
			Config: map[string]any{
				"start": "2024-01-01T00:00:00Z",
				"end":   "2024-12-31T23:59:59Z",
			},
			ExpectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(timeGen, tt.Config)

			if tt.ExpectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.ErrorMsg)
				} else if !strings.Contains(err.Error(), tt.ErrorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.ErrorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gen == nil {
				t.Fatal("expected generator, got nil")
			}

			// Verify it can generate values
			r := rng.New(testSeed)
			_, err = gen.Next(r)
			if err != nil {
				t.Fatalf("failed to generate value: %v", err)
			}
		})
	}
}
