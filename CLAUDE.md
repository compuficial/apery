# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Apery is a synthetic data generator for agents built in Go. It generates deterministic, schema-driven synthetic data using a declarative plan-based approach. The system is designed to be AI-friendly and supports various output formats.

## Development Commands

### Build and Run

```bash
# Build the project
go build -o apery ./cmd/apery

# Run the generator
go run ./cmd/apery

# Run with Go modules
go mod tidy  # Install/update dependencies
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test ./internal/registry
```

### Benchmarking

```bash
# Run runtime executor benchmarks
make bench
```

### Golden Determinism Tests

Five canonical plans (scalar, composite, row-aware, relational, dependent) in `internal/runtime/determinism_helpers_test.go` define the golden test fixtures. Both `TestGolden` and `TestStress` use these plans automatically.

When adding a new generator, add a field to the appropriate canonical plan and regenerate golden files with `-update`. The stress tests will pick up the change automatically. If a new generator doesn't fit any existing plan category, create a new canonical plan in `determinism_helpers_test.go` and add it to the `canonicalPlans` slice — both `TestGolden` and `TestStress` will pick it up automatically.

```bash
# Run golden tests (compare against stored reference output)
go test ./internal/runtime -run TestGolden -v

# Regenerate golden files after intentional output changes
go test ./internal/runtime -run TestGolden -update -v

# Run concurrency stress tests
go test ./internal/runtime -run TestStress -v
```

## Architecture

### Core Components

**Plan → Registry → Runtime → Writer** pipeline:

1. **Plan** (`internal/plan`): Declarative schema defining entities, fields, and generators
   - `Plan`: Top-level structure with seed and entities
   - `EntitySpec`: Defines a table/collection with name, count (or DrivenBy), and fields
   - `FieldSpec`: Individual field with generator name and config
   - `DrivenBy`: Configures parent-driven child row generation (1:M relationships)

2. **Registry** (`internal/registry`): Generator factory and plugin system
   - Global registry pattern with `Register()` (returns error), `MustRegister()` (panic on error), and `Get()`
   - Generators use `init()` for auto-registration (via `MustRegister`)
   - Each generator implements `Next(r *rng.Rng) (any, error)`
   - Built-in generators: `seq`, `pick` (values|file|url, optional weights), `const`, `bool`, `int`, `float`, `uuid`, `ulid`, `time`, `regex`, `normal_int`, `normal_float`, `zipf`, `object`, `list`, `sample`, `one_of`, `template`, `switch`, `expr`, `date_offset`, `rel_ref`
   - Interfaces: `Generator`, `RowAwareGenerator`, `DependencyDeclarer`, `ReadOnlyEntityStore`, `EntityStore`, `Resettable`

3. **Runtime/Executor** (`internal/runtime`): Orchestrates data generation
   - Executes plans by iterating entities and fields
   - Creates per-field RNGs using hierarchical seed derivation
   - Chunked parallel execution with configurable worker count and chunk size
   - Generates records row-by-row and writes via writer interface
   - Cross-entity column store (`mapEntityStore`) for relational generators
   - Two-phase driven_by execution: count generation then chunked parallel generation
   - Parent-aligned chunking for entities with unique rel_ref fields
   - Structured logging via `*slog.Logger` (Info for entity progress, Debug for seeds/chunks)

4. **Writer** (`internal/writer`): Output abstraction
   - `Writer` interface with `WriteRecord()` and `Close()`
   - `JSONLWriter`: Streams newline-delimited JSON to file or `io.Writer`
   - `CSVWriter`: Streams CSV rows with a header to file or `io.Writer`
   - `SplitWriter`: Routes records to per-entity files (omits `_entity` column)
   - Uses `OrderedMap` to preserve field order in output

5. **RNG** (`internal/rng`): Deterministic random number generation
   - `Derive(parent, label)`: Creates child seeds using FNV-1a hash
   - `GetSeed()`: Returns the construction seed, used by composite generators for child seed derivation
   - Ensures reproducibility: same seed + plan = identical output
   - Hierarchical: root → entity → field → row → sub-field derivation
   - Implements `io.Reader` for compatibility with libraries requiring entropy sources (e.g., ULID)

### Determinism Model

Critical design principle: **Plan + Seed = Reproducible Output**

The seed derivation hierarchy:

```
Root Seed (from Plan)
  └─> Entity Seed (derived from root + entity name/index)
      ├─> Field Seed (derived from entity seed + field name)
      │   └─> Row Seed (derived from field seed + row index)
      │       └─> Sub-Field Seed (derived from row seed + sub-field name, for composite generators)
      └─> Count Seed (derived from entity seed + "counts" + parent index, for driven_by child count per parent)
```

Composite generators (e.g., `object`) use `rng.Derive(r.GetSeed(), subFieldName)` to derive child seeds from the parent row seed, keeping the hierarchy clean and order-independent.

See `internal/runtime/executor.go:44` for seed derivation and `internal/rng/rng.go:25` for the `Derive()` function.

### Generator Pattern

All generators:

- Implement `Generator` interface with `Next(*rng.Rng) (any, error)`
- Auto-register via `init()` function
- Accept configuration via `map[string]any`
- Must be deterministic given the same RNG state

Example from `internal/registry/pick.go`:

```go
func init() {
    MustRegister("pick", func(config map[string]any) (Generator, error) {
        values := config["values"].([]any)
        return &PickGenerator{values: values}, nil
    })
}
```

### Composite Generators

Composite generators (e.g., `object`) instantiate sub-generators at factory time and store them in the struct. During `Next()`, they derive child seeds using `rng.Derive(r.GetSeed(), fieldName)` for each sub-field, ensuring deterministic output independent of field ordering.

### Relational Generators

**`rel_ref`** samples values from a previously generated entity's column. Supports uniform (default) and zipf distributions, with optional `unique: true` for deduplication within a parent batch. Uses `ReadOnlyEntityStore` injected via the `_store` config key at chunk time.

**`DrivenBy`** configures 1:M parent-driven child row generation. When set on `EntitySpec`, the executor generates Min to Max children per parent row instead of using Count. The parent field value is auto-injected into each child row under `DrivenBy.As`. Optionally, `DrivenBy.Expose` (`[]ParentField`) injects additional parent columns and `DrivenBy.IndexAs` injects the child's 0-based position within its parent batch — all as ordinary row columns that row-aware generators can read. Injection order: `As`, then `Expose` (declared order), then `IndexAs`.

**Cross-row dependent values** (`expr`, `date_offset`) are row-aware generators that compute a field from sibling/exposed/index columns: `expr` does arithmetic over `{field}` refs and numeric literals (`+ - * /`, parens; emits `int64` when whole, else `float64`); `date_offset` shifts a base date (literal or `{field}`) by an amount in years/months/days/hours/minutes/seconds, where the amount is an `expr`-style expression string (e.g. `"{event_index}"`, `"{q} * 3"`) — it reuses the `expr` engine (`compileExpr`) rather than reimplementing field arithmetic — or a bare number (`amount: 1`) accepted as a convenience for a constant offset. Both are pure functions of row values (ignore the RNG), so output stays chunk- and worker-independent.

**M:N relationships** are composed from `driven_by` (1:M from left entity) + `rel_ref` with `unique: true` (M:1 to right entity) on a junction entity.

Key design rules:

- Entities must be declared in dependency order (downstream after upstream)
- `DrivenBy.Min >= 1` (always produces at least one child per parent)
- Config keys starting with `_` are reserved for internal use
- `Resettable` generators are reset on parent transitions in driven_by chunks
- Standalone entities with `unique: true` `rel_ref` force single-threaded execution

### Adding New Generators

1. Create file in `internal/registry/` (e.g., `mygen.go`)
2. Implement `Generator` interface
3. Register in `init()` function with factory
4. Factory should validate config and return generator instance
5. Add a field using the new generator to the appropriate canonical plan in `internal/runtime/determinism_helpers_test.go` (scalar, composite, row-aware, relational, or dependent)
6. Regenerate golden files: `go test ./internal/runtime -run TestGolden -update -v`
7. Review the golden file diff and commit

The registry is global and thread-safe via init-time registration.

## Key Files

- `cmd/apery/main.go`: Cobra root command, version subcommand, exit codes
- `cmd/apery/generate.go`: Generate subcommand with all flags and writer wiring
- `cmd/apery/validate.go`: Validate subcommand
- `cmd/apery/generators.go`: `list generators` and `describe generator` subcommands
- `internal/plan/plan.go`: Data structures for declarative plans (`Plan`, `EntitySpec`, `FieldSpec`, `DrivenBy`)
- `internal/plan/load.go`: YAML/JSON plan file loading with `LoadFile()`
- `internal/plan/validate.go`: Plan validation including relational constraints
- `internal/registry/registry.go`: Generator registry, `GeneratorInfo`, `MustRegisterInfo`, `ListGenerators`
- `internal/registry/rel_ref.go`: Relational foreign key generator
- `internal/registry/expr.go`: Row-aware arithmetic generator (`{field}` refs + literals, `+ - * /`)
- `internal/registry/date_offset.go`: Row-aware temporal offset generator (date ± N units)
- `internal/runtime/executor.go`: Execution orchestrator with slog-based structured logging
- `internal/runtime/driven_by.go`: Driven-by execution path and layout computation
- `internal/runtime/entity_store.go`: Cross-entity column store
- `internal/writer/jsonl.go`: JSONL output writer (file, io.Writer, split mode)
- `internal/writer/csv.go`: CSV output writer (file, io.Writer, split mode)
- `internal/writer/split.go`: Per-entity file routing writer
- `internal/rng/rng.go`: Deterministic RNG with seed derivation
- `run.go`: Public API re-exports for library usage

## Design Documents

- `docs/spec.md`: Canonical specification — architecture, plan schema, generator reference, execution model, CLI + library API
- `docs/usage.md`: Practical CLI walkthrough with example plans

## Module Information

- Module name: `github.com/compuficial/apery`
- Go version: 1.24.3
- External dependencies:
  - `github.com/google/uuid` - UUID generation
  - `github.com/oklog/ulid/v2` - ULID generation
  - `github.com/spf13/cobra` - CLI framework
  - `gopkg.in/yaml.v3` - YAML plan file parsing

## Testing

Tests follow a consistent pattern using shared helpers in `registry_test_helpers.go`:

- `RunConfigTests()` - validates generator configuration
- `RunDeterminismTests()` / `AssertDeterministic()` - verifies same seed produces same output
- Generator-specific tests for format validation and distribution

Run tests for a specific generator:

```bash
go test -v ./internal/registry -run Bool
go test -v ./internal/registry -run ULID
```
