# Implementation Checklist

This checklist tracks work needed to complete Apery. Items are grouped by priority.
See `docs/spec-v2.md` for the full specification.

## Phase 1 — Core Engine (Done)

- [x] RNG and execution engine
  - [x] PCG-based RNG with `Seed` construction
  - [x] Hierarchical seed derivation (`Derive`, `DeriveIndex`) via FNV-1a + mix64
  - [x] Per-row RNG instantiation from derived seeds
  - [x] RNG implements `io.Reader` for entropy consumers (e.g., ULID)
  - [x] Chunk-based parallelism (default 50k rows)
  - [x] Deterministic row generation independent of worker scheduling
  - [x] Concurrency stress tests (randomized workers/chunk sizes, digest comparison)
  - [x] Golden determinism suite (Plan + Seed = identical output)
  - [x] Uniqueness enforcement (bounded retries, Resettable interface)
  - [x] Relational resolution (M:1 via rel_ref, 1:M via DrivenBy, M:N via composition)

- [x] Full generator set
  - [x] Scalar: `seq`, `const`, `bool`, `int`, `float`, `normal_int`, `normal_float`, `zipf`, `pick` (values|file|url, weighted), `uuid`, `ulid`, `time`, `regex`
  - [x] Composite: `object`, `list` (fixed/variable length), `sample`, `one_of`, `switch`, `template`
  - [x] Relational: `rel_ref` (uniform/zipf, unique mode), `DrivenBy`

- [x] Writers
  - [x] JSONL writer (streaming)
  - [x] CSV writer

- [x] Plan validation
  - [x] Entity/field structure validation
  - [x] Relational constraint validation (ordering, feasibility)
  - [x] Reserved config key (`_` prefix) rejection

## Phase 2 — CLI & YAML (Done)

- [x] Plan file loading
  - [x] YAML plan file support (gopkg.in/yaml.v3)
  - [x] JSON plan file support (encoding/json)
  - [x] Format detection by file extension (.yaml/.yml → YAML, .json → JSON)
  - [x] Add YAML/JSON struct tags to Plan, EntitySpec, FieldSpec, DrivenBy
- [x] CLI framework (Cobra)
  - [x] Root command with top-level help, version flag, `apery version` subcommand
  - [x] `apery generate -f plan.yaml -o jsonl` — run plan
  - [x] `apery validate -f plan.yaml` — validate without generating
  - [x] `apery list generators` — list available generators
  - [x] `apery describe generator <name>` — show generator config schema/docs
  - [x] Every command has short + long description with usage examples in --help
  - [x] Shell completion subcommand (built-in via Cobra)
- [x] Generate command flags
  - [x] `-f` / `--file` — plan file path (required)
  - [x] `-o` / `--output` — output format: jsonl (default), csv
  - [x] `--output-dir` — output directory (default: current directory)
  - [x] `--split-entities` — one file per entity (requires --output-dir)
  - [x] `--dry-run` — validate plan without generating
  - [x] `--seed` — override plan seed
  - [x] `--workers` / `--chunk-size` — runtime overrides
  - [x] `--verbose` — entity progress on stderr (silent by default)
  - [x] `--debug` — detailed debug output on stderr (seeds, chunks, layout)
- [x] Structured exit codes (0=success, 1=validation, 2=generation, 3=IO)
- [x] Structured logging via slog (Info for --verbose, Debug for --debug)
- [x] Generator metadata system (GeneratorInfo, MustRegisterInfo, all 20 generators documented)
- [x] Writer refactoring (NewCSVWriterFromWriter, SplitWriter, omitEntity for split mode)
- [x] Public API re-exports (LoadPlanFile, ListGenerators, WithWorkers, WithChunkSize, writer constructors)

## Phase 3 — Additional Writers

- [ ] Parquet writer
- [ ] SQL writer (INSERT statements)
- [ ] Train/val/test split modes

## Phase 4 — Catalogs

- [ ] Bundled catalogs (names, companies, domains, cities, products, words)
- [ ] User catalogs via local file
- [ ] Remote catalogs via allowlisted URL
- [ ] Weighted alias table conversion and caching

## Phase 5 — Performance & Polish

- [ ] Buffer reuse in writers
- [ ] Preallocation of record maps
- [ ] Statistical sanity checks for RNG-dependent generators
- [ ] RNG hot-path benchmarks (seed derivation + instantiation cost)
- [ ] Cross-version RNG compatibility and migration notes
- [ ] Seed serialization/format stability guarantees

## Future

- [ ] Cloud storage connectors (S3/GCS)
- [ ] Additional output formats as needed
