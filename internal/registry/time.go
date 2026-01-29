package registry

import (
	"apery/internal/rng"
	"fmt"
	"time"
)

const (
	defaultTimeFormat = time.RFC3339
	defaultTimezone   = "UTC"
	// Default time range: 2020-01-01 to 2030-12-31 (UTC)
	// Using fixed dates ensures determinism - same config always produces same output
	defaultStartTime = "2020-01-01T00:00:00Z"
	defaultEndTime   = "2030-12-31T23:59:59Z"
)

// TimeGenerator generates a timestamp within a specified range.
// Note: Generated timestamps have second-level precision only, regardless of format.
type TimeGenerator struct {
	start  time.Time
	end    time.Time
	format string
	loc    *time.Location
}

func (t *TimeGenerator) Next(r *rng.Rng) (any, error) {
	// Note: Uses Unix timestamps (seconds), so subsecond precision is lost
	start := t.start.Unix()
	end := t.end.Unix()
	randomUnixTimestamp := r.IntRange(start, end)
	timestamp := time.Unix(randomUnixTimestamp, 0)
	localTime := timestamp.In(t.loc)
	return localTime.Format(t.format), nil
}

func validateTimeConfig(config map[string]any) (time.Time, time.Time, string, *time.Location, error) {
	tzStr := defaultTimezone
	if tz, ok := config["tz"].(string); ok {
		tzStr = tz
	}
	loc, err := time.LoadLocation(tzStr)
	if err != nil {
		return time.Time{}, time.Time{}, "", nil, fmt.Errorf("time: invalid timezone %q: %w", tzStr, err)
	}

	format := defaultTimeFormat
	if f, ok := config["format"].(string); ok {
		format = f
	}

	// Parse default start/end times using the default format (RFC3339)
	// These are constants to ensure determinism
	defaultStart, _ := time.Parse(time.RFC3339, defaultStartTime)
	defaultEnd, _ := time.Parse(time.RFC3339, defaultEndTime)

	start, err := parseTimeFromConfig(config, "start", defaultStart, format)
	if err != nil {
		return time.Time{}, time.Time{}, "", nil, err
	}

	end, err := parseTimeFromConfig(config, "end", defaultEnd, format)
	if err != nil {
		return time.Time{}, time.Time{}, "", nil, err
	}

	if start.After(end) {
		return time.Time{}, time.Time{}, "", nil, fmt.Errorf("time: 'start' must be before 'end'")
	}

	if start.Equal(end) {
		return time.Time{}, time.Time{}, "", nil, fmt.Errorf("time: 'start' and 'end' cannot be equal")
	}

	return start, end, format, loc, nil
}

func parseTimeFromConfig(config map[string]any, key string, defaultVal time.Time, format string) (time.Time, error) {
	val, exists := config[key]
	if !exists {
		return defaultVal, nil
	}

	s, ok := val.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("time: '%s' must be a string, got %T", key, val)
	}

	parsed, err := time.Parse(format, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("time: invalid '%s': %w", key, err)
	}

	return parsed, nil
}

func init() {
	MustRegister("time", func(config map[string]any) (Generator, error) {
		start, end, format, loc, err := validateTimeConfig(config)
		if err != nil {
			return nil, err
		}
		return &TimeGenerator{start: start, end: end, format: format, loc: loc}, nil
	})
}
