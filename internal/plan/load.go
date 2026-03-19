package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFile reads a plan from a YAML or JSON file.
// Format is detected by file extension: .yaml/.yml for YAML, .json for JSON.
func LoadFile(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan file: %w", err)
	}

	var p Plan
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("parse YAML plan: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("parse JSON plan: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported plan file extension %q (use .yaml, .yml, or .json)", ext)
	}

	if err := Validate(&p); err != nil {
		return nil, err
	}

	return &p, nil
}
