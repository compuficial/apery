package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"testing"
)

const dateOffsetGen = "date_offset"

func TestDateOffsetGenerator_Config(t *testing.T) {
	RunConfigTests(t, dateOffsetGen, []ConfigTestCase{
		{Name: "field base and amount", Config: map[string]any{
			"base": "{start}", "amount": "{i}", "unit": "months", "format": "2006-01-02",
		}, ExpectError: false},
		{Name: "literal base, number amount", Config: map[string]any{
			"base": "2024-01-01", "amount": "1", "unit": "months", "format": "2006-01-02",
		}, ExpectError: false},
		{Name: "default format rfc3339", Config: map[string]any{
			"base": "2024-01-01T00:00:00Z", "amount": "3", "unit": "days",
		}, ExpectError: false},
		{Name: "negative amount", Config: map[string]any{
			"base": "{d}", "amount": "-2", "unit": "days",
		}, ExpectError: false},
		{Name: "arithmetic amount", Config: map[string]any{
			"base": "{d}", "amount": "{n} * 2", "unit": "days",
		}, ExpectError: false},
		{Name: "bare int amount (convenience)", Config: map[string]any{
			"base": "{d}", "amount": 1, "unit": "days",
		}, ExpectError: false},
		{Name: "bare float amount (convenience, e.g. JSON)", Config: map[string]any{
			"base": "{d}", "amount": 2.0, "unit": "days",
		}, ExpectError: false},
		{Name: "missing base", Config: map[string]any{"amount": "1", "unit": "days"}, ExpectError: true},
		{Name: "missing amount", Config: map[string]any{"base": "{d}", "unit": "days"}, ExpectError: true},
		{Name: "missing unit", Config: map[string]any{"base": "{d}", "amount": "1"}, ExpectError: true},
		{Name: "invalid unit", Config: map[string]any{"base": "{d}", "amount": "1", "unit": "fortnights"}, ExpectError: true},
		{Name: "base not string", Config: map[string]any{"base": 123, "amount": "1", "unit": "days"}, ExpectError: true},
		{Name: "amount wrong type", Config: map[string]any{"base": "{d}", "amount": true, "unit": "days"}, ExpectError: true},
		{Name: "amount bad expr", Config: map[string]any{"base": "{d}", "amount": "{x", "unit": "days"}, ExpectError: true},
		{Name: "literal base bad parse", Config: map[string]any{
			"base": "not-a-date", "amount": "1", "unit": "days", "format": "2006-01-02",
		}, ExpectError: true},
		{Name: "empty format", Config: map[string]any{"base": "{d}", "amount": "1", "unit": "days", "format": ""}, ExpectError: true},
	})
}

func offsetVal(t *testing.T, config map[string]any, row map[string]any) string {
	t.Helper()
	gen, err := Get(dateOffsetGen, config)
	if err != nil {
		t.Fatalf("compile %v: %v", config, err)
	}
	ra := gen.(RowAwareGenerator)
	val, err := ra.NextWithRow(rng.New(rng.SeedFromInt64(testSeed)), &testRowContext{data: row})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	return s
}

func TestDateOffsetGenerator_Output(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		row    map[string]any
		want   string
	}{
		{
			name:   "months from field index 0",
			config: map[string]any{"base": "{start}", "amount": "{i}", "unit": "months", "format": "2006-01-02"},
			row:    map[string]any{"start": "2024-01-01", "i": int64(0)},
			want:   "2024-01-01",
		},
		{
			name:   "months from field index 1",
			config: map[string]any{"base": "{start}", "amount": "{i}", "unit": "months", "format": "2006-01-02"},
			row:    map[string]any{"start": "2024-01-01", "i": int64(1)},
			want:   "2024-02-01",
		},
		{
			name:   "months from field index 11",
			config: map[string]any{"base": "{start}", "amount": "{i}", "unit": "months", "format": "2006-01-02"},
			row:    map[string]any{"start": "2024-01-01", "i": int64(11)},
			want:   "2024-12-01",
		},
		{
			name:   "one month later number amount rfc3339",
			config: map[string]any{"base": "{charged_at}", "amount": "1", "unit": "months"},
			row:    map[string]any{"charged_at": "2024-01-15T10:30:00Z"},
			want:   "2024-02-15T10:30:00Z",
		},
		{
			name:   "bare int amount convenience",
			config: map[string]any{"base": "2024-01-01", "amount": 1, "unit": "months", "format": "2006-01-02"},
			row:    map[string]any{},
			want:   "2024-02-01",
		},
		{
			name:   "literal base days",
			config: map[string]any{"base": "2024-03-01", "amount": "{n}", "unit": "days", "format": "2006-01-02"},
			row:    map[string]any{"n": int64(10)},
			want:   "2024-03-11",
		},
		{
			name:   "arithmetic amount",
			config: map[string]any{"base": "2024-01-01", "amount": "{q} * 3", "unit": "months", "format": "2006-01-02"},
			row:    map[string]any{"q": int64(2)}, // quarter 2 -> month 6
			want:   "2024-07-01",
		},
		{
			name:   "negative offset",
			config: map[string]any{"base": "{d}", "amount": "-2", "unit": "days", "format": "2006-01-02"},
			row:    map[string]any{"d": "2024-03-01"},
			want:   "2024-02-28",
		},
		{
			name:   "hours",
			config: map[string]any{"base": "{t}", "amount": "6", "unit": "hours"},
			row:    map[string]any{"t": "2024-01-01T00:00:00Z"},
			want:   "2024-01-01T06:00:00Z",
		},
		{
			name:   "years",
			config: map[string]any{"base": "2020-02-29", "amount": "1", "unit": "years", "format": "2006-01-02"},
			row:    map[string]any{},
			want:   "2021-03-01", // AddDate normalizes Feb 29 + 1y
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := offsetVal(t, tt.config, tt.row)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDateOffsetGenerator_RuntimeErrors(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		row    map[string]any
	}{
		{"base field missing", map[string]any{"base": "{d}", "amount": "1", "unit": "days"}, map[string]any{}},
		{"base field not string", map[string]any{"base": "{d}", "amount": "1", "unit": "days"}, map[string]any{"d": int64(5)}},
		{"amount field missing", map[string]any{"base": "2024-01-01", "amount": "{n}", "unit": "days", "format": "2006-01-02"}, map[string]any{}},
		{"amount field non-numeric", map[string]any{"base": "2024-01-01", "amount": "{n}", "unit": "days", "format": "2006-01-02"}, map[string]any{"n": "ten"}},
		{"amount evaluates fractional", map[string]any{"base": "2024-01-01", "amount": "{n}", "unit": "days", "format": "2006-01-02"}, map[string]any{"n": 1.5}},
		{"constant fractional amount", map[string]any{"base": "2024-01-01", "amount": 1.5, "unit": "days", "format": "2006-01-02"}, map[string]any{}},
		{"base unparseable at runtime", map[string]any{"base": "{d}", "amount": "1", "unit": "days", "format": "2006-01-02"}, map[string]any{"d": "15/01/2024"}},
		// A huge clock-unit amount would overflow time.Duration; must error, not wrap.
		{"hours overflow", map[string]any{"base": "2024-01-01T00:00:00Z", "amount": "{n}", "unit": "hours"}, map[string]any{"n": int64(3_000_000)}},
		{"seconds overflow", map[string]any{"base": "2024-01-01T00:00:00Z", "amount": "{n}", "unit": "seconds"}, map[string]any{"n": int64(9_300_000_000)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := Get(dateOffsetGen, tt.config)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			ra := gen.(RowAwareGenerator)
			_, err = ra.NextWithRow(rng.New(rng.SeedFromInt64(testSeed)), &testRowContext{data: tt.row})
			if err == nil {
				t.Errorf("expected runtime error, got none")
			}
		})
	}
}

func TestDateOffsetGenerator_Dependencies(t *testing.T) {
	gen, err := Get(dateOffsetGen, map[string]any{
		"base": "{start}", "amount": "{i} + {bump}", "unit": "months", "format": "2006-01-02",
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	deps := gen.(DependencyDeclarer).Dependencies()
	want := map[string]bool{"start": true, "i": true, "bump": true}
	if len(deps) != 3 {
		t.Fatalf("deps = %v, want start, i, bump", deps)
	}
	for _, d := range deps {
		if !want[d] {
			t.Errorf("unexpected dep %q in %v", d, deps)
		}
	}
}

func TestDateOffsetGenerator_NextWithoutRowContext(t *testing.T) {
	gen, err := Get(dateOffsetGen, map[string]any{"base": "{d}", "amount": "1", "unit": "days"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := gen.Next(rng.New(rng.SeedFromInt64(testSeed))); err == nil {
		t.Fatal("expected error when calling Next without row context")
	}
}

func TestDateOffsetGenerator_Determinism(t *testing.T) {
	config := map[string]any{"base": "{start}", "amount": "{i}", "unit": "months", "format": "2006-01-02"}
	gen1, _ := Get(dateOffsetGen, config)
	gen2, _ := Get(dateOffsetGen, config)
	ra1 := gen1.(RowAwareGenerator)
	ra2 := gen2.(RowAwareGenerator)

	for i := range testIterations {
		row := &testRowContext{data: map[string]any{"start": "2024-01-01", "i": int64(i % 12)}}
		v1, _ := ra1.NextWithRow(rng.New(rng.SeedFromInt64(int64(i))), row)
		v2, _ := ra2.NextWithRow(rng.New(rng.SeedFromInt64(int64(i))), row)
		if v1 != v2 {
			t.Errorf("determinism failed at %d: %v != %v", i, v1, v2)
		}
	}
}
