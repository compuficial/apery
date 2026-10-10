package registry

import (
	"fmt"

	"github.com/compuficial/apery/internal/rng"
)

const (
	distUniform = "uniform"
	distZipf    = "zipf"
)

// RelRefGenerator samples values from a previously generated entity's column.
// The executor binds the column via BindStore before generating.
type RelRefGenerator struct {
	ref  ColumnRef
	dist string    // distUniform or distZipf
	zipf *rng.Zipf // set when dist is distZipf, after binding
	s    float64   // zipf skewness
	pool []any     // bound column
}

// StoreRefs names the column this generator samples from.
func (g *RelRefGenerator) StoreRefs() []ColumnRef {
	return []ColumnRef{g.ref}
}

// BindStore resolves the referenced column once per instance.
func (g *RelRefGenerator) BindStore(store ReadOnlyEntityStore) error {
	col, ok := store.GetColumn(g.ref.Entity, g.ref.Field)
	if !ok {
		return fmt.Errorf("rel_ref: column %s.%s not found in store", g.ref.Entity, g.ref.Field)
	}
	if len(col) == 0 {
		return fmt.Errorf("rel_ref: column %s.%s is empty", g.ref.Entity, g.ref.Field)
	}
	g.bindPool(col)
	return nil
}

func (g *RelRefGenerator) bindPool(pool []any) {
	g.pool = pool
	if g.dist == distZipf {
		g.zipf = rng.NewZipf(g.s, 1.0, uint64(len(pool)-1))
	}
}

// Next returns a value sampled from the referenced entity's column.
func (g *RelRefGenerator) Next(r *rng.Rng) (any, error) {
	if g.pool == nil {
		return nil, fmt.Errorf("rel_ref: entity store not bound (internal error)")
	}
	return g.pool[g.sampleIndex(r)], nil
}

func (g *RelRefGenerator) sampleIndex(r *rng.Rng) int64 {
	if g.dist == distZipf {
		return int64(g.zipf.Uint64(r))
	}
	return r.IntRange(0, int64(len(g.pool)-1))
}

// uniqueRelRef draws distinct values without replacement within a batch (a
// driven_by parent's children, or a whole standalone entity). Uniqueness is
// by value: the pool is deduplicated at bind time.
type uniqueRelRef struct {
	RelRefGenerator

	// Uniform: sparse Fisher-Yates. Positions [0, drawn) of a virtual
	// permutation of pool indices are used; swapped holds the positions that
	// differ from identity, so each draw is O(1) whatever the pool size.
	swapped map[int64]int64
	// Zipf: indices already drawn; collisions probe to the next free index.
	taken map[int64]bool
	drawn int64
}

// BindStore resolves the column and keeps one copy of each distinct value.
func (g *uniqueRelRef) BindStore(store ReadOnlyEntityStore) error {
	if err := g.RelRefGenerator.BindStore(store); err != nil {
		return err
	}

	seen := make(map[any]bool, len(g.pool))
	distinct := make([]any, 0, len(g.pool))
	for _, v := range g.pool {
		switch v.(type) {
		case map[string]any, []any:
			return fmt.Errorf("rel_ref: unique needs scalar values, but %s.%s holds %T", g.ref.Entity, g.ref.Field, v)
		}
		if !seen[v] {
			seen[v] = true
			distinct = append(distinct, v)
		}
	}
	g.bindPool(distinct)
	return nil
}

// Next draws a value not yet drawn in this batch.
func (g *uniqueRelRef) Next(r *rng.Rng) (any, error) {
	if g.pool == nil {
		return nil, fmt.Errorf("rel_ref: entity store not bound (internal error)")
	}
	n := int64(len(g.pool))
	if g.drawn >= n {
		return nil, fmt.Errorf("rel_ref: unique pool exhausted: %s.%s has %d distinct values", g.ref.Entity, g.ref.Field, n)
	}

	var idx int64
	if g.dist == distZipf {
		idx = g.sampleIndex(r)
		for g.taken[idx] {
			idx = (idx + 1) % n
		}
		g.taken[idx] = true
	} else {
		pick := g.drawn + r.IntRange(0, n-1-g.drawn)
		idx = g.at(pick)
		g.swapped[pick] = g.at(g.drawn)
		delete(g.swapped, g.drawn) // position drawn is never read again
	}

	g.drawn++
	return g.pool[idx], nil
}

// at returns the pool index at a permutation position.
func (g *uniqueRelRef) at(pos int64) int64 {
	if idx, ok := g.swapped[pos]; ok {
		return idx
	}
	return pos
}

// Reset starts a new batch.
func (g *uniqueRelRef) Reset() {
	clear(g.swapped)
	clear(g.taken)
	g.drawn = 0
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

		dist := distUniform
		if d, exists := config["distribution"]; exists {
			ds, ok := d.(string)
			if !ok {
				return nil, fmt.Errorf("rel_ref: 'distribution' must be a string")
			}
			if ds != distUniform && ds != distZipf {
				return nil, fmt.Errorf("rel_ref: 'distribution' must be %q or %q, got %q", distUniform, distZipf, ds)
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

		base := RelRefGenerator{ref: ColumnRef{Entity: entity, Field: field}, dist: dist, s: zipfS}
		if !unique {
			return &base, nil
		}
		return &uniqueRelRef{
			RelRefGenerator: base,
			swapped:         make(map[int64]int64),
			taken:           make(map[int64]bool),
		}, nil
	})
	MustRegisterInfo("rel_ref", GeneratorInfo{
		Description: "Foreign key sampling from a previously generated entity",
		ConfigKeys: []ConfigKey{
			{Name: "entity", Type: "string", Required: true, Desc: "Source entity name"},
			{Name: "field", Type: "string", Required: true, Desc: "Source field name"},
			{Name: "distribution", Type: "string", Desc: "Distribution: uniform (default) or zipf", Default: "uniform"},
			{Name: "s", Type: "float", Desc: "Zipf skewness parameter (only with distribution: zipf)", Default: "1.5"},
			{Name: "unique", Type: "bool", Desc: "Draw distinct values without replacement within each parent batch (driven_by) or the whole entity", Default: "false"},
		},
		Example: `- name: user_id
  gen: rel_ref
  config:
    entity: User
    field: id
    distribution: zipf
    s: 1.5`,
	})
}
