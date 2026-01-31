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
	"strconv"

	"apery/internal/plan"
	"apery/internal/registry"
	"apery/internal/rng"
	"apery/internal/writer"
)

type Executor struct {
	writer writer.Writer
	logger Logger
}

type fieldRuntime struct {
	name string
	gen  registry.Generator
	seed int64
}

type Logger interface {
	Printf(format string, args ...any)
}

type Option func(*Executor)

// WithLogger configures a logger for execution diagnostics.
func WithLogger(logger Logger) Option {
	return func(e *Executor) {
		e.logger = logger
	}
}

// New constructs an Executor with the provided writer and options.
func New(w writer.Writer, opts ...Option) *Executor {
	executor := &Executor{writer: w}
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

	entitySeed := rng.Derive(seed, fmt.Sprintf("%s[%d]", entity.Name, entityIndex))
	fields, err := e.initFields(entity, entitySeed)
	if err != nil {
		return err
	}

	for row := int64(0); row < entity.Count; row++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		record := writer.NewOrderedMap()
		rowLabel := strconv.FormatInt(row, 10)

		for _, field := range fields {
			rowSeed := rng.Derive(field.seed, rowLabel)
			val, err := field.gen.Next(rng.New(rowSeed))
			if err != nil {
				return fmt.Errorf("row %d, field '%s': %w", row, field.name, err)
			}

			record.Set(field.name, val)
		}

		if err := e.writer.WriteRecord(entity.Name, record); err != nil {
			return err
		}
	}
	return nil
}

// initFields initializes generators and seeds for entity fields.
func (e *Executor) initFields(entity *plan.EntitySpec, entitySeed int64) ([]fieldRuntime, error) {
	fields := make([]fieldRuntime, 0, len(entity.Fields))

	for _, field := range entity.Fields {
		gen, err := registry.Get(field.Gen, field.Config)
		if err != nil {
			return nil, fmt.Errorf("field '%s': %w", field.Name, err)
		}

		fieldSeed := rng.Derive(entitySeed, field.Name)
		fields = append(fields, fieldRuntime{
			name: field.Name,
			gen:  gen,
			seed: fieldSeed,
		})
		e.logf("%s -> %s (seed: %d)", field.Name, field.Gen, fieldSeed)
	}

	return fields, nil
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
