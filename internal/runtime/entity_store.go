package runtime

import "github.com/compuficial/apery/internal/registry"

// mapEntityStore is the concrete implementation of registry.EntityStore.
// It stores column data keyed by "entity.field" and is populated by the
// executor after each entity finishes generating.
type mapEntityStore struct {
	data map[string][]any
}

func newMapEntityStore() *mapEntityStore {
	return &mapEntityStore{data: make(map[string][]any)}
}

func storeKey(entity, field string) string {
	return entity + "." + field
}

// StoreColumn saves a column of values for later retrieval by rel_ref generators.
func (s *mapEntityStore) StoreColumn(entity, field string, values []any) {
	s.data[storeKey(entity, field)] = values
}

// GetColumn retrieves a previously stored column. Returns false if not found.
func (s *mapEntityStore) GetColumn(entity, field string) ([]any, bool) {
	vals, ok := s.data[storeKey(entity, field)]
	return vals, ok
}

// Verify interface compliance at compile time.
var _ registry.EntityStore = (*mapEntityStore)(nil)
