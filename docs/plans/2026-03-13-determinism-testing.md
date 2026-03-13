# Determinism Testing Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add golden determinism regression tests and concurrency stress tests to catch accidental output drift and parallelism bugs.

**Architecture:** Refactor `JSONLWriter` to support buffer-backed output, define 3 canonical plans (scalar, composite, row-aware) as shared test fixtures, build golden comparison with `-update` flag, and stress test with randomized executor configs.

**Tech Stack:** Go 1.24, `crypto/sha256`, `encoding/hex`, existing `apery` module. No new dependencies.

**Spec:** `docs/specs/2026-03-13-determinism-testing-design.md`

---

## Chunk 1: Writer Refactor

### Task 1: Add buffer-backed JSONL writer constructor

**Files:**
- Modify: `internal/writer/jsonl.go`
- Modify: `internal/writer/jsonl_test.go`

- [ ] **Step 1: Write failing test for NewJSONLWriterFromWriter**

Add to `internal/writer/jsonl_test.go`:

```go
func TestJSONLWriterFromWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewJSONLWriterFromWriter(&buf)

	record := makeRecord("id", int64(1), "name", "alice")
	if err := w.WriteRecord("User", record); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	want := `{"_entity":"User","id":1,"name":"alice"}` + "\n"
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}
```

Add `"bytes"` to the imports (the file currently imports only `"testing"`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/writer/ -run TestJSONLWriterFromWriter -v`
Expected: FAIL — `NewJSONLWriterFromWriter` not defined

- [ ] **Step 3: Implement NewJSONLWriterFromWriter and update Close**

In `internal/writer/jsonl.go`, add `"io"` to imports, then add the new constructor:

```go
// NewJSONLWriterFromWriter creates a JSONL writer that writes to any io.Writer.
func NewJSONLWriterFromWriter(w io.Writer) *JSONLWriter {
	return &JSONLWriter{
		buf: bufio.NewWriter(w),
	}
}
```

Update `Close` to handle nil `file`:

```go
func (w *JSONLWriter) Close() error {
	if err := w.buf.Flush(); err != nil {
		return err
	}
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/writer/ -v`
Expected: All PASS (new test + existing `TestJSONLWriter` still works)

- [ ] **Step 5: Commit**

```bash
git add internal/writer/jsonl.go internal/writer/jsonl_test.go
git commit -m "feat: add buffer-backed JSONL writer constructor"
```

---

## Chunk 2: Shared Test Helpers and Canonical Plans

### Task 2: Create determinism test helpers

**Files:**
- Create: `internal/runtime/determinism_helpers_test.go`

- [ ] **Step 1: Create the helpers file with canonical plans and utility functions**

Create `internal/runtime/determinism_helpers_test.go`:

```go
package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"apery/internal/plan"
	"apery/internal/writer"
)

var update = flag.Bool("update", false, "update golden files")

const goldenDir = "testdata/golden"
const goldenSeed int64 = 12345

var scalarPlan = plan.Plan{
	Seed: goldenSeed,
	Entities: []plan.EntitySpec{
		{Name: "Scalars", Count: 10000, Fields: []plan.FieldSpec{
			{Name: "id", Gen: "seq", Config: map[string]any{"start": 1, "step": 1}},
			{Name: "counter", Gen: "int", Config: map[string]any{"min": 0, "max": 1000}},
			{Name: "score", Gen: "float", Config: map[string]any{"min": -100.0, "max": 100.0}},
			{Name: "active", Gen: "bool", Config: map[string]any{}},
			{Name: "color", Gen: "pick", Config: map[string]any{
				"values":  []any{"red", "green", "blue"},
				"weights": []any{5.0, 3.0, 2.0},
			}},
			{Name: "tag", Gen: "const", Config: map[string]any{"value": "v1"}},
			{Name: "uid", Gen: "uuid", Config: map[string]any{}},
			{Name: "ulid", Gen: "ulid", Config: map[string]any{}},
			{Name: "ts", Gen: "time", Config: map[string]any{
				"start": "2020-01-01T00:00:00Z", "end": "2025-12-31T00:00:00Z",
			}},
			{Name: "code", Gen: "regex", Config: map[string]any{"pattern": "[A-Z]{3}-[0-9]{4}"}},
			{Name: "iq", Gen: "normal_int", Config: map[string]any{
				"mu": 100.0, "sigma": 15.0, "clamp_min": 50, "clamp_max": 150,
			}},
			{Name: "temp", Gen: "normal_float", Config: map[string]any{
				"mu": 37.0, "sigma": 0.5,
			}},
			{Name: "rank", Gen: "zipf", Config: map[string]any{"s": 1.5, "v": 1, "imax": 100}},
		}},
	},
}

var compositePlan = plan.Plan{
	Seed: goldenSeed,
	Entities: []plan.EntitySpec{
		{Name: "Composites", Count: 10000, Fields: []plan.FieldSpec{
			{Name: "address", Gen: "object", Config: map[string]any{
				"fields": map[string]any{
					"city": map[string]any{"gen": "pick", "config": map[string]any{
						"values": []any{"NYC", "LA", "CHI"},
					}},
					"zip": map[string]any{"gen": "int", "config": map[string]any{
						"min": 10000, "max": 99999,
					}},
				},
			}},
			{Name: "tags", Gen: "list", Config: map[string]any{
				"item": map[string]any{"gen": "pick", "config": map[string]any{
					"values": []any{"alpha", "beta", "gamma"},
				}},
				"min_len": 1,
				"max_len": 4,
			}},
			{Name: "skills", Gen: "sample", Config: map[string]any{
				"values": []any{"Go", "Python", "Rust", "Java", "C++"},
				"min_n":  1,
				"max_n":  3,
			}},
			{Name: "value", Gen: "one_of", Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
					map[string]any{"gen": "float", "config": map[string]any{"min": 0.0, "max": 1.0}},
				},
				"weights": []any{7.0, 3.0},
			}},
		}},
	},
}

var rowAwarePlan = plan.Plan{
	Seed: goldenSeed,
	Entities: []plan.EntitySpec{
		{Name: "RowAware", Count: 10000, Fields: []plan.FieldSpec{
			{Name: "active", Gen: "bool", Config: map[string]any{}},
			{Name: "department", Gen: "pick", Config: map[string]any{
				"values": []any{"engineering", "sales", "support"},
			}},
			{Name: "score", Gen: "int", Config: map[string]any{"min": 1, "max": 100}},
			{Name: "greeting", Gen: "template", Config: map[string]any{
				"tpl": "Dept {department}, score={score}",
			}},
			{Name: "access", Gen: "switch", Config: map[string]any{
				"key": "department",
				"cases": map[string]any{
					"engineering": map[string]any{"gen": "const", "config": map[string]any{"value": "full"}},
					"sales":       map[string]any{"gen": "const", "config": map[string]any{"value": "read-only"}},
				},
				"default": map[string]any{"gen": "const", "config": map[string]any{"value": "limited"}},
			}},
		}},
	},
}

// canonicalPlans is the ordered set of plans used by golden and stress tests.
var canonicalPlans = []struct {
	name string
	plan *plan.Plan
}{
	{"scalar", &scalarPlan},
	{"composite", &compositePlan},
	{"row_aware", &rowAwarePlan},
}

// runPlanWithOpts runs a plan into a buffer and returns the raw JSONL output bytes.
// Note: Executor.Run() auto-closes the writer via defer, so we don't call w.Close() here.
func runPlanWithOpts(t *testing.T, p *plan.Plan, opts ...Option) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := writer.NewJSONLWriterFromWriter(&buf)
	e := New(w, opts...)
	if err := e.Run(t.Context(), p); err != nil {
		t.Fatalf("executor.Run: %v", err)
	}
	return buf.Bytes()
}

// computeDigest returns the SHA-256 hex string of the given bytes.
func computeDigest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// firstNLines returns the first n lines from data (including trailing newlines).
func firstNLines(data []byte, n int) []byte {
	var count int
	for i, b := range data {
		if b == '\n' {
			count++
			if count == n {
				return data[:i+1]
			}
		}
	}
	return data
}

// writeGolden writes digest and spot-check files to the golden dir.
func writeGolden(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(goldenDir, 0755); err != nil {
		t.Fatalf("mkdir golden dir: %v", err)
	}

	digest := computeDigest(data)
	digestPath := filepath.Join(goldenDir, name+".digest")
	if err := os.WriteFile(digestPath, []byte(digest+"\n"), 0644); err != nil {
		t.Fatalf("write digest: %v", err)
	}

	spotPath := filepath.Join(goldenDir, name+".jsonl")
	if err := os.WriteFile(spotPath, firstNLines(data, 10), 0644); err != nil {
		t.Fatalf("write spot-check: %v", err)
	}

	t.Logf("updated golden files for %s (digest: %s)", name, digest[:16])
}

// compareGolden reads golden files and compares against actual data.
func compareGolden(t *testing.T, name string, data []byte) {
	t.Helper()

	digestPath := filepath.Join(goldenDir, name+".digest")
	wantDigestBytes, err := os.ReadFile(digestPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden file not found: %s — run with -update to generate", digestPath)
		}
		t.Fatalf("read digest: %v", err)
	}
	wantDigest := bytes.TrimSpace(wantDigestBytes)
	gotDigest := computeDigest(data)

	if gotDigest != string(wantDigest) {
		// Dump actual output for debugging
		actualPath := filepath.Join(t.TempDir(), name+".actual.jsonl")
		os.WriteFile(actualPath, data, 0644)
		t.Fatalf("digest mismatch for %s:\n  want: %s\n  got:  %s\n  actual output dumped to: %s",
			name, wantDigest, gotDigest, actualPath)
	}

	spotPath := filepath.Join(goldenDir, name+".jsonl")
	wantSpot, err := os.ReadFile(spotPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden file not found: %s — run with -update to generate", spotPath)
		}
		t.Fatalf("read spot-check: %v", err)
	}

	gotSpot := firstNLines(data, 10)
	if !bytes.Equal(gotSpot, wantSpot) {
		t.Errorf("spot-check mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s",
			name, wantSpot, gotSpot)
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go vet ./internal/runtime/`
Expected: No errors (vet checks test files too)

- [ ] **Step 3: Commit**

```bash
git add internal/runtime/determinism_helpers_test.go
git commit -m "test: add determinism test helpers and canonical plans"
```

---

## Chunk 3: Golden Determinism Tests

### Task 3: Implement golden tests

**Files:**
- Create: `internal/runtime/golden_test.go`

- [ ] **Step 1: Create golden_test.go with TestGolden**

Create `internal/runtime/golden_test.go`:

```go
package runtime

import "testing"

func TestGolden(t *testing.T) {
	for _, tc := range canonicalPlans {
		t.Run(tc.name, func(t *testing.T) {
			data := runPlanWithOpts(t, tc.plan, WithWorkers(1), WithChunkSize(10000))

			if *update {
				writeGolden(t, tc.name, data)
				return
			}

			compareGolden(t, tc.name, data)
		})
	}
}
```

- [ ] **Step 2: Generate initial golden files**

Run: `go test ./internal/runtime/ -run TestGolden -update -v`
Expected: PASS — logs "updated golden files for scalar", "updated golden files for composite", "updated golden files for row_aware"

- [ ] **Step 3: Verify golden files were created**

Run: `ls -la internal/runtime/testdata/golden/`
Expected: 6 files — `scalar.digest`, `scalar.jsonl`, `composite.digest`, `composite.jsonl`, `row_aware.digest`, `row_aware.jsonl`

- [ ] **Step 4: Run golden tests in compare mode**

Run: `go test ./internal/runtime/ -run TestGolden -v`
Expected: All 3 subtests PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/golden_test.go internal/runtime/testdata/golden/
git commit -m "test: add golden determinism regression tests"
```

---

## Chunk 4: Concurrency Stress Tests

### Task 4: Implement stress tests

**Files:**
- Create: `internal/runtime/stress_test.go`

- [ ] **Step 1: Create stress_test.go with TestStress**

Create `internal/runtime/stress_test.go`:

```go
package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"apery/internal/rng"
)

const (
	stressIterations = 20
	stressSeed       = 99999
)

func TestStress(t *testing.T) {
	for _, tc := range canonicalPlans {
		t.Run(tc.name, func(t *testing.T) {
			baseline := runPlanWithOpts(t, tc.plan, WithWorkers(1), WithChunkSize(10000))
			baselineDigest := computeDigest(baseline)

			r := rng.New(rng.SeedFromInt64(stressSeed))
			for i := range stressIterations {
				workers := int(r.IntRange(1, 32))
				chunkSize := r.IntRange(1, 5000)

				actual := runPlanWithOpts(t, tc.plan, WithWorkers(workers), WithChunkSize(chunkSize))
				actualDigest := computeDigest(actual)

				if actualDigest != baselineDigest {
					dir := t.TempDir()
					basePath := filepath.Join(dir, "baseline.jsonl")
					actPath := filepath.Join(dir, "actual.jsonl")
					os.WriteFile(basePath, baseline, 0644)
					os.WriteFile(actPath, actual, 0644)
					t.Fatalf("iteration %d (workers=%d, chunkSize=%d): digest mismatch\n  baseline: %s\n  actual:   %s\n  diff: %s vs %s",
						i, workers, chunkSize, baselineDigest, actualDigest, basePath, actPath)
				}
			}

			t.Logf("passed %d iterations", stressIterations)
		})
	}
}
```

- [ ] **Step 2: Run stress tests**

Run: `go test ./internal/runtime/ -run TestStress -v`
Expected: All 3 subtests PASS, each logging "passed 20 iterations"

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add internal/runtime/stress_test.go
git commit -m "test: add concurrency stress tests for determinism"
```

---

## Chunk 5: Documentation Updates

### Task 5: Update project docs

**Files:**
- Modify: `docs/plan.md`
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update plan.md**

Check off the two items under "RNG rewrite and execution engine":

```
- [x] Concurrency stress tests that randomize worker counts/chunk sizes and compare digests.
- [x] Determinism regression suite keyed by (Plan + Seed + Version).
```

- [ ] **Step 2: Update CLAUDE.md**

In the `### Testing` section, after the existing benchmark command, add:

```markdown
### Golden Determinism Tests
```bash
# Run golden tests (compare against stored reference output)
go test ./internal/runtime -run TestGolden -v

# Regenerate golden files after intentional output changes
go test ./internal/runtime -run TestGolden -update -v

# Run concurrency stress tests
go test ./internal/runtime -run TestStress -v
```
```

- [ ] **Step 3: Update README.md**

Add a "Determinism Testing" subsection after the existing testing content. Include a brief explanation of golden tests and the `-update` flag workflow.

- [ ] **Step 4: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add docs/plan.md CLAUDE.md README.md
git commit -m "docs: add determinism testing documentation"
```
