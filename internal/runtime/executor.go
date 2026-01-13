// Package runtime orchestrates the execution of synthetic data generation plans.
//
// The Executor takes a Plan and Writer, instantiates the required generators from
// the registry, manages seed derivation for deterministic randomness, and coordinates
// row-by-row record generation. It implements the core execution loop that transforms
// declarative plans into actual synthetic data output.
package runtime

import (
	"apery/internal/plan"
	"apery/internal/registry"
	"apery/internal/rng"
	"apery/internal/writer"
	"fmt"
)

type Executor struct {
	writer writer.Writer
}

func New(w writer.Writer) *Executor {
	return &Executor{writer: w}
}

func (e *Executor) Run(p *plan.Plan) error {
	if err := plan.Validate(p); err != nil {
		return err
	}

	for idx, entity := range p.Entities {
		fmt.Printf("entity: %+v\n", entity)
		if err := e.runEntity(p.Seed, idx, &entity); err != nil {
			return fmt.Errorf("failed to generate %s entity: %w", entity.Name, err)
		}
	}

	return e.writer.Close()
}

func (e *Executor) runEntity(seed int64, entityIndex int, entity *plan.EntitySpec) error {
	// Get generators for each field
	gens := make([]registry.Generator, len(entity.Fields))
	// Get rngs for each field
	rngs := make([]*rng.Rng, len(entity.Fields))

	for i, field := range entity.Fields {
		gen, err := registry.Get(field.Gen, field.Config)
		if err != nil {
			return fmt.Errorf("field '%s': %w", field.Name, err)
		}

		gens[i] = gen

		label := fmt.Sprintf("%s[%d]:%s", entity.Name, entityIndex, field.Name)
		fieldSeed := rng.Derive(seed, label)
		rngs[i] = rng.New(fieldSeed)
		fmt.Printf("%s -> %s (seed: %d)\n", field.Name, field.Gen, fieldSeed)
	}

	for row := int64(0); row < entity.Count; row++ {
		record := writer.NewOrderedMap()

		for i, field := range entity.Fields {
			val, err := gens[i].Next(rngs[i])
			if err != nil {
				return fmt.Errorf(" row %d, field '%s': %w", row, field.Name, err)
			}

			record.Set(field.Name, val)
		}

		if err := e.writer.WriteRecord(entity.Name, record); err != nil {
			return err
		}
	}
	return nil
}
