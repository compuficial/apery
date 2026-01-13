// Package plan defines the declarative schema for synthetic data generation.
//
// A Plan is the top-level structure that describes what data to generate,
// consisting of entities (tables/collections) with fields that use configured
// generators. Plans are deterministic: the same plan with the same seed always
// produces identical output.
package plan

// Plan represents a full generation request
type Plan struct {
	Seed     int64
	Entities []EntitySpec
}

// EntitySpec defines a table/collection to generate
type EntitySpec struct {
	Name   string
	Count  int64
	Fields []FieldSpec
}

// FieldSpec defines a single column/field
type FieldSpec struct {
	Name   string
	Gen    string
	Config map[string]any
}
