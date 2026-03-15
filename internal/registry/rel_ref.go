package registry

import (
	"apery/internal/rng"
	"fmt"
)

const maxUniqueRetries = 100

// RelRefGenerator samples values from a previously generated entity's column.
// It reads from a ReadOnlyEntityStore injected via the "_store" config key.
type RelRefGenerator struct {
	entity string
	field  string
	store  ReadOnlyEntityStore
	dist   string // "uniform" or "zipf"
	zipfS  float64

	// unique tracking (stateful across Next calls within a parent batch)
	unique bool
	seen   map[any]bool
}

// Next returns a value sampled from the referenced entity's column.
// Returns an error if the store has not been injected.
func (g *RelRefGenerator) Next(r *rng.Rng) (any, error) {
	if g.store == nil {
		return nil, fmt.Errorf("rel_ref: entity store not available (internal error)")
	}

	col, ok := g.store.GetColumn(g.entity, g.field)
	if !ok {
		return nil, fmt.Errorf("rel_ref: column %s.%s not found in store", g.entity, g.field)
	}
	if len(col) == 0 {
		return nil, fmt.Errorf("rel_ref: column %s.%s is empty", g.entity, g.field)
	}

	n := int64(len(col))

	if !g.unique {
		idx := g.sampleIndex(r, n)
		return col[idx], nil
	}

	// Unique mode: retry on collision.
	for range maxUniqueRetries {
		idx := g.sampleIndex(r, n)
		val := col[idx]
		if !g.seen[val] {
			g.seen[val] = true
			return val, nil
		}
	}

	return nil, fmt.Errorf("rel_ref: unique constraint failed after %d retries (pool: %s.%s)",
		maxUniqueRetries, g.entity, g.field)
}

// Reset clears the uniqueness tracker between parent batches.
func (g *RelRefGenerator) Reset() {
	if g.seen != nil {
		clear(g.seen)
	}
}

func (g *RelRefGenerator) sampleIndex(r *rng.Rng, n int64) int64 {
	if g.dist == "zipf" {
		z := r.NewZipf(g.zipfS, 1.0, uint64(n-1))
		return int64(z.Uint64())
	}
	return r.IntRange(0, n-1)
}

func init() {
	MustRegister("rel_ref", func(config map[string]any) (Generator, error) {
		entity, ok := config["entity"].(string)
		if !ok || entity == "" {
			return nil, fmt.Errorf("rel_ref: 'entity' is required and must be a string")
		}
		field, ok := config["field"].(string)
		if !ok || field == "" {
			return nil, fmt.Errorf("rel_ref: 'field' is required and must be a string")
		}

		dist := "uniform"
		if d, exists := config["distribution"]; exists {
			ds, ok := d.(string)
			if !ok {
				return nil, fmt.Errorf("rel_ref: 'distribution' must be a string")
			}
			if ds != "uniform" && ds != "zipf" {
				return nil, fmt.Errorf("rel_ref: 'distribution' must be \"uniform\" or \"zipf\", got %q", ds)
			}
			dist = ds
		}

		zipfS := 1.5
		if s, exists := config["s"]; exists {
			v, err := extractFloat(s, "s", "rel_ref")
			if err != nil {
				return nil, err
			}
			if v <= 1 {
				return nil, fmt.Errorf("rel_ref: 's' must be > 1, got %f", v)
			}
			zipfS = v
		}

		unique := false
		if u, exists := config["unique"]; exists {
			ub, ok := u.(bool)
			if !ok {
				return nil, fmt.Errorf("rel_ref: 'unique' must be a bool")
			}
			unique = ub
		}

		// Store is optional at factory time (nil during initFields validation).
		var store ReadOnlyEntityStore
		if s, exists := config["_store"]; exists && s != nil {
			store, ok = s.(ReadOnlyEntityStore)
			if !ok {
				return nil, fmt.Errorf("rel_ref: '_store' has wrong type")
			}
		}

		gen := &RelRefGenerator{
			entity: entity,
			field:  field,
			store:  store,
			dist:   dist,
			zipfS:  zipfS,
			unique: unique,
		}
		if unique {
			gen.seen = make(map[any]bool)
		}

		return gen, nil
	})
}
