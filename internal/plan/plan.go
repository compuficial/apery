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

// EntitySpec defines a table/collection to generate.
// Exactly one of Count or DrivenBy must be set.
type EntitySpec struct {
	Name     string
	Count    int64
	DrivenBy *DrivenBy
	Fields   []FieldSpec
}

// DrivenBy configures parent-driven child row generation (1:M relationships).
// When set on an EntitySpec, the executor generates Min to Max child rows per
// parent row instead of using Count. The parent's Field value is auto-injected
// into each child row under the name As.
type DrivenBy struct {
	Entity string // parent entity name
	Field  string // parent field to inject into child rows
	As     string // field name in child row for the injected value
	Min    int64  // minimum children per parent (must be >= 1)
	Max    int64  // maximum children per parent (must be >= Min)
}

// FieldSpec defines a single column/field
type FieldSpec struct {
	Name   string
	Gen    string
	Config map[string]any
}
