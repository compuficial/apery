package plan

import (
	"errors"
	"fmt"
)

// Validate checks that a plan is well-formed and returns an error if not.
func Validate(p *Plan) error {
	if p == nil {
		return errors.New("plan: plan is nil")
	}

	if len(p.Entities) == 0 {
		return errors.New("plan: no entities defined")
	}

	entityNames := make(map[string]struct{}, len(p.Entities))
	for i := range p.Entities {
		e := &p.Entities[i]
		if err := validateEntity(e, i, entityNames); err != nil {
			return err
		}
		entityNames[e.Name] = struct{}{}
	}

	return nil
}

func validateEntity(e *EntitySpec, index int, existingNames map[string]struct{}) error {
	if e.Name == "" {
		return fmt.Errorf("plan: entity[%d]: missing name", index)
	}

	if _, dup := existingNames[e.Name]; dup {
		return fmt.Errorf("plan: duplicate entity name: %q", e.Name)
	}

	if e.Count <= 0 {
		return fmt.Errorf("plan: entity[%q]: count must be > 0, got %d", e.Name, e.Count)
	}

	if len(e.Fields) == 0 {
		return fmt.Errorf("plan: entity[%q]: no fields defined", e.Name)
	}

	fieldNames := make(map[string]struct{}, len(e.Fields))
	for i := range e.Fields {
		f := &e.Fields[i]
		if err := validateField(f, e.Name, i, fieldNames); err != nil {
			return err
		}
		fieldNames[f.Name] = struct{}{}
	}

	return nil
}

func validateField(f *FieldSpec, entityName string, index int, existingNames map[string]struct{}) error {
	if f.Name == "" {
		return fmt.Errorf("plan: entity[%q].fields[%d]: missing name", entityName, index)
	}

	if _, dup := existingNames[f.Name]; dup {
		return fmt.Errorf("plan: entity[%q]: duplicate field name: %q", entityName, f.Name)
	}

	if f.Gen == "" {
		return fmt.Errorf("plan: entity[%q].fields[%q]: missing generator", entityName, f.Name)
	}

	return nil
}
