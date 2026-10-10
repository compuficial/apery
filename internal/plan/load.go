package plan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFile reads a plan from a YAML or JSON file.
// Format is detected by file extension: .yaml/.yml for YAML, .json for JSON.
// Decoding is strict: keys the plan schema doesn't define are errors.
func LoadFile(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan file: %w", err)
	}

	var p Plan
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		err = decodeYAML(data, &p)
	case ".json":
		err = decodeJSON(data, &p)
	default:
		return nil, fmt.Errorf("unsupported plan file extension %q (use .yaml, .yml, or .json)", ext)
	}
	if err != nil {
		return nil, err
	}

	p.BaseDir = filepath.Dir(path)
	if err := Validate(&p); err != nil {
		return nil, err
	}

	return &p, nil
}

func decodeYAML(data []byte, p *Plan) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(p); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("parse YAML plan: %w", err)
	}
	return nil
}

// decodeJSON keeps config numbers exact: integers become int and other
// numbers float64, matching what the YAML decoder produces.
func decodeJSON(data []byte, p *Plan) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(p); err != nil {
		return fmt.Errorf("parse JSON plan: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("parse JSON plan: unexpected data after the plan object")
	}

	for i := range p.Entities {
		for j := range p.Entities[i].Fields {
			normalizeNumbers(p.Entities[i].Fields[j].Config)
		}
	}
	return nil
}

// normalizeNumbers replaces json.Number values, recursively, with int when
// integral and in range, or float64 otherwise.
func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			if int64(int(i)) == i {
				return int(i)
			}
			return i
		}
		f, _ := x.Float64()
		return f
	case map[string]any:
		for k, val := range x {
			x[k] = normalizeNumbers(val)
		}
	case []any:
		for i, val := range x {
			x[i] = normalizeNumbers(val)
		}
	}
	return v
}
