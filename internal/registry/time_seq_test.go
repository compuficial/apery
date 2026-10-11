package registry

import (
	"math"
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
		{Name: "fixed gaps", Config: valid(map[string]any{"gaps": "fixed", "jitter": "10s"})},
		{Name: "exponential gaps", Config: valid(map[string]any{"gaps": "exponential"})},
		{Name: "exponential gaps, zero jitter", Config: valid(map[string]any{"gaps": "exponential", "jitter": "0s"})},
		{Name: "exponential gaps with jitter", Config: valid(map[string]any{"gaps": "exponential", "jitter": "10s"}), ExpectError: true},
		{Name: "unknown gaps", Config: valid(map[string]any{"gaps": "poisson"}), ExpectError: true},
		{Name: "gaps not a string", Config: valid(map[string]any{"gaps": true}), ExpectError: true},
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

// time_seq tracks its row, or with exponential gaps builds on the row
// before, so composites must not nest it.
func TestTimeSeqGenerator_NotNestable(t *testing.T) {
	for _, gaps := range []string{"fixed", "exponential"} {
		_, err := Get("object", map[string]any{"fields": map[string]any{
			"at": map[string]any{"gen": "time_seq", "config": map[string]any{"start": "2024-01-01", "step": "1s", "gaps": gaps}},
		}})
		if err == nil || !strings.Contains(err.Error(), "per-row state") {
			t.Errorf("gaps %s: error = %v, want time_seq rejected inside object", gaps, err)
		}
	}
}

// drawArrivals returns the first n rows of a time_seq with exponential gaps,
// generated in order on one instance, each row from its own Rng as the
// executor does.
func drawArrivals(t *testing.T, config map[string]any, n int) ([]time.Time, error) {
	t.Helper()
	config["gaps"] = "exponential"
	config["format"] = time.RFC3339Nano
	gen, err := Get("time_seq", config)
	if err != nil {
		t.Fatal(err)
	}

	var rows []time.Time
	for i := range int64(n) {
		v, err := gen.Next(rng.New(rng.DeriveIndex(rng.SeedFromInt64(testSeed), i)))
		if err != nil {
			return rows, err
		}
		ts, err := time.Parse(time.RFC3339Nano, v.(string))
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, ts)
	}
	return rows, nil
}

// Row 0 is start and each later row adds a random gap with the exponential
// distribution's shape: mean step, standard deviation step, and e^-1 of gaps
// longer than step.
func TestTimeSeqGenerator_ExponentialGaps(t *testing.T) {
	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	const step, n = time.Minute, 200_000
	rows, err := drawArrivals(t, map[string]any{"start": start.Format(time.RFC3339), "step": step.String()}, n)
	if err != nil {
		t.Fatal(err)
	}
	if !rows[0].Equal(start) {
		t.Errorf("row 0 = %v, want start %v", rows[0], start)
	}

	var sum, sumSq float64
	longer := 0
	for i := 1; i < n; i++ {
		gap := rows[i].Sub(rows[i-1])
		if gap < 0 {
			t.Fatalf("row %d = %v, before row %d = %v", i, rows[i], i-1, rows[i-1])
		}
		if gap > step {
			longer++
		}
		g := gap.Seconds()
		sum += g
		sumSq += g * g
	}
	mean := sum / (n - 1)
	sd := math.Sqrt(sumSq/(n-1) - mean*mean)
	if math.Abs(mean-step.Seconds()) > 0.01*step.Seconds() || math.Abs(sd-step.Seconds()) > 0.02*step.Seconds() {
		t.Errorf("gap mean %.2fs, stddev %.2fs; want both about %v", mean, sd, step)
	}
	if frac := float64(longer) / (n - 1); math.Abs(frac-math.Exp(-1)) > 0.005 {
		t.Errorf("%.4f of gaps longer than step, want e^-1 = %.4f", frac, math.Exp(-1))
	}
}

func TestTimeSeqGenerator_ExponentialDeterminism(t *testing.T) {
	RunDeterminismTests(t, "time_seq", []DeterminismTestCase{
		{Name: "exponential gaps", Config: map[string]any{"start": "2024-01-01", "step": "90s", "gaps": "exponential"}},
	})
}

// Running past the representable time is an error, never a wrapped,
// earlier timestamp, even when one gap alone is too long.
func TestTimeSeqGenerator_ExponentialOverflow(t *testing.T) {
	for _, step := range []string{"100000h", "2562047h"} { // about 11 and 292 years
		rows, err := drawArrivals(t, map[string]any{"start": "2024-01-01", "step": step}, 1000)
		if err == nil || !strings.Contains(err.Error(), "292 years") {
			t.Errorf("step %s: error = %v after %d rows, want the 292-year limit", step, err, len(rows))
		}
		for i := 1; i < len(rows); i++ {
			if rows[i].Before(rows[i-1]) {
				t.Fatalf("step %s: row %d = %v, before row %d = %v", step, i, rows[i], i-1, rows[i-1])
			}
		}
	}
}
