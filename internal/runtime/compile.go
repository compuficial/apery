package runtime

import (
	"errors"
	"fmt"
	"maps"
	"sort"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/registry"
)

// compiledField is a plan field resolved against the registry.
type compiledField struct {
	name     string
	gen      string
	config   map[string]any // plan config plus the run's Env
	factory  registry.Factory
	refs     []registry.ColumnRef // columns of earlier entities this field reads
	deps     []string             // columns of its own row this field reads
	nullRate float64              // fraction of rows set to null
	// isBatchScoped is set when the generator keeps per-batch state
	// (Resettable).
	isBatchScoped bool
	// isSequential is set when each value builds on the row before
	// (Sequential).
	isSequential bool
}

// compiledEntity is an entity whose generators have all been built once.
type compiledEntity struct {
	spec   *plan.EntitySpec
	index  int
	fields []compiledField
	// isBatchScoped is set when a field keeps per-batch state (Resettable).
	// A standalone entity then runs as one sequential batch; driven_by
	// chunks align to parent boundaries.
	isBatchScoped bool
	// isSequential is set when a field builds on the row before
	// (Sequential). The entity then runs in order on one worker with one set
	// of instances.
	isSequential bool
}

// Validate checks a plan's structure and builds every generator once, so
// unknown generators, bad config, and field-ordering problems surface before
// any data is generated. Problems are reported as a *plan.ValidationError.
func Validate(p *plan.Plan) error {
	_, err := compile(p, registry.NewEnv(baseDirOf(p)))
	return err
}

func baseDirOf(p *plan.Plan) string {
	if p == nil {
		return ""
	}
	return p.BaseDir
}

// compile validates the plan and resolves every field, collecting all
// structural and generator issues into one *plan.ValidationError.
func compile(p *plan.Plan, env *registry.Env) ([]compiledEntity, error) {
	var issues []plan.Issue
	if err := plan.Validate(p); err != nil {
		var verr *plan.ValidationError
		if !errors.As(err, &verr) || p == nil || len(p.Entities) == 0 {
			return nil, err
		}
		issues = verr.Issues
	}

	entities := make([]compiledEntity, len(p.Entities))
	for i := range p.Entities {
		ent, entityIssues := compileEntity(&p.Entities[i], i, env)
		entities[i] = ent
		issues = append(issues, entityIssues...)
	}

	if len(issues) > 0 {
		return nil, &plan.ValidationError{Issues: issues}
	}
	return entities, nil
}

func compileEntity(e *plan.EntitySpec, index int, env *registry.Env) (compiledEntity, []plan.Issue) {
	ent := compiledEntity{spec: e, index: index}
	var issues []plan.Issue

	// driven_by injects these columns before any field runs.
	known := map[string]bool{}
	if db := e.DrivenBy; db != nil {
		known[db.As] = true
		for _, pf := range db.Expose {
			known[pf.ChildName()] = true
		}
		if db.IndexAs != "" {
			known[db.IndexAs] = true
		}
	}

	for i := range e.Fields {
		f := &e.Fields[i]
		path := fmt.Sprintf("entities[%d].fields[%d]", index, i)
		cf, gen, issue := compileField(f, env, known)
		known[f.Name] = true
		if issue != nil {
			issue.Path = path + issue.Path
			issue.Entity, issue.Field = e.Name, f.Name
			issues = append(issues, *issue)
			continue
		}
		if gen == nil {
			continue // structural issue already reported by plan.Validate
		}

		ent.isBatchScoped = ent.isBatchScoped || cf.isBatchScoped
		ent.isSequential = ent.isSequential || cf.isSequential
		ent.fields = append(ent.fields, cf)
	}
	return ent, issues
}

// compileField builds the field's generator once to validate it. The
// returned issue's Path is relative to the field.
func compileField(f *plan.FieldSpec, env *registry.Env, known map[string]bool) (compiledField, registry.Generator, *plan.Issue) {
	if f.Gen == "" {
		return compiledField{}, nil, nil
	}

	factory, err := registry.FactoryFor(f.Gen)
	if err != nil {
		hint := "run 'apery list generators'"
		var unknown *registry.UnknownGeneratorError
		if errors.As(err, &unknown) && unknown.Suggestion != "" {
			hint = fmt.Sprintf("did you mean %q?", unknown.Suggestion)
		}
		return compiledField{}, nil, &plan.Issue{Path: ".gen", Code: "unknown_generator",
			Message: fmt.Sprintf("unknown generator %q", f.Gen), Hint: hint}
	}

	config := withEnv(f.Config, env)
	gen, err := registry.Get(f.Gen, config)
	if err != nil {
		issue := &plan.Issue{Path: ".config", Code: "invalid_config", Message: err.Error(),
			Hint: fmt.Sprintf("run 'apery describe generator %s'", f.Gen)}
		if unknownKey, ok := errors.AsType[*registry.UnknownKeyError](err); ok {
			issue.Code, issue.Hint = "unknown_config_key", unknownKey.Hint()
		}
		return compiledField{}, nil, issue
	}

	var deps []string
	if dd, ok := gen.(registry.DependencyDeclarer); ok {
		deps = dd.Dependencies()
		for _, dep := range deps {
			if !known[dep] {
				return compiledField{}, nil, &plan.Issue{Path: ".config", Code: "dependency_order",
					Message: fmt.Sprintf("field '%s' references '%s', which must be declared before it", f.Name, dep)}
			}
		}
	}

	cf := compiledField{name: f.Name, gen: f.Gen, config: config, factory: factory, deps: deps, nullRate: f.NullRate}
	_, cf.isBatchScoped = gen.(registry.Resettable)
	_, cf.isSequential = gen.(registry.Sequential)
	if sc, ok := gen.(registry.StoreConsumer); ok {
		cf.refs = sc.StoreRefs()
	}
	return cf, gen, nil
}

// withEnv returns a copy of config carrying the run's Env.
func withEnv(config map[string]any, env *registry.Env) map[string]any {
	out := make(map[string]any, len(config)+1)
	maps.Copy(out, config)
	out[registry.EnvKey] = env
	return out
}

// requiredColumns lists, per entity, the columns later entities read: the
// driven_by parent columns and every field's store references.
func requiredColumns(entities []compiledEntity) map[string][]string {
	sets := make(map[string]map[string]bool)
	add := func(entity, field string) {
		if sets[entity] == nil {
			sets[entity] = make(map[string]bool)
		}
		sets[entity][field] = true
	}

	for _, ent := range entities {
		if db := ent.spec.DrivenBy; db != nil {
			add(db.Entity, db.Field)
			for _, pf := range db.Expose {
				add(db.Entity, pf.Field)
			}
		}
		for _, f := range ent.fields {
			for _, ref := range f.refs {
				add(ref.Entity, ref.Field)
			}
		}
	}

	required := make(map[string][]string, len(sets))
	for entity, set := range sets {
		fields := make([]string, 0, len(set))
		for field := range set {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		required[entity] = fields
	}
	return required
}
