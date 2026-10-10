package runtime

import (
	"fmt"
	"slices"
	"strings"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/registry"
	"github.com/compuficial/apery/internal/suggest"
)

// Selection is the part of a plan a run produces when limited to some
// entities (WithEntities). Both lists are in plan order.
type Selection struct {
	Written      []string // the requested entities
	Dependencies []string // generated only because written entities reference them
}

// Select validates p and resolves names the way a run limited to them would.
// With no names, every entity is written.
func Select(p *plan.Plan, names []string) (Selection, error) {
	entities, err := compile(p, registry.NewEnv(baseDirOf(p)))
	if err != nil {
		return Selection{}, err
	}
	written, err := selectEntities(entities, names)
	if err != nil {
		return Selection{}, err
	}

	var out Selection
	for _, ent := range entitiesToRun(entities, written) {
		if name := ent.spec.Name; written[name] {
			out.Written = append(out.Written, name)
		} else {
			out.Dependencies = append(out.Dependencies, name)
		}
	}
	return out, nil
}

// selectEntities resolves names against the plan's entities and returns the
// set to write. With no names, every entity is written.
func selectEntities(entities []compiledEntity, names []string) (map[string]bool, error) {
	known := make([]string, len(entities))
	for i, ent := range entities {
		known[i] = ent.spec.Name
	}
	if len(names) == 0 {
		names = known
	}

	written := make(map[string]bool, len(names))
	var issues []plan.Issue
	for _, name := range names {
		if !slices.Contains(known, name) {
			issues = append(issues, unknownEntity(name, known))
			continue
		}
		written[name] = true
	}
	if len(issues) > 0 {
		return nil, &plan.ValidationError{Issues: issues}
	}
	return written, nil
}

// entitiesToRun returns, in plan order, the entities a run generates: the
// written ones, plus every entity they read through driven_by or rel_ref,
// transitively. An entity that is only a dependency keeps just the fields
// later entities read and the fields those depend on.
//
// Dependencies always precede their dependents, so one pass from the end
// knows everything an entity must provide before reaching it. Every entity
// and field has its own seed, so what is kept generates exactly what a full
// run would.
func entitiesToRun(entities []compiledEntity, written map[string]bool) []compiledEntity {
	read := map[string]map[string]bool{} // entity -> columns later entities read
	markRead := func(entity, column string) {
		if read[entity] == nil {
			read[entity] = map[string]bool{}
		}
		read[entity][column] = true
	}

	var run []compiledEntity
	for i := len(entities) - 1; i >= 0; i-- {
		ent := entities[i]
		switch name := ent.spec.Name; {
		case written[name]:
		case len(read[name]) > 0:
			ent = keepFieldsFor(ent, read[name])
		default:
			continue // nothing reads it
		}

		if db := ent.spec.DrivenBy; db != nil {
			markRead(db.Entity, db.Field)
			for _, pf := range db.Expose {
				markRead(db.Entity, pf.Field)
			}
		}
		for _, f := range ent.fields {
			for _, ref := range f.refs {
				markRead(ref.Entity, ref.Field)
			}
		}
		run = append(run, ent)
	}
	slices.Reverse(run)
	return run
}

// keepFieldsFor drops the fields of ent that neither produce one of columns
// nor feed, through row dependencies, a field that does. Row dependencies
// point to earlier fields, so one pass from the end finds them all.
func keepFieldsFor(ent compiledEntity, columns map[string]bool) compiledEntity {
	needed := make(map[string]bool, len(columns))
	for c := range columns {
		needed[c] = true
	}
	for _, f := range slices.Backward(ent.fields) {
		if needed[f.name] {
			for _, dep := range f.deps {
				needed[dep] = true
			}
		}
	}

	kept := ent
	kept.fields, kept.isBatchScoped = nil, false
	for _, f := range ent.fields {
		if needed[f.name] {
			kept.fields = append(kept.fields, f)
			kept.isBatchScoped = kept.isBatchScoped || f.isBatchScoped
		}
	}
	return kept
}

// unknownEntity is the issue for selecting an entity the plan doesn't have.
func unknownEntity(name string, known []string) plan.Issue {
	hint := "the plan's entities are " + strings.Join(known, ", ")
	if guess := suggest.Closest(name, known); guess != "" {
		hint = fmt.Sprintf("did you mean %q?", guess)
	}
	return plan.Issue{Path: "entities", Code: "unknown_entity",
		Message: fmt.Sprintf("no entity named %q to select", name), Hint: hint}
}
