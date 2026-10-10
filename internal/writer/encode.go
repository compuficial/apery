package writer

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// appendObject appends record as a JSON object. A non-empty entity is
// written first as "_entity". Output is byte-identical to encoding/json:
// common scalars take a fast path and everything else (maps, slices,
// unusual types) goes through json.Marshal.
func appendObject(dst []byte, record *OrderedMap, entity string) ([]byte, error) {
	dst = append(dst, '{')
	needsComma := false
	if entity != "" {
		dst = appendString(dst, "_entity")
		dst = append(dst, ':')
		dst = appendString(dst, entity)
		needsComma = true
	}

	for i, key := range record.keys {
		if needsComma {
			dst = append(dst, ',')
		}
		needsComma = true

		dst = appendString(dst, key)
		dst = append(dst, ':')
		var err error
		if dst, err = appendValue(dst, record.values[i]); err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
	}
	return append(dst, '}'), nil
}

func appendValue(dst []byte, v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return append(dst, "null"...), nil
	case string:
		return appendString(dst, x), nil
	case bool:
		return strconv.AppendBool(dst, x), nil
	case int64:
		return strconv.AppendInt(dst, x, 10), nil
	case int:
		return strconv.AppendInt(dst, int64(x), 10), nil
	case float64:
		return appendFloat(dst, x)
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(dst, b...), nil
}

// appendString copies plain ASCII through and defers anything that needs
// escaping (quotes, control bytes, HTML-sensitive <>&, non-ASCII) to
// encoding/json so escaping rules stay identical.
func appendString(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c > 0x7e || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			b, _ := json.Marshal(s) // strings always marshal
			return append(dst, b...)
		}
	}
	dst = append(dst, '"')
	dst = append(dst, s...)
	return append(dst, '"')
}

// appendFloat formats like encoding/json: shortest representation, 'f'
// notation except for very small or very large magnitudes.
func appendFloat(dst []byte, f float64) ([]byte, error) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return nil, fmt.Errorf("value %v is not representable in JSON", f)
	}

	format := byte('f')
	if abs := math.Abs(f); abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	dst = strconv.AppendFloat(dst, f, format, -1, 64)

	// encoding/json writes e-9, not e-09.
	if format == 'e' {
		n := len(dst)
		if n >= 4 && dst[n-4] == 'e' && dst[n-3] == '-' && dst[n-2] == '0' {
			dst[n-2] = dst[n-1]
			dst = dst[:n-1]
		}
	}
	return dst, nil
}
