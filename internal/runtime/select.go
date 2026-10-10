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
	sel, err := selectEntities(entities, names)
	if err != nil {
		return Selection{}, err
	}

	var out Selection
	for _, ent := range entities {
		switch name := ent.spec.Name; {
		case sel.write[name]:
			out.Written = append(out.Written, name)
		case sel.generate[name]:
			out.Dependencies = append(out.Dependencies, name)
		}
	}
	return out, nil
}

// selection says which entities a run generates and which it writes.
type selection struct {
	generate map[string]bool // written entities plus their dependencies
	write    map[string]bool
}

// selectEntities resolves names against the plan's entities. A selected
// entity's driven_by parent and rel_ref targets must be generated too,
// transitively. Dependencies always precede their dependents, so one pass
// from the end finds them all. Entity seeds come from plan positions, so a
// selected entity's output is the same as in a full run.
func selectEntities(entities []compiledEntity, names []string) (selection, error) {
	sel := selection{generate: map[string]bool{}, write: map[string]bool{}}
	known := make([]string, len(entities))
	for i, ent := range entities {
		known[i] = ent.spec.Name
	}
	if len(names) == 0 {
		names = known
	}

	var issues []plan.Issue
	for _, name := range names {
		if !slices.Contains(known, name) {
			issues = append(issues, unknownEntity(name, known))
			continue
		}
		sel.write[name], sel.generate[name] = true, true
	}
	if len(issues) > 0 {
		return selection{}, &plan.ValidationError{Issues: issues}
	}

	for i := len(entities) - 1; i >= 0; i-- {
		ent := &entities[i]
		if !sel.generate[ent.spec.Name] {
			continue
		}
		if db := ent.spec.DrivenBy; db != nil {
			sel.generate[db.Entity] = true
		}
		for _, f := range ent.fields {
			for _, ref := range f.refs {
				sel.generate[ref.Entity] = true
			}
		}
	}
	return sel, nil
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
