package plan

import "strings"

// Issue is one validation problem, located by a path into the plan.
//
// Example: {Path: "entities[1].fields[2].config", Entity: "Order",
// Field: "total", Code: "invalid_config", Message: "float: 'min' must be a number"}
type Issue struct {
	Path    string `json:"path,omitempty"`
	Entity  string `json:"entity,omitempty"`
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// String renders the issue for humans: location, message, then hint.
func (i Issue) String() string {
	var b strings.Builder
	switch {
	case i.Entity != "" && i.Field != "":
		b.WriteString("entity \"" + i.Entity + "\", field \"" + i.Field + "\": ")
	case i.Entity != "":
		b.WriteString("entity \"" + i.Entity + "\": ")
	case i.Path != "":
		b.WriteString(i.Path + ": ")
	}
	b.WriteString(i.Message)
	if i.Hint != "" {
		b.WriteString(" (" + i.Hint + ")")
	}
	return b.String()
}

// ValidationError lists every problem found in a plan.
type ValidationError struct {
	Issues []Issue
}

// Error renders one issue per line.
func (e *ValidationError) Error() string {
	lines := make([]string, len(e.Issues))
	for i, issue := range e.Issues {
		lines[i] = "plan: " + issue.String()
	}
	return strings.Join(lines, "\n")
}
