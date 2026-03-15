package runtime

import (
	"context"
	"fmt"
	"sort"
	"sync"

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

	for i := int64(0); i < n; i++ {
		countSeed := rng.Derive(entitySeed, fmt.Sprintf("count[%d]", i))
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
// using binary search on the prefix sum.
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	chunkCh := make(chan chunk)
	resultCh := make(chan chunkResult, len(chunks))

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	setErr := func(err error) {
		errOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	workers := e.workerCount()
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ch := range chunkCh {
				records, err := e.runDrivenByChunk(ctx, entity, fields, ch, store, layout)
				if err != nil {
					setErr(err)
					return
				}
				resultCh <- chunkResult{index: ch.Index, records: records}
			}
		}()
	}
	for _, ch := range chunks {
		chunkCh <- ch
	}
	close(chunkCh)
	wg.Wait()
	close(resultCh)

	results := make([][]*writer.OrderedMap, len(chunks))
	for res := range resultCh {
		results[res.index] = res.records
	}
	if firstErr != nil {
		return nil, firstErr
	}

	var all []*writer.OrderedMap
	for _, records := range results {
		all = append(all, records...)
	}
	return all, nil
}

// runDrivenByChunk generates rows for a driven_by entity chunk.
// It maps global row indices to parent indices, injects the parent value,
// and resets Resettable generators on parent transitions.
func (e *Executor) runDrivenByChunk(ctx context.Context, entity *plan.EntitySpec, fields []fieldRuntime, ch chunk, store registry.ReadOnlyEntityStore, layout *drivenByLayout) ([]*writer.OrderedMap, error) {
	db := entity.DrivenBy

	chunkFields := make([]chunkField, 0, len(fields))
	for _, field := range fields {
		var gen registry.Generator
		var err error
		if field.genName == "rel_ref" && store != nil {
			cfg := copyConfig(field.config)
			cfg["_store"] = store
			gen, err = field.factory(cfg)
		} else {
			gen, err = field.factory(field.config)
		}
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field.name, err)
		}
		if seeker, ok := gen.(seekableGenerator); ok {
			if err := seeker.SeekRow(ch.Start); err != nil {
				return nil, fmt.Errorf("field '%s': %w", field.name, err)
			}
		}
		chunkFields = append(chunkFields, chunkField{name: field.name, gen: gen, seed: field.seed})
	}

	records := make([]*writer.OrderedMap, 0, int(ch.End-ch.Start))
	lastParent := int64(-1)

	for row := ch.Start; row < ch.End; row++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		parentIdx := layout.parentForRow(row)

		// Reset Resettable generators on parent transition.
		if parentIdx != lastParent {
			if lastParent != -1 {
				for _, cf := range chunkFields {
					if r, ok := cf.gen.(registry.Resettable); ok {
						r.Reset()
					}
				}
			}
			lastParent = parentIdx
		}

		record := writer.NewOrderedMap()
		// Inject parent value as first field.
		record.Set(db.As, layout.parentCol[parentIdx])

		for _, field := range chunkFields {
			rowSeed := rng.DeriveIndex(field.seed, row)
			r := rng.New(rowSeed)

			var val any
			var err error
			if ra, ok := field.gen.(registry.RowAwareGenerator); ok {
				val, err = ra.NextWithRow(r, record)
			} else {
				val, err = field.gen.Next(r)
			}
			if err != nil {
				return nil, fmt.Errorf("row %d, field '%s': %w", row, field.name, err)
			}
			record.Set(field.name, val)
		}
		records = append(records, record)
	}
	return records, nil
}
