package runtime

import (
	"context"
	"fmt"
	"sort"

	"apery/internal/plan"
	"apery/internal/registry"
	"apery/internal/rng"
	"apery/internal/writer"
)

// drivenByLayout holds the precomputed child counts and prefix sums
// for a driven_by entity's two-phase execution.
type drivenByLayout struct {
	counts    []int64 // number of children per parent
	prefixSum []int64 // prefixSum[i] = total rows before parent i
	total     int64   // total child rows across all parents
	parentCol []any   // parent field values to inject
}

// computeDrivenByLayout runs Phase 1: determines child counts per parent.
func computeDrivenByLayout(entitySeed rng.Seed, db *plan.DrivenBy, parentValues []any) *drivenByLayout {
	n := int64(len(parentValues))
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
		counts:    counts,
		prefixSum: prefixSum,
		total:     total,
		parentCol: parentValues,
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

	parentCol, ok := store.GetColumn(db.Entity, db.Field)
	if !ok {
		return nil, fmt.Errorf("driven_by: column %s.%s not found in store", db.Entity, db.Field)
	}

	layout := computeDrivenByLayout(entitySeed, db, parentCol)
	if layout.total == 0 {
		return nil, nil
	}

	fields, err := e.initFields(entity, entitySeed)
	if err != nil {
		return nil, err
	}

	needsAlignment := hasUniqueRelRef(entity)
	chunks := makeDrivenByChunks(layout, e.chunkSize, needsAlignment)

	return runChunksParallel(ctx, chunks, e.workerCount(), func(ctx context.Context, ch chunk) ([]*writer.OrderedMap, error) {
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
		// Inject parent value as first field.
		record.Set(db.As, layout.parentCol[parentIdx])

		if err := generateFieldValues(chunkFields, row, record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}
