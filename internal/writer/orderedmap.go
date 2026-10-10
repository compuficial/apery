package writer

import "slices"

// indexThreshold is the key count past which lookups switch from a linear
// scan to an index map. Most records are narrower, so most never build one.
const indexThreshold = 16

// OrderedMap preserves insertion order. Keys and values live in parallel
// slices; the zero value is ready to use.
type OrderedMap struct {
	keys   []string
	values []any
	index  map[string]int // nil until the record outgrows linear scan
}

// NewOrderedMap creates a new ordered map
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{}
}

// NewOrderedMapSize creates an ordered map with room for n keys.
func NewOrderedMapSize(n int) *OrderedMap {
	return &OrderedMap{keys: make([]string, 0, n), values: make([]any, 0, n)}
}

// find returns the position of key, or -1.
func (om *OrderedMap) find(key string) int {
	if om.index != nil {
		if i, ok := om.index[key]; ok {
			return i
		}
		return -1
	}
	for i, k := range om.keys {
		if k == key {
			return i
		}
	}
	return -1
}

func (om *OrderedMap) buildIndex() {
	om.index = make(map[string]int, 2*len(om.keys))
	for i, k := range om.keys {
		om.index[k] = i
	}
}

// Clone returns a shallow copy of the ordered map
func (om *OrderedMap) Clone() *OrderedMap {
	clone := &OrderedMap{keys: slices.Clone(om.keys), values: slices.Clone(om.values)}
	if om.index != nil {
		clone.buildIndex()
	}
	return clone
}

// Set adds or updates a key-value pair
func (om *OrderedMap) Set(key string, value any) {
	if i := om.find(key); i >= 0 {
		om.values[i] = value
		return
	}

	om.keys = append(om.keys, key)
	om.values = append(om.values, value)
	switch {
	case om.index != nil:
		om.index[key] = len(om.keys) - 1
	case len(om.keys) > indexThreshold:
		om.buildIndex()
	}
}

// Keys returns a copy of the keys in insertion order
func (om *OrderedMap) Keys() []string {
	return slices.Clone(om.keys)
}

// Get returns a value for a key and whether it exists
func (om *OrderedMap) Get(key string) (any, bool) {
	i := om.find(key)
	if i < 0 {
		return nil, false
	}
	return om.values[i], true
}

// MarshalJSON implements json.Marshaler, keeping key order.
func (om *OrderedMap) MarshalJSON() ([]byte, error) {
	return appendObject(nil, om, "")
}

// Prepend adds a key-value pair at the beginning
func (om *OrderedMap) Prepend(key string, value any) {
	if i := om.find(key); i >= 0 {
		om.keys = slices.Delete(om.keys, i, i+1)
		om.values = slices.Delete(om.values, i, i+1)
	}
	om.keys = slices.Insert(om.keys, 0, key)
	om.values = slices.Insert(om.values, 0, value)
	if om.index != nil {
		om.buildIndex()
	}
}
