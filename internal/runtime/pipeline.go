package runtime

import (
	"context"
	"sync"

	"github.com/compuficial/apery/internal/writer"
)

// chunkRunner generates one chunk's records.
type chunkRunner func(ctx context.Context, ch chunk) ([]*writer.OrderedMap, error)

// chunkResult is one finished chunk on its way to emission.
type chunkResult struct {
	index   int
	records []*writer.OrderedMap
	err     error
}

// streamChunks generates chunks on up to workers goroutines and hands each
// chunk's records to emit in chunk order. At most workers+1 chunks are in
// flight (dispatched but not yet emitted): every worker stays busy while one
// finished chunk waits its turn. Memory is bounded by workers × chunk size,
// not row count. The first error from a chunk, from emit, or from ctx
// cancels everything else, and every goroutine exits before returning.
//
//	dispatcher ──chunks in order──▶ workers ──results, any order──▶ caller
//	    ▲                                                            │
//	    └──────────── slot freed when a chunk is emitted ◀───────────┘
func streamChunks(ctx context.Context, chunks []chunk, workers int, run chunkRunner, emit func([]*writer.OrderedMap) error) error {
	if len(chunks) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	workers = max(1, min(workers, len(chunks)))
	window := workers + 1
	todo := make(chan chunk)
	done := make(chan chunkResult, window)
	slots := make(chan struct{}, window)

	var wg sync.WaitGroup
	wg.Go(func() {
		defer close(todo)
		for _, ch := range chunks {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			select {
			case todo <- ch:
			case <-ctx.Done():
				return
			}
		}
	})

	for range workers {
		wg.Go(func() {
			for ch := range todo {
				records, err := run(ctx, ch)
				select {
				case done <- chunkResult{index: ch.Index, records: records, err: err}:
				case <-ctx.Done():
					return
				}
			}
		})
	}

	err := emitInOrder(ctx, len(chunks), done, slots, emit)
	cancel()
	wg.Wait()
	return err
}

// emitInOrder buffers results that finish early and emits chunks strictly
// by index, freeing a dispatch slot per emitted chunk.
func emitInOrder(ctx context.Context, total int, done <-chan chunkResult, slots <-chan struct{}, emit func([]*writer.OrderedMap) error) error {
	pending := make(map[int][]*writer.OrderedMap)
	for next := 0; next < total; {
		select {
		case res := <-done:
			if res.err != nil {
				return res.err
			}
			pending[res.index] = res.records
		case <-ctx.Done():
			return ctx.Err()
		}

		for records, ok := pending[next]; ok; records, ok = pending[next] {
			delete(pending, next)
			if err := emit(records); err != nil {
				return err
			}
			next++
			<-slots
		}
	}
	return nil
}
