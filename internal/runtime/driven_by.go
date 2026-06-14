package runtime

import (
	"context"
	"fmt"
	"sort"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/registry"
	"github.com/compuficial/apery/internal/rng"
	"github.com/compuficial/apery/internal/writer"
)

// parentInjection is a parent column auto-injected into every child row.
// The first is the join key (As); the rest come from DrivenBy.Expose.
type parentInjection struct {
	as     string
	values []any
}

// drivenByLayout holds the precomputed child counts and prefix sums
// for a driven_by entity's two-phase execution.
type drivenByLayout struct {
	counts     []int64           // number of children per parent
	prefixSum  []int64           // prefixSum[i] = total rows before parent i
	total      int64             // total child rows across all parents
	injections []parentInjection // parent columns to inject (join key first)
}

// computeDrivenByLayout runs Phase 1: determines child counts per parent.
// The parent count comes from the first injection (the join key).
func computeDrivenByLayout(entitySeed rng.Seed, db *plan.DrivenBy, injections []parentInjection) *drivenByLayout {
	n := int64(len(injections[0].values)) // injections[0] is the join key: one value per parent
	counts := make([]int64, n)
	prefixSum := make([]int64, n)
	var total int64

	countBaseSeed := rng.Derive(entitySeed, "counts")
	for i := int64(0); i < n; i++ {
		countSeed := rng.DeriveIndex(countBaseSeed, i)
		r := rng.New(countSeed)
		count := db.Min
		if db.Max > db.Min {
			count = r.IntRange(db.Min, db.Max)
		}
		counts[i] = count
		prefixSum[i] = total
		total += count
	}

	return &drivenByLayout{
		counts:     counts,
		prefixSum:  prefixSum,
		total:      total,
		injections: injections,
	}
}

// parentForRow returns the parent index for a given global row index
// using binary search on the prefix sum. Used only during chunk boundary
// alignment; the hot path uses linear tracking instead.
func (l *drivenByLayout) parentForRow(globalRow int64) int64 {
	idx := sort.Search(len(l.prefixSum), func(i int) bool {
		return l.prefixSum[i] > globalRow
	})
	return int64(idx - 1)
}

// makeDrivenByChunks creates parent-aligned chunks when unique fields exist,
// or standard row-based chunks otherwise.
func makeDrivenByChunks(layout *drivenByLayout, chunkSize int64, needsAlignment bool) []chunk {
	if !needsAlignment {
		return makeChunks(layout.total, chunkSize)
	}

	// Parent-aligned: each chunk boundary falls on a parent boundary.
	var chunks []chunk
	idx := 0
	start := int64(0)
	for start < layout.total {
		end := start + chunkSize
		if end > layout.total {
			end = layout.total
		}
		// Align end to the next parent boundary.
		if end < layout.total {
			parentIdx := layout.parentForRow(end - 1)
			if parentIdx+1 < int64(len(layout.prefixSum)) {
				end = layout.prefixSum[parentIdx+1]
			} else {
				end = layout.total
			}
		}
		chunks = append(chunks, chunk{Start: start, End: end, Index: idx})
		idx++
		start = end
	}
	return chunks
}

// runDrivenByEntity generates rows for an entity driven by a parent entity.
// Phase 1: compute child counts per parent. Phase 2: chunked parallel generation.
func (e *Executor) runDrivenByEntity(ctx context.Context, seed int64, entityIndex int, entity *plan.EntitySpec, store registry.ReadOnlyEntityStore) ([]*writer.OrderedMap, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	db := entity.DrivenBy
	entitySeed := rng.Derive(rng.SeedFromInt64(seed), fmt.Sprintf("%s[%d]", entity.Name, entityIndex))

	// Injected columns: the join key first, then Expose fields in declared order.
	refs := append([]plan.ParentField{{Field: db.Field, As: db.As}}, db.Expose...)
	injections := make([]parentInjection, 0, len(refs))
	for _, ref := range refs {
		col, ok := store.GetColumn(db.Entity, ref.Field)
		if !ok {
			return nil, fmt.Errorf("driven_by: column %s.%s not found in store", db.Entity, ref.Field)
		}
		injections = append(injections, parentInjection{as: ref.ChildName(), values: col})
	}

	layout := computeDrivenByLayout(entitySeed, db, injections)
	if layout.total == 0 {
		return nil, nil
	}

	fields, err := e.initFields(entity, entitySeed)
	if err != nil {
		return nil, err
	}

	needsAlignment := hasUniqueRelRef(entity)
	chunks := makeDrivenByChunks(layout, e.chunkSize, needsAlignment)

	e.logger.Debug("driven_by.layout", "entity", entity.Name, "parent", db.Entity, "total_children", layout.total)

	return e.runChunksParallel(ctx, chunks, e.workerCount(), func(ctx context.Context, ch chunk) ([]*writer.OrderedMap, error) {
		return e.runDrivenByChunk(ctx, entity, fields, ch, store, layout)
	})
}

// runDrivenByChunk generates rows for a driven_by entity chunk.
// It tracks parent transitions linearly (O(1) amortized), injects the parent
// value, and resets Resettable generators on parent transitions.
func (e *Executor) runDrivenByChunk(ctx context.Context, entity *plan.EntitySpec, fields []fieldRuntime, ch chunk, store registry.ReadOnlyEntityStore, layout *drivenByLayout) ([]*writer.OrderedMap, error) {
	db := entity.DrivenBy

	chunkFields, err := instantiateChunkFields(fields, ch, store)
	if err != nil {
		return nil, err
	}

	// Precompute which fields are Resettable to avoid per-row type assertions.
	resettables := make([]registry.Resettable, 0)
	for _, cf := range chunkFields {
		if r, ok := cf.gen.(registry.Resettable); ok {
			resettables = append(resettables, r)
		}
	}

	records := make([]*writer.OrderedMap, 0, int(ch.End-ch.Start))

	// Linear parent tracking: advance parentIdx when row crosses the boundary.
	parentIdx := layout.parentForRow(ch.Start)
	nextParentRow := layout.total // sentinel: no more parents
	if parentIdx+1 < int64(len(layout.prefixSum)) {
		nextParentRow = layout.prefixSum[parentIdx+1]
	}

	for row := ch.Start; row < ch.End; row++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Advance parent when row crosses boundary (O(1) amortized).
		if row >= nextParentRow {
			for _, r := range resettables {
				r.Reset()
			}
			parentIdx++
			if parentIdx+1 < int64(len(layout.prefixSum)) {
				nextParentRow = layout.prefixSum[parentIdx+1]
			} else {
				nextParentRow = layout.total
			}
		}

		record := writer.NewOrderedMap()
		// Inject parent columns before fields so row-aware generators can read them.
		for _, inj := range layout.injections {
			record.Set(inj.as, inj.values[parentIdx])
		}
		// 0-based child index within this parent's batch.
		if db.IndexAs != "" {
			record.Set(db.IndexAs, row-layout.prefixSum[parentIdx])
		}

		if err := generateFieldValues(chunkFields, row, record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}
