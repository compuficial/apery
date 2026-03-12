// Package runtime orchestrates the execution of synthetic data generation plans.
//
// The Executor takes a Plan and Writer, instantiates the required generators from
// the registry, manages seed derivation for deterministic randomness, and coordinates
// row-by-row record generation. It implements the core execution loop that transforms
// declarative plans into actual synthetic data output.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"apery/internal/plan"
	"apery/internal/registry"
	"apery/internal/rng"
	"apery/internal/writer"
)

type Executor struct {
	writer    writer.Writer
	logger    Logger
	chunkSize int64
	workers   int
}

type fieldRuntime struct {
	name    string
	genName string
	config  map[string]any
	factory registry.Factory
	seed    rng.Seed
}

type Logger interface {
	Printf(format string, args ...any)
}

type Option func(*Executor)

const (
	defaultChunkSize = int64(50000)
	maxWorkers       = 64
)

// WithLogger configures a logger for execution diagnostics.
func WithLogger(logger Logger) Option {
	return func(e *Executor) {
		e.logger = logger
	}
}

// WithChunkSize configures the row count per chunk.
func WithChunkSize(size int64) Option {
	return func(e *Executor) {
		e.chunkSize = size
	}
}

// WithWorkers configures the number of worker goroutines.
func WithWorkers(workers int) Option {
	return func(e *Executor) {
		e.workers = workers
	}
}

// New constructs an Executor with the provided writer and options.
func New(w writer.Writer, opts ...Option) *Executor {
	executor := &Executor{writer: w, chunkSize: defaultChunkSize}
	for _, opt := range opts {
		opt(executor)
	}
	return executor
}

// Run executes a plan and writes generated records via the writer.
func (e *Executor) Run(ctx context.Context, p *plan.Plan) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	defer e.closeWithError(&err)

	if err := plan.Validate(p); err != nil {
		return err
	}

	for idx := range p.Entities {
		if err := e.runEntity(ctx, p.Seed, idx, &p.Entities[idx]); err != nil {
			return fmt.Errorf("failed to generate %s entity: %w", p.Entities[idx].Name, err)
		}
	}

	return nil
}

// runEntity generates all rows for a single entity.
func (e *Executor) runEntity(ctx context.Context, seed int64, entityIndex int, entity *plan.EntitySpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	entitySeed := rng.Derive(rng.SeedFromInt64(seed), fmt.Sprintf("%s[%d]", entity.Name, entityIndex))
	fields, err := e.initFields(entity, entitySeed)
	if err != nil {
		return err
	}

	chunks := makeChunks(entity.Count, e.chunkSize)
	if len(chunks) == 0 {
		return nil
	}

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
				records, err := e.runChunk(ctx, entity, fields, ch)
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
		return firstErr
	}

	for _, records := range results {
		for _, record := range records {
			if err := e.writer.WriteRecord(entity.Name, record); err != nil {
				return err
			}
		}
	}

	return nil
}

// initFields initializes generators and seeds for entity fields.
func (e *Executor) initFields(entity *plan.EntitySpec, entitySeed rng.Seed) ([]fieldRuntime, error) {
	fields := make([]fieldRuntime, 0, len(entity.Fields))
	knownFields := make(map[string]bool)

	for _, field := range entity.Fields {
		factory, err := registry.FactoryFor(field.Gen)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field.Name, err)
		}

		gen, err := factory(field.Config)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field.Name, err)
		}

		// Validate dependency ordering for row-aware generators
		if dd, ok := gen.(registry.DependencyDeclarer); ok {
			for _, dep := range dd.Dependencies() {
				if !knownFields[dep] {
					return nil, fmt.Errorf("field '%s' references '%s', which must be declared before it", field.Name, dep)
				}
			}
		}

		fieldSeed := rng.Derive(entitySeed, field.Name)
		knownFields[field.Name] = true
		fields = append(fields, fieldRuntime{
			name:    field.Name,
			genName: field.Gen,
			config:  field.Config,
			factory: factory,
			seed:    fieldSeed,
		})
		e.logf("%s -> %s (seed: %d)", field.Name, field.Gen, fieldSeed)
	}

	return fields, nil
}

type chunkResult struct {
	index   int
	records []*writer.OrderedMap
}

type seekableGenerator interface {
	SeekRow(row int64) error
}

type chunkField struct {
	name string
	gen  registry.Generator
	seed rng.Seed
}

func (e *Executor) runChunk(ctx context.Context, entity *plan.EntitySpec, fields []fieldRuntime, ch chunk) ([]*writer.OrderedMap, error) {
	chunkFields := make([]chunkField, 0, len(fields))
	for _, field := range fields {
		gen, err := field.factory(field.config)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field.name, err)
		}
		if seeker, ok := gen.(seekableGenerator); ok {
			if err := seeker.SeekRow(ch.Start); err != nil {
				return nil, fmt.Errorf("field '%s': %w", field.name, err)
			}
		}
		chunkFields = append(chunkFields, chunkField{
			name: field.name,
			gen:  gen,
			seed: field.seed,
		})
	}

	records := make([]*writer.OrderedMap, 0, int(ch.End-ch.Start))
	for row := ch.Start; row < ch.End; row++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		record := writer.NewOrderedMap()
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

// closeWithError closes the writer and joins errors if needed.
func (e *Executor) closeWithError(err *error) {
	closeErr := e.writer.Close()
	if closeErr == nil {
		return
	}
	if *err != nil {
		*err = errors.Join(*err, closeErr)
		return
	}
	*err = closeErr
}

// logf writes formatted logs if a logger is configured.
func (e *Executor) logf(format string, args ...any) {
	if e.logger == nil {
		return
	}
	e.logger.Printf(format, args...)
}

func (e *Executor) workerCount() int {
	if e.workers > 0 {
		return e.workers
	}
	workers := runtime.NumCPU() * 2
	if workers < 1 {
		return 1
	}
	if workers > maxWorkers {
		return maxWorkers
	}
	return workers
}
