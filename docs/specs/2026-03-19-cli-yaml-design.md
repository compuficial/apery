# CLI & YAML Plan Loading — Design Spec

## Context

Apery's core engine is complete (generators, runtime, writers, determinism tests). The next step is making it usable without writing Go code. This spec covers YAML/JSON plan file loading and a Cobra-based CLI following kubectl/oc patterns.

## Design Decisions

- **Stdout by default** — `apery generate -f plan.yaml` writes to stdout for unix composability. `--output-dir` writes to files instead.
- **Split entities opt-in** — `--split-entities` (requires `--output-dir`) writes one file per entity. Default is single stream/file.
- **Silent by default** — no progress output unless `-v`/`--verbose`. Errors always go to stderr. Agent-first.
- **JSON and YAML plans** — format detected by file extension (`.yaml`/`.yml` vs `.json`).
- **Generator self-description** — each generator registers a `GeneratorInfo` struct with description, config schema, and example. Powers `generators list` and `generators describe`.

## 1. Plan File Loading

### Struct Tags

Add `json` and `yaml` tags to all plan structs in `internal/plan/plan.go`:

```go
type Plan struct {
    Seed     int64        `json:"seed" yaml:"seed"`
    Entities []EntitySpec `json:"entities" yaml:"entities"`
}

type EntitySpec struct {
    Name     string      `json:"name" yaml:"name"`
    Count    int64       `json:"count,omitempty" yaml:"count,omitempty"`
    DrivenBy *DrivenBy   `json:"driven_by,omitempty" yaml:"driven_by,omitempty"`
    Fields   []FieldSpec `json:"fields" yaml:"fields"`
}

type DrivenBy struct {
    Entity string `json:"entity" yaml:"entity"`
    Field  string `json:"field" yaml:"field"`
    As     string `json:"as" yaml:"as"`
    Min    int64  `json:"min" yaml:"min"`
    Max    int64  `json:"max" yaml:"max"`
}

type FieldSpec struct {
    Name   string         `json:"name" yaml:"name"`
    Gen    string         `json:"gen" yaml:"gen"`
    Config map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
}
```

### Load Function

New file `internal/plan/load.go`:

```go
func LoadFile(path string) (*Plan, error)
```

- Reads file, detects format by extension (`.yaml`/`.yml` → `yaml.v3`, `.json` → `encoding/json`)
- Returns error for unknown extensions
- Validates after loading via existing `Validate()`

New dependency: `gopkg.in/yaml.v3`

## 2. Generator Metadata

### Types

Add to `internal/registry/registry.go`:

```go
type GeneratorInfo struct {
    Name        string      // generator name (matches registration key)
    Description string      // one-line summary
    ConfigKeys  []ConfigKey // documented config parameters
    Example     string      // short YAML example showing plan syntax
}

type ConfigKey struct {
    Name     string // config key name
    Type     string // "int", "float", "string", "bool", "[]any", "map[string]any"
    Required bool
    Default  string // human-readable default, empty if none
    Desc     string // one-line description
}
```

### Registration

New global map alongside `generators`:

```go
var generatorInfos = make(map[string]GeneratorInfo)
```

New functions:

```go
func MustRegisterInfo(name string, info GeneratorInfo) // panics if generator name not already registered
func ListGenerators() []GeneratorInfo                  // all generators, sorted by name
func GetInfo(name string) (GeneratorInfo, bool)        // single generator lookup
```

Only `MustRegisterInfo` is provided (no unchecked `RegisterInfo`) — it panics if the generator name hasn't been registered via `MustRegister` first, preventing orphaned metadata.

Each generator's `init()` adds a `MustRegisterInfo()` call after `MustRegister()`. Existing `MustRegister` signature unchanged.

## 3. CLI Structure

### Dependencies

New: `github.com/spf13/cobra`

### File Layout

```
cmd/apery/
    main.go        — root command, version flag
    generate.go    — generate subcommand
    validate.go    — validate subcommand
    generators.go  — generators parent + list/describe subcommands
```

### Root Command

```
apery — Deterministic synthetic data generator

Usage:
  apery [command]

Available Commands:
  generate     Generate synthetic data from a plan file
  validate     Validate a plan file without generating
  generators   List and describe available generators

Flags:
  -h, --help      help for apery
      --version   version for apery
```

Version is populated via `var Version = "dev"` in `main.go`, overridden at build time with `-ldflags "-X main.Version=..."` in the Makefile.

### Generate Command

```
apery generate -f plan.yaml [-o format] [--output-dir dir] [flags]

Flags:
  -f, --file string         Plan file path (required)
  -o, --output string       Output format: jsonl, csv (default "jsonl")
      --output-dir string   Write output to directory instead of stdout
      --split-entities      Write one file per entity (requires --output-dir)
      --dry-run             Validate plan without generating
      --seed int            Override plan seed
      --workers int         Number of parallel workers
      --chunk-size int      Rows per chunk
      --verbose             Show progress on stderr
```

**Behavior:**
- No `--output-dir`: create writer over `os.Stdout`, all entities in one stream. JSONL is recommended for multi-entity stdout (each record is self-contained with `_entity` field). CSV to stdout with multiple entities of different schemas uses the first entity's columns — missing fields are empty. For mixed-schema CSV, use `--output-dir --split-entities`.
- `--output-dir` without `--split-entities`: single file `output.jsonl` or `output.csv`
- `--output-dir` with `--split-entities`: one file per entity (`<Entity>.jsonl` or `<Entity>.csv`). The `_entity` column is omitted in split mode since the filename carries the entity identity.
- `--dry-run`: load plan, validate, print "Plan is valid" to stdout, exit 0
- `--seed`: overrides `plan.Seed` after loading
- `--verbose`: print entity completion lines to stderr (`User: 20000 rows (1.2s)`)

### Validate Command

```
apery validate -f plan.yaml

Flags:
  -f, --file string   Plan file path (required)
```

Loads plan, runs validation, prints result to stdout. Exit 0 on valid, exit 1 with error message on invalid.

### Generators Command

```
apery generators list
apery generators describe <name>
```

**list**: prints a table of all generators with name and one-line description, sorted alphabetically.

**describe**: prints full info for one generator — description, config keys (name, type, required, default, description), and YAML example.

### Exit Codes

| Code | Constant | Meaning |
|------|----------|---------|
| 0 | — | Success |
| 1 | `ExitValidation` | Plan validation error |
| 2 | `ExitGeneration` | Generation/runtime error |
| 3 | `ExitIO` | I/O error (file not found, permission denied) |

### Help Text

Every command has:
- `Short`: one line shown in parent command's help
- `Long`: paragraph with context
- `Example`: concrete usage examples shown in `--help`

## 4. Writer Changes

### io.Writer-based Constructors

`NewJSONLWriterFromWriter(w io.Writer)` already exists — the path-based constructor is already a wrapper around it.

Add matching `NewCSVWriterFromWriter(w io.Writer)` to `internal/writer/csv.go`. This requires refactoring `CSVWriter` internals to separate file ownership from writing (currently the path-based constructor writes directly to the file). After refactor, the path-based `NewCSVWriter(path)` becomes a wrapper: open file, call `NewCSVWriterFromWriter`. `Close()` only closes the underlying file if the writer opened it (not for stdout).

### SplitWriter

New file `internal/writer/split.go`:

```go
type SplitWriter struct {
    dir    string
    format string           // "jsonl" or "csv"
    writers map[string]Writer // lazy-created per entity
}

func NewSplitWriter(dir, format string) *SplitWriter
func (sw *SplitWriter) WriteRecord(entity string, record *OrderedMap) error
func (sw *SplitWriter) Close() error
```

On first `WriteRecord` for a given entity name, creates `<dir>/<Entity>.<format>` via the appropriate writer constructor. Routes subsequent records to the correct writer. `Close()` closes all open writers.

## 5. Public API Updates

Update `run.go` to re-export new types:

```go
type GeneratorInfo = registry.GeneratorInfo
type ConfigKey = registry.ConfigKey

func LoadPlanFile(path string) (*Plan, error)
func ListGenerators() []GeneratorInfo
func NewJSONLWriterFromWriter(w io.Writer) *JSONLWriter
func NewCSVWriterFromWriter(w io.Writer) *CSVWriter
func NewSplitWriter(dir, format string) *SplitWriter
```

## Files Changed

| File | Change |
|------|--------|
| `internal/plan/plan.go` | Add json/yaml struct tags |
| `internal/plan/load.go` | **New** — `LoadFile()` |
| `internal/registry/registry.go` | Add `GeneratorInfo`, `ConfigKey`, `ListGenerators()`, `GetInfo()` |
| `internal/registry/*.go` (each generator) | Add `MustRegisterInfo()` in `init()` |
| `internal/writer/jsonl.go` | Add `NewJSONLWriterFromWriter()`, refactor path constructor |
| `internal/writer/csv.go` | Add `NewCSVWriterFromWriter()`, refactor path constructor |
| `internal/writer/split.go` | **New** — `SplitWriter` |
| `cmd/apery/main.go` | **Rewrite** — Cobra root command |
| `cmd/apery/generate.go` | **New** — generate subcommand |
| `cmd/apery/validate.go` | **New** — validate subcommand |
| `cmd/apery/generators.go` | **New** — generators list/describe |
| `run.go` | Add new re-exports |
| `go.mod` | Add `gopkg.in/yaml.v3`, `github.com/spf13/cobra` |
