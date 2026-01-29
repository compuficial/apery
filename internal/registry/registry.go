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
