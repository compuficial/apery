package writer

import (
	"encoding/json"
	"slices"
)

// OrderedMap preserves insertion order
type OrderedMap struct {
	keys   []string
	values map[string]any
}

// NewOrderedMap creates a new ordered map
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{
		keys:   make([]string, 0),
		values: make(map[string]any),
	}
}

// Set adds or updates a key-value pair
func (om *OrderedMap) Set(key string, value any) {
	if _, exists := om.values[key]; !exists {
		om.keys = append(om.keys, key)
	}
	om.values[key] = value
}

// MarshalJSON implements json.Marshaler interface
func (om *OrderedMap) MarshalJSON() ([]byte, error) {
	// Build JSON manually in key order
	result := "{"
	for i, key := range om.keys {
		if i > 0 {
			result += ","
		}

		keyJSON, _ := json.Marshal(key)
		valJSON, _ := json.Marshal(om.values[key])

		result += string(keyJSON) + ":" + string(valJSON)
	}
	result += "}"

	return []byte(result), nil
}

// Prepend adds a key-value pair at the beginning
func (om *OrderedMap) Prepend(key string, value any) {
	if _, exists := om.values[key]; exists {
		for i, k := range om.keys {
			if k == key {
				om.keys = slices.Delete(om.keys, i, i+1)
				break
			}
		}
	}
	om.keys = append([]string{key}, om.keys...)
	om.values[key] = value
}
