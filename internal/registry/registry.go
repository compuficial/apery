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
	"apery/internal/rng"
	"fmt"
)

// Generator creates values
type Generator interface {
	Next(r *rng.Rng) (any, error)
}

// Factory creates a generator from config
type Factory func(config map[string]any) (Generator, error)

// Registry holds all generators
var generators = make(map[string]Factory)

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
