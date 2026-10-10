package main

import "github.com/compuficial/apery/internal/registry"

// namePattern mirrors the plan validator's identifier rule.
const namePattern = `^[A-Za-z_][A-Za-z0-9_.-]*$`

// jsonTypes maps ConfigKey.Type to JSON Schema types. Unlisted types
// (e.g. "any") accept any value.
var jsonTypes = map[string]string{
	"int":            "integer",
	"float":          "number",
	"string":         "string",
	"bool":           "boolean",
	"[]any":          "array",
	"map[string]any": "object",
}

type schema = map[string]any

// planSchema builds a JSON Schema for plan files: the plan structure plus
// one field variant per registered generator, with that generator's config
// keys. Nested generator specs inside config are typed loosely; 'apery
// validate' checks them fully.
func planSchema() schema {
	var variants []any
	for _, info := range registry.ListGenerators() {
		variants = append(variants, generatorSchema(info))
	}

	return schema{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"title":                "Apery plan",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"entities"},
		"properties": schema{
			"seed":     schema{"type": "integer", "description": "Root seed; same plan and seed give identical output (default 0)"},
			"entities": schema{"type": "array", "minItems": 1, "items": schema{"$ref": "#/$defs/entity"}},
		},
		"$defs": schema{
			"name": schema{"type": "string", "pattern": namePattern},
			"entity": schema{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"name", "fields"},
				"oneOf":                []any{schema{"required": []string{"count"}}, schema{"required": []string{"driven_by"}}},
				"properties": schema{
					"name":      schema{"$ref": "#/$defs/name"},
					"count":     schema{"type": "integer", "minimum": 1},
					"driven_by": schema{"$ref": "#/$defs/driven_by"},
					"fields":    schema{"type": "array", "minItems": 1, "items": schema{"$ref": "#/$defs/field"}},
				},
			},
			"driven_by": schema{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"entity", "field", "as", "min", "max"},
				"properties": schema{
					"entity":   schema{"$ref": "#/$defs/name"},
					"field":    schema{"type": "string"},
					"as":       schema{"$ref": "#/$defs/name"},
					"min":      schema{"type": "integer", "minimum": 1},
					"max":      schema{"type": "integer", "minimum": 1},
					"index_as": schema{"$ref": "#/$defs/name"},
					"expose": schema{"type": "array", "items": schema{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"field"},
						"properties": schema{
							"field": schema{"type": "string"},
							"as":    schema{"$ref": "#/$defs/name"},
						},
					}},
				},
			},
			"field": schema{"oneOf": variants},
		},
	}
}

// generatorSchema is the field variant for one generator.
func generatorSchema(info registry.GeneratorInfo) schema {
	props := schema{}
	var required []string
	for _, ck := range info.ConfigKeys {
		prop := schema{"description": ck.Desc}
		if t, ok := jsonTypes[ck.Type]; ok {
			prop["type"] = t
		}
		props[ck.Name] = prop
		if ck.Required {
			required = append(required, ck.Name)
		}
	}

	config := schema{"type": "object", "additionalProperties": false, "properties": props}
	if len(required) > 0 {
		config["required"] = required
	}

	fieldRequired := []string{"name", "gen"}
	if len(required) > 0 {
		fieldRequired = append(fieldRequired, "config")
	}
	return schema{
		"type":                 "object",
		"description":          info.Description,
		"additionalProperties": false,
		"required":             fieldRequired,
		"properties": schema{
			"name":   schema{"$ref": "#/$defs/name"},
			"gen":    schema{"const": info.Name},
			"config": config,
		},
	}
}
