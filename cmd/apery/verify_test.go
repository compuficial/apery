package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// generateInto runs generate with args into a fresh directory and returns it.
func generateInto(t *testing.T, plan string, args ...string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "out")
	args = append([]string{"generate", "-f", plan, "--output-dir", dir}, args...)
	if _, stderr, code := runCLI(t, args...); code != exitOK {
		t.Fatalf("generate %v: code=%d stderr=%s", args, code, stderr)
	}
	return dir
}

// verifyJSON runs verify with a JSON report and decodes it.
func verifyJSON(t *testing.T, plan, dir string, args ...string) (verifyResult, int) {
	t.Helper()
	args = append([]string{"verify", "-f", plan, "--output-dir", dir, "--report", "json"}, args...)
	stdout, stderr, code := runCLI(t, args...)
	var r verifyResult
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("verify %v: code=%d stdout=%q stderr=%s", args, code, stdout, stderr)
	}
	return r, code
}

// Files generate wrote verify clean, with the same flags.
func TestVerify_Matches(t *testing.T) {
	dir := t.TempDir()
	two := writeFile(t, dir, "two.yaml", twoEntityPlan)
	one := writeFile(t, dir, "one.yaml", "seed: 3\nentities:\n  - name: U\n    count: 20\n    fields: [{name: id, gen: seq}, {name: x, gen: float}]\n")

	for _, c := range []struct {
		plan string
		args []string
	}{
		{two, nil},
		{two, []string{"--split-entities"}},
		{two, []string{"--split-entities", "-o", "csv"}},
		{two, []string{"--split-entities", "--entities", "Order"}},
		{two, []string{"--limit", "2", "--seed", "9"}},
		{one, []string{"-o", "csv"}},
	} {
		out := generateInto(t, c.plan, c.args...)
		r, code := verifyJSON(t, c.plan, out, c.args...)
		if code != exitOK || !r.Match || len(r.Entities) == 0 || r.Entities[0].Rows == 0 {
			t.Errorf("%v: code=%d result=%+v", c.args, code, r)
		}
	}

	out := generateInto(t, two)
	stdout, _, code := runCLI(t, "verify", "-f", two, "--output-dir", out)
	if code != exitOK || !strings.HasPrefix(stdout, "OK: ") || !strings.Contains(stdout, "User 3") {
		t.Errorf("text report: code=%d stdout=%s", code, stdout)
	}
}

// Every kind of difference exits 4 and says where.
func TestVerify_Mismatches(t *testing.T) {
	plan := writeFile(t, t.TempDir(), "plan.yaml", twoEntityPlan)
	cases := []struct {
		name   string
		damage func(t *testing.T, dir string)
		args   []string
		reason string
		line   int64
		file   string
	}{
		{"changed value", func(t *testing.T, dir string) {
			rewrite(t, filepath.Join(dir, "Order.jsonl"), func(s string) string { return strings.Replace(s, `"total":`, `"total":7`, 1) })
		}, []string{"--split-entities"}, "content differs", 1, "Order.jsonl"},
		{"truncated", func(t *testing.T, dir string) {
			rewrite(t, filepath.Join(dir, "output.jsonl"), func(s string) string { return s[:len(s)-10] })
		}, nil, "file ends early", 0, "output.jsonl"},
		{"extra data", func(t *testing.T, dir string) {
			rewrite(t, filepath.Join(dir, "User.jsonl"), func(s string) string { return s + "{\"id\":4}\n" })
		}, []string{"--split-entities"}, "file has extra data", 4, "User.jsonl"},
		{"missing file", func(t *testing.T, dir string) {
			os.Remove(filepath.Join(dir, "Order.jsonl"))
		}, []string{"--split-entities"}, "missing file", 0, "Order.jsonl"},
		{"unexpected file", func(t *testing.T, dir string) {
			writeFile(t, dir, "Refund.jsonl", "{}\n")
		}, []string{"--split-entities"}, "unexpected file", 0, "Refund.jsonl"},
	}
	for _, c := range cases {
		dir := generateInto(t, plan, c.args...)
		c.damage(t, dir)
		r, code := verifyJSON(t, plan, dir, c.args...)
		m := r.Mismatch
		if code != exitMismatch || r.Match || m == nil || m.Reason != c.reason || filepath.Base(m.File) != c.file || (c.line > 0 && m.Line != c.line) {
			t.Errorf("%s: code=%d result=%+v mismatch=%+v", c.name, code, r, m)
		}
	}

	// A different seed changes the data; a file for an unselected entity
	// isn't checked.
	dir := generateInto(t, plan, "--split-entities")
	if r, code := verifyJSON(t, plan, dir, "--split-entities", "--seed", "2"); code != exitMismatch || r.Match {
		t.Errorf("other seed: code=%d match=%v", code, r.Match)
	}
	writeFile(t, dir, "Refund.jsonl", "{}\n")
	if r, code := verifyJSON(t, plan, dir, "--split-entities", "--entities", "Order"); code != exitOK || !r.Match {
		t.Errorf("--entities ignores other files: code=%d result=%+v", code, r)
	}
}

func TestVerify_TextMismatchReport(t *testing.T) {
	plan := writeFile(t, t.TempDir(), "plan.yaml", twoEntityPlan)
	dir := generateInto(t, plan)
	rewrite(t, filepath.Join(dir, "output.jsonl"), func(s string) string { return strings.Replace(s, `"id":2`, `"id":20`, 1) })

	stdout, _, code := runCLI(t, "verify", "-f", plan, "--output-dir", dir)
	want := `output.jsonl:2: content differs
  want: {"_entity":"User","id":2}
  got:  {"_entity":"User","id":20}
`
	if code != exitMismatch || !strings.HasPrefix(stdout, "MISMATCH: ") || !strings.HasSuffix(stdout, want) {
		t.Errorf("code=%d stdout=%s", code, stdout)
	}
}

// Problems that aren't differences keep their usual exit codes.
func TestVerify_OtherFailures(t *testing.T) {
	dir := t.TempDir()
	plan := writeFile(t, dir, "plan.yaml", twoEntityPlan)
	out := generateInto(t, plan)

	cases := []struct {
		args []string
		code int
	}{
		{[]string{"verify", "-f", plan}, exitValidation}, // --output-dir is required
		{[]string{"verify", "-f", plan, "--output-dir", out, "--report", "xml"}, exitValidation},
		{[]string{"verify", "-f", writeFile(t, dir, "bad.yaml", unknownGenPlan), "--output-dir", out}, exitValidation},
		// A's rows match, then generating B fails.
		{[]string{"verify", "-f", writeFile(t, dir, "err.yaml", runtimeErrorPlan), "--output-dir", aRowsOnly(t)}, exitGeneration},
	}
	for _, c := range cases {
		if _, stderr, code := runCLI(t, c.args...); code != c.code {
			t.Errorf("%v: code=%d, want %d (stderr %s)", c.args, code, c.code, stderr)
		}
	}
}

// aRowsOnly is a directory whose output.jsonl holds runtimeErrorPlan's A rows.
func aRowsOnly(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "output.jsonl", "{\"_entity\":\"A\",\"id\":1}\n{\"_entity\":\"A\",\"id\":2}\n")
	return dir
}

func rewrite(t *testing.T, path string, edit func(string) string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(edit(string(b))), 0o644); err != nil {
		t.Fatal(err)
	}
}
