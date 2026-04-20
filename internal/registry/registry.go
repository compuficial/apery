// Package registry provides the generator plugin system for synthetic data generation.
//
// Generators implement the Generator interface and register themselves via init()
// functions. The registry uses a factory pattern where each generator name maps to
// a factory function that creates instances from configuration maps.
//
// Built-in generators include: seq (sequential), pick (random selection), bool
// (weighted boolean), int, float, and uuid.
package registry

import (
	"github.com/compuficial/apery/internal/rng"
	"fmt"
	"sort"
)

// GeneratorInfo describes a generator for CLI help and agent discovery.
type GeneratorInfo struct {
	Name        string
	Description string
	ConfigKeys  []ConfigKey
	Example     string
}

// ConfigKey documents a single configuration parameter.
type ConfigKey struct {
	Name     string
	Type     string
	Required bool
	Default  string
	Desc     string
}

// Generator creates values
type Generator interface {
	Next(r *rng.Rng) (any, error)
}

// Factory creates a generator from config
type Factory func(config map[string]any) (Generator, error)

// Registry holds all generators
var generators = make(map[string]Factory)

var generatorInfos = make(map[string]GeneratorInfo)

// Register adds a generator
func Register(name string, factory Factory) error {
	if name == "" {
		return fmt.Errorf("registry: generator name is empty")
	}
	if factory == nil {
		return fmt.Errorf("registry: factory is nil for %q", name)
	}
	if _, exists := generators[name]; exists {
		return fmt.Errorf("registry: generator %q already registered", name)
	}
	generators[name] = factory
	return nil
}

// MustRegister adds a generator and panics on error
func MustRegister(name string, factory Factory) {
	if err := Register(name, factory); err != nil {
		panic(err)
	}
}

// Get retrieves a generator by name
func Get(name string, config map[string]any) (Generator, error) {
	factory, ok := generators[name]
	if !ok {
		return nil, fmt.Errorf("registry: generator %q not found", name)
	}
	return factory(config)
}

// MustRegisterInfo adds metadata for a previously registered generator and panics if the generator is unknown.
func MustRegisterInfo(name string, info GeneratorInfo) {
	if _, ok := generators[name]; !ok {
		panic(fmt.Sprintf("registry: cannot register info for unknown generator %q", name))
	}
	info.Name = name
	generatorInfos[name] = info
}

// ListGenerators returns all generators with registered metadata, sorted by name.
func ListGenerators() []GeneratorInfo {
	infos := make([]GeneratorInfo, 0, len(generatorInfos))
	for _, info := range generatorInfos {
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	return infos
}

// GetInfo returns the metadata for a named generator.
func GetInfo(name string) (GeneratorInfo, bool) {
	info, ok := generatorInfos[name]
	return info, ok
}

// FactoryFor retrieves the factory function by generator name.
func FactoryFor(name string) (Factory, error) {
	factory, ok := generators[name]
	if !ok {
		return nil, fmt.Errorf("registry: generator %q not found", name)
	}
	return factory, nil
}

// RowContext provides read access to already-generated field values in the current row.
type RowContext interface {
	Get(fieldName string) (any, bool)
}

// RowAwareGenerator is a generator that needs access to other fields in the current row.
// The executor calls NextWithRow instead of Next when this interface is satisfied.
// Next() should return an error as a safety net for incorrect usage (e.g., nesting inside object/list).
type RowAwareGenerator interface {
	Generator
	NextWithRow(r *rng.Rng, row RowContext) (any, error)
}

// DependencyDeclarer is implemented by generators that reference other fields in the row.
// The executor validates that all declared dependencies appear earlier in the field list.
type DependencyDeclarer interface {
	Dependencies() []string
}

// ReadOnlyEntityStore provides read access to columns from previously generated entities.
// Generators (e.g., rel_ref) use this to sample foreign key values.
type ReadOnlyEntityStore interface {
	// GetColumn retrieves a previously stored column. Returns false if not found.
	GetColumn(entity, field string) ([]any, bool)
}

// EntityStore extends ReadOnlyEntityStore with write access.
// The executor populates the store after each entity completes generation.
type EntityStore interface {
	ReadOnlyEntityStore
	// StoreColumn saves all values for the given entity and field.
	StoreColumn(entity, field string, values []any)
}

// Resettable is implemented by generators with internal state that must be
// cleared between parent batches in driven_by entities (e.g., unique trackers).
type Resettable interface {
	Reset()
}
