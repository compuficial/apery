package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/writer"
)

type discardWriter struct{}

func (w *discardWriter) WriteRecord(entity string, record *writer.OrderedMap) error {
	return nil
}

func (w *discardWriter) Close() error {
	return nil
}

type benchCase struct {
	name      string
	workers   int
	chunkSize int64
}

func tunedChunkSize(rows int64, workers int) int64 {
	if workers < 1 {
		workers = 1
	}
	// Aim for ~4 chunks per worker, but avoid tiny chunks.
	target := rows / int64(workers*4)
	if target < 1000 {
		return 1000
	}
	return target
}

func BenchmarkExecutor(b *testing.B) {
	counts := []int64{2000, 20000, 200000, 2000000}
	w := &discardWriter{}
	defaultWorkers := New(w).workerCount()

	for _, count := range counts {
		cases := []benchCase{
			{name: "workers=1", workers: 1, chunkSize: defaultChunkSize},
			{name: "workers=default", workers: 0, chunkSize: defaultChunkSize},
			{
				name:      fmt.Sprintf("workers=default/chunk=%d", tunedChunkSize(count, defaultWorkers)),
				workers:   0,
				chunkSize: tunedChunkSize(count, defaultWorkers),
			},
		}

		for _, bc := range cases {
			b.Run(fmt.Sprintf("rows=%d/%s", count, bc.name), func(b *testing.B) {
				p := &plan.Plan{
					Seed: 1,
					Entities: []plan.EntitySpec{
						{
							Name:  "User",
							Count: count,
							Fields: []plan.FieldSpec{
								{Name: "id", Gen: "seq"},
								{Name: "score", Gen: "int", Config: map[string]any{"min": 1, "max": 100}},
								{Name: "active", Gen: "bool", Config: map[string]any{"probability": 0.5}},
							},
						},
					},
				}

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					opts := []Option{WithChunkSize(bc.chunkSize)}
					if bc.workers > 0 {
						opts = append(opts, WithWorkers(bc.workers))
					}
					executor := New(w, opts...)
					if err := executor.Run(context.Background(), p); err != nil {
						b.Fatalf("run: %v", err)
					}
				}

				b.StopTimer()
				if elapsed := b.Elapsed().Seconds(); elapsed > 0 {
					rowsPerSec := float64(count) * float64(b.N) / elapsed
					b.ReportMetric(rowsPerSec, "rows/s")
				}
			})
		}
	}
}
