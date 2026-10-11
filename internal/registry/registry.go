// Package registry provides the generator plugin system for synthetic data generation.
//
// Generators implement the Generator interface and register themselves via init()
// functions. The registry uses a factory pattern where each generator name maps to
// a factory function that creates instances from configuration maps. Get checks
// config keys against each generator's GeneratorInfo before calling its factory.
//
// Run `apery list generators` for the built-in catalog.
package registry

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/compuficial/apery/internal/rng"
	"github.com/compuficial/apery/internal/suggest"
)

// GeneratorInfo describes a generator for CLI help and agent discovery.
type GeneratorInfo struct {
	Name        string      `json:"name" yaml:"name"`
	Description string      `json:"description" yaml:"description"`
	ConfigKeys  []ConfigKey `json:"config_keys" yaml:"config_keys"`
	Example     string      `json:"example" yaml:"example"`
}

// ConfigKey documents a single configuration parameter.
type ConfigKey struct {
	Name     string `json:"name" yaml:"name"`
	Type     string `json:"type" yaml:"type"`
	Required bool   `json:"required" yaml:"required"`
	Default  string `json:"default,omitempty" yaml:"default,omitempty"`
	Desc     string `json:"description" yaml:"description"`
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

// Get builds a generator by name after rejecting config keys its
// GeneratorInfo doesn't declare. Keys starting with "_" are reserved for the
// executor and always pass.
func Get(name string, config map[string]any) (Generator, error) {
	factory, err := FactoryFor(name)
	if err != nil {
		return nil, err
	}
	if err := checkConfigKeys(name, config); err != nil {
		return nil, err
	}
	return factory(config)
}

// UnknownGeneratorError reports a generator name the registry doesn't know.
type UnknownGeneratorError struct {
	Name       string
	Suggestion string // closest registered name, or ""
}

func (e *UnknownGeneratorError) Error() string {
	msg := fmt.Sprintf("registry: generator %q not found", e.Name)
	if e.Suggestion != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", e.Suggestion)
	}
	return msg
}

// UnknownKey is one undeclared config key and its closest valid key.
type UnknownKey struct {
	Name       string
	Suggestion string // "" when nothing is close
}

// UnknownKeyError reports config keys a generator doesn't declare.
type UnknownKeyError struct {
	Generator string
	Keys      []UnknownKey
	Valid     []string // every key the generator accepts
}

// Error lists each unknown key with its suggestion, e.g.
// int: unknown config key "mn" (did you mean "min"?)
func (e *UnknownKeyError) Error() string {
	parts := make([]string, len(e.Keys))
	for i, k := range e.Keys {
		parts[i] = fmt.Sprintf("%q", k.Name)
		if k.Suggestion != "" {
			parts[i] += fmt.Sprintf(" (did you mean %q?)", k.Suggestion)
		}
	}
	noun := "key"
	if len(e.Keys) > 1 {
		noun = "keys"
	}
	return fmt.Sprintf("%s: unknown config %s %s", e.Generator, noun, strings.Join(parts, ", "))
}

// Hint lists the keys the generator accepts.
func (e *UnknownKeyError) Hint() string {
	if len(e.Valid) == 0 {
		return e.Generator + " takes no config"
	}
	return "valid keys: " + strings.Join(e.Valid, ", ")
}

// checkConfigKeys reports every undeclared key, in sorted order so the
// error is stable, each with a spelling suggestion when one is close.
func checkConfigKeys(name string, config map[string]any) error {
	info, ok := generatorInfos[name]
	if !ok {
		return nil
	}

	allowed := make([]string, len(info.ConfigKeys))
	for i, ck := range info.ConfigKeys {
		allowed[i] = ck.Name
	}

	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var unknown []UnknownKey
	for _, key := range keys {
		if strings.HasPrefix(key, "_") || contains(allowed, key) {
			continue
		}
		unknown = append(unknown, UnknownKey{Name: key, Suggestion: suggest.Closest(key, allowed)})
	}
	if len(unknown) == 0 {
		return nil
	}
	return &UnknownKeyError{Generator: name, Keys: unknown, Valid: allowed}
}

func contains(list []string, s string) bool {
	return slices.Contains(list, s)
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
		names := make([]string, 0, len(generatorInfos))
		for n := range generatorInfos {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, &UnknownGeneratorError{Name: name, Suggestion: suggest.Closest(name, names)}
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

// Resettable is implemented by generators with per-batch state (e.g. unique
// rel_ref). In driven_by entities each parent's children form a batch and the
// executor resets the generator between them; a standalone entity is one batch.
type Resettable interface {
	Reset()
}

// Seekable is implemented by generators whose value depends on the row's
// position (e.g. seq). The executor seeks each chunk's instance to its first
// row so output is independent of chunking.
type Seekable interface {
	SeekRow(row int64) error
}

// Sequential is implemented by generators whose value builds on the row
// before (e.g. time_seq with exponential gaps), so no chunk can start on its
// own. The executor runs the entity's rows in order on one worker, with one
// instance for every chunk.
type Sequential interface {
	Generator
	buildsOnPreviousRow()
}

// DateProducer is implemented by generators whose values are dates written
// in one Go time layout (e.g. time, time_seq).
type DateProducer interface {
	DateLayout() string
}

// DateConsumer is implemented by generators that parse a column of their row
// as a date (e.g. date_offset). Validation passes the layout of every earlier
// column known to hold dates; an error means the column can't be parsed, so
// the plan fails before any row is generated.
type DateConsumer interface {
	CheckDateLayouts(layouts map[string]string) error
}

// ColumnRef names a column of a previously generated entity.
type ColumnRef struct {
	Entity string
	Field  string
}

// StoreConsumer is implemented by generators that read columns of earlier
// entities (e.g. rel_ref). The executor keeps the columns named by StoreRefs
// and binds the store to each chunk's instance before generating.
type StoreConsumer interface {
	StoreRefs() []ColumnRef
	BindStore(store ReadOnlyEntityStore) error
}
