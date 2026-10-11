package registry

import (
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/compuficial/apery/internal/rng"
	"github.com/compuficial/apery/internal/suggest"
)

// specKeys are the keys a composite child spec may have.
var specKeys = []string{"gen", "config", "null_rate"}

// nesting says which generators a composite may contain.
type nesting int

const (
	// nestPure allows only generators whose value is a pure function of the
	// row seed (object, list, one_of).
	nestPure nesting = iota
	// nestRowAware also allows generators that read the current row (switch,
	// which passes the row through to its cases).
	nestRowAware
)

// buildSubGenerator builds a composite's child from a {gen, config,
// null_rate} spec. where prefixes errors, e.g. "object: field 'zip'". The
// child inherits the parent's run Env. Children that keep per-row state or
// read other entities are rejected: composites build them per chunk and
// never seek or bind them, so their output would depend on chunking.
func buildSubGenerator(raw any, parent map[string]any, where string, rule nesting) (Generator, error) {
	spec, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a map, got %T", where, raw)
	}
	if err := checkSpecKeys(spec, where); err != nil {
		return nil, err
	}

	genName, ok := spec["gen"].(string)
	if !ok || genName == "" {
		return nil, fmt.Errorf("%s missing 'gen'", where)
	}

	config, err := childConfig(spec["config"], parent, where)
	if err != nil {
		return nil, err
	}

	gen, err := Get(genName, config)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", where, err)
	}
	if err := checkNestable(gen, genName, where, rule); err != nil {
		return nil, err
	}

	rate, err := childNullRate(spec["null_rate"], where)
	if err != nil {
		return nil, err
	}
	if rate > 0 {
		return withNullRate(gen, rate), nil
	}
	return gen, nil
}

// childNullRate reads an optional null_rate in [0, 1].
func childNullRate(raw any, where string) (float64, error) {
	if raw == nil {
		return 0, nil
	}
	rate, err := extractFloat(raw, "null_rate", where)
	if err != nil {
		return 0, err
	}
	if !(rate >= 0 && rate <= 1) {
		return 0, fmt.Errorf("%s: 'null_rate' must be between 0 and 1, got %v", where, rate)
	}
	return rate, nil
}

// checkSpecKeys rejects keys outside specKeys, so a misspelled "confg"
// can't silently drop the child's settings.
func checkSpecKeys(spec map[string]any, where string) error {
	keys := make([]string, 0, len(spec))
	for key := range spec {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if slices.Contains(specKeys, key) {
			continue
		}
		msg := fmt.Sprintf("%s: unknown key %q", where, key)
		if guess := suggest.Closest(key, specKeys); guess != "" {
			msg += fmt.Sprintf(" (did you mean %q?)", guess)
		}
		return fmt.Errorf("%s; a generator spec takes only gen, config, and null_rate", msg)
	}
	return nil
}

// withNullRate makes a child's value null at rate. The decision draws from
// its own stream, Derive(childSeed, "null"), after the child runs, so values
// that aren't null are the same as without null_rate, as for top-level
// fields. A row-aware child (a switch case) stays row-aware.
func withNullRate(gen Generator, rate float64) Generator {
	n := nullable{gen: gen, rate: rate, decide: rng.New(0)}
	if ra, ok := gen.(RowAwareGenerator); ok {
		return &nullableRowAware{nullable: n, rowAware: ra}
	}
	return &n
}

// nullable is a composite child with a null_rate.
type nullable struct {
	gen    Generator
	rate   float64
	decide *rng.Rng // reseeded per value, for the null decision
}

func (n *nullable) Next(r *rng.Rng) (any, error) {
	seed := r.GetSeed()
	val, err := n.gen.Next(r)
	return n.apply(seed, val, err)
}

// apply replaces val with null at the rate, deciding from seed.
func (n *nullable) apply(seed rng.Seed, val any, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	n.decide.Reseed(rng.Derive(seed, "null"))
	if n.decide.Float64() < n.rate {
		return nil, nil
	}
	return val, nil
}

// nullableRowAware is a nullable switch case that reads the row.
type nullableRowAware struct {
	nullable
	rowAware RowAwareGenerator
}

func (n *nullableRowAware) NextWithRow(r *rng.Rng, row RowContext) (any, error) {
	seed := r.GetSeed()
	val, err := n.rowAware.NextWithRow(r, row)
	return n.apply(seed, val, err)
}

// Dependencies passes on the wrapped child's row dependencies.
func (n *nullableRowAware) Dependencies() []string {
	if dd, ok := n.gen.(DependencyDeclarer); ok {
		return dd.Dependencies()
	}
	return nil
}

// childConfig returns the child's config with the parent's Env added.
func childConfig(raw any, parent map[string]any, where string) (map[string]any, error) {
	config := map[string]any{}
	if raw != nil {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: 'config' must be a map, got %T", where, raw)
		}
		config = m
	}

	env, ok := parent[EnvKey]
	if !ok {
		return config, nil
	}
	withEnv := make(map[string]any, len(config)+1)
	maps.Copy(withEnv, config)
	withEnv[EnvKey] = env
	return withEnv, nil
}

func checkNestable(gen Generator, name, where string, rule nesting) error {
	if _, ok := gen.(StoreConsumer); ok {
		return fmt.Errorf("%s: %q reads other entities and must be a top-level field", where, name)
	}
	_, seekable := gen.(Seekable)
	_, resettable := gen.(Resettable)
	_, sequential := gen.(Sequential)
	if seekable || resettable || sequential {
		return fmt.Errorf("%s: %q keeps per-row state, so nested output would depend on chunk size; use it as a top-level field", where, name)
	}
	if _, ok := gen.(RowAwareGenerator); ok && rule == nestPure {
		return fmt.Errorf("%s: %q needs the current row and must be a top-level field or a switch case", where, name)
	}
	return nil
}
