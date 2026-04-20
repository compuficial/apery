package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"fmt"
	"strings"
)

// TemplateGenerator performs string interpolation using field values from the current row.
type TemplateGenerator struct {
	parts []templatePart
	deps  []string
}

type templatePart struct {
	literal bool
	value   string
}

// Next returns an error because template requires row context.
func (tg *TemplateGenerator) Next(_ *rng.Rng) (any, error) {
	return nil, fmt.Errorf("template: requires row context")
}

// NextWithRow performs string interpolation using values from the row context.
func (tg *TemplateGenerator) NextWithRow(_ *rng.Rng, row RowContext) (any, error) {
	var b strings.Builder
	for _, part := range tg.parts {
		if part.literal {
			b.WriteString(part.value)
			continue
		}
		v, ok := row.Get(part.value)
		if !ok {
			return nil, fmt.Errorf("template: field '%s' not found in row context", part.value)
		}
		b.WriteString(fmt.Sprint(v))
	}
	return b.String(), nil
}

// Dependencies returns the field names this template references.
func (tg *TemplateGenerator) Dependencies() []string {
	return tg.deps
}

// parseTemplate parses a template string into literal and reference parts.
// Supports {field_name} for references and {{ / }} for escaped braces.
func parseTemplate(tpl string) ([]templatePart, []string, error) {
	var parts []templatePart
	var deps []string
	seen := make(map[string]bool)

	i := 0
	var literal strings.Builder

	for i < len(tpl) {
		ch := tpl[i]

		if ch == '{' {
			if i+1 < len(tpl) && tpl[i+1] == '{' {
				literal.WriteByte('{')
				i += 2
				continue
			}

			if literal.Len() > 0 {
				parts = append(parts, templatePart{literal: true, value: literal.String()})
				literal.Reset()
			}

			end := strings.IndexByte(tpl[i+1:], '}')
			if end == -1 {
				return nil, nil, fmt.Errorf("template: unclosed '{' at position %d", i)
			}

			fieldName := tpl[i+1 : i+1+end]

			if strings.ContainsAny(fieldName, "{}") {
				return nil, nil, fmt.Errorf("template: nested braces not allowed at position %d", i)
			}

			if fieldName == "" {
				return nil, nil, fmt.Errorf("template: empty placeholder '{}' at position %d", i)
			}

			parts = append(parts, templatePart{literal: false, value: fieldName})
			if !seen[fieldName] {
				deps = append(deps, fieldName)
				seen[fieldName] = true
			}

			i += 1 + end + 1
			continue
		}

		if ch == '}' {
			if i+1 < len(tpl) && tpl[i+1] == '}' {
				literal.WriteByte('}')
				i += 2
				continue
			}
			return nil, nil, fmt.Errorf("template: unmatched '}' at position %d", i)
		}

		literal.WriteByte(ch)
		i++
	}

	if literal.Len() > 0 {
		parts = append(parts, templatePart{literal: true, value: literal.String()})
	}

	return parts, deps, nil
}

func init() {
	MustRegister("template", func(config map[string]any) (Generator, error) {
		tpl, ok := config["tpl"].(string)
		if !ok {
			return nil, fmt.Errorf("template: 'tpl' must be a string")
		}

		parts, deps, err := parseTemplate(tpl)
		if err != nil {
			return nil, err
		}

		return &TemplateGenerator{parts: parts, deps: deps}, nil
	})
	MustRegisterInfo("template", GeneratorInfo{
		Description: "String interpolation with {field_name} placeholders from the current row",
		ConfigKeys: []ConfigKey{
			{Name: "tpl", Type: "string", Required: true, Desc: "Template string with {field} references"},
		},
		Example: `- name: email
  gen: template
  config:
    tpl: "{username}@{domain}"`,
	})
}
