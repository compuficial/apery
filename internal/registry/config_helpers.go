package registry

import "fmt"

// extractFloat extracts a float64 from a config value.
// Accepts both float64 and int types for user convenience.
func extractFloat(val any, param, gen string) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("%s: '%s' must be a number, got %T (use: 1.5 or 1)", gen, param, val)
	}
}

// extractInt extracts an int64 from a config value.
// Only accepts int type to ensure no precision loss.
func extractInt(val any, param, gen string) (int64, error) {
	i, ok := val.(int)
	if !ok {
		return 0, fmt.Errorf("%s: '%s' must be an integer, got %T (use: 10, not 10.0)", gen, param, val)
	}
	return int64(i), nil
}

// extractUint extracts a uint64 from a config value.
// Only accepts int type and validates that the value is positive.
func extractUint(val any, param, gen string) (uint64, error) {
	i, ok := val.(int)
	if !ok {
		return 0, fmt.Errorf("%s: '%s' must be an integer, got %T (use: 10, not 10.0)", gen, param, val)
	}
	if i < 0 {
		return 0, fmt.Errorf("%s: '%s' must be positive, got %d", gen, param, i)
	}
	return uint64(i), nil
}
