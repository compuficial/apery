package registry

import (
	"apery/internal/rng"
	"fmt"
	"regexp/syntax"
	"strings"
)

const (
	defaultMaxRepeat = 10
)

// RegexGenerator generates strings matching a regex pattern
type RegexGenerator struct {
	re        *syntax.Regexp
	maxRepeat int
}

// Next returns a random string matching the configured pattern
func (g *RegexGenerator) Next(r *rng.Rng) (any, error) {
	return generate(g.re, r, g.maxRepeat)
}

// generate recursively walks the regex AST and generates a matching string
func generate(re *syntax.Regexp, r *rng.Rng, maxRepeat int) (string, error) {
	switch re.Op {
	case syntax.OpLiteral:
		return string(re.Rune), nil

	case syntax.OpEmptyMatch:
		return "", nil

	case syntax.OpCharClass:
		ch := pickFromCharClass(re.Rune, r)
		return string(ch), nil

	case syntax.OpAnyCharNotNL:
		// Printable ASCII except newline (32-126, excluding 10)
		ch := rune(32 + r.Intn(95)) // 95 printable chars
		return string(ch), nil

	case syntax.OpAnyChar:
		// Printable ASCII including all (32-126)
		ch := rune(32 + r.Intn(95))
		return string(ch), nil

	case syntax.OpConcat:
		var buf strings.Builder
		for _, sub := range re.Sub {
			s, err := generate(sub, r, maxRepeat)
			if err != nil {
				return "", err
			}
			buf.WriteString(s)
		}
		return buf.String(), nil

	case syntax.OpAlternate:
		idx := r.Intn(len(re.Sub))
		return generate(re.Sub[idx], r, maxRepeat)

	case syntax.OpCapture:
		return generate(re.Sub[0], r, maxRepeat)

	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat:
		return handleQuantifier(re, r, maxRepeat)

	// Anchors - return empty string (position assertions, not characters)
	case syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return "", nil

	case syntax.OpNoMatch:
		return "", fmt.Errorf("regex: pattern cannot match anything")

	default:
		return "", fmt.Errorf("regex: unsupported op %v", re.Op)
	}
}

// pickFromCharClass selects a random character from a character class.
// The runes slice contains pairs: [lo1, hi1, lo2, hi2, ...]
func pickFromCharClass(runes []rune, r *rng.Rng) rune {
	// Count total characters across all ranges
	total := 0
	for i := 0; i < len(runes); i += 2 {
		total += int(runes[i+1] - runes[i] + 1)
	}

	if total == 0 {
		return 0
	}

	// Pick random index
	idx := r.Intn(total)

	// Map index to character
	for i := 0; i < len(runes); i += 2 {
		rangeSize := int(runes[i+1] - runes[i] + 1)
		if idx < rangeSize {
			return runes[i] + rune(idx)
		}
		idx -= rangeSize
	}

	// Should never reach here
	return runes[0]
}

// handleQuantifier generates repeated instances of a sub-expression
func handleQuantifier(re *syntax.Regexp, r *rng.Rng, maxRepeat int) (string, error) {
	min, max := getRepeatBounds(re, maxRepeat)

	// Determine count
	count := min
	if max > min {
		count = min + r.Intn(max-min+1)
	}

	var buf strings.Builder
	for i := 0; i < count; i++ {
		s, err := generate(re.Sub[0], r, maxRepeat)
		if err != nil {
			return "", err
		}
		buf.WriteString(s)
	}
	return buf.String(), nil
}

// getRepeatBounds returns the min and max repetition counts for a quantifier
func getRepeatBounds(re *syntax.Regexp, maxRepeat int) (int, int) {
	switch re.Op {
	case syntax.OpStar:
		return 0, maxRepeat
	case syntax.OpPlus:
		return 1, maxRepeat
	case syntax.OpQuest:
		return 0, 1
	case syntax.OpRepeat:
		max := re.Max
		if max == -1 { // unbounded
			max = maxRepeat
		}
		return re.Min, max
	}
	return 0, 0
}

// validateRegexConfig validates and parses config for regex generator
func validateRegexConfig(config map[string]any) (*syntax.Regexp, int, error) {
	// Validate pattern - required
	patternVal, exists := config["pattern"]
	if !exists {
		return nil, 0, fmt.Errorf("regex: 'pattern' is required")
	}
	pattern, ok := patternVal.(string)
	if !ok {
		return nil, 0, fmt.Errorf("regex: 'pattern' must be a string, got %T", patternVal)
	}
	if pattern == "" {
		return nil, 0, fmt.Errorf("regex: 'pattern' cannot be empty")
	}

	// Parse regex with Perl syntax
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, 0, fmt.Errorf("regex: invalid pattern: %w", err)
	}

	// Validate max_repeat - optional, must be int
	maxRepeat := defaultMaxRepeat
	if val, exists := config["max_repeat"]; exists {
		m, ok := val.(int)
		if !ok {
			return nil, 0, fmt.Errorf("regex: 'max_repeat' must be an integer, got %T (use: 10, not 10.0)", val)
		}
		if m < 1 {
			return nil, 0, fmt.Errorf("regex: 'max_repeat' must be >= 1, got %d", m)
		}
		maxRepeat = m
	}

	return re, maxRepeat, nil
}

func init() {
	MustRegister("regex", func(config map[string]any) (Generator, error) {
		re, maxRepeat, err := validateRegexConfig(config)
		if err != nil {
			return nil, err
		}
		return &RegexGenerator{
			re:        re,
			maxRepeat: maxRepeat,
		}, nil
	})
}
