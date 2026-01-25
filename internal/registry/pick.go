package registry

import (
	"apery/internal/rng"
	"bufio"
	"fmt"
	"os"
	"strings"
)

// PickGenerator randomly selects values from a configured list
type PickGenerator struct {
	values []any
}

// Next returns a random value from the list
func (p *PickGenerator) Next(r *rng.Rng) (any, error) {
	ran := r.Intn(len(p.values))
	return p.values[ran], nil
}

// validatePickConfig validates and parses config for pick generator
func validatePickConfig(config map[string]any) ([]any, error) {
	hasValues := config["values"] != nil
	hasFile := config["file"] != nil

	// Must have exactly one source
	if !hasValues && !hasFile {
		return nil, fmt.Errorf("pick: must specify 'values' or 'file'")
	}
	if hasValues && hasFile {
		return nil, fmt.Errorf("pick: cannot specify both 'values' and 'file'")
	}

	if hasValues {
		return validatePickValues(config["values"])
	}
	return loadPickFile(config["file"])
}

// validatePickValues validates the inline values array
func validatePickValues(val any) ([]any, error) {
	values, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("pick: 'values' must be an array, got %T", val)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("pick: 'values' cannot be empty")
	}
	return values, nil
}

// loadPickFile loads values from a file, one per line
func loadPickFile(fileVal any) ([]any, error) {
	path, ok := fileVal.(string)
	if !ok {
		return nil, fmt.Errorf("pick: 'file' must be a string, got %T", fileVal)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("pick: cannot open file: %w", err)
	}
	defer file.Close()

	var values []any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // skip empty lines
		}
		values = append(values, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("pick: error reading file: %w", err)
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("pick: file is empty or contains only blank lines")
	}

	return values, nil
}

func init() {
	Register("pick", func(config map[string]any) (Generator, error) {
		values, err := validatePickConfig(config)
		if err != nil {
			return nil, err
		}
		return &PickGenerator{values: values}, nil
	})
}
