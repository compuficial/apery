package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"regexp"
	"testing"
)

const regexGen = "regex"

func TestRegexGenerator_Config(t *testing.T) {
	RunConfigTests(t, regexGen, []ConfigTestCase{
		// valid configs
		{Name: "simple pattern", Config: map[string]any{"pattern": "[a-z]+"}, ExpectError: false},
		{Name: "with max_repeat", Config: map[string]any{"pattern": "a*", "max_repeat": 5}, ExpectError: false},
		{Name: "complex pattern", Config: map[string]any{"pattern": `[A-Z]{2}\d{4}`}, ExpectError: false},
		{Name: "alternation", Config: map[string]any{"pattern": "(foo|bar|baz)"}, ExpectError: false},
		{Name: "escape sequences", Config: map[string]any{"pattern": `\d+\.\d+`}, ExpectError: false},
		{Name: "character class", Config: map[string]any{"pattern": "[a-zA-Z0-9_]+"}, ExpectError: false},
		{Name: "anchors", Config: map[string]any{"pattern": "^foo$"}, ExpectError: false},
		{Name: "optional", Config: map[string]any{"pattern": "colou?r"}, ExpectError: false},
		{Name: "groups", Config: map[string]any{"pattern": "((a|b)c)+"}, ExpectError: false},
		{Name: "literal only", Config: map[string]any{"pattern": "hello"}, ExpectError: false},

		// invalid configs
		{Name: "missing pattern", Config: map[string]any{}, ExpectError: true},
		{Name: "empty pattern", Config: map[string]any{"pattern": ""}, ExpectError: true},
		{Name: "invalid pattern type", Config: map[string]any{"pattern": 123}, ExpectError: true},
		{Name: "invalid regex", Config: map[string]any{"pattern": "["}, ExpectError: true},
		{Name: "invalid regex unclosed group", Config: map[string]any{"pattern": "(abc"}, ExpectError: true},
		{Name: "invalid max_repeat type string", Config: map[string]any{"pattern": "a", "max_repeat": "5"}, ExpectError: true},
		{Name: "invalid max_repeat type float", Config: map[string]any{"pattern": "a", "max_repeat": 5.0}, ExpectError: true},
		{Name: "max_repeat zero", Config: map[string]any{"pattern": "a", "max_repeat": 0}, ExpectError: true},
		{Name: "max_repeat negative", Config: map[string]any{"pattern": "a", "max_repeat": -1}, ExpectError: true},
		{Name: "word boundary unsupported", Config: map[string]any{"pattern": `foo\bbar`}, ExpectError: true},
		{Name: "anchor inside quantifier", Config: map[string]any{"pattern": `(^a)+`}, ExpectError: true},
		{Name: "begin anchor not at start", Config: map[string]any{"pattern": `a(^b)`}, ExpectError: true},
		{Name: "end anchor not at end", Config: map[string]any{"pattern": `(a$)b`}, ExpectError: true},
	})
}

func TestRegexGenerator_Determinism(t *testing.T) {
	RunDeterminismTests(t, regexGen, []DeterminismTestCase{
		{Name: "literal", Config: map[string]any{"pattern": "abc"}},
		{Name: "char class", Config: map[string]any{"pattern": "[a-z]{5}"}},
		{Name: "alternation", Config: map[string]any{"pattern": "(foo|bar|baz)"}},
		{Name: "digits", Config: map[string]any{"pattern": `\d{3}-\d{4}`}},
		{Name: "complex", Config: map[string]any{"pattern": `[A-Z]{2}\d{4}[a-z]?`}},
		{Name: "star", Config: map[string]any{"pattern": "a*", "max_repeat": 5}},
		{Name: "plus", Config: map[string]any{"pattern": "a+", "max_repeat": 5}},
		{Name: "nested", Config: map[string]any{"pattern": "((a|b){2,3}c)+", "max_repeat": 3}},
	})
}

func TestRegexGenerator_OutputMatchesPattern(t *testing.T) {
	tests := []struct {
		Name    string
		Pattern string
	}{
		{Name: "digits", Pattern: `\d{5}`},
		{Name: "phone", Pattern: `\d{3}-\d{3}-\d{4}`},
		{Name: "license plate", Pattern: `[A-Z]{3}-\d{4}`},
		{Name: "alternation", Pattern: `(red|green|blue)`},
		{Name: "optional", Pattern: `colou?r`},
		{Name: "mixed", Pattern: `[A-Z][a-z]{2,5}\d{2}`},
		{Name: "word chars", Pattern: `\w{10}`},
		{Name: "whitespace", Pattern: `a\s+b`},
		{Name: "dot", Pattern: `a.b`},
		{Name: "escape", Pattern: `\.\*\+`},
		{Name: "range", Pattern: `[a-f]{3,6}`},
		{Name: "negated class", Pattern: `[^0-9]{5}`},
		{Name: "anchors ignored", Pattern: `^start$`},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(regexGen, map[string]any{"pattern": tt.Pattern, "max_repeat": 10})
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			// Compile pattern for validation (anchored)
			compiled := regexp.MustCompile("^(?:" + tt.Pattern + ")$")

			r := rng.New(rng.SeedFromInt64(testSeed))
			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}
				s := val.(string)
				if !compiled.MatchString(s) {
					t.Errorf("generated %q does not match pattern %s", s, tt.Pattern)
				}
			}
		})
	}
}

func TestRegexGenerator_QuantifierBounds(t *testing.T) {
	tests := []struct {
		Name      string
		Pattern   string
		MaxRepeat int
		MinLen    int
		MaxLen    int
	}{
		{Name: "star", Pattern: "a*", MaxRepeat: 5, MinLen: 0, MaxLen: 5},
		{Name: "plus", Pattern: "a+", MaxRepeat: 5, MinLen: 1, MaxLen: 5},
		{Name: "quest", Pattern: "a?", MaxRepeat: 10, MinLen: 0, MaxLen: 1},
		{Name: "exact", Pattern: "a{3}", MaxRepeat: 10, MinLen: 3, MaxLen: 3},
		{Name: "range", Pattern: "a{2,4}", MaxRepeat: 10, MinLen: 2, MaxLen: 4},
		{Name: "unbounded capped", Pattern: "a{2,}", MaxRepeat: 5, MinLen: 2, MaxLen: 5},
		{Name: "explicit max capped", Pattern: "a{5,1000}", MaxRepeat: 10, MinLen: 5, MaxLen: 10},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			gen, err := Get(regexGen, map[string]any{"pattern": tt.Pattern, "max_repeat": tt.MaxRepeat})
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			r := rng.New(rng.SeedFromInt64(testSeed))
			for i := range testIterations {
				val, err := gen.Next(r)
				if err != nil {
					t.Fatalf("generation error at %d: %v", i, err)
				}
				s := val.(string)
				if len(s) < tt.MinLen || len(s) > tt.MaxLen {
					t.Errorf("length %d outside bounds [%d, %d] for %q", len(s), tt.MinLen, tt.MaxLen, s)
				}
			}
		})
	}
}

func TestRegexGenerator_MaxRepeatMinError(t *testing.T) {
	gen, err := Get(regexGen, map[string]any{"pattern": "a{100,}", "max_repeat": 10})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	if _, err := gen.Next(r); err == nil {
		t.Fatalf("expected error when min exceeds max_repeat")
	}
}

func TestRegexGenerator_Literal(t *testing.T) {
	// Literal patterns should always produce the same output
	gen, err := Get(regexGen, map[string]any{"pattern": "hello world"})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	for i := range testIterations {
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("generation error at %d: %v", i, err)
		}
		if val.(string) != "hello world" {
			t.Errorf("expected 'hello world', got %q", val.(string))
		}
	}
}

func TestRegexGenerator_CharClass(t *testing.T) {
	// Test that character class produces only valid characters
	gen, err := Get(regexGen, map[string]any{"pattern": "[abc]{100}", "max_repeat": 100})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("generation error: %v", err)
	}

	s := val.(string)
	for i, ch := range s {
		if ch != 'a' && ch != 'b' && ch != 'c' {
			t.Errorf("invalid char %q at position %d", ch, i)
		}
	}
}

func TestRegexGenerator_Alternation(t *testing.T) {
	// Test that alternation produces only valid alternatives
	gen, err := Get(regexGen, map[string]any{"pattern": "(apple|banana|cherry)"})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	valid := map[string]bool{"apple": true, "banana": true, "cherry": true}
	r := rng.New(rng.SeedFromInt64(testSeed))

	for i := range testIterations {
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("generation error at %d: %v", i, err)
		}
		if !valid[val.(string)] {
			t.Errorf("unexpected value %q", val.(string))
		}
	}
}

func TestRegexGenerator_EmptyMatch(t *testing.T) {
	// Optional pattern can produce empty string
	gen, err := Get(regexGen, map[string]any{"pattern": "a?"})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	foundEmpty := false
	foundA := false

	for range 1000 {
		val, _ := gen.Next(r)
		s := val.(string)
		if s == "" {
			foundEmpty = true
		}
		if s == "a" {
			foundA = true
		}
		if foundEmpty && foundA {
			break
		}
	}

	if !foundEmpty {
		t.Error("expected to find empty string from a?")
	}
	if !foundA {
		t.Error("expected to find 'a' from a?")
	}
}

func TestRegexGenerator_OutputType(t *testing.T) {
	gen, err := Get(regexGen, map[string]any{"pattern": "[a-z]+"})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("generation error: %v", err)
	}

	if _, ok := val.(string); !ok {
		t.Errorf("expected string, got %T", val)
	}
}

func TestRegexGenerator_Unicode(t *testing.T) {
	// Test unicode character ranges
	gen, err := Get(regexGen, map[string]any{"pattern": "[α-ω]{5}"})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	r := rng.New(rng.SeedFromInt64(testSeed))
	val, err := gen.Next(r)
	if err != nil {
		t.Fatalf("generation error: %v", err)
	}

	s := val.(string)
	if len([]rune(s)) != 5 {
		t.Errorf("expected 5 runes, got %d", len([]rune(s)))
	}

	for _, ch := range s {
		if ch < 'α' || ch > 'ω' {
			t.Errorf("char %q outside range [α-ω]", ch)
		}
	}
}

func TestRegexGenerator_NestedGroups(t *testing.T) {
	// Test nested groups work correctly
	gen, err := Get(regexGen, map[string]any{"pattern": "((a|b)(c|d))+"})
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	compiled := regexp.MustCompile(`^((a|b)(c|d))+$`)
	r := rng.New(rng.SeedFromInt64(testSeed))

	for i := range testIterations {
		val, err := gen.Next(r)
		if err != nil {
			t.Fatalf("generation error at %d: %v", i, err)
		}
		s := val.(string)
		if !compiled.MatchString(s) {
			t.Errorf("generated %q does not match pattern", s)
		}
	}
}
