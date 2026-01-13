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
func Register(name string, factory Factory) {
	generators[name] = factory
}

// Get retrieves a generator by name
func Get(name string, config map[string]any) (Generator, error) {
	factory, ok := generators[name]
	if !ok {
		return nil, fmt.Errorf("generator %q not found", name)
	}
	return factory(config)
}
