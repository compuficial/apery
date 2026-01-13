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
- `seq` - Sequential integers (int64)
- `pick` - Random selection from a list
- `bool` - Boolean with configurable probability
- `int` - Random integers within a range (int64)

## Example

```go
plan := &plan.Plan{
    Seed: 42,
    Entities: []plan.EntitySpec{
        {
            Name:  "User",
            Count: 100,
            Fields: []plan.FieldSpec{
                {Name: "id", Gen: "seq"},
                {Name: "age", Gen: "int", Config: map[string]any{"min": 18, "max": 65}},
                {Name: "active", Gen: "bool", Config: map[string]any{"probability": 0.8}},
            },
        },
    },
}
```
