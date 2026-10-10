package registry

import (
	"math"
	"testing"
)

// JSON decodes numbers as float64 and YAML can produce int64/uint64, so
// integer config accepts any of them when the value is a whole number.
func TestExtractInt(t *testing.T) {
	valid := []struct {
		in   any
		want int64
	}{
		{5, 5},
		{int64(-7), -7},
		{uint64(9), 9},
		{float64(12), 12},
		{float64(-3), -3},
	}
	for _, tt := range valid {
		got, err := extractInt(tt.in, "n", "test")
		if err != nil || got != tt.want {
			t.Errorf("extractInt(%#v) = %d, %v; want %d", tt.in, got, err, tt.want)
		}
	}

	invalid := []any{5.5, "5", float64(1e19), uint64(math.MaxUint64), math.NaN(), math.Inf(1), nil}
	for _, in := range invalid {
		if _, err := extractInt(in, "n", "test"); err == nil {
			t.Errorf("extractInt(%#v): expected error", in)
		}
	}
}

func TestIntGenerator_AcceptsJSONNumbers(t *testing.T) {
	gen, err := Get("int", map[string]any{"min": 10.0, "max": 20.0})
	if err != nil {
		t.Fatalf("whole float64 bounds rejected: %v", err)
	}
	v, _ := gen.Next(newTestRng(1))
	if n := v.(int64); n < 10 || n > 20 {
		t.Fatalf("value %d out of range", n)
	}
}
