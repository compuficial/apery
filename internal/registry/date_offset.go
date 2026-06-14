package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"fmt"
	"math"
	"strings"
	"time"
)

// DateOffsetGenerator shifts a base date by an expression-valued amount of
// calendar or clock units (the temporal counterpart to expr).
type DateOffsetGenerator struct {
	baseLiteral string       // set when base is a literal date
	baseField   string       // set when base is a {field} reference
	amount      compiledExpr // offset expression
	unit        string
	format      string
	deps        []string
}

var calendarUnits = map[string]bool{"years": true, "months": true, "days": true}

var clockUnits = map[string]time.Duration{
	"hours": time.Hour, "minutes": time.Minute, "seconds": time.Second,
}

// Next returns an error because date_offset requires row context.
func (g *DateOffsetGenerator) Next(_ *rng.Rng) (any, error) {
	return nil, fmt.Errorf("date_offset: requires row context")
}

// NextWithRow shifts the resolved base date by the evaluated amount.
func (g *DateOffsetGenerator) NextWithRow(_ *rng.Rng, row RowContext) (any, error) {
	baseStr := g.baseLiteral
	if g.baseField != "" {
		v, ok := row.Get(g.baseField)
		if !ok {
			return nil, fmt.Errorf("date_offset: base field %q not found in row context", g.baseField)
		}
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("date_offset: base field %q must be a string date, got %T", g.baseField, v)
		}
		baseStr = s
	}

	base, err := time.Parse(g.format, baseStr)
	if err != nil {
		return nil, fmt.Errorf("date_offset: cannot parse base %q with format %q: %w", baseStr, g.format, err)
	}

	f, err := g.amount.eval(row)
	if err != nil {
		return nil, err
	}
	if f != math.Trunc(f) {
		return nil, fmt.Errorf("date_offset: amount must be a whole number, got %v", f)
	}
	n := int(f)

	switch g.unit {
	case "years":
		return base.AddDate(n, 0, 0).Format(g.format), nil
	case "months":
		return base.AddDate(0, n, 0).Format(g.format), nil
	case "days":
		return base.AddDate(0, 0, n).Format(g.format), nil
	default: // clock unit, validated at config time
		d, err := scaleDuration(int64(n), clockUnits[g.unit])
		if err != nil {
			return nil, err
		}
		return base.Add(d).Format(g.format), nil
	}
}

// Dependencies returns the row fields referenced by the base and amount.
func (g *DateOffsetGenerator) Dependencies() []string {
	return g.deps
}

// scaleDuration multiplies amount by unit, erroring instead of overflowing int64.
func scaleDuration(amount int64, unit time.Duration) (time.Duration, error) {
	per := int64(unit)
	if amount > math.MaxInt64/per || amount < math.MinInt64/per {
		return 0, fmt.Errorf("date_offset: amount %d overflows the representable duration range", amount)
	}
	return time.Duration(amount) * unit, nil
}

// parseFieldRef returns the inner name if s is a single {field} reference.
func parseFieldRef(s string) (string, bool) {
	if len(s) >= 2 && s[0] == '{' && s[len(s)-1] == '}' {
		inner := s[1 : len(s)-1]
		if inner != "" && !strings.ContainsAny(inner, "{}") {
			return inner, true
		}
	}
	return "", false
}

// validateDateOffsetConfig parses and validates config into a generator.
func validateDateOffsetConfig(config map[string]any) (*DateOffsetGenerator, error) {
	g := &DateOffsetGenerator{format: time.RFC3339}

	unit, ok := config["unit"].(string)
	if !ok || (!calendarUnits[unit] && clockUnits[unit] == 0) {
		return nil, fmt.Errorf("date_offset: 'unit' must be one of years|months|days|hours|minutes|seconds")
	}
	g.unit = unit

	if raw, exists := config["format"]; exists {
		f, ok := raw.(string)
		if !ok || f == "" {
			return nil, fmt.Errorf("date_offset: 'format' must be a non-empty string")
		}
		g.format = f
	}

	base, ok := config["base"].(string)
	if !ok {
		return nil, fmt.Errorf("date_offset: 'base' must be a string (a date or {field})")
	}
	if field, isRef := parseFieldRef(base); isRef {
		g.baseField = field
		g.deps = append(g.deps, field)
	} else if _, err := time.Parse(g.format, base); err != nil {
		return nil, fmt.Errorf("date_offset: literal 'base' %q does not parse with format %q: %w", base, g.format, err)
	} else {
		g.baseLiteral = base
	}

	// amount is an expr; a bare number is accepted as a constant offset.
	amountRaw, ok := config["amount"]
	if !ok {
		return nil, fmt.Errorf("date_offset: 'amount' is required (a number or expression)")
	}
	amountSrc, ok := amountRaw.(string)
	if !ok {
		if _, isNum := toNumber(amountRaw); !isNum {
			return nil, fmt.Errorf("date_offset: 'amount' must be a number or expression string, got %T", amountRaw)
		}
		amountSrc = fmt.Sprint(amountRaw)
	}
	amount, err := compileExpr(amountSrc)
	if err != nil {
		return nil, err
	}
	g.amount = amount
	g.deps = append(g.deps, amount.deps...)

	return g, nil
}

func init() {
	MustRegister("date_offset", func(config map[string]any) (Generator, error) {
		return validateDateOffsetConfig(config)
	})
	MustRegisterInfo("date_offset", GeneratorInfo{
		Description: "Shift a base date by N units (years|months|days|hours|minutes|seconds)",
		ConfigKeys: []ConfigKey{
			{Name: "base", Type: "string", Required: true, Desc: "Date literal or {field} reference"},
			{Name: "amount", Type: "string", Required: true, Desc: "Offset: a number (1) or an expr over {field}s (e.g. \"{event_index}\", \"{q} * 3\")"},
			{Name: "unit", Type: "string", Required: true, Desc: "years|months|days|hours|minutes|seconds"},
			{Name: "format", Type: "string", Default: "RFC3339", Desc: "Go time layout for parsing base and formatting output"},
		},
		Example: `- name: recognized_at
  gen: date_offset
  config:
    base: "{sub_start}"
    amount: "{event_index}"
    unit: months
    format: "2006-01-02"`,
	})
}
