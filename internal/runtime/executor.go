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
	"strings"
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

	store := newMapEntityStore()
	required := requiredColumns(p.Entities)

	for idx := range p.Entities {
		entity := &p.Entities[idx]
		var records []*writer.OrderedMap
		var genErr error

		if entity.DrivenBy != nil {
			records, genErr = e.runDrivenByEntity(ctx, p.Seed, idx, entity, store)
		} else {
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

		// Extract and store required columns.
		for key := range required {
			parts := strings.SplitN(key, ".", 2)
			if parts[0] == entity.Name {
				store.StoreColumn(entity.Name, parts[1], extractColumn(records, parts[1]))
			}
		}
	}

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
	// Unique rel_ref requires serial execution for entity-scoped uniqueness.
	if hasUniqueRelRef(entity) {
		workers = 1
	}

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ch := range chunkCh {
				records, err := e.runChunk(ctx, entity, fields, ch, store)
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

	var allRecords []*writer.OrderedMap
	for _, records := range results {
		allRecords = append(allRecords, records...)
	}

	return allRecords, nil
}

// initFields initializes generators and seeds for entity fields.
func (e *Executor) initFields(entity *plan.EntitySpec, entitySeed rng.Seed) ([]fieldRuntime, error) {
	fields := make([]fieldRuntime, 0, len(entity.Fields))
	knownFields := make(map[string]bool)

	// If driven_by, the As field is auto-injected and counts as known.
	if entity.DrivenBy != nil {
		knownFields[entity.DrivenBy.As] = true
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

func (e *Executor) runChunk(ctx context.Context, entity *plan.EntitySpec, fields []fieldRuntime, ch chunk, store registry.ReadOnlyEntityStore) ([]*writer.OrderedMap, error) {
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

// copyConfig returns a shallow copy of a config map.
func copyConfig(cfg map[string]any) map[string]any {
	out := make(map[string]any, len(cfg)+1)
	for k, v := range cfg {
		out[k] = v
	}
	return out
}

// requiredColumns scans all entities to determine which (entity, field) pairs
// need to be stored for downstream rel_ref and DrivenBy references.
func requiredColumns(entities []plan.EntitySpec) map[string]bool {
	required := make(map[string]bool)
	for _, e := range entities {
		if e.DrivenBy != nil {
			required[e.DrivenBy.Entity+"."+e.DrivenBy.Field] = true
		}
		for _, f := range e.Fields {
			if f.Gen == "rel_ref" {
				entity, _ := f.Config["entity"].(string)
				field, _ := f.Config["field"].(string)
				if entity != "" && field != "" {
					required[entity+"."+field] = true
				}
			}
		}
	}
	return required
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
		if f.Gen == "rel_ref" {
			if u, ok := f.Config["unique"].(bool); ok && u {
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
