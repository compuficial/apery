package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI runs the CLI in-process and returns stdout, stderr, and exit code.
func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const twoEntityPlan = `seed: 1
entities:
  - name: User
    count: 3
    fields:
      - {name: id, gen: seq}
  - name: Order
    driven_by: {entity: User, field: id, as: user_id, min: 1, max: 3}
    fields:
      - {name: total, gen: int}
`

const unknownGenPlan = `seed: 1
entities:
  - name: U
    count: 3
    fields:
      - {name: a, gen: uuidd}
`

// Row 0 divides by zero: a generation (not validation) failure.
const runtimeErrorPlan = `seed: 1
entities:
  - name: A
    count: 2
    fields: [{name: id, gen: seq}]
  - name: B
    count: 10
    fields:
      - {name: z, gen: int, config: {min: 0, max: 0}}
      - {name: q, gen: expr, config: {expr: "1 / {z}"}}
`

func TestValidate_ValidPlan(t *testing.T) {
	path := writeFile(t, t.TempDir(), "p.yaml", twoEntityPlan)
	stdout, _, code := runCLI(t, "validate", "-f", path)
	if code != exitOK || !strings.Contains(stdout, "Plan is valid.") {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
}

// validate must build generators, not just check structure.
func TestValidate_UnknownGenerator(t *testing.T) {
	path := writeFile(t, t.TempDir(), "p.yaml", unknownGenPlan)
	_, stderr, code := runCLI(t, "validate", "-f", path)
	if code != exitValidation || !strings.Contains(stderr, `unknown generator "uuidd"`) || !strings.Contains(stderr, `did you mean "uuid"`) {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestValidate_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	var result struct {
		Valid  bool `json:"valid"`
		Issues []struct {
			Path string `json:"path"`
			Code string `json:"code"`
			Hint string `json:"hint"`
		} `json:"issues"`
		Entities []struct {
			Name    string   `json:"name"`
			Columns []string `json:"columns"`
		} `json:"entities"`
	}

	stdout, _, code := runCLI(t, "validate", "-f", writeFile(t, dir, "bad.yaml", unknownGenPlan), "-o", "json")
	if code != exitValidation {
		t.Fatalf("code = %d", code)
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if result.Valid || len(result.Issues) != 1 || result.Issues[0].Code != "unknown_generator" || result.Issues[0].Path != "entities[0].fields[0].gen" {
		t.Fatalf("result = %+v", result)
	}

	stdout, _, code = runCLI(t, "validate", "-f", writeFile(t, dir, "ok.yaml", twoEntityPlan), "-o", "json")
	if code != exitOK {
		t.Fatalf("code = %d", code)
	}
	result.Issues = nil
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Valid || len(result.Entities) != 2 || strings.Join(result.Entities[1].Columns, ",") != "user_id,total" {
		t.Fatalf("result = %+v", result)
	}
}

// Parse errors are issues too, so agents get one JSON shape.
func TestValidate_JSONParseError(t *testing.T) {
	path := writeFile(t, t.TempDir(), "p.yaml", "seed: 1\nentities:\n  - name: U\n    cout: 3\n")
	stdout, _, code := runCLI(t, "validate", "-f", path, "-o", "json")
	if code != exitValidation || !strings.Contains(stdout, `"parse_error"`) || !strings.Contains(stdout, "cout") {
		t.Fatalf("code=%d stdout=%s", code, stdout)
	}
}

// Plan problems found by generate are validation errors (1), not
// generation errors (2), and nothing is written.
func TestGenerate_PlanErrorExitsValidation(t *testing.T) {
	path := writeFile(t, t.TempDir(), "p.yaml", unknownGenPlan)
	stdout, _, code := runCLI(t, "generate", "-f", path)
	if code != exitValidation || stdout != "" {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
}

func TestGenerate_RuntimeErrorExitsGeneration(t *testing.T) {
	path := writeFile(t, t.TempDir(), "p.yaml", runtimeErrorPlan)
	_, stderr, code := runCLI(t, "generate", "-f", path, "--workers", "1", "--chunk-size", "2")
	if code != exitGeneration || !strings.Contains(stderr, "division by zero") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

// One CSV stream can hold one entity; refuse before generating.
func TestGenerate_CSVMultiEntityNeedsSplit(t *testing.T) {
	path := writeFile(t, t.TempDir(), "p.yaml", twoEntityPlan)
	stdout, stderr, code := runCLI(t, "generate", "-f", path, "-o", "csv")
	if code != exitValidation || stdout != "" || !strings.Contains(stderr, "--split-entities") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestGenerate_DryRunSummary(t *testing.T) {
	path := writeFile(t, t.TempDir(), "p.yaml", twoEntityPlan)
	stdout, _, code := runCLI(t, "generate", "-f", path, "--dry-run")
	if code != exitOK {
		t.Fatalf("code = %d", code)
	}
	for _, want := range []string{"Plan is valid.", "User", "3", "Order", "3-9 (~6)", "user_id, total"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, stdout)
		}
	}
}

func TestGenerate_Limit(t *testing.T) {
	path := writeFile(t, t.TempDir(), "p.yaml", twoEntityPlan)
	stdout, _, code := runCLI(t, "generate", "-f", path, "--limit", "2")
	if code != exitOK {
		t.Fatalf("code = %d", code)
	}
	if got := strings.Count(stdout, `"_entity":"User"`); got != 2 {
		t.Errorf("User rows = %d, want 2", got)
	}
	if got := strings.Count(stdout, `"_entity":"Order"`); got != 2 {
		t.Errorf("Order rows = %d, want 2", got)
	}
}

// A failed run must not leave plausible-looking partial files behind.
func TestGenerate_OutputDirIsAtomic(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")

	_, _, code := runCLI(t, "generate", "-f", writeFile(t, dir, "bad.yaml", runtimeErrorPlan), "--output-dir", out, "--workers", "1")
	if code != exitGeneration {
		t.Fatalf("code = %d", code)
	}
	if entries, _ := os.ReadDir(out); len(entries) != 0 {
		t.Fatalf("failed run left files: %v", entries)
	}

	_, _, code = runCLI(t, "generate", "-f", writeFile(t, dir, "ok.yaml", twoEntityPlan), "--output-dir", out, "--split-entities")
	if code != exitOK {
		t.Fatalf("code = %d", code)
	}
	entries, _ := os.ReadDir(out)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "Order.jsonl,User.jsonl" {
		t.Fatalf("output dir = %v", names)
	}
}

func TestGenerate_UnknownFormat(t *testing.T) {
	path := writeFile(t, t.TempDir(), "p.yaml", twoEntityPlan)
	if _, _, code := runCLI(t, "generate", "-f", path, "-o", "xml"); code != exitValidation {
		t.Fatalf("code = %d", code)
	}
}

// Every shipped example must validate and generate (regression for JSON
// plans, which once failed on integer config).
func TestExamplesGenerate(t *testing.T) {
	plans, _ := filepath.Glob("../../examples/*.*")
	if len(plans) == 0 {
		t.Fatal("no examples found")
	}
	for _, path := range plans {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, stderr, code := runCLI(t, "validate", "-f", path); code != exitOK {
				t.Fatalf("validate: code=%d stderr=%s", code, stderr)
			}
			if _, stderr, code := runCLI(t, "generate", "-f", path, "--output-dir", t.TempDir(), "--split-entities"); code != exitOK {
				t.Fatalf("generate: code=%d stderr=%s", code, stderr)
			}
		})
	}
}

func TestListGenerators_JSON(t *testing.T) {
	stdout, _, code := runCLI(t, "list", "generators", "-o", "json")
	var infos []struct {
		Name       string `json:"name"`
		ConfigKeys []any  `json:"config_keys"`
	}
	if code != exitOK || json.Unmarshal([]byte(stdout), &infos) != nil || len(infos) < 20 {
		t.Fatalf("code=%d stdout=%.200s", code, stdout)
	}
}

func TestListGenerators_YAML(t *testing.T) {
	stdout, _, code := runCLI(t, "list", "generators", "-o", "yaml")
	if code != exitOK || !strings.Contains(stdout, "- name: bool") {
		t.Fatalf("code=%d stdout=%.200s", code, stdout)
	}
}

func TestDescribeGenerator(t *testing.T) {
	stdout, _, code := runCLI(t, "describe", "generator", "int")
	if code != exitOK || !strings.Contains(stdout, "Generator: int") {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}

	stdout, _, code = runCLI(t, "describe", "generator", "int", "-o", "json")
	var info struct {
		Name string `json:"name"`
	}
	if code != exitOK || json.Unmarshal([]byte(stdout), &info) != nil || info.Name != "int" {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}

	if _, stderr, code := runCLI(t, "describe", "generator", "nope"); code != exitValidation || !strings.Contains(stderr, "unknown generator") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestDescribePlanSchema(t *testing.T) {
	stdout, _, code := runCLI(t, "describe", "plan-schema")
	var schema map[string]any
	if code != exitOK || json.Unmarshal([]byte(stdout), &schema) != nil {
		t.Fatalf("code=%d stdout=%.200s", code, stdout)
	}
	if schema["$schema"] == nil || !strings.Contains(stdout, `"const": "rel_ref"`) || !strings.Contains(stdout, `"null_rate"`) {
		t.Fatalf("schema missing expected content: %.300s", stdout)
	}
}

// A null renders as an empty CSV cell and a JSON null.
func TestGenerate_NullRate(t *testing.T) {
	path := writeFile(t, t.TempDir(), "plan.yaml", `seed: 1
entities:
  - name: U
    count: 4
    fields:
      - {name: id, gen: seq}
      - {name: x, gen: const, config: {value: v}, null_rate: 1}
`)
	stdout, stderr, code := runCLI(t, "generate", "-f", path, "-o", "csv")
	if code != exitOK || stdout != "_entity,id,x\nU,1,\nU,2,\nU,3,\nU,4,\n" {
		t.Fatalf("csv: code=%d stdout=%q stderr=%s", code, stdout, stderr)
	}
	stdout, _, code = runCLI(t, "generate", "-f", path, "--limit", "1")
	if code != exitOK || stdout != `{"_entity":"U","id":1,"x":null}`+"\n" {
		t.Fatalf("jsonl: code=%d stdout=%q", code, stdout)
	}
}

// --entities writes one entity of a multi-entity plan, so it can stream as
// CSV, and its rows match the full run.
func TestGenerate_Entities(t *testing.T) {
	path := writeFile(t, t.TempDir(), "plan.yaml", twoEntityPlan)
	full, _, _ := runCLI(t, "generate", "-f", path)
	var want strings.Builder
	for line := range strings.Lines(full) {
		if strings.HasPrefix(line, `{"_entity":"Order"`) {
			want.WriteString(line)
		}
	}

	got, stderr, code := runCLI(t, "generate", "-f", path, "--entities", "Order")
	if code != exitOK || got != want.String() || want.Len() == 0 {
		t.Fatalf("code=%d stderr=%s\ngot  %q\nwant %q", code, stderr, got, want.String())
	}

	csv, _, code := runCLI(t, "generate", "-f", path, "--entities", " Order,Order ", "-o", "csv")
	if code != exitOK || !strings.HasPrefix(csv, "_entity,user_id,total\nOrder,") {
		t.Errorf("csv: code=%d stdout=%.80q", code, csv)
	}

	dry, _, code := runCLI(t, "generate", "-f", path, "--entities", "Order", "--dry-run")
	if code != exitOK || !strings.Contains(dry, "User (dependency)") || !strings.Contains(dry, "not written") {
		t.Errorf("dry-run: code=%d stdout=%s", code, dry)
	}

	_, stderr, code = runCLI(t, "generate", "-f", path, "--entities", "Ordr")
	if code != exitValidation || !strings.Contains(stderr, `no entity named "Ordr" to select (did you mean "Order"?)`) {
		t.Errorf("unknown entity: code=%d stderr=%s", code, stderr)
	}

	_, stderr, code = runCLI(t, "generate", "-f", path, "--entities", "User,Order", "-o", "csv")
	if code != exitValidation || !strings.Contains(stderr, "writes 2") {
		t.Errorf("csv with two entities: code=%d stderr=%s", code, stderr)
	}
}

// A regex repeat above max_repeat is a plan error (exit 1) reported by
// validate, not a generation failure on some row (exit 2).
func TestValidate_RegexRepeatAboveMaxRepeat(t *testing.T) {
	path := writeFile(t, t.TempDir(), "plan.yaml", `seed: 1
entities:
  - name: U
    count: 3
    fields:
      - {name: bio, gen: regex, config: {pattern: "[a-z]{40,80}"}}
`)
	for _, cmd := range []string{"validate", "generate"} {
		_, stderr, code := runCLI(t, cmd, "-f", path)
		if code != exitValidation || !strings.Contains(stderr, "set max_repeat to at least 40") {
			t.Errorf("%s: code=%d stderr=%s", cmd, code, stderr)
		}
	}
}

func TestMissingFileFlag(t *testing.T) {
	if _, stderr, code := runCLI(t, "generate"); code != exitValidation || !strings.Contains(stderr, "file") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}
