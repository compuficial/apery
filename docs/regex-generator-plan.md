# Implementation Plan: `regex` Generator

## Overview

The `regex` generator produces random strings that match a given regular expression pattern. This is useful for generating formatted data like phone numbers, license plates, product codes, etc.

**Spec reference:** `sdg.md` section 5.1
> `regex(pattern)` — generates strings matching regex pattern

## Design Decision: Custom Implementation

We will implement our own regex-to-string generator rather than using an external library.

**Rationale:**
1. **Direct RNG integration** - Uses our `*rng.Rng` throughout for perfect determinism
2. **No external dependencies** - Only uses `regexp/syntax` from Go's stdlib
3. **Full control** - Customize behavior exactly as needed
4. **Minimal code** - Approximately 150-200 lines
5. **Testability** - We understand and control every line

**Key insight:** Go's `regexp/syntax` package already handles the hard part (parsing). We only need to walk the AST and generate strings.

---

## Files to Create

| File | Purpose |
|------|---------|
| `internal/registry/regex.go` | Generator implementation |
| `internal/registry/regex_test.go` | Test suite |

---

## Configuration Schema

```json
{
  "gen": "regex",
  "config": {
    "pattern": "[A-Z]{3}-\\d{4}",
    "max_repeat": 10
  }
}
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `pattern` | string | Yes | - | The regex pattern to match |
| `max_repeat` | int | No | 10 | Maximum repetitions for unbounded quantifiers (`*`, `+`) |

### Validation Rules

1. `pattern` must be present and non-empty
2. `pattern` must be a valid regex (parseable by `regexp/syntax.Parse`)
3. `max_repeat` must be >= 1 if provided
4. `max_repeat` must be an integer type (following `int.go` pattern)

---

## Implementation Details

### Step 1: Understand `regexp/syntax` AST

The `regexp/syntax` package parses regex into an AST. Each node is a `*syntax.Regexp` with an `Op` field indicating the operation type.

```go
type Regexp struct {
    Op       Op         // operator type
    Flags    Flags      // regex flags
    Sub      []*Regexp  // subexpressions
    Rune     []rune     // for OpLiteral and OpCharClass
    Min, Max int        // for OpRepeat
    Cap      int        // capture group index
    Name     string     // capture group name
}
```

### Step 2: Handle Each Op Type

#### Category A: Literal Output

| Op | Description | Implementation |
|----|-------------|----------------|
| `OpLiteral` | Matches rune sequence | Return `string(re.Rune)` |
| `OpEmptyMatch` | Matches empty string | Return `""` |

#### Category B: Character Selection

| Op | Description | Implementation |
|----|-------------|----------------|
| `OpCharClass` | Character class `[a-z]` | Pick random char from ranges |
| `OpAnyChar` | Matches `.` (any char) | Pick random printable ASCII |
| `OpAnyCharNotNL` | Matches `.` (no newline) | Pick random printable ASCII except `\n` |

**OpCharClass detail:** The `Rune` field contains pairs: `[lo1, hi1, lo2, hi2, ...]`

For `[a-zA-Z0-9]`:
```
Rune = ['0', '9', 'A', 'Z', 'a', 'z']
```

Algorithm:
1. Count total characters across all ranges
2. Pick random index in `[0, total)`
3. Map index back to actual character

```go
func pickFromCharClass(runes []rune, r *rng.Rng) rune {
    // Count total chars
    total := 0
    for i := 0; i < len(runes); i += 2 {
        total += int(runes[i+1] - runes[i] + 1)
    }

    // Pick random index
    idx := r.Intn(total)

    // Map to character
    for i := 0; i < len(runes); i += 2 {
        rangeSize := int(runes[i+1] - runes[i] + 1)
        if idx < rangeSize {
            return runes[i] + rune(idx)
        }
        idx -= rangeSize
    }
    panic("unreachable")
}
```

#### Category C: Quantifiers

| Op | Description | Min | Max |
|----|-------------|-----|-----|
| `OpStar` | `*` (zero or more) | 0 | maxRepeat |
| `OpPlus` | `+` (one or more) | 1 | maxRepeat |
| `OpQuest` | `?` (zero or one) | 0 | 1 |
| `OpRepeat` | `{n}`, `{n,m}`, `{n,}` | re.Min | re.Max (or maxRepeat if -1) |

Implementation:
```go
func handleQuantifier(re *syntax.Regexp, r *rng.Rng, maxRepeat int) (string, error) {
    min, max := getRepeatBounds(re, maxRepeat)
    count := min
    if max > min {
        count = min + r.Intn(max - min + 1)
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
```

#### Category D: Composition

| Op | Description | Implementation |
|----|-------------|----------------|
| `OpConcat` | Concatenation | Generate all `Sub` and join |
| `OpAlternate` | Alternation `(a\|b)` | Pick random `Sub` and generate |
| `OpCapture` | Capture group | Generate `Sub[0]` |

```go
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
```

#### Category E: Anchors (No Output)

These are position assertions, not character matchers. Return empty string.

| Op | Description |
|----|-------------|
| `OpBeginLine` | `^` (start of line) |
| `OpEndLine` | `$` (end of line) |
| `OpBeginText` | `\A` (start of text) |
| `OpEndText` | `\z` (end of text) |
| `OpWordBoundary` | `\b` |
| `OpNoWordBoundary` | `\B` |

```go
case syntax.OpBeginLine, syntax.OpEndLine,
     syntax.OpBeginText, syntax.OpEndText,
     syntax.OpWordBoundary, syntax.OpNoWordBoundary:
    return "", nil
```

#### Category F: Error Cases

| Op | Description | Handling |
|----|-------------|----------|
| `OpNoMatch` | Cannot match anything | Return error |

---

### Step 3: Generator Structure

```go
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

func (g *RegexGenerator) Next(r *rng.Rng) (any, error) {
    return generate(g.re, r, g.maxRepeat)
}

func generate(re *syntax.Regexp, r *rng.Rng, maxRepeat int) (string, error) {
    switch re.Op {
    // ... all cases
    }
}

func validateRegexConfig(config map[string]any) (*syntax.Regexp, int, error) {
    // Validate pattern
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

    // Parse regex
    re, err := syntax.Parse(pattern, syntax.Perl)
    if err != nil {
        return nil, 0, fmt.Errorf("regex: invalid pattern: %w", err)
    }

    // Validate max_repeat
    maxRepeat := defaultMaxRepeat
    if val, exists := config["max_repeat"]; exists {
        m, ok := val.(int)
        if !ok {
            return nil, 0, fmt.Errorf("regex: 'max_repeat' must be an integer, got %T", val)
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
```

---

## Test Plan

### 1. Config Tests

| Test Case | Config | Expected |
|-----------|--------|----------|
| Valid simple pattern | `{"pattern": "[a-z]+"}` | Success |
| Valid with max_repeat | `{"pattern": "a*", "max_repeat": 5}` | Success |
| Missing pattern | `{}` | Error |
| Empty pattern | `{"pattern": ""}` | Error |
| Invalid pattern type | `{"pattern": 123}` | Error |
| Invalid regex | `{"pattern": "["}` | Error |
| Invalid max_repeat type | `{"pattern": "a", "max_repeat": "5"}` | Error |
| max_repeat < 1 | `{"pattern": "a", "max_repeat": 0}` | Error |
| max_repeat as float | `{"pattern": "a", "max_repeat": 5.0}` | Error |

### 2. Determinism Tests

| Test Case | Config |
|-----------|--------|
| Simple literal | `{"pattern": "abc"}` |
| Character class | `{"pattern": "[a-z]{5}"}` |
| Alternation | `{"pattern": "(foo\|bar\|baz)"}` |
| Quantifiers | `{"pattern": "\\d{3}-\\d{4}"}` |
| Complex pattern | `{"pattern": "[A-Z]{2}\\d{4}[a-z]?"}` |

### 3. Output Validation Tests

Verify generated strings actually match the pattern:

```go
func TestRegexGenerator_OutputMatchesPattern(t *testing.T) {
    tests := []struct {
        Name    string
        Pattern string
    }{
        {"digits", `\d{5}`},
        {"phone", `\d{3}-\d{3}-\d{4}`},
        {"license plate", `[A-Z]{3}-\d{4}`},
        {"email-like", `[a-z]+@[a-z]+\.com`},
        {"alternation", `(red|green|blue)`},
        {"optional", `colou?r`},
        {"mixed", `[A-Z][a-z]{2,5}\d{2}`},
    }

    for _, tt := range tests {
        t.Run(tt.Name, func(t *testing.T) {
            gen, _ := Get("regex", map[string]any{"pattern": tt.Pattern})
            compiled := regexp.MustCompile("^" + tt.Pattern + "$")

            r := rng.New(testSeed)
            for i := 0; i < testIterations; i++ {
                val, _ := gen.Next(r)
                if !compiled.MatchString(val.(string)) {
                    t.Errorf("generated %q does not match pattern %s", val, tt.Pattern)
                }
            }
        })
    }
}
```

### 4. Range Tests for Quantifiers

```go
func TestRegexGenerator_QuantifierBounds(t *testing.T) {
    tests := []struct {
        Name      string
        Pattern   string
        MaxRepeat int
        MinLen    int
        MaxLen    int
    }{
        {"star default", "a*", 10, 0, 10},
        {"star custom", "a*", 5, 0, 5},
        {"plus default", "a+", 10, 1, 10},
        {"exact", "a{3}", 10, 3, 3},
        {"range", "a{2,4}", 10, 2, 4},
        {"unbounded", "a{2,}", 10, 2, 10},
    }
    // ... verify string lengths fall within bounds
}
```

### 5. Edge Case Tests

| Test Case | Pattern | Notes |
|-----------|---------|-------|
| Literal only | `abc` | No randomness |
| Empty match | `a?` with empty result | Should work |
| Nested groups | `((a\|b)c)+` | Recursion |
| Anchors | `^foo$` | Should ignore anchors |
| Unicode | `[α-ω]+` | Non-ASCII ranges |
| Escape sequences | `\d\w\s` | Character classes |

### 6. Output Type Test

```go
func TestRegexGenerator_OutputType(t *testing.T) {
    gen, _ := Get("regex", map[string]any{"pattern": "[a-z]+"})
    r := rng.New(testSeed)
    val, _ := gen.Next(r)

    if _, ok := val.(string); !ok {
        t.Errorf("expected string, got %T", val)
    }
}
```

---

## Character Ranges for Common Classes

When `syntax.Parse` with `syntax.Perl` flag encounters `\d`, `\w`, `\s`, it expands them to character classes:

| Escape | Expands To |
|--------|------------|
| `\d` | `[0-9]` |
| `\D` | `[^0-9]` |
| `\w` | `[0-9A-Za-z_]` |
| `\W` | `[^0-9A-Za-z_]` |
| `\s` | `[\t\n\f\r ]` |
| `\S` | `[^\t\n\f\r ]` |

The parser handles this automatically - we just see `OpCharClass` with the appropriate ranges.

---

## Edge Cases and Limitations

### Handled

1. **Unbounded quantifiers** - Capped by `max_repeat`
2. **Empty alternatives** - `(|a)` can match empty string
3. **Nested quantifiers** - `(a+)+` works recursively
4. **Unicode ranges** - Rune-based, supports full Unicode

### Not Supported (Return Error)

1. **Backreferences** - `\1` - Cannot generate without tracking captures
2. **Lookahead/lookbehind** - Would need complex state tracking

### Graceful Handling

1. **Anchors** - Ignored (return empty string)
2. **Word boundaries** - Ignored (return empty string)

---

## Implementation Checklist

- [ ] Create `internal/registry/regex.go`
  - [ ] Define `RegexGenerator` struct
  - [ ] Implement `validateRegexConfig()`
  - [ ] Implement `generate()` recursive function
  - [ ] Handle `OpLiteral`
  - [ ] Handle `OpCharClass` with `pickFromCharClass()`
  - [ ] Handle `OpAnyChar` and `OpAnyCharNotNL`
  - [ ] Handle `OpConcat`
  - [ ] Handle `OpAlternate`
  - [ ] Handle `OpCapture`
  - [ ] Handle `OpStar`, `OpPlus`, `OpQuest`, `OpRepeat`
  - [ ] Handle anchors (return empty)
  - [ ] Handle `OpNoMatch` (return error)
  - [ ] Handle `OpEmptyMatch`
  - [ ] Register generator in `init()`

- [ ] Create `internal/registry/regex_test.go`
  - [ ] Config validation tests
  - [ ] Determinism tests
  - [ ] Output validation tests (verify matches pattern)
  - [ ] Quantifier bounds tests
  - [ ] Edge case tests
  - [ ] Output type test

- [ ] Run tests
  - [ ] `go test -v ./internal/registry -run Regex`
  - [ ] `go test ./...` (no regressions)

---

## Example Usage

```json
{
  "entities": [
    {
      "name": "products",
      "count": 1000,
      "fields": [
        {
          "name": "sku",
          "gen": "regex",
          "config": {
            "pattern": "[A-Z]{2}-\\d{6}"
          }
        },
        {
          "name": "phone",
          "gen": "regex",
          "config": {
            "pattern": "\\(\\d{3}\\) \\d{3}-\\d{4}"
          }
        }
      ]
    }
  ]
}
```

Output:
```json
{"sku": "AB-123456", "phone": "(555) 867-5309"}
{"sku": "XY-987654", "phone": "(212) 555-1234"}
```

---

## Verification Commands

```bash
# Run regex tests only
go test -v ./internal/registry -run Regex

# Run all tests to check for regressions
go test ./...

# Verify no new dependencies added
go mod tidy && git diff go.mod go.sum
```
