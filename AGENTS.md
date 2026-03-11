# AGENTS.md

This file provides guidance to coding agents when working with code in this repository.

## Project Overview

Apery is a synthetic data generator (SDG) built in Go. It generates deterministic, schema-driven synthetic data using a declarative plan-based approach. The system is designed to be AI-friendly and supports various output formats.

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

## Architecture

### Core Components

**Plan → Registry → Runtime → Writer** pipeline:

1. **Plan** (`internal/plan`): Declarative schema defining entities, fields, and generators
   - `Plan`: Top-level structure with seed and entities
   - `EntitySpec`: Defines a table/collection with name, count, and fields
   - `FieldSpec`: Individual field with generator name and config

2. **Registry** (`internal/registry`): Generator factory and plugin system
   - Global registry pattern with `Register()` (returns error), `MustRegister()` (panic on error), and `Get()`
   - Generators use `init()` for auto-registration (via `MustRegister`)
   - Each generator implements `Next(r *rng.Rng) (any, error)`
   - Built-in generators: `seq`, `pick` (values|file|url with allowlist), `bool`, `int`, `float`, `uuid`, `ulid`, `time`

3. **Runtime/Executor** (`internal/runtime`): Orchestrates data generation
   - Executes plans by iterating entities and fields
   - Creates per-field RNGs using hierarchical seed derivation
   - Chunked parallel execution with configurable worker count and chunk size
   - Generates records row-by-row and writes via writer interface

4. **Writer** (`internal/writer`): Output abstraction
   - `Writer` interface with `WriteRecord()` and `Close()`
   - `JSONLWriter`: Streams newline-delimited JSON to file
   - `CSVWriter`: Streams CSV rows with a header
   - Uses `OrderedMap` to preserve field order in output

5. **RNG** (`internal/rng`): Deterministic random number generation
   - `Derive(parent, label)`: Creates child seeds using FNV-1a hash
   - Ensures reproducibility: same seed + plan = identical output
   - Hierarchical: root → entity → field → row derivation
   - Implements `io.Reader` for compatibility with libraries requiring entropy sources (e.g., ULID)

### Determinism Model

Critical design principle: **Plan + Seed = Reproducible Output**

The seed derivation hierarchy:

```
Root Seed (from Plan)
  └─> Entity Seed (derived from root + entity name/index)
      └─> Field Seed (derived from entity seed + field name)
          └─> Row Seed (derived from field seed + row index)
```

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

### Adding New Generators

1. Create file in `internal/registry/` (e.g., `mygen.go`)
2. Implement `Generator` interface
3. Register in `init()` function with factory
4. Factory should validate config and return generator instance

The registry is global and thread-safe via init-time registration.

## Key Files

- `cmd/apery/main.go`: Entry point with example plan
- `internal/plan/plan.go`: Data structures for declarative plans
- `internal/registry/registry.go`: Generator registry core
- `internal/runtime/executor.go`: Execution orchestrator
- `internal/writer/jsonl.go`: JSONL output writer
- `internal/writer/csv.go`: CSV output writer
- `internal/rng/rng.go`: Deterministic RNG with seed derivation

## Design Document

The `sdg-spec.md` file contains the full specification and design philosophy. Key sections:

- Determinism guarantees (Plan + Seed + Version = Identical Output)
- Composition over code philosophy (minimal primitives + combinators)
- AI-first design with MCP integration plans
- Planned features: GraphQL API, NLP plan compiler, advanced writers

## Module Information

- Module name: `apery`
- Go version: 1.24.3
- External dependencies:
  - `github.com/google/uuid` - UUID generation
  - `github.com/oklog/ulid/v2` - ULID generation

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
