// Package plan defines the declarative schema for synthetic data generation.
//
// A Plan is the top-level structure that describes what data to generate,
// consisting of entities (tables/collections) with fields that use configured
// generators. Plans are deterministic: the same plan with the same seed always
// produces identical output.
package plan

// Plan represents a full generation request
type Plan struct {
	Seed     int64        `json:"seed" yaml:"seed"`
	Entities []EntitySpec `json:"entities" yaml:"entities"`
}

// EntitySpec defines a table/collection to generate.
// Exactly one of Count or DrivenBy must be set.
type EntitySpec struct {
	Name     string      `json:"name" yaml:"name"`
	Count    int64       `json:"count,omitempty" yaml:"count,omitempty"`
	DrivenBy *DrivenBy   `json:"driven_by,omitempty" yaml:"driven_by,omitempty"`
	Fields   []FieldSpec `json:"fields" yaml:"fields"`
}

// DrivenBy configures parent-driven child row generation (1:M relationships).
// When set on an EntitySpec, the executor generates Min to Max child rows per
// parent row instead of using Count. The parent's Field value is auto-injected
// into each child row under the name As.
type DrivenBy struct {
	Entity string `json:"entity" yaml:"entity"` // parent entity name
	Field  string `json:"field" yaml:"field"`   // parent field to inject into child rows
	As     string `json:"as" yaml:"as"`          // field name in child row for the injected value
	Min    int64  `json:"min" yaml:"min"`        // minimum children per parent (must be >= 1)
	Max    int64  `json:"max" yaml:"max"`        // maximum children per parent (must be >= Min)
}

// FieldSpec defines a single column/field
type FieldSpec struct {
	Name   string         `json:"name" yaml:"name"`
	Gen    string         `json:"gen" yaml:"gen"`
	Config map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
}
