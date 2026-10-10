package registry

import (
	"strings"
	"testing"
	"time"

	"github.com/compuficial/apery/internal/rng"
)

func TestTimeSeqGenerator_Config(t *testing.T) {
	valid := func(extra map[string]any) map[string]any {
		config := map[string]any{"start": "2024-01-01T00:00:00Z", "step": "90s"}
		for k, v := range extra {
			config[k] = v
		}
		return config
	}
	RunConfigTests(t, "time_seq", []ConfigTestCase{
		{Name: "start and step", Config: valid(nil)},
		{Name: "date-only start", Config: valid(map[string]any{"start": "2024-01-01"})},
		{Name: "jitter", Config: valid(map[string]any{"jitter": "89s"})},
		{Name: "tz and format", Config: valid(map[string]any{"tz": "Europe/Paris", "format": time.RFC3339Nano})},
		{Name: "missing start", Config: map[string]any{"step": "1s"}, ExpectError: true},
		{Name: "missing step", Config: map[string]any{"start": "2024-01-01"}, ExpectError: true},
		{Name: "zero step", Config: valid(map[string]any{"step": "0s"}), ExpectError: true},
		{Name: "negative step", Config: valid(map[string]any{"step": "-1m"}), ExpectError: true},
		{Name: "step as number", Config: valid(map[string]any{"step": 90}), ExpectError: true},
		{Name: "day unit", Config: valid(map[string]any{"step": "1d"}), ExpectError: true},
		{Name: "jitter equal to step", Config: valid(map[string]any{"jitter": "90s"}), ExpectError: true},
		{Name: "negative jitter", Config: valid(map[string]any{"jitter": "-1s"}), ExpectError: true},
		{Name: "bad start", Config: valid(map[string]any{"start": "soon"}), ExpectError: true},
		{Name: "bad tz", Config: valid(map[string]any{"tz": "Mars/Base"}), ExpectError: true},
		{Name: "unknown key", Config: valid(map[string]any{"end": "2025-01-01"}), ExpectError: true},
	})
}

// drawTimes returns rows first..first+n-1 of a time_seq, each from its own
// row Rng as the executor does.
func drawTimes(t *testing.T, config map[string]any, first int64, n int) []string {
	t.Helper()
	gen, err := Get("time_seq", config)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.(Seekable).SeekRow(first); err != nil {
		t.Fatal(err)
	}
	out := make([]string, n)
	for i := range out {
		v, err := gen.Next(rng.New(rng.DeriveIndex(rng.SeedFromInt64(testSeed), first+int64(i))))
		if err != nil {
			t.Fatalf("row %d: %v", first+int64(i), err)
		}
		out[i] = v.(string)
	}
	return out
}

func TestTimeSeqGenerator_Steps(t *testing.T) {
	got := drawTimes(t, map[string]any{"start": "2024-01-01", "step": "90s"}, 0, 3)
	want := []string{"2024-01-01T00:00:00Z", "2024-01-01T00:01:30Z", "2024-01-01T00:03:00Z"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", got, want)
	}

	// Seeking gives the row's value directly, as a chunk start does.
	if got := drawTimes(t, map[string]any{"start": "2024-01-01", "step": "1h"}, 1000, 1)[0]; got != "2024-02-11T16:00:00Z" {
		t.Errorf("row 1000 = %s", got)
	}
}

// With jitter each row lands in [start + i*step, start + i*step + jitter),
// so timestamps strictly increase.
func TestTimeSeqGenerator_JitterKeepsOrder(t *testing.T) {
	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	const step, jitter = 10 * time.Second, 9*time.Second + 999*time.Millisecond
	rows := drawTimes(t, map[string]any{
		"start": start.Format(time.RFC3339), "step": step.String(), "jitter": jitter.String(), "format": time.RFC3339Nano,
	}, 0, 50_000)

	var prev time.Time
	moved := 0
	for i, s := range rows {
		ts, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		base := start.Add(time.Duration(i) * step)
		if ts.Before(base) || !ts.Before(base.Add(jitter)) {
			t.Fatalf("row %d = %s, want within [%s, +%v)", i, s, base, jitter)
		}
		if i > 0 && !ts.After(prev) {
			t.Fatalf("row %d = %s, not after %s", i, s, prev)
		}
		if !ts.Equal(base) {
			moved++
		}
		prev = ts
	}
	if moved < len(rows)/2 {
		t.Errorf("only %d of %d rows were jittered", moved, len(rows))
	}
}

// Steps are real elapsed time: across a DST change the wall clock jumps but
// the gap stays one hour.
func TestTimeSeqGenerator_TimeZone(t *testing.T) {
	got := drawTimes(t, map[string]any{"start": "2024-03-10T00:00:00", "step": "1h", "tz": "America/New_York", "format": "2006-01-02T15:04:05"}, 0, 4)
	want := []string{"2024-03-10T00:00:00", "2024-03-10T01:00:00", "2024-03-10T03:00:00", "2024-03-10T04:00:00"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTimeSeqGenerator_Overflow(t *testing.T) {
	gen, err := Get("time_seq", map[string]any{"start": "2024-01-01", "step": "24h"})
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.(Seekable).SeekRow(200_000); err != nil { // about 548 years in
		t.Fatal(err)
	}
	if _, err := gen.Next(rng.New(1)); err == nil || !strings.Contains(err.Error(), "292 years") {
		t.Errorf("error = %v, want the 292-year limit", err)
	}
}

// time_seq tracks its row, so composites must not nest it.
func TestTimeSeqGenerator_NotNestable(t *testing.T) {
	_, err := Get("object", map[string]any{"fields": map[string]any{
		"at": map[string]any{"gen": "time_seq", "config": map[string]any{"start": "2024-01-01", "step": "1s"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "per-row state") {
		t.Errorf("error = %v, want time_seq rejected inside object", err)
	}
}
