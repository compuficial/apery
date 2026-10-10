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
	"log/slog"
	"runtime"
	"time"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/registry"
	"github.com/compuficial/apery/internal/rng"
	"github.com/compuficial/apery/internal/writer"
)

// ErrWrite marks failures writing or closing output, as opposed to
// generation failures.
var ErrWrite = errors.New("write output")

// discardHandler is a slog.Handler that discards all log records.
var discardHandler = slog.NewTextHandler(nopWriter{}, &slog.HandlerOptions{Level: slog.Level(99)})

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

type Executor struct {
	writer    writer.Writer
	logger    *slog.Logger
	chunkSize int64
	workers   int
	rowLimit  int64 // 0 = no limit
}

// fieldRuntime is a compiled field with its entity-scoped seed.
type fieldRuntime struct {
	compiledField
	seed rng.Seed
}

type Option func(*Executor)

// Defaults keep memory low (at most workers+1 chunks are in flight) without
// costing throughput: generation is CPU-bound and output is written by one
// goroutine, so more workers or bigger chunks only add buffered rows.
const (
	defaultChunkSize = int64(10000)
	maxWorkers       = 64
)

// WithLogger configures structured logging for execution.
func WithLogger(logger *slog.Logger) Option {
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

// WithRowLimit caps every entity at n rows (driven_by children included),
// for quick previews. Capped parents mean children and references sample
// from fewer rows, so values can differ from an uncapped run.
func WithRowLimit(n int64) Option {
	return func(e *Executor) {
		e.rowLimit = n
	}
}

// New constructs an Executor with the provided writer and options.
func New(w writer.Writer, opts ...Option) *Executor {
	executor := &Executor{
		writer:    w,
		chunkSize: defaultChunkSize,
		logger:    slog.New(discardHandler),
	}
	for _, opt := range opts {
		opt(executor)
	}
	return executor
}

// Run validates the plan, then generates entities in order and streams their
// records to the writer. Plan problems are returned as *plan.ValidationError
// before anything is written. Run takes ownership of the writer and closes
// it when done, on success or failure.
func (e *Executor) Run(ctx context.Context, p *plan.Plan) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	defer e.closeWithError(&err)

	entities, err := compile(p, registry.NewEnv(baseDirOf(p)))
	if err != nil {
		return err
	}

	store := newMapEntityStore()
	required := requiredColumns(entities)

	runStart := time.Now()
	e.logger.Info("run.start", "entities", len(entities), "seed", p.Seed)

	var totalRows int64
	for i := range entities {
		ent := &entities[i]
		if err := ctx.Err(); err != nil {
			return err
		}

		entityStart := time.Now()
		if db := ent.spec.DrivenBy; db != nil {
			e.logger.Info("entity.start", "entity", ent.spec.Name, "type", "driven_by", "parent", db.Entity)
		} else {
			e.logger.Info("entity.start", "entity", ent.spec.Name, "rows", ent.spec.Count)
		}

		// Write records as chunks complete, keeping only the columns later
		// entities reference.
		cols := newColumnCollector(required[ent.spec.Name])
		emit := func(records []*writer.OrderedMap) error {
			for _, record := range records {
				if err := e.writer.WriteRecord(ent.spec.Name, record); err != nil {
					return fmt.Errorf("%w: %w", ErrWrite, err)
				}
				cols.add(record)
			}
			return nil
		}

		rows, err := e.runEntity(ctx, p.Seed, ent, store, emit)
		if err != nil {
			return fmt.Errorf("failed to generate %s entity: %w", ent.spec.Name, err)
		}
		cols.storeInto(store, ent.spec.Name)

		totalRows += rows
		e.logger.Info("entity.complete", "entity", ent.spec.Name, "rows", rows, "duration", time.Since(entityStart).Round(time.Millisecond))
	}

	e.logger.Info("run.complete", "entities", len(entities), "rows", totalRows, "duration", time.Since(runStart).Round(time.Millisecond))
	return nil
}

// runEntity derives the entity's field seeds and generates its rows,
// returning the row count.
func (e *Executor) runEntity(ctx context.Context, seed int64, ent *compiledEntity, store registry.ReadOnlyEntityStore, emit func([]*writer.OrderedMap) error) (int64, error) {
	entitySeed := rng.Derive(rng.SeedFromInt64(seed), fmt.Sprintf("%s[%d]", ent.spec.Name, ent.index))
	fields := make([]fieldRuntime, len(ent.fields))
	for i, f := range ent.fields {
		fields[i] = fieldRuntime{compiledField: f, seed: rng.Derive(entitySeed, f.name)}
		e.logger.Debug("field.init", "entity", ent.spec.Name, "field", f.name, "gen", f.gen, "seed", fields[i].seed)
	}

	if ent.spec.DrivenBy != nil {
		return e.runDrivenByEntity(ctx, entitySeed, ent, fields, store, emit)
	}
	return e.runStandaloneEntity(ctx, ent, fields, store, emit)
}

// runStandaloneEntity generates an entity with a fixed Count.
func (e *Executor) runStandaloneEntity(ctx context.Context, ent *compiledEntity, fields []fieldRuntime, store registry.ReadOnlyEntityStore, emit func([]*writer.OrderedMap) error) (int64, error) {
	count := e.capRows(ent.spec.Count)
	chunks := makeChunks(count, e.chunkSize)
	workers := e.workerCount()
	instances := chunkInstances(fields, store)

	// A batch-scoped generator (unique rel_ref) treats the whole entity as
	// one batch: one set of instances, shared by chunks run in order.
	if ent.isBatchScoped {
		shared, err := instantiateFields(fields, store)
		if err != nil {
			return 0, err
		}
		workers = 1
		instances = func() ([]chunkField, error) { return shared, nil }
	}

	return count, streamChunks(ctx, chunks, workers, func(ctx context.Context, ch chunk) ([]*writer.OrderedMap, error) {
		e.logger.Debug("chunk.start", "entity", ent.spec.Name, "chunk", ch.Index, "start", ch.Start, "end", ch.End)
		cfs, err := instances()
		if err != nil {
			return nil, err
		}
		if err := seekFields(cfs, ch.Start); err != nil {
			return nil, err
		}

		r := rng.New(0)
		records := make([]*writer.OrderedMap, 0, int(ch.End-ch.Start))
		for row := ch.Start; row < ch.End; row++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			record := writer.NewOrderedMapSize(len(cfs))
			if err := generateRow(cfs, row, record, r); err != nil {
				return nil, err
			}
			records = append(records, record)
		}
		return records, nil
	}, emit)
}

// chunkField is a chunk's generator instance with precomputed type flags.
type chunkField struct {
	name     string
	gen      registry.Generator
	seed     rng.Seed
	rowAware registry.RowAwareGenerator // non-nil if gen reads the row
	seekable registry.Seekable          // non-nil if gen tracks row position
}

// chunkInstances returns a builder of fresh generator instances per chunk.
func chunkInstances(fields []fieldRuntime, store registry.ReadOnlyEntityStore) func() ([]chunkField, error) {
	return func() ([]chunkField, error) { return instantiateFields(fields, store) }
}

// instantiateFields builds generator instances and binds the entity store
// for generators that read other entities. Config was validated at compile
// time, so the factories are called directly.
func instantiateFields(fields []fieldRuntime, store registry.ReadOnlyEntityStore) ([]chunkField, error) {
	cfs := make([]chunkField, 0, len(fields))
	for _, field := range fields {
		gen, err := field.factory(field.config)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field.name, err)
		}
		if consumer, ok := gen.(registry.StoreConsumer); ok {
			if err := consumer.BindStore(store); err != nil {
				return nil, fmt.Errorf("field '%s': %w", field.name, err)
			}
		}

		cf := chunkField{name: field.name, gen: gen, seed: field.seed}
		cf.rowAware, _ = gen.(registry.RowAwareGenerator)
		cf.seekable, _ = gen.(registry.Seekable)
		cfs = append(cfs, cf)
	}
	return cfs, nil
}

// seekFields positions row-position generators at a chunk's first row.
func seekFields(fields []chunkField, row int64) error {
	for _, f := range fields {
		if f.seekable == nil {
			continue
		}
		if err := f.seekable.SeekRow(row); err != nil {
			return fmt.Errorf("field '%s': %w", f.name, err)
		}
	}
	return nil
}

// generateRow sets every field's value for one row. r is reseeded per field
// from the row seed, so values don't depend on chunking or field order.
func generateRow(fields []chunkField, row int64, record *writer.OrderedMap, r *rng.Rng) error {
	for i := range fields {
		f := &fields[i]
		r.Reseed(rng.DeriveIndex(f.seed, row))

		var val any
		var err error
		if f.rowAware != nil {
			val, err = f.rowAware.NextWithRow(r, record)
		} else {
			val, err = f.gen.Next(r)
		}
		if err != nil {
			return fmt.Errorf("row %d, field '%s': %w", row, f.name, err)
		}
		record.Set(f.name, val)
	}
	return nil
}

// columnCollector accumulates the columns later entities reference while
// records stream past.
type columnCollector struct {
	names  []string
	values [][]any
}

func newColumnCollector(names []string) *columnCollector {
	return &columnCollector{names: names, values: make([][]any, len(names))}
}

func (c *columnCollector) add(record *writer.OrderedMap) {
	for i, name := range c.names {
		val, _ := record.Get(name)
		c.values[i] = append(c.values[i], val)
	}
}

func (c *columnCollector) storeInto(store registry.EntityStore, entity string) {
	for i, name := range c.names {
		store.StoreColumn(entity, name, c.values[i])
	}
}

// capRows applies the row limit, if any.
func (e *Executor) capRows(n int64) int64 {
	if e.rowLimit > 0 && n > e.rowLimit {
		return e.rowLimit
	}
	return n
}

// closeWithError closes the writer and joins errors if needed.
func (e *Executor) closeWithError(err *error) {
	closeErr := e.writer.Close()
	if closeErr == nil {
		return
	}
	closeErr = fmt.Errorf("%w: %w", ErrWrite, closeErr)
	if *err != nil {
		*err = errors.Join(*err, closeErr)
		return
	}
	*err = closeErr
}

func (e *Executor) workerCount() int {
	if e.workers > 0 {
		return e.workers
	}
	workers := runtime.NumCPU()
	if workers < 1 {
		return 1
	}
	if workers > maxWorkers {
		return maxWorkers
	}
	return workers
}
