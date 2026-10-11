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
	for i := range n {
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

// chunking says where driven_by chunk boundaries may fall.
type chunking int

const (
	rowChunks           chunking = iota // any row
	parentAlignedChunks                 // only between parents' batches
)

// makeDrivenByChunks splits the first total child rows into chunks. Batch-
// scoped generators need parentAlignedChunks so a parent's state never
// spans two workers.
func makeDrivenByChunks(layout *drivenByLayout, total, chunkSize int64, mode chunking) []chunk {
	if mode == rowChunks {
		return makeChunks(total, chunkSize)
	}

	var chunks []chunk
	idx := 0
	start := int64(0)
	for start < total {
		end := min(start+chunkSize, total)
		// Align end to the next parent boundary.
		if end < total {
			parentIdx := layout.parentForRow(end - 1)
			if parentIdx+1 < int64(len(layout.prefixSum)) {
				end = min(layout.prefixSum[parentIdx+1], total)
			} else {
				end = total
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
func (e *Executor) runDrivenByEntity(ctx context.Context, entitySeed rng.Seed, ent *compiledEntity, fields []fieldRuntime, store registry.ReadOnlyEntityStore, emit func([]*writer.OrderedMap) error) (int64, error) {
	db := ent.spec.DrivenBy

	// Injected columns: the join key first, then Expose fields in declared order.
	refs := append([]plan.ParentField{{Field: db.Field, As: db.As}}, db.Expose...)
	injections := make([]parentInjection, 0, len(refs))
	for _, ref := range refs {
		col, ok := store.GetColumn(db.Entity, ref.Field)
		if !ok {
			return 0, fmt.Errorf("driven_by: column %s.%s not found in store", db.Entity, ref.Field)
		}
		injections = append(injections, parentInjection{as: ref.ChildName(), values: col})
	}

	layout := computeDrivenByLayout(entitySeed, db, injections)
	total := e.capRows(layout.total)
	mode := rowChunks
	if ent.isBatchScoped {
		mode = parentAlignedChunks
	}
	chunks := makeDrivenByChunks(layout, total, e.chunkSize, mode)

	// A sequential generator continues from one parent's children to the
	// next, so it needs the previous chunk's instance.
	sharing := perChunkInstances
	if ent.isSequential {
		sharing = sharedInstances
	}
	instances, workers, err := e.chunkInstances(fields, store, sharing)
	if err != nil {
		return 0, err
	}

	e.logger.Debug("driven_by.layout", "entity", ent.spec.Name, "parent", db.Entity, "total_children", layout.total)

	return total, streamChunks(ctx, chunks, workers, func(ctx context.Context, ch chunk) ([]*writer.OrderedMap, error) {
		e.logger.Debug("chunk.start", "entity", ent.spec.Name, "chunk", ch.Index, "start", ch.Start, "end", ch.End)
		cfs, err := instances()
		if err != nil {
			return nil, err
		}
		if err := seekFields(cfs, ch.Start); err != nil {
			return nil, err
		}
		return runDrivenByChunk(ctx, db, cfs, ch, layout)
	}, emit)
}

// runDrivenByChunk generates rows for a driven_by entity chunk.
// It tracks parent transitions linearly (O(1) amortized), injects the parent
// value, and resets Resettable generators at each parent's first child.
func runDrivenByChunk(ctx context.Context, db *plan.DrivenBy, chunkFields []chunkField, ch chunk, layout *drivenByLayout) ([]*writer.OrderedMap, error) {
	// Precompute which fields are Resettable to avoid per-row type assertions.
	resettables := make([]registry.Resettable, 0)
	for _, cf := range chunkFields {
		if r, ok := cf.gen.(registry.Resettable); ok {
			resettables = append(resettables, r)
		}
	}

	width := len(layout.injections) + len(chunkFields)
	if db.IndexAs != "" {
		width++
	}
	r := rng.New(0)
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
			parentIdx++
			if parentIdx+1 < int64(len(layout.prefixSum)) {
				nextParentRow = layout.prefixSum[parentIdx+1]
			} else {
				nextParentRow = layout.total
			}
		}
		// Each parent's children are a batch. A chunk's first row can start
		// one too: shared instances still hold the previous chunk's state.
		if row == layout.prefixSum[parentIdx] {
			for _, r := range resettables {
				r.Reset()
			}
		}

		record := writer.NewOrderedMapSize(width)
		// Inject parent columns before fields so row-aware generators can read them.
		for _, inj := range layout.injections {
			record.Set(inj.as, inj.values[parentIdx])
		}
		// 0-based child index within this parent's batch.
		if db.IndexAs != "" {
			record.Set(db.IndexAs, row-layout.prefixSum[parentIdx])
		}

		if err := generateRow(chunkFields, row, record, r); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}
