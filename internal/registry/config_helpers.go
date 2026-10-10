package registry

import (
	"fmt"
	"math"
)

// extractFloat extracts a float64 from a config value.
// Accepts float and integer types for user convenience.
func extractFloat(val any, param, gen string) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("%s: '%s' must be a number, got %T (use: 1.5 or 1)", gen, param, val)
	}
}

// extractInt extracts an int64 from a config value. YAML yields int (or
// int64/uint64 for large values) and JSON yields float64, so any of them is
// accepted when it holds a whole number in int64 range.
func extractInt(val any, param, gen string) (int64, error) {
	switch v := val.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case uint64:
		if v <= math.MaxInt64 {
			return int64(v), nil
		}
	case float64:
		// 2^63 is exactly representable; anything below it converts exactly.
		if v == math.Trunc(v) && v >= math.MinInt64 && v < math.MaxInt64 {
			return int64(v), nil
		}
	}
	return 0, fmt.Errorf("%s: '%s' must be an integer, got %v (%T)", gen, param, val, val)
}

// extractUint extracts a uint64 from a config value, rejecting negatives.
func extractUint(val any, param, gen string) (uint64, error) {
	i, err := extractInt(val, param, gen)
	if err != nil {
		return 0, err
	}
	if i < 0 {
		return 0, fmt.Errorf("%s: '%s' must be non-negative, got %d", gen, param, i)
	}
	return uint64(i), nil
}

// buildCDF validates positive weights, one per item, and returns their
// cumulative distribution. items names what the weights pair with.
// Example: weights [1, 3] → CDF [0.25, 1.0].
func buildCDF(weightsVal any, numItems int, gen, items string) ([]float64, error) {
	raw, ok := weightsVal.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: 'weights' must be an array, got %T", gen, weightsVal)
	}
	if len(raw) != numItems {
		return nil, fmt.Errorf("%s: 'weights' length (%d) must match '%s' length (%d)", gen, len(raw), items, numItems)
	}

	weights := make([]float64, len(raw))
	var total float64
	for i, w := range raw {
		v, err := extractFloat(w, "weights", gen)
		if err != nil {
			return nil, err
		}
		if v <= 0 {
			return nil, fmt.Errorf("%s: 'weights[%d]' must be > 0, got %v", gen, i, v)
		}
		weights[i] = v
		total += v
	}

	cdf := make([]float64, len(weights))
	cumulative := 0.0
	for i, w := range weights {
		cumulative += w / total
		cdf[i] = cumulative
	}
	cdf[len(cdf)-1] = 1.0 // clamp last to exactly 1.0

	return cdf, nil
}

// cdfIndex returns the first index whose cumulative weight exceeds f.
func cdfIndex(cdf []float64, f float64) int {
	for i, threshold := range cdf {
		if f < threshold {
			return i
		}
	}
	return len(cdf) - 1
}
