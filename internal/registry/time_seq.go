package registry

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/compuficial/apery/internal/rng"
)

// The gaps between time_seq rows.
const (
	fixedGaps       = "fixed"       // step apart, plus jitter
	exponentialGaps = "exponential" // random, averaging step
)

// timeline formats offsets from a start time.
type timeline struct {
	start  time.Time
	format string
	loc    *time.Location
}

// at formats the time offset after start.
func (t timeline) at(offset time.Duration) string {
	return t.start.Add(offset).In(t.loc).Format(t.format)
}

// TimeSeqGenerator generates increasing timestamps for event streams and
// logs: row i is start + i*step, plus up to jitter. With jitter below step
// every timestamp is later than the one before. The value depends only on
// the row index and the row's draw, so any chunking gives the same output.
type TimeSeqGenerator struct {
	timeline
	step   time.Duration
	jitter time.Duration
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
	return g.at(offset), nil
}

// SeekRow positions the generator at a zero-based row index.
func (g *TimeSeqGenerator) SeekRow(row int64) error {
	if row < 0 {
		return fmt.Errorf("time_seq: row index must be >= 0")
	}
	g.row = row
	return nil
}

// timeArrivals generates the times of independent random events (a Poisson
// process): row 0 is at start and each later row adds an exponential gap
// averaging step. Each value builds on the one before, so it is Sequential.
type timeArrivals struct {
	timeline
	meanGap    float64       // nanoseconds
	offset     time.Duration // of the previous row
	hasStarted bool          // whether row 0 has been generated
}

// Next returns the time of the next event.
func (g *timeArrivals) Next(r *rng.Rng) (any, error) {
	if g.hasStarted {
		// Compare as integers: the float sum could round past the limit.
		gap := r.Exponential(g.meanGap)
		if gap >= 1<<63 || time.Duration(gap) > math.MaxInt64-g.offset {
			return nil, errors.New("time_seq: more than about 292 years after 'start'")
		}
		g.offset += time.Duration(gap)
	}
	g.hasStarted = true
	return g.at(g.offset), nil
}

func (g *timeArrivals) buildsOnPreviousRow() {}

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

// parseGaps reads the gaps key, fixedGaps by default.
func parseGaps(config map[string]any) (string, error) {
	val, exists := config["gaps"]
	if !exists {
		return fixedGaps, nil
	}
	gaps, _ := val.(string)
	if gaps != fixedGaps && gaps != exponentialGaps {
		return "", fmt.Errorf("time_seq: 'gaps' must be %q or %q, got %v", fixedGaps, exponentialGaps, val)
	}
	return gaps, nil
}

// newTimeSeq builds a time_seq with fixed or exponential gaps.
func newTimeSeq(config map[string]any) (Generator, error) {
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
	line := timeline{start: start, format: format, loc: loc}

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

	gaps, err := parseGaps(config)
	if err != nil {
		return nil, err
	}
	if gaps == fixedGaps {
		return &TimeSeqGenerator{timeline: line, step: step, jitter: jitter}, nil
	}
	if jitter != 0 {
		return nil, fmt.Errorf("time_seq: 'jitter' applies to fixed gaps; exponential gaps are already random")
	}
	return &timeArrivals{timeline: line, meanGap: float64(step)}, nil
}

// init registers the time_seq generator.
func init() {
	MustRegister("time_seq", newTimeSeq)
	MustRegisterInfo("time_seq", GeneratorInfo{
		Description: "Increasing timestamps for event streams and logs: row i = start + i*step, plus jitter, or random exponential gaps",
		ConfigKeys: []ConfigKey{
			{Name: "start", Type: "string", Required: true, Desc: "First timestamp (in format, or YYYY-MM-DD)"},
			{Name: "step", Type: "string", Required: true, Desc: "Gap between rows as a Go duration: \"90s\", \"15m\", \"24h\"; the average gap with exponential gaps"},
			{Name: "jitter", Type: "string", Default: "0s", Desc: "Random extra delay per row, less than step, so order is kept (fixed gaps only)"},
			{Name: "gaps", Type: "string", Default: "fixed", Desc: "fixed: step apart; exponential: random gaps averaging step, like independent events (the entity then runs on one worker)"},
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
