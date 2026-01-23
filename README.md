# Apery

A deterministic synthetic data generator built in Go.

## Overview

Apery generates synthetic data from declarative plans with guaranteed reproducibility. Given the same plan and seed, it produces identical output every time.

**Key Features:**
- Deterministic generation (Plan + Seed + Version = Identical Output)
- Platform-independent output (explicit `int64`/`float64` types)
- Hierarchical RNG seeding for parallel execution
- Extensible generator registry
- Multiple output formats (JSONL, CSV, Parquet, SQL, etc.)

See [sdg.md](sdg.md) for full design specification.

## Quick Start

```bash
# Build the project
make build

# Run the generator
make run

# Format code
make fmt
```

## Architecture

```
internal/
├── plan/       - Plan data structures
├── registry/   - Generator registry and primitives
├── rng/        - Deterministic RNG with hierarchical seeding
├── runtime/    - Execution orchestrator
└── writer/     - Output writers (JSONL, CSV, etc.)
```

## Generators

Currently implemented:
- `seq` - Sequential integers with configurable start/step
- `pick` - Random selection from a list
- `bool` - Boolean with configurable probability
- `int` - Random integers within a range
- `float` - Random floats within a range
- `uuid` - UUID v4 strings
- `ulid` - ULID strings (sortable unique identifiers)
- `time` - Timestamps within a configurable range

## Example

```go
plan := &plan.Plan{
    Seed: 42,
    Entities: []plan.EntitySpec{
        {
            Name:  "User",
            Count: 100,
            Fields: []plan.FieldSpec{
                {Name: "id", Gen: "ulid"},
                {Name: "age", Gen: "int", Config: map[string]any{"min": 18, "max": 65}},
                {Name: "score", Gen: "float", Config: map[string]any{"min": 0.0, "max": 100.0}},
                {Name: "active", Gen: "bool", Config: map[string]any{"probability": 0.8}},
                {Name: "role", Gen: "pick", Config: map[string]any{"values": []any{"admin", "user", "guest"}}},
                {Name: "created_at", Gen: "time", Config: map[string]any{
                    "start": "2024-01-01T00:00:00Z",
                    "end":   "2024-12-31T23:59:59Z",
                }},
            },
        },
    },
}
```
