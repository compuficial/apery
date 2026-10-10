package registry

import (
	"fmt"
	"maps"
	"sort"

	"github.com/compuficial/apery/internal/suggest"
)

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

// buildSubGenerator builds a composite's child from a {gen, config} spec.
// where prefixes errors, e.g. "object: field 'zip'". The child inherits the
// parent's run Env. Children that keep per-row state or read other entities
// are rejected: composites build them per chunk and never seek or bind them,
// so their output would depend on chunking.
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
	return gen, nil
}

// checkSpecKeys rejects anything but gen and config, so a misspelled
// "confg" can't silently drop the child's settings.
func checkSpecKeys(spec map[string]any, where string) error {
	keys := make([]string, 0, len(spec))
	for key := range spec {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if key == "gen" || key == "config" {
			continue
		}
		msg := fmt.Sprintf("%s: unknown key %q", where, key)
		if guess := suggest.Closest(key, []string{"gen", "config"}); guess != "" {
			msg += fmt.Sprintf(" (did you mean %q?)", guess)
		}
		return fmt.Errorf("%s; a generator spec takes only gen and config", msg)
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
	if seekable || resettable {
		return fmt.Errorf("%s: %q keeps per-row state, so nested output would depend on chunk size; use it as a top-level field", where, name)
	}
	if _, ok := gen.(RowAwareGenerator); ok && rule == nestPure {
		return fmt.Errorf("%s: %q needs the current row and must be a top-level field or a switch case", where, name)
	}
	return nil
}
