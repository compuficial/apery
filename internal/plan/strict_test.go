package plan

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePlan(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A misspelled key must fail loudly instead of silently dropping config.
func TestLoadFile_RejectsUnknownKeys(t *testing.T) {
	yamlPlan := "seed: 1\nentities:\n  - name: U\n    count: 1\n    fields:\n      - name: a\n        gen: int\n        confg: {min: 1}\n"
	if _, err := LoadFile(writePlan(t, "p.yaml", yamlPlan)); err == nil || !strings.Contains(err.Error(), "confg") {
		t.Fatalf("YAML error = %v, want unknown key confg", err)
	}

	jsonPlan := `{"seed": 1, "entities": [{"name": "U", "count": 1, "fields": [{"name": "a", "gen": "int", "confg": {}}]}]}`
	if _, err := LoadFile(writePlan(t, "p.json", jsonPlan)); err == nil || !strings.Contains(err.Error(), "confg") {
		t.Fatalf("JSON error = %v, want unknown key confg", err)
	}
}

func TestLoadFile_RejectsTrailingJSON(t *testing.T) {
	body := `{"seed": 1, "entities": [{"name": "U", "count": 1, "fields": [{"name": "a", "gen": "seq"}]}]} {}`
	if _, err := LoadFile(writePlan(t, "p.json", body)); err == nil {
		t.Fatal("expected error for trailing JSON")
	}
}

// JSON config numbers keep integer type and full precision, like YAML.
func TestLoadFile_JSONIntegersAreExact(t *testing.T) {
	body := `{"seed": 1, "entities": [{"name": "U", "count": 1, "fields": [
		{"name": "a", "gen": "seq", "config": {"start": 9007199254740993}},
		{"name": "b", "gen": "float", "config": {"min": 0.5, "max": 2}},
		{"name": "c", "gen": "pick", "config": {"values": [1, 2.5, {"n": 3}]}}]}]}`
	p, err := LoadFile(writePlan(t, "p.json", body))
	if err != nil {
		t.Fatal(err)
	}
	fields := p.Entities[0].Fields
	if got := fields[0].Config["start"]; got != 9007199254740993 {
		t.Errorf("start = %#v, want int 9007199254740993", got)
	}
	if got := fields[1].Config["min"]; got != 0.5 {
		t.Errorf("min = %#v, want 0.5", got)
	}
	if got := fields[1].Config["max"]; got != 2 {
		t.Errorf("max = %#v, want int 2", got)
	}
	values := fields[2].Config["values"].([]any)
	if values[0] != 1 || values[1] != 2.5 || values[2].(map[string]any)["n"] != 3 {
		t.Errorf("values = %#v", values)
	}
}

// Relative paths inside the plan resolve against the plan's directory.
func TestLoadFile_SetsBaseDir(t *testing.T) {
	path := writePlan(t, "p.yaml", "seed: 1\nentities:\n  - name: U\n    count: 1\n    fields: [{name: a, gen: seq}]\n")
	p, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.BaseDir != filepath.Dir(path) {
		t.Fatalf("BaseDir = %q, want %q", p.BaseDir, filepath.Dir(path))
	}
}

// Validation reports every problem at once, each with a path and code.
func TestValidate_ReportsAllIssues(t *testing.T) {
	p := &Plan{Entities: []EntitySpec{
		{Name: "A", Count: 1, Fields: []FieldSpec{{Name: "x"}}},
		{Name: "B", Count: 1, Fields: []FieldSpec{{Name: "y", Gen: "seq"}, {Name: "y", Gen: "seq"}}},
	}}
	err := Validate(p)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error %v is not a *ValidationError", err)
	}
	if len(verr.Issues) != 2 {
		t.Fatalf("issues = %+v, want 2", verr.Issues)
	}
	want := []Issue{
		{Path: "entities[0].fields[0].gen", Entity: "A", Field: "x", Code: "missing_generator"},
		{Path: "entities[1].fields[1].name", Entity: "B", Field: "y", Code: "duplicate_field"},
	}
	for i, w := range want {
		got := verr.Issues[i]
		if got.Path != w.Path || got.Entity != w.Entity || got.Field != w.Field || got.Code != w.Code {
			t.Errorf("issue %d = %+v, want %+v", i, got, w)
		}
	}
}

// Entity names become file names, so they must be plain identifiers.
func TestValidate_RejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"../escaped", "a/b", `a\b`, "a b", "1abc", ".hidden"} {
		p := &Plan{Entities: []EntitySpec{{Name: name, Count: 1, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}}}}
		if err := Validate(p); err == nil || !strings.Contains(err.Error(), "invalid_name") && !strings.Contains(err.Error(), "must start with") {
			t.Errorf("entity %q: error = %v, want invalid name", name, err)
		}
	}
	p := &Plan{Entities: []EntitySpec{{Name: "U", Count: 1, Fields: []FieldSpec{{Name: "{x}", Gen: "seq"}}}}}
	if err := Validate(p); err == nil {
		t.Error("field name with braces accepted")
	}
}

// A standalone entity can't draw more unique values than its target holds.
func TestValidate_StandaloneUniqueFeasibility(t *testing.T) {
	p := &Plan{Entities: []EntitySpec{
		{Name: "Tag", Count: 5, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
		{Name: "X", Count: 10, Fields: []FieldSpec{{Name: "tag_id", Gen: "rel_ref",
			Config: map[string]any{"entity": "Tag", "field": "id", "unique": true}}}},
	}}
	if err := Validate(p); err == nil || !strings.Contains(err.Error(), "infeasible") {
		t.Fatalf("error = %v, want infeasible", err)
	}
}

// Summarize estimates rows (exact for count, a range for driven_by) and
// lists columns in output order.
func TestSummarize(t *testing.T) {
	p := &Plan{Entities: []EntitySpec{
		{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
		{Name: "Order", DrivenBy: &DrivenBy{Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 5,
			Expose: []ParentField{{Field: "id", As: "uid2"}}, IndexAs: "n"},
			Fields: []FieldSpec{{Name: "total", Gen: "int"}}},
		{Name: "Line", DrivenBy: &DrivenBy{Entity: "Order", Field: "user_id", As: "o", Min: 2, Max: 2},
			Fields: []FieldSpec{{Name: "sku", Gen: "seq"}}},
	}}
	got := Summarize(p)
	want := []EntitySummary{
		{Name: "User", Rows: RowEstimate{Min: 10, Max: 10, Expected: 10}, Columns: []string{"id"}},
		{Name: "Order", Rows: RowEstimate{Min: 10, Max: 50, Expected: 30}, Columns: []string{"user_id", "uid2", "n", "total"}},
		{Name: "Line", Rows: RowEstimate{Min: 20, Max: 100, Expected: 60}, Columns: []string{"o", "sku"}},
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].Rows != want[i].Rows || strings.Join(got[i].Columns, ",") != strings.Join(want[i].Columns, ",") {
			t.Errorf("summary %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
