package plan

import (
	"errors"
	"fmt"
	"strings"
)

// validationContext holds shared state accumulated during plan validation.
type validationContext struct {
	entityNames  map[string]struct{}
	entityFields map[string]map[string]bool // entity name -> set of field names (including DrivenBy.As)
	entitySpecs  map[string]*EntitySpec
}

// Validate checks that a plan is well-formed and returns an error if not.
func Validate(p *Plan) error {
	if p == nil {
		return errors.New("plan: plan is nil")
	}

	if len(p.Entities) == 0 {
		return errors.New("plan: no entities defined")
	}

	vc := &validationContext{
		entityNames:  make(map[string]struct{}, len(p.Entities)),
		entityFields: make(map[string]map[string]bool, len(p.Entities)),
		entitySpecs:  make(map[string]*EntitySpec, len(p.Entities)),
	}

	for i := range p.Entities {
		e := &p.Entities[i]
		if err := validateEntity(e, i, vc); err != nil {
			return err
		}
		vc.entityNames[e.Name] = struct{}{}
		vc.entitySpecs[e.Name] = e

		// Build field set for this entity (including auto-injected driven_by columns).
		fields := make(map[string]bool, len(e.Fields)+1)
		if e.DrivenBy != nil {
			fields[e.DrivenBy.As] = true
			for _, pf := range e.DrivenBy.Expose {
				fields[pf.ChildName()] = true
			}
			if e.DrivenBy.IndexAs != "" {
				fields[e.DrivenBy.IndexAs] = true
			}
		}
		for _, f := range e.Fields {
			fields[f.Name] = true
		}
		vc.entityFields[e.Name] = fields
	}
	return nil
}

func validateEntity(e *EntitySpec, index int, vc *validationContext) error {
	if e.Name == "" {
		return fmt.Errorf("plan: entity[%d]: missing name", index)
	}

	if _, dup := vc.entityNames[e.Name]; dup {
		return fmt.Errorf("plan: duplicate entity name: %q", e.Name)
	}

	// Exactly one of Count or DrivenBy must be set.
	hasCount := e.Count > 0
	hasDrivenBy := e.DrivenBy != nil
	if hasCount == hasDrivenBy {
		return fmt.Errorf("plan: entity[%q]: exactly one of Count or DrivenBy must be set", e.Name)
	}

	if hasDrivenBy {
		if err := validateDrivenBy(e, vc); err != nil {
			return err
		}
	}

	if len(e.Fields) == 0 {
		return fmt.Errorf("plan: entity[%q]: no fields defined", e.Name)
	}

	fieldNames := make(map[string]struct{}, len(e.Fields))
	for i := range e.Fields {
		f := &e.Fields[i]
		if err := validateField(f, e, i, fieldNames, vc); err != nil {
			return err
		}
		fieldNames[f.Name] = struct{}{}
	}

	return nil
}

func validateDrivenBy(e *EntitySpec, vc *validationContext) error {
	db := e.DrivenBy

	// Parent entity must be declared before this entity.
	if _, exists := vc.entityNames[db.Entity]; !exists {
		return fmt.Errorf("plan: entity[%q]: driven_by entity %q not declared before %q", e.Name, db.Entity, e.Name)
	}

	// Parent field must exist in the parent entity.
	parentFields := vc.entityFields[db.Entity]
	if !parentFields[db.Field] {
		return fmt.Errorf("plan: entity[%q]: driven_by field '%s' does not exist in entity %q", e.Name, db.Field, db.Entity)
	}

	if db.As == "" {
		return fmt.Errorf("plan: entity[%q]: driven_by 'as' must not be empty", e.Name)
	}

	// Collect auto-injected columns and ensure none collide.
	injected := map[string]bool{}
	add := func(name string) error {
		if injected[name] {
			return fmt.Errorf("plan: entity[%q]: driven_by injects %q more than once", e.Name, name)
		}
		injected[name] = true
		return nil
	}
	if err := add(db.As); err != nil {
		return err
	}

	// Each exposed parent field must exist and inject a unique column.
	for _, pf := range db.Expose {
		if pf.Field == "" {
			return fmt.Errorf("plan: entity[%q]: driven_by expose entry missing 'field'", e.Name)
		}
		if !parentFields[pf.Field] {
			return fmt.Errorf("plan: entity[%q]: driven_by expose field '%s' does not exist in entity %q", e.Name, pf.Field, db.Entity)
		}
		if err := add(pf.ChildName()); err != nil {
			return err
		}
	}

	if db.IndexAs != "" {
		if err := add(db.IndexAs); err != nil {
			return err
		}
	}

	// No injected column may conflict with an explicitly declared child field.
	for _, f := range e.Fields {
		if _, clash := injected[f.Name]; clash {
			return fmt.Errorf("plan: entity[%q]: driven_by-injected column %q conflicts with declared field", e.Name, f.Name)
		}
	}

	if db.Min < 1 {
		return fmt.Errorf("plan: entity[%q]: driven_by min must be >= 1, got %d", e.Name, db.Min)
	}
	if db.Max < db.Min {
		return fmt.Errorf("plan: entity[%q]: driven_by max must be >= min (%d), got %d", e.Name, db.Min, db.Max)
	}

	return nil
}

func validateField(f *FieldSpec, entity *EntitySpec, index int, existingNames map[string]struct{}, vc *validationContext) error {
	if f.Name == "" {
		return fmt.Errorf("plan: entity[%q].fields[%d]: missing name", entity.Name, index)
	}

	if _, dup := existingNames[f.Name]; dup {
		return fmt.Errorf("plan: entity[%q]: duplicate field name: %q", entity.Name, f.Name)
	}

	if f.Gen == "" {
		return fmt.Errorf("plan: entity[%q].fields[%q]: missing generator", entity.Name, f.Name)
	}

	// Check for reserved config keys (underscore prefix).
	for key := range f.Config {
		if strings.HasPrefix(key, "_") {
			return fmt.Errorf("plan: entity[%q].fields[%q]: config key %q is reserved for internal use", entity.Name, f.Name, key)
		}
	}

	// Validate rel_ref cross-entity references.
	if f.Gen == "rel_ref" {
		if err := validateRelRef(f, entity, vc); err != nil {
			return err
		}
	}

	return nil
}

func validateRelRef(f *FieldSpec, entity *EntitySpec, vc *validationContext) error {
	targetEntity, _ := f.Config["entity"].(string)
	targetField, _ := f.Config["field"].(string)

	if targetEntity == "" {
		return fmt.Errorf("plan: entity[%q].fields[%q]: rel_ref requires 'entity' config", entity.Name, f.Name)
	}
	if targetField == "" {
		return fmt.Errorf("plan: entity[%q].fields[%q]: rel_ref requires 'field' config", entity.Name, f.Name)
	}

	// Target entity must be declared before this entity.
	if _, exists := vc.entityNames[targetEntity]; !exists {
		return fmt.Errorf("plan: entity[%q].fields[%q]: rel_ref entity %q not declared before %q", entity.Name, f.Name, targetEntity, entity.Name)
	}

	// Target field must exist in the target entity.
	targetFields := vc.entityFields[targetEntity]
	if !targetFields[targetField] {
		return fmt.Errorf("plan: entity[%q].fields[%q]: rel_ref field '%s' does not exist in entity %q", entity.Name, f.Name, targetField, targetEntity)
	}

	// Unique feasibility: for driven_by entities with unique rel_ref,
	// DrivenBy.Max must not exceed the target entity's Count.
	unique, _ := f.Config["unique"].(bool)
	if unique && entity.DrivenBy != nil {
		targetSpec := vc.entitySpecs[targetEntity]
		if targetSpec != nil && targetSpec.Count > 0 && entity.DrivenBy.Max > targetSpec.Count {
			return fmt.Errorf("plan: entity[%q].fields[%q]: unique rel_ref to %q (count=%d) infeasible: driven_by max=%d exceeds available values",
				entity.Name, f.Name, targetEntity, targetSpec.Count, entity.DrivenBy.Max)
		}
	}

	return nil
}
