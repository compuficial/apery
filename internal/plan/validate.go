package plan

import (
	"fmt"
	"regexp"
	"strings"
)

// namePattern restricts entity and column names to identifiers: entity names
// become file names in split output, and {name} references can't contain
// braces.
var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// validator accumulates issues across the whole plan.
type validator struct {
	issues       []Issue
	entityFields map[string]map[string]bool // entity name -> columns, including driven_by injections
	entitySpecs  map[string]*EntitySpec
}

// Validate checks that a plan is well-formed. It reports every problem it
// finds as a *ValidationError, not just the first.
func Validate(p *Plan) error {
	if p == nil {
		return &ValidationError{Issues: []Issue{{Code: "plan_nil", Message: "plan is nil"}}}
	}
	if len(p.Entities) == 0 {
		return &ValidationError{Issues: []Issue{{Path: "entities", Code: "no_entities", Message: "no entities defined"}}}
	}

	v := &validator{
		entityFields: make(map[string]map[string]bool, len(p.Entities)),
		entitySpecs:  make(map[string]*EntitySpec, len(p.Entities)),
	}
	for i := range p.Entities {
		e := &p.Entities[i]
		v.checkEntity(e, i)

		// Register even invalid entities, so references to them don't
		// cascade into misleading "not declared" issues.
		if e.Name != "" && v.entitySpecs[e.Name] == nil {
			v.entitySpecs[e.Name] = e
			v.entityFields[e.Name] = Columns(e)
		}
	}

	if len(v.issues) > 0 {
		return &ValidationError{Issues: v.issues}
	}
	return nil
}

// Columns returns the set of column names an entity produces: driven_by
// injections plus declared fields.
func Columns(e *EntitySpec) map[string]bool {
	cols := make(map[string]bool, len(e.Fields)+1)
	if db := e.DrivenBy; db != nil {
		cols[db.As] = true
		for _, pf := range db.Expose {
			cols[pf.ChildName()] = true
		}
		if db.IndexAs != "" {
			cols[db.IndexAs] = true
		}
	}
	for _, f := range e.Fields {
		cols[f.Name] = true
	}
	return cols
}

func (v *validator) add(issue Issue, format string, args ...any) {
	issue.Message = fmt.Sprintf(format, args...)
	v.issues = append(v.issues, issue)
}

func (v *validator) checkName(issue Issue, name string) {
	if namePattern.MatchString(name) {
		return
	}
	issue.Code = "invalid_name"
	v.add(issue, "name %q must start with a letter or underscore and contain only letters, digits, '_', '.', or '-'", name)
}

func (v *validator) checkEntity(e *EntitySpec, index int) {
	path := fmt.Sprintf("entities[%d]", index)
	at := Issue{Path: path, Entity: e.Name}

	switch {
	case e.Name == "":
		v.add(Issue{Path: path + ".name", Code: "missing_name"}, "missing name")
	case v.entitySpecs[e.Name] != nil:
		v.add(Issue{Path: path + ".name", Entity: e.Name, Code: "duplicate_entity"}, "duplicate entity name: %q", e.Name)
	default:
		v.checkName(Issue{Path: path + ".name", Entity: e.Name}, e.Name)
	}

	// Exactly one of Count or DrivenBy must be set.
	if (e.Count > 0) == (e.DrivenBy != nil) {
		at.Code = "count_or_driven_by"
		v.add(at, "exactly one of Count or DrivenBy must be set")
	}
	if e.DrivenBy != nil {
		v.checkDrivenBy(e, path)
	}

	if len(e.Fields) == 0 {
		at.Code = "no_fields"
		v.add(at, "no fields defined")
	}

	seen := make(map[string]bool, len(e.Fields))
	for i := range e.Fields {
		v.checkField(&e.Fields[i], e, fmt.Sprintf("%s.fields[%d]", path, i), seen)
	}
}

func (v *validator) checkDrivenBy(e *EntitySpec, entityPath string) {
	db := e.DrivenBy
	path := entityPath + ".driven_by"
	at := func(suffix, code string) Issue {
		return Issue{Path: path + suffix, Entity: e.Name, Code: code}
	}

	// Parent entity must be declared before this entity; field checks only
	// make sense once it is.
	parentFields, declared := v.entityFields[db.Entity]
	if !declared {
		v.add(at(".entity", "driven_by_order"), "driven_by entity %q not declared before %q", db.Entity, e.Name)
	} else if !parentFields[db.Field] {
		v.add(at(".field", "driven_by_field"), "driven_by field '%s' does not exist in entity %q", db.Field, db.Entity)
	} else if isNullable(v.entitySpecs[db.Entity], db.Field) {
		v.add(at(".field", "nullable_parent_key"), "driven_by field '%s' has a null_rate; children would point at null parents", db.Field)
	}

	// Injected columns must be valid names, distinct, and not shadow fields.
	injected := map[string]bool{}
	inject := func(name, suffix string) {
		v.checkName(Issue{Path: path + suffix, Entity: e.Name}, name)
		if injected[name] {
			v.add(at(suffix, "duplicate_injection"), "driven_by injects %q more than once", name)
		}
		injected[name] = true
	}

	if db.As == "" {
		v.add(at(".as", "driven_by_as"), "driven_by 'as' must not be empty")
	} else {
		inject(db.As, ".as")
	}
	for i, pf := range db.Expose {
		suffix := fmt.Sprintf(".expose[%d]", i)
		switch {
		case pf.Field == "":
			v.add(at(suffix, "driven_by_expose"), "driven_by expose entry missing 'field'")
			continue
		case declared && !parentFields[pf.Field]:
			v.add(at(suffix+".field", "driven_by_expose"), "driven_by expose field '%s' does not exist in entity %q", pf.Field, db.Entity)
		}
		inject(pf.ChildName(), suffix)
	}
	if db.IndexAs != "" {
		inject(db.IndexAs, ".index_as")
	}

	for i, f := range e.Fields {
		if injected[f.Name] {
			v.add(Issue{Path: fmt.Sprintf("%s.fields[%d].name", entityPath, i), Entity: e.Name, Field: f.Name, Code: "injection_conflict"},
				"driven_by-injected column %q conflicts with declared field", f.Name)
		}
	}

	if db.Min < 1 {
		v.add(at(".min", "driven_by_range"), "driven_by min must be >= 1, got %d", db.Min)
	}
	if db.Max < db.Min {
		v.add(at(".max", "driven_by_range"), "driven_by max must be >= min (%d), got %d", db.Min, db.Max)
	}
}

func (v *validator) checkField(f *FieldSpec, e *EntitySpec, path string, seen map[string]bool) {
	at := func(suffix, code string) Issue {
		return Issue{Path: path + suffix, Entity: e.Name, Field: f.Name, Code: code}
	}

	switch {
	case f.Name == "":
		v.add(at(".name", "missing_name"), "missing name")
	case seen[f.Name]:
		v.add(at(".name", "duplicate_field"), "duplicate field name: %q", f.Name)
	default:
		v.checkName(at(".name", ""), f.Name)
	}
	seen[f.Name] = true

	if f.Gen == "" {
		v.add(at(".gen", "missing_generator"), "missing generator")
	}
	if !(f.NullRate >= 0 && f.NullRate <= 1) {
		v.add(at(".null_rate", "invalid_null_rate"), "null_rate must be between 0 and 1, got %v", f.NullRate)
	}

	for key := range f.Config {
		if strings.HasPrefix(key, "_") {
			v.add(at(".config."+key, "reserved_config_key"), "config key %q is reserved for internal use", key)
		}
	}

	if f.Gen == "rel_ref" {
		v.checkRelRef(f, e, at)
	}
}

// isNullable reports whether e declares field with a null_rate.
func isNullable(e *EntitySpec, field string) bool {
	for _, f := range e.Fields {
		if f.Name == field {
			return f.NullRate > 0
		}
	}
	return false
}

func (v *validator) checkRelRef(f *FieldSpec, e *EntitySpec, at func(suffix, code string) Issue) {
	targetEntity, _ := f.Config["entity"].(string)
	targetField, _ := f.Config["field"].(string)

	if targetEntity == "" {
		v.add(at(".config.entity", "rel_ref_config"), "rel_ref requires 'entity' config")
		return
	}
	if targetField == "" {
		v.add(at(".config.field", "rel_ref_config"), "rel_ref requires 'field' config")
		return
	}

	// Target entity must be declared before this entity, with the field.
	targetFields, declared := v.entityFields[targetEntity]
	if !declared {
		v.add(at(".config.entity", "rel_ref_order"), "rel_ref entity %q not declared before %q", targetEntity, e.Name)
		return
	}
	if !targetFields[targetField] {
		v.add(at(".config.field", "rel_ref_field"), "rel_ref field '%s' does not exist in entity %q", targetField, targetEntity)
		return
	}

	// A unique draw can't take more values than the target has rows: per
	// parent batch for driven_by, across the whole entity otherwise.
	unique, _ := f.Config["unique"].(bool)
	target := v.entitySpecs[targetEntity]
	if !unique || target.Count == 0 {
		return
	}
	draws, scope := e.Count, "count"
	if e.DrivenBy != nil {
		draws, scope = e.DrivenBy.Max, "driven_by max"
	}
	if draws > target.Count {
		v.add(at(".config.unique", "unique_infeasible"),
			"unique rel_ref to %q (count=%d) infeasible: %s=%d exceeds available values", targetEntity, target.Count, scope, draws)
	}
}
