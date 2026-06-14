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
	"sync"
	"time"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/registry"
	"github.com/compuficial/apery/internal/rng"
	"github.com/compuficial/apery/internal/writer"
)

// Generator name and config key constants used for special-case behavior.
const (
	genRelRef     = "rel_ref"
	cfgStore      = "_store"
	cfgEntity     = "entity"
	cfgField      = "field"
	cfgUnique     = "unique"
)

// discardHandler is a slog.Handler that discards all log records.
var discardHandler = slog.NewTextHandler(nopWriter{}, &slog.HandlerOptions{Level: slog.Level(99)})

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

type Executor struct {
	writer    writer.Writer
	logger    *slog.Logger
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

type Option func(*Executor)

const (
	defaultChunkSize = int64(50000)
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

// Run executes a plan and writes generated records via the writer.
func (e *Executor) Run(ctx context.Context, p *plan.Plan) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	defer e.closeWithError(&err)

	if err := plan.Validate(p); err != nil {
		return err
	}

	store := newMapEntityStore()
	required := requiredColumns(p.Entities)

	runStart := time.Now()
	e.logger.Info("run.start", "entities", len(p.Entities), "seed", p.Seed)

	var totalRows int64
	for idx := range p.Entities {
		entity := &p.Entities[idx]
		var records []*writer.OrderedMap
		var genErr error

		entityStart := time.Now()
		rowCount := entity.Count
		if entity.DrivenBy != nil {
			e.logger.Info("entity.start", "entity", entity.Name, "type", "driven_by", "parent", entity.DrivenBy.Entity)
			records, genErr = e.runDrivenByEntity(ctx, p.Seed, idx, entity, store)
			rowCount = int64(len(records))
		} else {
			e.logger.Info("entity.start", "entity", entity.Name, "rows", entity.Count)
			records, genErr = e.runEntity(ctx, p.Seed, idx, entity, store)
		}
		if genErr != nil {
			return fmt.Errorf("failed to generate %s entity: %w", entity.Name, genErr)
		}

		for _, record := range records {
			if err := e.writer.WriteRecord(entity.Name, record); err != nil {
				return err
			}
		}

		totalRows += rowCount
		e.logger.Info("entity.complete", "entity", entity.Name, "rows", rowCount, "duration", time.Since(entityStart).Round(time.Millisecond))

		// Extract and store required columns.
		if fields, ok := required[entity.Name]; ok {
			for _, fieldName := range fields {
				store.StoreColumn(entity.Name, fieldName, extractColumn(records, fieldName))
			}
		}
	}

	e.logger.Info("run.complete", "entities", len(p.Entities), "rows", totalRows, "duration", time.Since(runStart).Round(time.Millisecond))
	return nil
}

// runEntity generates all rows for a standalone entity (one with Count set).
func (e *Executor) runEntity(ctx context.Context, seed int64, entityIndex int, entity *plan.EntitySpec, store registry.ReadOnlyEntityStore) ([]*writer.OrderedMap, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entitySeed := rng.Derive(rng.SeedFromInt64(seed), fmt.Sprintf("%s[%d]", entity.Name, entityIndex))
	fields, err := e.initFields(entity, entitySeed)
	if err != nil {
		return nil, err
	}

	chunks := makeChunks(entity.Count, e.chunkSize)
	if len(chunks) == 0 {
		return nil, nil
	}

	workers := e.workerCount()
	// Unique rel_ref requires serial execution for entity-scoped uniqueness.
	if hasUniqueRelRef(entity) {
		workers = 1
	}

	return e.runChunksParallel(ctx, chunks, workers, func(ctx context.Context, ch chunk) ([]*writer.OrderedMap, error) {
		return e.runChunk(ctx, fields, ch, store)
	})
}

// initFields initializes generators and seeds for entity fields.
func (e *Executor) initFields(entity *plan.EntitySpec, entitySeed rng.Seed) ([]fieldRuntime, error) {
	fields := make([]fieldRuntime, 0, len(entity.Fields))
	knownFields := make(map[string]bool)

	// driven_by auto-injects these columns, so they count as known.
	if entity.DrivenBy != nil {
		knownFields[entity.DrivenBy.As] = true
		for _, pf := range entity.DrivenBy.Expose {
			knownFields[pf.ChildName()] = true
		}
		if entity.DrivenBy.IndexAs != "" {
			knownFields[entity.DrivenBy.IndexAs] = true
		}
	}

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
		e.logger.Debug("field.init", "entity", entity.Name, "field", field.Name, "gen", field.Gen, "seed", fieldSeed)
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

// chunkField holds a per-chunk generator instance with precomputed type flags.
type chunkField struct {
	name     string
	gen      registry.Generator
	seed     rng.Seed
	rowAware registry.RowAwareGenerator // non-nil if gen implements RowAwareGenerator
}

// chunkRunner is a function that processes a single chunk and returns records.
type chunkRunner func(ctx context.Context, ch chunk) ([]*writer.OrderedMap, error)

// runChunksParallel distributes chunks across workers, collects results in order,
// and returns the flattened records. This is the shared fan-out pattern used by
// both runEntity and runDrivenByEntity.
func (e *Executor) runChunksParallel(ctx context.Context, chunks []chunk, workers int, runner chunkRunner) ([]*writer.OrderedMap, error) {
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

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ch := range chunkCh {
				records, err := runner(ctx, ch)
				if err != nil {
					setErr(err)
					return
				}
				resultCh <- chunkResult{index: ch.Index, records: records}
			}
		}()
	}

	for _, ch := range chunks {
		e.logger.Debug("chunk.dispatch", "chunk", ch.Index, "start", ch.Start, "end", ch.End)
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

	var allRecords []*writer.OrderedMap
	for _, records := range results {
		allRecords = append(allRecords, records...)
	}

	return allRecords, nil
}

// instantiateChunkFields creates per-chunk generator instances from field
// runtimes, injecting the entity store for rel_ref generators and seeking
// seekable generators to the chunk start row.
func instantiateChunkFields(fields []fieldRuntime, ch chunk, store registry.ReadOnlyEntityStore) ([]chunkField, error) {
	cfs := make([]chunkField, 0, len(fields))
	for _, field := range fields {
		var gen registry.Generator
		var err error
		if field.genName == genRelRef && store != nil {
			cfg := copyConfig(field.config)
			cfg[cfgStore] = store
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
		cf := chunkField{name: field.name, gen: gen, seed: field.seed}
		if ra, ok := gen.(registry.RowAwareGenerator); ok {
			cf.rowAware = ra
		}
		cfs = append(cfs, cf)
	}
	return cfs, nil
}

// generateFieldValues produces values for all fields in a single row and sets
// them on the record. This is the shared inner loop used by both runChunk and
// runDrivenByChunk.
func generateFieldValues(fields []chunkField, row int64, record *writer.OrderedMap) error {
	for i := range fields {
		f := &fields[i]
		rowSeed := rng.DeriveIndex(f.seed, row)
		r := rng.New(rowSeed)

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

func (e *Executor) runChunk(ctx context.Context, fields []fieldRuntime, ch chunk, store registry.ReadOnlyEntityStore) ([]*writer.OrderedMap, error) {
	chunkFields, err := instantiateChunkFields(fields, ch, store)
	if err != nil {
		return nil, err
	}

	records := make([]*writer.OrderedMap, 0, int(ch.End-ch.Start))
	for row := ch.Start; row < ch.End; row++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		record := writer.NewOrderedMap()
		if err := generateFieldValues(chunkFields, row, record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, nil
}

// copyConfig returns a shallow copy of a config map.
func copyConfig(cfg map[string]any) map[string]any {
	out := make(map[string]any, len(cfg)+1)
	for k, v := range cfg {
		out[k] = v
	}
	return out
}

// requiredColumns scans all entities to determine which fields per entity
// need to be stored for downstream rel_ref and DrivenBy references.
// Returns a map of entity name -> list of field names to store.
func requiredColumns(entities []plan.EntitySpec) map[string][]string {
	// Use a set to deduplicate, then convert to slices.
	sets := make(map[string]map[string]bool)
	addRequired := func(entity, field string) {
		if sets[entity] == nil {
			sets[entity] = make(map[string]bool)
		}
		sets[entity][field] = true
	}

	for _, e := range entities {
		if e.DrivenBy != nil {
			addRequired(e.DrivenBy.Entity, e.DrivenBy.Field)
			// Cache exposed parent columns too.
			for _, pf := range e.DrivenBy.Expose {
				addRequired(e.DrivenBy.Entity, pf.Field)
			}
		}
		for _, f := range e.Fields {
			if f.Gen == genRelRef {
				entity, _ := f.Config[cfgEntity].(string)
				field, _ := f.Config[cfgField].(string)
				if entity != "" && field != "" {
					addRequired(entity, field)
				}
			}
		}
	}

	result := make(map[string][]string, len(sets))
	for entity, fieldSet := range sets {
		fields := make([]string, 0, len(fieldSet))
		for field := range fieldSet {
			fields = append(fields, field)
		}
		result[entity] = fields
	}
	return result
}

// extractColumn collects a single field's values from generated records.
func extractColumn(records []*writer.OrderedMap, fieldName string) []any {
	col := make([]any, len(records))
	for i, rec := range records {
		val, _ := rec.Get(fieldName)
		col[i] = val
	}
	return col
}

// hasUniqueRelRef returns true if any field in the entity is a rel_ref
// generator with unique: true in its config.
func hasUniqueRelRef(entity *plan.EntitySpec) bool {
	for _, f := range entity.Fields {
		if f.Gen == genRelRef {
			if u, ok := f.Config[cfgUnique].(bool); ok && u {
				return true
			}
		}
	}
	return false
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
