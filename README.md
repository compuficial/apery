# Apery

A deterministic synthetic data generator built in Go.

## Overview

Apery generates synthetic data from declarative plans with guaranteed reproducibility. Given the same plan and seed, it produces identical output every time.

**Key Features:**

- Deterministic generation (Plan + Seed + Version = Identical Output)
- Platform-independent output (explicit `int64`/`float64` types)
- Hierarchical RNG seeding for parallel execution
- Chunked parallel execution with configurable worker count and chunk size
- Extensible generator registry
- Multiple output formats (JSONL, CSV)

See [spec.md](docs/spec.md) for full design specification.

## Quick Start

```bash
# Build the project
make build

# Run the generator
make run

# Format code
make fmt

# Run benchmarks
make bench
```

## Architecture

```
internal/
├── plan/       - Plan data structures
├── registry/   - Generator registry and primitives
├── rng/        - Deterministic RNG with hierarchical seeding
├── runtime/    - Execution orchestrator
└── writer/     - Output writers (JSONL, CSV)
```

## Generators

### Scalar Generators

- `seq` - Sequential integers with configurable start/step
- `pick` - Random selection from a list (inline, file, or URL), with optional weights
- `const` - Fixed constant value (string, int, float, bool, or null)
- `bool` - Boolean with configurable probability
- `int` - Random integers within a range
- `float` - Random floats within a range
- `normal_int` - Normally distributed integers with optional clamping
- `normal_float` - Normally distributed floats
- `zipf` - Zipf-distributed integers
- `regex` - Strings matching a limited regex subset
- `uuid` - UUID v4 strings
- `ulid` - ULID strings (sortable unique identifiers)
- `time` - Timestamps within a configurable range

### Composite Generators

- `object` - Nested objects with named sub-fields, each with its own generator
- `list` - Arrays of items from a single generator (fixed `len` or variable `min_len`/`max_len`)
- `sample` - Select N unique items without replacement (fixed `n` or variable `min_n`/`max_n`)
- `one_of` - Randomly dispatch to one of several generators, with optional weights
- `template` - String interpolation with `{field_name}` placeholders from the current row
- `switch` - Dispatch to sub-generator based on another field's value, with optional default

## Example

```go
p := apery.Plan{
    Seed: 4,
    Entities: []apery.EntitySpec{
        {
            Name:  "User",
            Count: 20_000,
            Fields: []apery.FieldSpec{
                {Name: "id", Gen: "seq"},
                {Name: "employee_number", Gen: "seq"},
                {Name: "is_active", Gen: "bool", Config: map[string]any{"probability": 0.7}},
                {Name: "department", Gen: "pick", Config: map[string]any{
                    "values":  []any{"engineering", "sales", "marketing", "support"},
                    "weights": []any{40, 30, 20, 10},
                }},
                {Name: "department_code", Gen: "int", Config: map[string]any{"max": 100}},
                {Name: "idn", Gen: "ulid"},
                {Name: "timestamp", Gen: "time", Config: map[string]any{"format": "2006-01-02"}},
                {Name: "phone", Gen: "regex", Config: map[string]any{"pattern": `\(\d{3}\) \d{3}-\d{4}`}},
                {Name: "sku", Gen: "regex", Config: map[string]any{"pattern": `[A-Z]{2}-\d{6}`}},
                {Name: "license_plate", Gen: "regex", Config: map[string]any{"pattern": `[A-Z]{3}-\d{4}`}},
                {Name: "address", Gen: "object", Config: map[string]any{
                    "fields": map[string]any{
                        "city":  map[string]any{"gen": "pick", "config": map[string]any{"values": []any{"New York", "Los Angeles", "Chicago", "Houston", "Phoenix"}}},
                        "zip":   map[string]any{"gen": "int", "config": map[string]any{"min": 10000, "max": 99999}},
                        "suite": map[string]any{"gen": "regex", "config": map[string]any{"pattern": `[A-Z]\d{3}`}},
                        "geo": map[string]any{"gen": "object", "config": map[string]any{
                            "fields": map[string]any{
                                "lat": map[string]any{"gen": "float", "config": map[string]any{"min": -90.0, "max": 90.0}},
                                "lng": map[string]any{"gen": "float", "config": map[string]any{"min": -180.0, "max": 180.0}},
                            },
                        }},
                    },
                }},
                {Name: "status", Gen: "const", Config: map[string]any{"value": "active"}},
                {Name: "tags", Gen: "list", Config: map[string]any{
                    "min_len": 1,
                    "max_len": 4,
                    "item": map[string]any{"gen": "pick", "config": map[string]any{"values": []any{"admin", "beta", "premium", "internal", "vip"}}},
                }},
                {Name: "skills", Gen: "sample", Config: map[string]any{
                    "values": []any{"Go", "Python", "Rust", "TypeScript", "Java", "C++", "Ruby", "Kotlin"},
                    "min_n":  2,
                    "max_n":  5,
                }},
                {Name: "contact_method", Gen: "one_of", Config: map[string]any{
                    "generators": []any{
                        map[string]any{"gen": "regex", "config": map[string]any{"pattern": `[a-z]{5,10}@(gmail|yahoo|outlook)\.com`}},
                        map[string]any{"gen": "regex", "config": map[string]any{"pattern": `\+1-\d{3}-\d{3}-\d{4}`}},
                    },
                    "weights": []any{7.0, 3.0},
                }},
                {Name: "greeting", Gen: "template", Config: map[string]any{
                    "tpl": "Welcome, employee #{id} from {department}!",
                }},
                {Name: "access_level", Gen: "switch", Config: map[string]any{
                    "key": "department",
                    "cases": map[string]any{
                        "engineering": map[string]any{"gen": "const", "config": map[string]any{"value": "full"}},
                        "sales":       map[string]any{"gen": "const", "config": map[string]any{"value": "read-only"}},
                        "marketing":   map[string]any{"gen": "const", "config": map[string]any{"value": "read-only"}},
                        "support":     map[string]any{"gen": "const", "config": map[string]any{"value": "limited"}},
                    },
                    "default": map[string]any{"gen": "const", "config": map[string]any{"value": "standard"}},
                }},
            },
        },
    },
}
```

You can also run plans via the library API:

```go
w, err := apery.NewJSONLWriter("output.jsonl")
if err != nil {
    return err
}
if err := apery.Run(context.Background(), &p, w,
    runtime.WithWorkers(16),
    runtime.WithChunkSize(10000),
); err != nil {
    return err
}
```

## Determinism Testing

Apery guarantees **Plan + Seed = Identical Output**. Golden tests catch accidental output drift, and stress tests verify determinism across different parallelism configurations.

```bash
# Run golden tests (compare against stored reference output)
go test ./internal/runtime -run TestGolden -v

# Regenerate golden files after intentional output changes
go test ./internal/runtime -run TestGolden -update -v

# Run concurrency stress tests (randomized worker/chunk configs)
go test ./internal/runtime -run TestStress -v
```

## Benchmarking

```bash
make bench
```
