# Determinism Testing Design

**Date:** 2026-03-13
**Scope:** Golden determinism regression suite, concurrency stress tests, writer buffer support

---

## Problem

Apery's core guarantee is **Plan + Seed = Identical Output**. We have unit-level determinism tests (same generator, same seed, same output) and a few executor-level comparative tests (different worker/chunk configs, same output). What we lack:

1. **Regression detection**: No way to catch accidental determinism breaks across code changes. A refactor that subtly alters seed derivation or field ordering would pass all current tests.
2. **Stress coverage**: Existing comparative tests use 2-3 hardcoded configs. Real-world scheduling variability is much wider.

---

## Design Decisions

1. **Hybrid golden files** — SHA-256 digests for CI comparison, plus first 10 rows as JSONL for human-readable spot-checking on failure.
2. **`-update` test flag** — standard Go pattern for regenerating golden files intentionally.
3. **Separate plans per concern** — scalar, composite, row-aware. A change to one generator type only invalidates its golden file.
4. **Buffer-backed writer** — `NewJSONLWriterFromWriter(io.Writer)` to capture output in memory for hashing without temp files.

---

## 1. Writer Refactor

**File:** `internal/writer/jsonl.go`

Currently `JSONLWriter` requires a file path. We refactor to support any `io.Writer`:

### Struct

The struct fields are unchanged (`file *os.File`, `buf *bufio.Writer`). The semantic change is that `file` is now nilable — it's `nil` when the writer is created from an `io.Writer` rather than a file path.

### New Constructor

```go
// NewJSONLWriterFromWriter creates a JSONL writer that writes to any io.Writer.
func NewJSONLWriterFromWriter(w io.Writer) *JSONLWriter {
    return &JSONLWriter{
        buf: bufio.NewWriter(w),
    }
}
```

### Updated Existing Constructor

`NewJSONLWriter(path)` opens the file, stores it in `file`, and creates the bufio writer from it. No signature change — existing callers unaffected.

### Updated Close

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

### WriteRecord

No changes. It already writes to `w.buf`, which is a `*bufio.Writer` wrapping whatever underlying writer was provided.

**Note:** `CSVWriter` has the same file-path-only limitation but is out of scope for this change. It can be refactored the same way later if needed.

---

## 2. Golden Determinism Suite

**File:** `internal/runtime/golden_test.go`

### File Structure

```
internal/runtime/testdata/golden/
├── scalar.digest          # SHA-256 hex string (single line)
├── scalar.jsonl           # first 10 rows of output
├── composite.digest
├── composite.jsonl
├── row_aware.digest
└── row_aware.jsonl
```

### Test Flag

```go
var update = flag.Bool("update", false, "update golden files")
```

Usage:
```bash
go test ./internal/runtime -run TestGolden              # compare mode
go test ./internal/runtime -run TestGolden -update       # regenerate mode
```

### Canonical Plans

Three plans defined as Go structs in `determinism_helpers_test.go` (shared with stress tests). All use seed `12345`. All generate 10,000 rows from a single entity.

**Scalar plan** — exercises every scalar generator:

```go
{Name: "Scalars", Count: 10000, Fields: []plan.FieldSpec{
    {Name: "id",        Gen: "seq",          Config: map[string]any{"start": 1, "step": 1}},
    {Name: "counter",   Gen: "int",          Config: map[string]any{"min": 0, "max": 1000}},
    {Name: "score",     Gen: "float",        Config: map[string]any{"min": -100.0, "max": 100.0}},
    {Name: "active",    Gen: "bool",         Config: map[string]any{}},
    {Name: "color",     Gen: "pick",         Config: map[string]any{
        "values":  []any{"red", "green", "blue"},
        "weights": []any{5.0, 3.0, 2.0},
    }},
    {Name: "tag",       Gen: "const",        Config: map[string]any{"value": "v1"}},
    {Name: "uid",       Gen: "uuid",         Config: map[string]any{}},
    {Name: "ulid",      Gen: "ulid",         Config: map[string]any{}},
    {Name: "ts",        Gen: "time",         Config: map[string]any{
        "start": "2020-01-01T00:00:00Z", "end": "2025-12-31T00:00:00Z",
    }},
    {Name: "code",      Gen: "regex",        Config: map[string]any{"pattern": "[A-Z]{3}-[0-9]{4}"}},
    {Name: "iq",        Gen: "normal_int",   Config: map[string]any{
        "mu": 100.0, "sigma": 15.0, "clamp_min": 50, "clamp_max": 150,
    }},
    {Name: "temp",      Gen: "normal_float", Config: map[string]any{
        "mu": 37.0, "sigma": 0.5,
    }},
    {Name: "rank",      Gen: "zipf",         Config: map[string]any{"s": 1.5, "v": 1, "imax": 100}},
}}
```

**Composite plan** — exercises nested generators:

```go
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
            map[string]any{"gen": "int",   "config": map[string]any{"min": 1, "max": 100}},
            map[string]any{"gen": "float", "config": map[string]any{"min": 0.0, "max": 1.0}},
        },
        "weights": []any{7.0, 3.0},
    }},
}}
```

**Row-aware plan** — exercises row-context generators:

```go
{Name: "RowAware", Count: 10000, Fields: []plan.FieldSpec{
    {Name: "active",     Gen: "bool", Config: map[string]any{}},
    {Name: "department", Gen: "pick", Config: map[string]any{
        "values": []any{"engineering", "sales", "support"},
    }},
    {Name: "score",      Gen: "int",  Config: map[string]any{"min": 1, "max": 100}},
    {Name: "greeting",   Gen: "template", Config: map[string]any{
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
}}
```

These plans are immutable. Changing a plan invalidates its golden files, which is intentional — you'd run `-update` and review the diff.

### Test Flow

For each plan:

1. Run the plan through the executor with `workers=1, chunkSize=10000` into a `bytes.Buffer` via `NewJSONLWriterFromWriter`. Single worker, single chunk — this produces the canonical "reference" output with no concurrency variability.
2. Compute SHA-256 of the full buffer contents
3. Extract first 10 lines from the buffer

**If `-update`:**
- Write digest to `testdata/golden/<name>.digest`
- Write first 10 lines to `testdata/golden/<name>.jsonl`
- Log that the golden file was updated
- Return (skip comparison)

**If not `-update`:**
- Read stored `<name>.digest`, compare against computed digest
- Read stored `<name>.jsonl`, compare against first 10 lines
- On digest mismatch: write full actual output to `t.TempDir()/<name>.actual.jsonl`, report the path in the failure message so the developer can diff
- On spot-check mismatch: show the line-by-line diff in the test output

### First Run Behavior

Golden files are committed to the repository. A fresh clone includes them. If golden files are missing (e.g., a new plan was added but `-update` wasn't run), `compareGolden` fails with a clear message: `"golden file not found: %s — run with -update to generate"`.

### Helper Functions

All helpers live in `determinism_helpers_test.go`.

```go
// runPlan runs a plan into a buffer and returns the raw output bytes.
// Accepts optional executor options for varying workers/chunk size.
func runPlan(t *testing.T, p *plan.Plan, opts ...Option) []byte

// computeDigest returns the SHA-256 hex string of the given bytes.
func computeDigest(data []byte) string

// firstNLines returns the first n lines from data (including trailing newlines).
func firstNLines(data []byte, n int) []byte

// writeGolden writes digest and spot-check files to the golden dir.
func writeGolden(t *testing.T, name string, data []byte)

// compareGolden reads golden files and compares against actual data.
// On failure, dumps actual output to temp dir and reports path.
// If golden files don't exist, fails with instructions to run -update.
func compareGolden(t *testing.T, name string, data []byte)
```

---

## 3. Concurrency Stress Tests

**File:** `internal/runtime/stress_test.go`

### Approach

For each of the 3 canonical plans, run 20 iterations with randomized worker counts and chunk sizes, comparing SHA-256 digests against a single-threaded baseline.

### Test Flow

```
for each plan:
    baseline = run(workers=1, chunkSize=10000)
    baselineDigest = sha256(baseline)

    rng = deterministic source (seed=99999)
    for i in 0..19:
        workers = rng.IntRange(1, 32)
        chunkSize = rng.IntRange(1, 5000)
        actual = run(workers, chunkSize)
        actualDigest = sha256(actual)
        assert actualDigest == baselineDigest
```

### Parameters

- **20 iterations per plan**, 60 total
- **Workers**: 1 to 32
- **Chunk sizes**: 1 to 5,000 (small sizes like 1 maximize goroutine scheduling variability)
- **Row count**: 10,000 (same canonical plans as golden suite)
- **Config randomization seed**: fixed at `99999` so the test itself is reproducible
- **Note**: `rng.IntRange` returns `int64`; cast to `int` for `WithWorkers` (`WithChunkSize` already takes `int64`)

### On Failure

Reports:
- Iteration number
- Worker count and chunk size that triggered the failure
- Dumps both baseline and actual output to `t.TempDir()` for diffing

### Shared Infrastructure

The canonical plan definitions and `runPlan`/`computeDigest` helpers are shared between `golden_test.go` and `stress_test.go`. These will live in a shared test helper file (`determinism_helpers_test.go`) to avoid duplication.

---

## 4. Documentation Updates

After implementation, update the following:

- **`docs/plan.md`**: Check off "Concurrency stress tests that randomize worker counts/chunk sizes and compare digests" and "Determinism regression suite keyed by (Plan + Seed + Version)"
- **`CLAUDE.md`**: Add a note about the `-update` flag and golden test workflow to the Testing section
- **`README.md`**: Add a "Determinism Testing" subsection documenting how to run golden tests and regenerate golden files

---

## 5. File Summary

| File | Change |
|------|--------|
| `internal/writer/jsonl.go` | Add `NewJSONLWriterFromWriter`, refactor `Close()` for nil file |
| `internal/runtime/golden_test.go` | Golden determinism tests with `-update` flag |
| `internal/runtime/stress_test.go` | Concurrency stress tests with randomized configs |
| `internal/runtime/determinism_helpers_test.go` | Shared canonical plans, `runPlan`, `computeDigest`, `firstNLines` helpers |
| `internal/runtime/testdata/golden/*.digest` | SHA-256 digest files (generated) |
| `internal/runtime/testdata/golden/*.jsonl` | First 10 rows spot-check files (generated) |

---

## 6. Testing Strategy

### Golden Suite Validates

- Output hasn't changed for any canonical plan (digest comparison)
- First 10 rows match expected content (spot-check comparison)
- `-update` flag correctly regenerates golden files

### Stress Suite Validates

- Output is identical regardless of worker count (1-32)
- Output is identical regardless of chunk size (1-5000)
- No race conditions or ordering bugs under heavy parallelism

### Existing Tests Unaffected

- All current executor and generator tests continue to pass
- New tests are additive — no existing behavior modified except the writer refactor, which preserves the existing API
