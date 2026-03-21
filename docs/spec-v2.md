# Apery — Specification v2

## 1. Introduction

Apery is a deterministic synthetic data generator (SDG) written in Go. It generates schema-driven synthetic data from declarative plans, producing identical output given the same plan and seed.

Apery is built for **AI agents as first-class citizens**. The primary interface is a CLI with structured input/output, designed for programmatic use by autonomous agents, scripts, and pipelines. Human usability follows naturally from good machine interfaces.

### Design Goals

- **Deterministic**: Plan + Seed = identical output, always
- **Declarative**: Plans describe *what* to generate, not *how*
- **Composable**: ~20 universal primitives combine to model any schema
- **Fast**: Chunked parallel execution scales to millions of rows
- **Agent-friendly**: CLI with YAML/JSON plans, structured output, clear exit codes

---

## 2. Core Philosophy

### 2.1 Composition Over Code

The generator set is deliberately small. Rather than shipping hundreds of domain-specific generators, Apery provides ~20 universal primitives that compose via nesting, templates, conditional dispatch, and value lists.

Examples:
- **Full name**: `template("{first_name} {last_name}")` over `pick` fields
- **Email**: `template("{username}@{domain}")` with `lower: true`
- **Address**: `object` with nested `pick`, `int`, `regex` sub-fields
- **Tagged union**: `switch` on a `pick` field to dispatch different `object` shapes
- **Nested arrays**: `list` wrapping `object` wrapping more generators

This keeps the engine small, auditable, and predictable.

### 2.2 Determinism Under Seed

The system guarantees bit-perfect reproducibility:

```
Plan + Seed = Identical Output
```

This holds across:
- Parallel execution (any worker count, any chunk size)
- Platform (explicit `int64`/`float64` types throughout)
- Foreign-key relations and driven-by child counts
- Composite generators (order-independent sub-field seed derivation)

### 2.3 Agent-First Design

Apery is software built primarily for agents to use:
- **CLI** as the sole interface — no HTTP server, no GraphQL, no MCP
- **YAML/JSON plans** as input — declarative, versionable, diffable
- **Structured output** — JSONL, CSV, with format selection via flags
- **Clear exit codes** — machine-parseable success/failure
- **kubectl/oc-style UX** — familiar patterns for k8s-native tooling and agents

---

## 3. Architecture

```
Plan (YAML/JSON) → Validation → Registry (Generator Factories) → Runtime (Executor) → Writer (Output)
```

### Components

| Component | Package | Role |
|-----------|---------|------|
| **Plan** | `internal/plan` | Declarative schema: entities, fields, generators, relations |
| **Registry** | `internal/registry` | Generator factories, interfaces, auto-registration via `init()` |
| **Runtime** | `internal/runtime` | Execution orchestrator: seed derivation, chunking, parallelism, entity store |
| **Writer** | `internal/writer` | Output abstraction: JSONL, CSV with ordered field output |
| **RNG** | `internal/rng` | Deterministic PRNG with hierarchical seed derivation (PCG + FNV-1a) |
| **CLI** | `cmd/apery` | Entry point (currently example-based; CLI subcommands planned) |

### Public API

`run.go` re-exports the essential types and functions for programmatic use:

```go
apery.Run(ctx, &plan, writer, opts...)        // execute a plan
apery.ValidatePlan(&plan)                      // validate without executing
apery.NewJSONLWriter(path)                     // create JSONL writer
apery.NewCSVWriter(path)                       // create CSV writer
```

Options: `WithWorkers(n)`, `WithChunkSize(n)`, `WithLogger(l)`.

---

## 4. Declarative Data Model

### 4.1 Plan

A plan is the top-level input. It contains a seed and an ordered list of entities.

```go
type Plan struct {
    Seed     int64
    Entities []EntitySpec
}
```

**Entity ordering matters**: entities that reference other entities (via `rel_ref` or `driven_by`) must appear after their dependencies.

### 4.2 Entity

Each entity represents a table or collection. Exactly one of `Count` or `DrivenBy` must be set.

```go
type EntitySpec struct {
    Name     string
    Count    int64       // standalone entity: generate this many rows
    DrivenBy *DrivenBy   // driven entity: generate children per parent row
    Fields   []FieldSpec
}
```

### 4.3 Field

Each field names a generator and its configuration.

```go
type FieldSpec struct {
    Name   string
    Gen    string         // generator name (e.g., "int", "pick", "rel_ref")
    Config map[string]any // generator-specific configuration
}
```

### 4.4 DrivenBy (1:M Relationships)

When set on an entity, the executor generates `Min` to `Max` child rows per parent row instead of using `Count`. The parent's field value is auto-injected into each child row.

```go
type DrivenBy struct {
    Entity string // parent entity name
    Field  string // parent field to sample
    As     string // field name in child row for the injected value
    Min    int64  // minimum children per parent (>= 1)
    Max    int64  // maximum children per parent (>= Min)
}
```

### 4.5 YAML Plan Format (Planned)

Plans will be authored as YAML files and passed via `-f`:

```yaml
seed: 42
entities:
  - name: User
    count: 10000
    fields:
      - name: id
        gen: seq
        config: { start: 1 }
      - name: name
        gen: pick
        config: { values: ["Alice", "Bob", "Carol"] }
      - name: active
        gen: bool
        config: { p: 0.8 }

  - name: Order
    driven_by:
      entity: User
      field: id
      as: user_id
      min: 1
      max: 5
    fields:
      - name: order_id
        gen: ulid
      - name: amount
        gen: float
        config: { min: 9.99, max: 999.99 }
```

---

## 5. Generator Registry

### 5.1 Interfaces

```go
// Core: every generator implements this
type Generator interface {
    Next(r *rng.Rng) (any, error)
}

// Row-aware: generators that read other fields in the current row (e.g., template, switch)
type RowAwareGenerator interface {
    Generator
    NextWithRow(r *rng.Rng, row RowContext) (any, error)
}

// Dependency declaration: generators that reference other fields declare them for validation
type DependencyDeclarer interface {
    Dependencies() []string
}

// Resettable: generators with state that resets between parent batches (e.g., unique rel_ref)
type Resettable interface {
    Reset()
}
```

### 5.2 Registration

Generators self-register at init time:

```go
func init() {
    MustRegister("pick", func(config map[string]any) (Generator, error) {
        // validate config, build generator
        return &PickGenerator{values: values}, nil
    })
}
```

The registry is global and populated at program startup. `Register()` returns an error; `MustRegister()` panics on duplicate names.

### 5.3 Generator Catalog

#### Scalar Generators

| Generator | Config | Output | Description |
|-----------|--------|--------|-------------|
| `seq` | `start`, `step` | `int64` | Sequential integers |
| `const` | `value` | `any` | Fixed literal value |
| `bool` | `p` | `bool` | Weighted boolean (probability p for true) |
| `int` | `min`, `max` | `int64` | Uniform random integer in range |
| `float` | `min`, `max` | `float64` | Uniform random float in range |
| `normal_int` | `mean`, `stddev`, `min`, `max` | `int64` | Normally distributed integer with optional clamp |
| `normal_float` | `mean`, `stddev` | `float64` | Normally distributed float |
| `zipf` | `s`, `v`, `max` | `uint64` | Zipf-distributed integer |
| `pick` | `values`\|`file`\|`url`, `weights` | `any` | Random selection from list; optional weighted mode |
| `uuid` | — | `string` | UUID v4 |
| `ulid` | — | `string` | ULID (Universally Unique Lexicographically Sortable ID) |
| `time` | `start`, `end`, `format` | `string` | Random timestamp in range |
| `regex` | `pattern`, `max_repeat` | `string` | String matching regex pattern subset |

#### Composite Generators

| Generator | Config | Output | Description |
|-----------|--------|--------|-------------|
| `object` | `fields` (nested FieldSpecs) | `map` | Nested object with sub-generators |
| `list` | `item`, `len`\|`min_len`+`max_len` | `[]any` | Array of generated items |
| `sample` | `values`, `n`\|`min_n`+`max_n` | `[]any` | N unique items without replacement |
| `one_of` | `generators`, `weights` | `any` | Weighted random generator dispatch |
| `template` | `tpl`, `lower` | `string` | String interpolation with `{field}` placeholders from row |
| `switch` | `key`, `cases`, `default` | `any` | Conditional dispatch based on another field's value |

#### Relational Generator

| Generator | Config | Output | Description |
|-----------|--------|--------|-------------|
| `rel_ref` | `entity`, `field`, `dist`, `unique` | `any` | Sample from previously generated entity column |

### 5.4 Regex Subset

The `regex` generator supports a generatable subset:

**Supported**: literals, concatenation, alternation, grouping, character classes (including Unicode), quantifiers (`*`, `+`, `?`, `{m}`, `{m,n}`, `{m,}`), anchors `^`/`$` at start/end only.

**Not supported** (validation error): word boundaries (`\b`/`\B`), lookahead/lookbehind, backreferences, recursion, conditionals.

**Character domain**: `.` generates printable ASCII (32–126). Character classes are rune-based and cover full Unicode.

---

## 6. Relational Model

### 6.1 M:1 — Foreign Key Sampling (`rel_ref`)

`rel_ref` samples values from a previously generated entity's column:

```yaml
- name: user_id
  gen: rel_ref
  config:
    entity: User
    field: id
    dist: uniform    # or "zipf" with s/v params
    unique: false    # true for deduplication within parent batch
```

Distribution options:
- **uniform** (default): equal probability across all parent values
- **zipf**: skewed distribution (hot keys), configured with `s` and `v` parameters

### 6.2 1:M — Parent-Driven Generation (`driven_by`)

`driven_by` on an entity generates variable child rows per parent:

```yaml
- name: Order
  driven_by:
    entity: User
    field: id
    as: user_id
    min: 1
    max: 5
```

The parent field value is auto-injected into each child row as the `as` field. Child count per parent is deterministic (derived from parent index + count seed).

### 6.3 M:N — Composition

M:N relationships compose `driven_by` (1:M from left) with `rel_ref unique` (M:1 to right) on a junction entity:

```yaml
# Left entity: User (standalone, count: 1000)
# Right entity: Tag (standalone, count: 50)

- name: UserTag          # junction entity
  driven_by:
    entity: User
    field: id
    as: user_id
    min: 1
    max: 5
  fields:
    - name: tag_id
      gen: rel_ref
      config:
        entity: Tag
        field: id
        unique: true      # no duplicate tags per user
```

### 6.4 Design Rules

- Entities must be declared in dependency order (downstream after upstream)
- `DrivenBy.Min >= 1` (always produces at least one child per parent)
- Config keys starting with `_` are reserved for internal use (e.g., `_store`)
- `Resettable` generators reset on parent transitions in driven_by chunks
- Standalone entities with `unique: true` `rel_ref` force single-threaded execution

---

## 7. Execution Engine

### 7.1 Seed Derivation Hierarchy

```
Root Seed (Plan.Seed → SeedFromInt64)
  └─ Entity Seed: Derive(root, "EntityName[index]")
      ├─ Field Seed: Derive(entitySeed, fieldName)
      │   └─ Row Seed: DeriveIndex(fieldSeed, rowIndex)
      │       └─ Sub-Field Seed: Derive(rowSeed, subFieldName)  [composite generators]
      └─ Count Seed: Derive(entitySeed, "counts")               [driven_by]
          └─ Per-Parent: DeriveIndex(countSeed, parentIndex)
```

Key properties:
- **Hierarchical**: each level derives from parent, ensuring isolation
- **Order-independent**: composite generators derive sub-seeds by name, not position
- **Parallel-safe**: row seeds depend only on field seed + row index, not execution order

### 7.2 Chunk-Based Parallelism

The executor splits row ranges into chunks and processes them in parallel:

- **Default chunk size**: 50,000 rows
- **Default workers**: `min(2 × NumCPU, 64)`
- Each chunk gets its own generator instances (factories called per-chunk)
- Results are collected and written in order

For driven_by entities with unique `rel_ref` fields, chunks are aligned to parent boundaries so `Resettable` generators reset correctly.

### 7.3 Generation Flow

1. **Validate** plan structure and relational constraints
2. **Initialize** entity store (`mapEntityStore`) for cross-entity column access
3. **For each entity** (in declaration order):
   - **Standalone** (`Count` set): create chunks → fan out to workers → generate rows → write
   - **Driven** (`DrivenBy` set): Phase 1: compute layout (child counts per parent, prefix sums). Phase 2: chunked parallel generation with parent tracking
4. **Store columns** needed by downstream entities in the entity store
5. **Close** writer

### 7.4 Row Generation

Per row within a chunk:
1. Derive row-level RNG from field seed + row index
2. For each field:
   - If `RowAwareGenerator`: call `NextWithRow(rng, rowContext)` (has access to prior fields)
   - Else: call `Next(rng)`
3. Collect field values into `OrderedMap`
4. Write record via writer

### 7.5 Entity Store

The `mapEntityStore` holds columns from completed entities for downstream `rel_ref` access:

```go
type EntityStore interface {
    ReadOnlyEntityStore
    StoreColumn(entity, field string, values []any)
}

type ReadOnlyEntityStore interface {
    GetColumn(entity, field string) ([]any, bool)
}
```

The store is injected into `rel_ref` generators via the reserved `_store` config key at chunk instantiation time.

---

## 8. Writers

### 8.1 Interface

```go
type Writer interface {
    WriteRecord(entity string, record *OrderedMap) error
    Close() error
}
```

`OrderedMap` preserves field insertion order for deterministic output column ordering.

### 8.2 JSONL Writer

Streams newline-delimited JSON. Each record is a JSON object with an `_entity` field prepended:

```json
{"_entity":"User","id":1,"name":"Alice","active":true}
{"_entity":"User","id":2,"name":"Bob","active":false}
```

### 8.3 CSV Writer

Streams CSV with a header row. The entity name is included as the first column:

```csv
_entity,id,name,active
User,1,Alice,true
User,2,Bob,false
```

Header is emitted on the first record; subsequent records follow the same column order.

### 8.4 Planned Writers

- **Parquet**: columnar binary for data lakes and analytics
- **SQL**: INSERT statements for database seeding

---

## 9. RNG & Determinism

### 9.1 Implementation

The RNG wraps Go's `math/rand` with PCG (`rand.NewPCG(seed, 0)`):

- `Seed` type is `uint64`
- `SeedFromInt64(v)` converts plan seeds
- `New(seed)` creates an RNG instance
- `GetSeed()` returns the construction seed (used by composite generators for child derivation)

### 9.2 Seed Derivation

Two derivation functions:

```go
// String-labeled derivation (entity names, field names)
func Derive(parent Seed, label string) Seed
// FNV-1a hash of label, XOR with parent, then mix64

// Index-based derivation (row indices, parent indices)
func DeriveIndex(parent Seed, index int64) Seed
// XOR parent with index, then mix64
```

`mix64` applies bit mixing (shifts + multiplies) to ensure good distribution properties.

### 9.3 io.Reader Compatibility

`Rng` implements `io.Reader`, filling byte slices from the PRNG stream. This enables compatibility with libraries requiring entropy sources (e.g., ULID generation via `ulid.New()`).

---

## 10. CLI Interface

### 10.1 Design Principles

- **kubectl/oc patterns**: resource-oriented subcommands, familiar flags
- **Declarative config**: plans as YAML/JSON files
- **Machine-first output**: structured formats, clear exit codes
- **Preview before generate**: `--dry-run` to validate without producing output

### 10.2 Command Structure

```
apery generate -f plan.yaml -o jsonl              # generate data
apery generate -f plan.yaml -o csv --output-dir ./out
apery generate -f plan.yaml --dry-run              # validate only
apery validate -f plan.yaml                        # validate plan
apery list generators                              # list available generators
apery describe generator <name>                    # show generator config schema
apery version                                      # print version
```

### 10.3 Flags

| Flag | Description |
|------|-------------|
| `-f`, `--file` | Plan file path (YAML or JSON) |
| `-o`, `--output` | Output format: `jsonl`, `csv` |
| `--output-dir` | Write output to directory instead of stdout |
| `--split-entities` | One file per entity (requires `--output-dir`) |
| `--dry-run` | Validate plan without generating |
| `--seed` | Override the seed defined in the plan file |
| `--workers` | Number of parallel workers, auto-detected if not set |
| `--chunk-size` | Rows per chunk, defaults to 50000 if not set |
| `--verbose` | Show entity progress on stderr (silent by default) |
| `--debug` | Show detailed debug output on stderr (seeds, chunks, layout) |

### 10.4 Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Plan validation error |
| 2 | Generation error |
| 3 | I/O error (file not found, permission denied) |

---

## 11. Plan Validation

Validation catches errors before execution:

### Entity-Level
- Name is non-empty and unique
- Exactly one of `Count` or `DrivenBy` is set
- At least one field defined
- No duplicate field names
- No reserved config keys (prefix `_`) in field configs

### Relational Constraints
- `DrivenBy` parent entity declared before child (ordering)
- `DrivenBy` parent field exists in parent entity
- `DrivenBy.As` does not conflict with declared child fields
- `DrivenBy.Min >= 1`, `DrivenBy.Max >= DrivenBy.Min`
- `rel_ref` target entity declared before referencing entity
- `rel_ref` target field exists in target entity
- Unique `rel_ref` in driven_by: `DrivenBy.Max <= target entity Count` (feasibility)

---

## 12. Roadmap

### Near-Term (Active)
- [ ] CLI with subcommand structure (generate, validate, generators)
- [ ] YAML plan file support (`-f plan.yaml`)
- [ ] Output format flag (`-o jsonl/csv`)
- [ ] `--dry-run` / validation-only mode
- [ ] Bundled catalogs (names, companies, domains, cities)

### Medium-Term
- [ ] Parquet writer
- [ ] SQL writer (INSERT statements)
- [ ] Train/val/test split modes
- [ ] Weighted alias table for catalog performance
- [ ] Buffer reuse and preallocation in writers

### Future
- [ ] Cloud storage connectors (S3/GCS)
- [ ] Remote catalog loading with caching
- [ ] Statistical sanity checks for RNG-dependent generators
- [ ] Cross-version RNG compatibility notes
