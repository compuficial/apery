package registry

import (
	"fmt"
	"math"
	"time"

	"github.com/compuficial/apery/internal/rng"
)

// TimeSeqGenerator generates increasing timestamps for event streams and
// logs: row i is start + i*step, plus up to jitter. With jitter below step
// every timestamp is later than the one before. The value depends only on
// the row index and the row's draw, so any chunking gives the same output.
type TimeSeqGenerator struct {
	start  time.Time
	step   time.Duration
	jitter time.Duration
	format string
	loc    *time.Location
	row    int64 // index of the next row
}

// Next returns the timestamp for the current row and advances to the next.
func (g *TimeSeqGenerator) Next(r *rng.Rng) (any, error) {
	row := g.row
	g.row++
	if row > (math.MaxInt64-int64(g.jitter))/int64(g.step) {
		return nil, fmt.Errorf("time_seq: row %d is more than about 292 years after 'start' at step %v", row, g.step)
	}

	offset := time.Duration(row) * g.step
	if g.jitter > 0 {
		offset += time.Duration(r.Float64() * float64(g.jitter))
	}
	return g.start.Add(offset).In(g.loc).Format(g.format), nil
}

// SeekRow positions the generator at a zero-based row index.
func (g *TimeSeqGenerator) SeekRow(row int64) error {
	if row < 0 {
		return fmt.Errorf("time_seq: row index must be >= 0")
	}
	g.row = row
	return nil
}

// parseDuration reads a duration key such as "90s" or "1h30m".
func parseDuration(config map[string]any, key string) (time.Duration, bool, error) {
	val, exists := config[key]
	if !exists {
		return 0, false, nil
	}
	s, ok := val.(string)
	if !ok {
		return 0, false, fmt.Errorf("time_seq: '%s' must be a duration string such as \"90s\" or \"24h\", got %T", key, val)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, false, fmt.Errorf("time_seq: invalid '%s': %w (units: ns, us, ms, s, m, h; a day is \"24h\")", key, err)
	}
	return d, true, nil
}

// init registers the time_seq generator.
func init() {
	MustRegister("time_seq", func(config map[string]any) (Generator, error) {
		format, loc, err := parseFormatAndZone(config, "time_seq")
		if err != nil {
			return nil, err
		}
		if _, ok := config["start"]; !ok {
			return nil, fmt.Errorf("time_seq: 'start' is required")
		}
		start, err := parseTimeFromConfig(config, "time_seq", "start", time.Time{}, format, loc)
		if err != nil {
			return nil, err
		}

		step, ok, err := parseDuration(config, "step")
		switch {
		case err != nil:
			return nil, err
		case !ok:
			return nil, fmt.Errorf("time_seq: 'step' is required")
		case step <= 0:
			return nil, fmt.Errorf("time_seq: 'step' must be positive, got %v", step)
		}
		jitter, _, err := parseDuration(config, "jitter")
		if err != nil {
			return nil, err
		}
		if jitter < 0 || jitter >= step {
			return nil, fmt.Errorf("time_seq: 'jitter' must be >= 0 and less than 'step' (%v), got %v", step, jitter)
		}

		return &TimeSeqGenerator{start: start, step: step, jitter: jitter, format: format, loc: loc}, nil
	})
	MustRegisterInfo("time_seq", GeneratorInfo{
		Description: "Increasing timestamps (row i = start + i*step, plus jitter) for event streams and logs",
		ConfigKeys: []ConfigKey{
			{Name: "start", Type: "string", Required: true, Desc: "First timestamp (in format, or YYYY-MM-DD)"},
			{Name: "step", Type: "string", Required: true, Desc: "Gap between rows as a Go duration: \"90s\", \"15m\", \"24h\""},
			{Name: "jitter", Type: "string", Default: "0s", Desc: "Random extra delay per row, less than step, so order is kept"},
			{Name: "format", Type: "string", Default: "RFC3339", Desc: "Go time layout for parsing start and formatting output; use RFC3339Nano for sub-second steps"},
			{Name: "tz", Type: "string", Default: "UTC", Desc: "IANA time zone for output and offset-less start"},
		},
		Example: `- name: logged_at
  gen: time_seq
  config:
    start: "2024-06-01T00:00:00Z"
    step: 90s
    jitter: 30s`,
	})
}
