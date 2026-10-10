package plan

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// singleIssue returns err's only issue, failing unless there is exactly one.
func singleIssue(t *testing.T, err error) Issue {
	t.Helper()
	verr, ok := errors.AsType[*ValidationError](err)
	if !ok || len(verr.Issues) != 1 {
		t.Fatalf("error = %v, want exactly one issue", err)
	}
	return verr.Issues[0]
}

func TestValidate_NullRateRange(t *testing.T) {
	withRate := func(rate float64) *Plan {
		return &Plan{Entities: []EntitySpec{{Name: "U", Count: 1, Fields: []FieldSpec{{Name: "x", Gen: "seq", NullRate: rate}}}}}
	}

	for _, rate := range []float64{-0.1, 1.5, math.NaN(), math.Inf(1)} {
		issue := singleIssue(t, Validate(withRate(rate)))
		if issue.Code != "invalid_null_rate" || issue.Path != "entities[0].fields[0].null_rate" {
			t.Errorf("null_rate %v: issue = %+v, want invalid_null_rate at .null_rate", rate, issue)
		}
	}
	for _, rate := range []float64{0, 0.5, 1} {
		if err := Validate(withRate(rate)); err != nil {
			t.Errorf("null_rate %v: %v", rate, err)
		}
	}
}

// A driven_by parent key can't be nullable, since children would point at a
// null parent. Exposing a nullable column is fine.
func TestValidate_NullableParentKey(t *testing.T) {
	p := &Plan{Entities: []EntitySpec{
		{Name: "User", Count: 3, Fields: []FieldSpec{
			{Name: "id", Gen: "seq", NullRate: 0.2},
			{Name: "email", Gen: "seq", NullRate: 0.5},
		}},
		{Name: "Order", DrivenBy: &DrivenBy{Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 2,
			Expose: []ParentField{{Field: "email"}}},
			Fields: []FieldSpec{{Name: "n", Gen: "seq"}}},
	}}
	issue := singleIssue(t, Validate(p))
	if issue.Code != "nullable_parent_key" || issue.Path != "entities[1].driven_by.field" || issue.Entity != "Order" {
		t.Errorf("issue = %+v, want nullable_parent_key at entities[1].driven_by.field", issue)
	}

	p.Entities[0].Fields[0].NullRate = 0
	if err := Validate(p); err != nil {
		t.Errorf("non-null parent key: %v", err)
	}
}

// Plan files accept null_rate in both formats.
func TestLoadFile_NullRate(t *testing.T) {
	files := map[string]string{
		"plan.yaml": "seed: 1\nentities:\n  - name: U\n    count: 1\n    fields:\n      - name: x\n        gen: seq\n        null_rate: 0.25\n",
		"plan.json": `{"seed": 1, "entities": [{"name": "U", "count": 1, "fields": [{"name": "x", "gen": "seq", "null_rate": 0.25}]}]}`,
	}
	for name, body := range files {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := LoadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := p.Entities[0].Fields[0].NullRate; got != 0.25 {
			t.Errorf("%s: null_rate = %v, want 0.25", name, got)
		}
	}
}
