# RNG Rewrite + Chunked Execution Plan

This plan targets deterministic, parallel execution with per-chunk RNG derivation while preserving the core guarantee: **Plan + Seed = Reproducible Output**.

## Goals

- Deterministic output independent of worker count and scheduling.
- Chunk-based parallelism with default chunk size 50k rows.
- Clean, idiomatic Go with small, composable types and clear responsibilities.
- Maintain generator-facing API (`Next(*rng.Rng) (any, error)`), minimizing churn.

## Non-Goals

- Changing generator semantics or output formats.
- Adding new generators or writer types.
- Optimizing catalog subsystem or uniqueness enforcement (tracked separately).

## Constraints

- Writers are not currently concurrency-safe; output ordering should remain stable.
- Determinism must not depend on chunk size; stress tests will randomize chunk size and worker count.
- RNG should be explicit about seed derivation paths (root → entity → field → row).

## Proposed Design

### 1) RNG Package Rewrite (`internal/rng`)

**Primary change:** introduce a clear separation between *seed derivation* and *random stream*.

- **Seed type**
  - Add `type Seed uint64` to make derivation functions explicit and type-safe.

- **Derivation**
  - Implement a SplitMix64-based mixer for deterministic seed derivation.
  - Provide helpers that avoid `fmt.Sprintf` in hot paths.
  - Suggested API:
    - `func Derive(parent Seed, label string) Seed`
    - `func DeriveIndex(parent Seed, index int64) Seed`
    - `func DerivePair(parent Seed, a uint64, b uint64) Seed` (optional helper)

- **Stream**
  - Keep `Rng` as a thin wrapper around a fast PRNG (PCG) seeded by `Seed`.
  - Provide `New(seed Seed) *Rng` and retain existing methods (`Intn`, `Float64`, `Read`, `NewZipf`, etc.).

- **Determinism contract**
  - Row seeds must depend only on `{root seed, entity name/index, field name, row index}`.
  - Chunk size must not affect row seeds.
  - Chunk RNGs are allowed for chunk-level operations, but row/field values must not depend on chunk boundaries.

**Files**
- `internal/rng/rng.go` (replace current `int64` seed usage with `Seed`, add SplitMix64 mixer)
- Add `internal/rng/seed.go` (if separating derivation helpers keeps `rng.go` clean)

### 2) Chunked Execution (`internal/runtime`)

**Primary change:** introduce chunking and worker pool with deterministic ordering and single-writer flow.

- **Chunk model**
  - Add `chunk` type and `makeChunks(total, size int64) []chunk` (as in `sdg.md`).
  - Default size 50k; allow override via option.

- **Executor options**
  - `WithChunkSize(size int64)`
  - `WithWorkers(n int)` (default: `min(2×CPU, 64)`)

- **Deterministic processing**
  - Workers generate records for a chunk; results are written by a single writer goroutine to avoid concurrency issues.
  - Preserve stable output order (entity order, then row order). Options:
    1) Buffer each chunk in memory then write in chunk order.
    2) Use a coordinator that writes completed chunks in order (minimal buffering, but chunk order must be enforced).

- **Seed derivation flow**
  - `entitySeed := rng.Derive(rootSeed, entityName + "[idx]")`
  - `fieldSeed := rng.Derive(entitySeed, fieldName)`
  - `rowSeed := rng.DeriveIndex(fieldSeed, rowIndex)`
  - Chunk seeds may be derived for chunk-local work, but row values must use `rowSeed` to avoid chunk-size dependence.

**Files**
- Add `internal/runtime/chunk.go`
- Update `internal/runtime/executor.go` to use chunking, worker pool, and single-writer coordination.

### 3) Tests

- **Determinism stress tests**
  - Randomize worker counts and chunk sizes; compare SHA-256 digests.
  - Ensure output is identical for same plan+seed across configurations.

- **RNG tests**
  - Verify `Derive` and `DeriveIndex` stability across runs.
  - Assert that row seeds are independent of chunk size.

- **Concurrency safety**
  - Validate writer is only used from one goroutine (or add explicit writer lock if needed).

**Files**
- Add `internal/runtime/executor_test.go` for concurrency/determinism tests.
- Add `internal/rng/rng_test.go` for derivation and stream consistency.

## Incremental Steps (Implementation Order)

1) Implement new RNG seed type and derivation helpers in `internal/rng`.
2) Update call sites to use `rng.Seed` and `DeriveIndex` (runtime + registry tests).
3) Add chunk model and worker pool in runtime, keeping writer single-threaded.
4) Add determinism tests (vary workers/chunk sizes).
5) Document new derivation contract in `sdg.md` if needed.

## Open Decisions

- Should output order be strictly row-ordered, or is stable chunk order acceptable?
- Are we willing to buffer full chunks in memory, or should we implement a streaming coordinator?
- Should `rng.Derive` keep `string` labels only, or add `[]byte`/`uint64` helpers for perf?

## Success Criteria

- Identical outputs for same plan+seed across:
  - worker counts
  - chunk sizes
  - execution order
- Clean, readable RNG and runtime code with minimal changes to generator APIs.
