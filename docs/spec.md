# Synthetic Data Generator (Go + GraphQL) — Full Expanded Design / Specification / Requirements

## 1. Introduction

This document provides a deeply technical and comprehensive design and implementation specification for an **AI‑centric Synthetic Data Generator (SDG)** implemented in **Go**, exposed via **GraphQL**, and intended for use by:

* LLMs (Large Language Models)
* Autonomous AI Agents
* MCP (Model Context Protocol) clients
* Humans needing deterministic, schema‑driven synthetic data at scale

The system is not merely a test‑data tool but a fully declarative, extensible, deterministic synthetic data universe that AI systems and humans can call, understand, reason about, and modify.

The generator must:

* Support declarative schemas
* Allow fully deterministic generation under a global seed
* Scale to millions of rows
* Provide relational integrity
* Support primitive and composite generators
* Allow AI agents to define or mutate schemas
* Output structured, semi‑structured, conversational, and tool‑trace data formats

This guide describes how to **build the entire system end‑to‑end**, including architecture, generator registry, execution engine, catalogs, LLM integrations, NLP plan compiler, and runtime design.

---

## 2. Core Philosophy

### 2.1 Composition Over Code

The core design rests on **minimal primitives** plus **combinators**, making user‑defined compositions far more powerful than a large fixed set of coded generators.

The SDG does **not** ship 200 custom generators.
It ships **~22 universal primitives**, and all complexity is built via:

* nested objects
* lists
* templates
* conditional dispatch
* catalogs

This keeps the engine small, fast, and predictable.

### 2.2 Determinism Under Seed

The system guarantees bit‑perfect reproducibility:

```
Plan + Seed + Version = Exactly Identical Output
```

This includes:

* parallel execution
* foreign‑key relations
* normal distributions
* template outputs
* platform independence (32-bit vs 64-bit systems)

**Implementation:** All generators use explicit sized types (`int64`, `float64`) to ensure identical behavior across platforms.

### 2.3 AI‑First Design

The system must:

* Accept **LLM function‑calling input**
* Expose **MCP resources and tools**
* Allow AI to discover generators, catalogs, and fields
* Provide a **natural‑language → plan compiler**
* Provide examples for LLMs to condition on

### 2.4 Catalog‑Driven Realism

Catalogs come from:

* built‑in system catalogs
* user‑uploaded catalogs
* remote URLs
* LLM‑synthesized catalogs

### 2.5 Mixed‑Mode Output

Supports:

* relational data
* JSONL
* CSV
* Parquet
* SQL dumps
* SFT datasets
* DPO/RLHF preference pairs
* Chat message arrays
* Tool call traces
* RAG evaluation triples

---

## 3. High‑Level Architecture

The SDG architecture includes 9 major components:

```
+---------------------------------------------------------------+
| AI‑Centric Synthetic Data Generator                            |
+------------+----------------+---------------------------------+
| 1. Plan    | 2. Registry    | 3. Catalog Loader               |
| Compiler   | (Primitives)   | (Files, URLs, Inline)           |
+------------+----------------+---------------------------------+
| 4. Execution Orchestrator (Parallel deterministic RNG engine) |
+---------------------------------------------------------------+
| 5. Writers: JSONL | CSV | Parquet | SQL | SFT | DPO | Chat |  |
|             RAG | Tool-Trace                                    |
+---------------------------------------------------------------+
| 6. Expression Engine (CEL or Starlark)                        |
+---------------------------------------------------------------+
| 7. Interfaces: GraphQL | HTTP | CLI | MCP | SDK               |
+---------------------------------------------------------------+
| 8. NL Plan Compiler (LLM-driven)                              |
+---------------------------------------------------------------+
| 9. Telemetry and Observability                                |
+---------------------------------------------------------------+
```

---

## 4. Declarative Data Model

### 4.1 Plan

A plan defines:

* Entities
* Fields
* Generators
* Relations
* Constraints
* Output formats

Plans are declarative JSON or GraphQL inputs.

### 4.2 Entities

Each represents a table, e.g.:

* `User`
* `Company`
* `Encounter`
* `Chat`

Includes:

* row count
* fields
* relations
* uniqueness

### 4.3 Fields

A field includes:

* name
* type
* generator name
* generator config
* dependency list
* uniqueness
* null probability

### 4.4 Relations

Types:

* many‑to‑one
* one‑to‑many
* many‑to‑many

Foreign key sampling uses weighted alias tables.

---

## 5. Generator Registry (Core of the System)

The SDG ships a **minimal set** of primitives:

### 5.1 Scalar Generators

* `uniform_int(min,max)` — generates `int64` values uniformly distributed in range
* `uniform_float(min,max)` — generates `float64` values uniformly distributed in range
* `normal_float(mu,sigma)` — generates `float64` values with normal distribution
* `normal_int(mu,sigma,clamp)` — generates `int64` values with normal distribution
* `zipf(s,vmax)` — generates `int64` values following Zipf distribution
* `bool(p)` — generates boolean values with probability p
* `regex(pattern)` — generates strings matching a supported regex pattern subset (see below)
* `time(start,end,tz)` — generates timestamps in range
* `uuid_v4()` — generates UUID v4 strings
* `ulid()` — generates ULID strings
* `seq(start,step)` — generates sequential `int64` values
* `pick(values|file|url)` — randomly selects from list
* `pick(values|file|url, weights)` — weighted random selection from list; weights array must match values length, normalized automatically
* `const(value)` — emits a fixed literal value on every row

### 5.2 Composite Generators

* `object(fields)`
* `list(len|min_len+max_len, item)` — generates arrays; `len` for fixed length, or `min_len`/`max_len` for variable length (random uniform within range)
* `sample(values|file|url, n|min_n+max_n)` — selects N unique items without replacement from a value set; errors if N exceeds available values
* `one_of(gens,weights)`
* `switch(key,cases)`
* `template(tpl)` — string interpolation with `{field_name}` placeholders resolved from the current row

### 5.3 Relational Generators

* `rel_ref(target,field)`
* `m2m(target,meanDegree)`

These primitives can build anything: healthcare records, LLM training dialogues, financial schemas, telemetry data, etc.

---

### Regex Generator Subset

The `regex(pattern)` generator supports a **regular-expression subset** that is guaranteed to be generatable and deterministic. If a pattern falls outside this subset, plan validation must fail.

Supported:
- literals, concatenation, alternation, grouping/capture
- character classes (including Unicode ranges)
- quantifiers: `*`, `+`, `?`, `{m}`, `{m,n}`, `{m,}` (unbounded and explicit max are capped by `max_repeat`; error if `m > max_repeat`)
- anchors `^` and `$` **only at the start/end of a concatenation** and never under quantifiers

Not supported (must error):
- word boundaries `\b` / `\B`
- lookahead/lookbehind
- backreferences/recursion/conditionals or other non-regular extensions

Character domain:
- `.` generates printable ASCII (code points 32–126) for readability.
- Character classes are rune-based and can cover full Unicode ranges.

## 6. Catalog Subsystem

### 6.1 Bundled Catalogs

Bundled datasets include:

* first names
* last names
* companies
* domains
* cities
* product names
* words

### 6.2 User Catalogs

Loaded via:

```json
{"file": "/data/custom/names.txt"}
```

### 6.3 Remote Catalogs

Loaded via URL with allowlisting.

### 6.4 Virtual (LLM‑Generated) Catalogs

Agents can create catalogs via natural language.

All catalogs are converted to weighted alias tables.

---

## 7. Execution Engine

### 7.1 Hierarchical RNG Model

```
Root Seed
 ├── Entity Seed
 │      ├── Field Seed
 │      │      └── Row Seed
 │      └── Field Seed
 └── Entity Seed
```

Ensures deterministic parallel execution.

### 7.2 Chunk-Based Parallelism

* default chunk size: **50k rows**
* worker count: **min(2×CPU, 64)**

### 7.3 Deterministic Row Generation

Uses splitmix64 or PCG.

### 7.4 Uniqueness Enforcement

* bounded retries
* entropy estimation
* Bloom prechecks

### 7.5 Relation Resolution

* M:1 sampling via alias table
* 1:M via multinomial split
* M:N via degree distribution + dedupe

---

## 8. Writers

### Supported Formats

* **JSONL** (streaming)
* **CSV**
* **Parquet** — columnar binary format for data lakes, Spark, DuckDB, analytics
* **SQL dumps** — executable INSERT statements for seeding databases
* **SFT JSON** (instruction/input/output)
* **DPO/RLHF preference pairs** — chosen/rejected pairs for LLM alignment training
* **Chat message arrays**
* **Tool-call traces**
* **RAG evaluation triples**

All writers support split modes:

```
train / val / test
```

---

## 9. Natural-Language Plan Compiler

Enables:

* prompt → structured plan
* schema inference
* intent detection
* generator selection

Example input:

```
"Generate 500 retail transactions with realistic product categories."
```

Output plan includes:

* Entities
* Fields
* pick(file|list)
* amounts
* timestamps
* merchant IDs

---

## 10. AI Agent Integration

### 10.1 MCP Tools

Tools exposed:

* `generate_data`
* `compile_plan`
* `validate_plan`
* `list_generators`
* `list_catalogs`

### 10.2 Function Calling

OpenAI schema:

```json
{
  "name": "generate_data",
  "parameters": {
    "type": "object",
    "properties": {
      "plan": {"type": "object"},
      "sample": {"type": "integer"}
    }
  }
}
```

### 10.3 Agent Workflow

1. LLM interprets user prompt → draft plan
2. SDG validates plan
3. Agent previews
4. User edits spec
5. Agent regenerates final dataset

---

## 11. Go Runtime Implementation

### 11.1 Project Layout

```
/cmd/sdg
/internal/plan
/internal/runtime
/internal/registry
/internal/catalog
/internal/writer
/internal/http
/internal/mcp
/pkg/sdg (public SDK)

```

# Synthetic Data Generator – Spec + Code Templates

This document complements the existing design/spec canvas and adds the Go code templates and GraphQL schema.

## Go Code Templates by Subsystem

### 1. cmd/sdg/main.go

```go
package main

import (
 "context"
 "log"
 "net/http"
 "os"
 "os/signal"
 "syscall"
 "time"

 httpapi "sdg/internal/http"
)

func main() {
 addr := getenv("SDG_HTTP_ADDR", ":8080")

 router := httpapi.NewRouter()

 srv := &http.Server{
  Addr:         addr,
  Handler:      router,
  ReadTimeout:  30 * time.Second,
  WriteTimeout: 30 * time.Second,
 }

 go func() {
  log.Printf("sdg: listening on %s", addr)
  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
   log.Fatalf("sdg: server error: %v", err)
  }
 }()

 // graceful shutdown
 sigCh := make(chan os.Signal, 1)
 signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
 <-sigCh

 ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
 defer cancel()

 log.Printf("sdg: shutting down...")
 if err := srv.Shutdown(ctx); err != nil {
  log.Printf("sdg: shutdown error: %v", err)
 }
}

func getenv(key, def string) string {
 if v := os.Getenv(key); v != "" {
  return v
 }
 return def
}
```

### 2. internal/plan/plan.go

```go
package plan

// Plan represents a full generation request.
type Plan struct {
 Seed     int64        `json:"seed"`
 Entities []EntitySpec `json:"entities"`
 Output   OutputSpec   `json:"output"`
}

type EntitySpec struct {
 Name    string       `json:"name"`
 Count   int64        `json:"count"`
 Fields  []FieldSpec  `json:"fields"`
 Indexes [][]string   `json:"indexes,omitempty"`
 Rels    []Relation   `json:"rels,omitempty"`
}

type FieldSpec struct {
 Name      string                 `json:"name"`
 Kind      string                 `json:"kind"`
 Gen       string                 `json:"gen"`
 Config    map[string]any         `json:"config,omitempty"`
 Unique    bool                   `json:"unique,omitempty"`
 Nullable  float64                `json:"nullable,omitempty"`
 DependsOn []string               `json:"dependsOn,omitempty"`
 Meta      map[string]interface{} `json:"meta,omitempty"`
}

type Relation struct {
 Field       string             `json:"field"`
 Target      string             `json:"target"`
 Cardinality string             `json:"cardinality"`
 Dist        string             `json:"dist"`
 Weights     map[string]float64 `json:"weights,omitempty"`
}

type OutputSpec struct {
 Format string             `json:"format"`
 File   string             `json:"file,omitempty"`
 Sample int                `json:"sample,omitempty"`
 Splits map[string]float64 `json:"splits,omitempty"`
}
```

### 3. internal/plan/validate.go

```go
package plan

import (
 "errors"
 "fmt"
)

func Validate(p *Plan) error {
 if len(p.Entities) == 0 {
  return errors.New("plan: no entities defined")
 }
 if p.Output.Format == "" {
  p.Output.Format = "JSONL"
 }

 entityNames := make(map[string]struct{}, len(p.Entities))
 for i := range p.Entities {
  e := &p.Entities[i]
  if e.Name == "" {
   return fmt.Errorf("plan: entity[%d]: missing name", i)
  }
  if e.Count <= 0 {
   return fmt.Errorf("plan: entity[%s]: count must be > 0", e.Name)
  }
  if _, dup := entityNames[e.Name]; dup {
   return fmt.Errorf("plan: duplicate entity name: %s", e.Name)
  }
  entityNames[e.Name] = struct{}{}
  if err := validateEntity(e); err != nil {
   return err
  }
 }

 for _, e := range p.Entities {
  for _, r := range e.Rels {
   if _, ok := entityNames[r.Target]; !ok {
    return fmt.Errorf("plan: entity[%s].rels: target %q not found", e.Name, r.Target)
   }
  }
 }

 return nil
}

func validateEntity(e *EntitySpec) error {
 if len(e.Fields) == 0 {
  return fmt.Errorf("plan: entity[%s]: no fields defined", e.Name)
 }
 fieldNames := make(map[string]struct{}, len(e.Fields))
 for i := range e.Fields {
  f := &e.Fields[i]
  if f.Name == "" {
   return fmt.Errorf("plan: entity[%s].fields[%d]: missing name", e.Name, i)
  }
  if f.Gen == "" {
   return fmt.Errorf("plan: entity[%s].fields[%s]: missing gen", e.Name, f.Name)
  }
  if _, dup := fieldNames[f.Name]; dup {
   return fmt.Errorf("plan: entity[%s]: duplicate field %q", e.Name, f.Name)
  }
  fieldNames[f.Name] = struct{}{}
 }
 return nil
}
```

### 4. internal/registry/registry.go

```go
package registry

import (
 "fmt"
 "sync"

 "sdg/internal/plan"
 "sdg/internal/rng"
)

type Context struct {
 Entity   string
 Field    string
 RowIndex int64
 State    map[string]any
 Rng      *rng.Rng
}

type Generator interface {
 Next(*Context) (any, error)
}

type Factory func(spec plan.FieldSpec) (Generator, error)

var (
 mu       sync.RWMutex
 registry = make(map[string]Factory)
)

func Register(name string, f Factory) {
 mu.Lock()
 defer mu.Unlock()
 if _, ok := registry[name]; ok {
  panic("registry: duplicate generator: " + name)
 }
 registry[name] = f
}

func Get(name string) (Factory, error) {
 mu.RLock()
 defer mu.RUnlock()
 f, ok := registry[name]
 if !ok {
  return nil, fmt.Errorf("registry: generator %q not found", name)
 }
 return f, nil
}

func List() []string {
 mu.RLock()
 defer mu.RUnlock()
 out := make([]string, 0, len(registry))
 for k := range registry {
  out = append(out, k)
 }
 return out
}
```

### 5. internal/registry/builtins.go

```go
package registry

import (
 "fmt"
 "math"
 "strings"

 "sdg/internal/plan"
)

type GenFunc func(*Context) (any, error)

func (f GenFunc) Next(ctx *Context) (any, error) { return f(ctx) }

func RegisterBuiltins() {
 Register("uniform_int", uniformIntFactory)
 Register("bool", boolFactory)
 Register("template", templateFactory)
 Register("pick", pickFactory)
}

func uniformIntFactory(spec plan.FieldSpec) (Generator, error) {
 cfg := spec.Config
 min, ok := cfg["min"].(float64)
 if !ok {
  return nil, fmt.Errorf("uniform_int: config.min required")
 }
 max, ok := cfg["max"].(float64)
 if !ok {
  return nil, fmt.Errorf("uniform_int: config.max required")
 }
 if max < min {
  return nil, fmt.Errorf("uniform_int: max < min")
 }
 return GenFunc(func(c *Context) (any, error) {
  n := c.Rng.Int63n(int64(max-min+1)) + int64(min)
  return n, nil
 }), nil
}

func boolFactory(spec plan.FieldSpec) (Generator, error) {
 p := 0.5
 if v, ok := spec.Config["p"].(float64); ok {
  p = v
 }
 if p <= 0 || p >= 1 {
  return nil, fmt.Errorf("bool: p must be in (0,1)")
 }
 return GenFunc(func(c *Context) (any, error) {
  return c.Rng.Float64() < p, nil
 }), nil
}

func templateFactory(spec plan.FieldSpec) (Generator, error) {
 raw, ok := spec.Config["tpl"].(string)
 if !ok {
  return nil, fmt.Errorf("template: config.tpl required")
 }
 lower := false
 if v, ok := spec.Config["lower"].(bool); ok {
  lower = v
 }
 return GenFunc(func(c *Context) (any, error) {
  out := raw
  for k, v := range c.State {
   placeholder := "{{" + k + "}}"
   out = strings.ReplaceAll(out, placeholder, fmt.Sprint(v))
  }
  if lower {
   out = strings.ToLower(out)
  }
  return out, nil
 }), nil
}

func pickFactory(spec plan.FieldSpec) (Generator, error) {
 values, ok := spec.Config["values"].([]any)
 if !ok || len(values) == 0 {
  return nil, fmt.Errorf("pick: config.values must be non-empty array")
 }
 return GenFunc(func(c *Context) (any, error) {
  i := c.Rng.Int63n(int64(len(values)))
  return values[i], nil
 }), nil
}

func clamp(v, lo, hi int) int {
 return int(math.Max(float64(lo), math.Min(float64(hi), float64(v))))
}
```

### 6. internal/catalog/catalog.go

```go
package catalog

import (
 "bufio"
 "fmt"
 "math/rand"
 "os"
 "sync"
)

type Item struct {
 Value  any
 Weight float64
}

type Catalog struct {
 Name  string
 Items []Item
 prob  []float64
 alias []int
}

var (
 mu       sync.RWMutex
 catalogs = make(map[string]*Catalog)
)

func LoadFromFile(name, path string) (*Catalog, error) {
 f, err := os.Open(path)
 if err != nil {
  return nil, fmt.Errorf("catalog: open %s: %w", path, err)
 }
 defer f.Close()

 var items []Item
 sc := bufio.NewScanner(f)
 for sc.Scan() {
  line := sc.Text()
  if line == "" {
   continue
  }
  items = append(items, Item{Value: line, Weight: 1})
 }
 if err := sc.Err(); err != nil {
  return nil, fmt.Errorf("catalog: read %s: %w", path, err)
 }
 if len(items) == 0 {
  return nil, fmt.Errorf("catalog: %s is empty", path)
 }
 c := &Catalog{Name: name, Items: items}
 c.buildAlias()
 return c, nil
}

func (c *Catalog) buildAlias() {
 n := len(c.Items)
 prob := make([]float64, n)
 alias := make([]int, n)

 sum := 0.0
 for _, it := range c.Items {
  sum += it.Weight
 }
 scaled := make([]float64, n)
 for i, it := range c.Items {
  scaled[i] = float64(n) * (it.Weight / sum)
 }
 var small, large []int
 for i, v := range scaled {
  if v < 1.0 {
   small = append(small, i)
  } else {
   large = append(large, i)
  }
 }
 for len(small) > 0 && len(large) > 0 {
  l := small[len(small)-1]
  small = small[:len(small)-1]
  g := large[len(large)-1]
  large = large[:len(large)-1]

  prob[l] = scaled[l]
  alias[l] = g

  scaled[g] = (scaled[g] + scaled[l]) - 1
  if scaled[g] < 1 {
   small = append(small, g)
  } else {
   large = append(large, g)
  }
 }
 for _, i := range append(small, large...) {
  prob[i] = 1
 }
 c.prob = prob
 c.alias = alias
}

func (c *Catalog) Sample(r *rand.Rand) any {
 n := len(c.Items)
 i := r.Intn(n)
 if r.Float64() < c.prob[i] {
  return c.Items[i].Value
 }
 return c.Items[c.alias[i]].Value
}

func Get(name string) (*Catalog, bool) {
 mu.RLock()
 defer mu.RUnlock()
 c, ok := catalogs[name]
 return c, ok
}

func Register(c *Catalog) {
 mu.Lock()
 defer mu.Unlock()
 catalogs[c.Name] = c
}
```

### 7. internal/rng/rng.go

```go
package rng

import (
 "encoding/binary"
 "hash/fnv"
 "math/rand"
)

type Rng struct {
 r *rand.Rand
}

func New(seed int64) *Rng {
 return &Rng{r: rand.New(rand.NewSource(seed))}
}

func (r *Rng) Int63n(n int64) int64 { return r.r.Int63n(n) }
func (r *Rng) Float64() float64    { return r.r.Float64() }
func (r *Rng) Uint32() uint32      { return r.r.Uint32() }
func (r *Rng) Intn(n int) int      { return r.r.Intn(n) }

func Derive(parent int64, label string) int64 {
 h := fnv.New64a()
 var buf [8]byte
 binary.LittleEndian.PutUint64(buf[:], uint64(parent))
 h.Write(buf[:])
 h.Write([]byte(label))
 return int64(h.Sum64())
}
```

### 9. internal/runtime/chunk.go

```go
package runtime

type chunk struct {
 Start int64
 End   int64
}

func makeChunks(total int64, size int64) []chunk {
 if size <= 0 {
  size = 50000
 }
 var chunks []chunk
 for start := int64(0); start < total; start += size {
  end := start + size
  if end > total {
   end = total
  }
  chunks = append(chunks, chunk{Start: start, End: end})
 }
 return chunks
}
```

### 10. internal/runtime/executor.go

```go
package runtime

import (
 "context"
 "fmt"
 "runtime"
 "sync"

 "sdg/internal/plan"
 "sdg/internal/registry"
 "sdg/internal/rng"
 "sdg/internal/writer"
)

type Executor struct {
 rootSeed int64
 w        writer.Writer
}

func NewExecutor(seed int64, w writer.Writer) *Executor {
 return &Executor{rootSeed: seed, w: w}
}

func (e *Executor) Run(ctx context.Context, p *plan.Plan) error {
 for _, ent := range p.Entities {
  if err := e.runEntity(ctx, &ent); err != nil {
   return fmt.Errorf("executor: entity %s: %w", ent.Name, err)
  }
 }
 if err := e.w.Close(); err != nil {
  return fmt.Errorf("executor: writer close: %w", err)
 }
 return nil
}

func (e *Executor) runEntity(ctx context.Context, ent *plan.EntitySpec) error {
 seed := rng.Derive(e.rootSeed, "entity:"+ent.Name)
 entityRng := rng.New(seed)

 gens := make([]registry.Generator, len(ent.Fields))
 for i, f := range ent.Fields {
  factory, err := registry.Get(f.Gen)
  if err != nil {
   return fmt.Errorf("bind field %s: %w", f.Name, err)
  }
  gen, err := factory(f)
  if err != nil {
   return fmt.Errorf("build generator for %s: %w", f.Name, err)
  }
  gens[i] = gen
 }

 numCPU := runtime.NumCPU()
 workers := numCPU * 2
 if workers < 1 {
  workers = 1
 }

 chunks := makeChunks(ent.Count, 50000)
 chCh := make(chan chunk, len(chunks))
 errCh := make(chan error, workers)
 var wg sync.WaitGroup

 for _, c := range chunks {
  chCh <- c
 }
 close(chCh)

 for i := 0; i < workers; i++ {
  wg.Add(1)
  go func(workerID int) {
   defer wg.Done()
   for c := range chCh {
    if err := e.runChunk(ctx, ent, entityRng, gens, c); err != nil {
     select {
     case errCh <- err:
     default:
     }
     return
    }
   }
  }(i)
 }

 wg.Wait()
 select {
 case err := <-errCh:
  return err
 default:
  return nil
 }
}

func (e *Executor) runChunk(
 ctx context.Context,
 ent *plan.EntitySpec,
 entityRng *rng.Rng,
 gens []registry.Generator,
 ch chunk,
) error {
 for row := ch.Start; row < ch.End; row++ {
  select {
  case <-ctx.Done():
   return ctx.Err()
  default:
  }
  rowState := make(map[string]any, len(ent.Fields))
  record := make(map[string]any, len(ent.Fields))

  for i, f := range ent.Fields {
   fieldSeed := rng.Derive(int64(entityRng.Uint32()), "field:"+f.Name)
   r := rng.New(rng.Derive(fieldSeed, fmt.Sprintf("row:%d", row)))
   ctxGen := &registry.Context{
    Entity:   ent.Name,
    Field:    f.Name,
    RowIndex: row,
    State:    rowState,
    Rng:      r,
   }
   val, err := gens[i].Next(ctxGen)
   if err != nil {
    return fmt.Errorf("row %d field %s: %w", row, f.Name, err)
   }
   rowState[f.Name] = val
   record[f.Name] = val
  }

  if err := e.w.WriteRecord(ent.Name, record); err != nil {
   return fmt.Errorf("writer: %w", err)
  }
 }
 return nil
}
```

### 11. internal/writer/writer.go

```go
package writer

type Writer interface {
 WriteRecord(entity string, record map[string]any) error
 Close() error
}
```

### 12. internal/writer/jsonl.go

```go
package writer

import (
 "bufio"
 "encoding/json"
 "fmt"
 "io"
 "os"
)

type JSONLWriter struct {
 out     io.WriteCloser
 buf     *bufio.Writer
 closeFn func() error
}

func NewJSONLFileWriter(path string) (*JSONLWriter, error) {
 f, err := os.Create(path)
 if err != nil {
  return nil, fmt.Errorf("jsonl: create %s: %w", path, err)
 }
 w := bufio.NewWriter(f)
 return &JSONLWriter{
  out: f,
  buf: w,
  closeFn: func() error {
   if err := w.Flush(); err != nil {
    return err
   }
   return f.Close()
  },
 }, nil
}

func (w *JSONLWriter) WriteRecord(entity string, record map[string]any) error {
 record["_entity"] = entity
 b, err := json.Marshal(record)
 if err != nil {
  return fmt.Errorf("jsonl: marshal: %w", err)
 }
 if _, err := w.buf.Write(b); err != nil {
  return fmt.Errorf("jsonl: write: %w", err)
 }
 if err := w.buf.WriteByte('\n'); err != nil {
  return fmt.Errorf("jsonl: newline: %w", err)
 }
 return nil
}

func (w *JSONLWriter) Close() error {
 if w.closeFn != nil {
  return w.closeFn()
 }
 return nil
}
```

### 13. internal/writer/csv.go

```go
package writer

import (
 "encoding/csv"
 "fmt"
 "os"
 "sort"
)

type CSVWriter struct {
 file  *os.File
 w     *csv.Writer
 keys  []string
 initd bool
}

func NewCSVFileWriter(path string) (*CSVWriter, error) {
 f, err := os.Create(path)
 if err != nil {
  return nil, fmt.Errorf("csv: create %s: %w", path, err)
 }
 return &CSVWriter{file: f, w: csv.NewWriter(f)}, nil
}

func (w *CSVWriter) WriteRecord(entity string, record map[string]any) error {
 if !w.initd {
  keys := make([]string, 0, len(record))
  for k := range record {
   if k == "_entity" {
    continue
   }
   keys = append(keys, k)
  }
  sort.Strings(keys)
  w.keys = keys
  if err := w.w.Write(keys); err != nil {
   return fmt.Errorf("csv: header: %w", err)
  }
  w.initd = true
 }

 row := make([]string, len(w.keys))
 for i, k := range w.keys {
  v := record[k]
  if v == nil {
   row[i] = ""
  } else {
   row[i] = fmt.Sprint(v)
  }
 }
 if err := w.w.Write(row); err != nil {
  return fmt.Errorf("csv: write: %w", err)
 }
 return nil
}

func (w *CSVWriter) Close() error {
 w.w.Flush()
 if err := w.w.Error(); err != nil {
  return fmt.Errorf("csv: flush: %w", err)
 }
 return w.file.Close()
}
```

### 14. internal/http/server.go

```go
package httpapi

import (
 "net/http"

 "github.com/99designs/gqlgen/graphql/handler"
 "github.com/99designs/gqlgen/graphql/playground"

 "sdg/internal/http/generated"
)

func NewRouter() http.Handler {
 mux := http.NewServeMux()

 srv := handler.NewDefaultServer(generated.NewExecutableSchema(
  generated.Config{Resolvers: &Resolver{}},
 ))

 mux.Handle("/query", srv)
 mux.Handle("/", playground.Handler("GraphQL playground", "/query"))

 mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
  w.WriteHeader(http.StatusOK)
  _, _ = w.Write([]byte("ok"))
 })

 return mux
}
```

### 15. internal/http/resolvers.go (skeleton)

```go
package httpapi

import (
 "context"
 "fmt"

 "sdg/internal/plan"
 "sdg/internal/registry"
 "sdg/internal/runtime"
 "sdg/internal/writer"
 "sdg/internal/http/generated"
)

type Resolver struct{}

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }

func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }
func (r *Resolver) Query() generated.QueryResolver       { return &queryResolver{r} }

func (r *mutationResolver) Generate(ctx context.Context, input generated.PlanInput) (*generated.GenerateResult, error) {
 p := convertPlanInput(input)

 if err := plan.Validate(&p); err != nil {
  return nil, fmt.Errorf("validate plan: %w", err)
 }

 w, err := writer.NewJSONLFileWriter("out.jsonl")
 if err != nil {
  return nil, fmt.Errorf("writer: %w", err)
 }

 registry.RegisterBuiltins()

 ex := runtime.NewExecutor(p.Seed, w)
 if err := ex.Run(ctx, &p); err != nil {
  return nil, err
 }

 return &generated.GenerateResult{
  Ok:    true,
  Files: []string{"out.jsonl"},
 }, nil
}

func (r *queryResolver) Generators(ctx context.Context) ([]string, error) {
 return registry.List(), nil
}

func convertPlanInput(in generated.PlanInput) plan.Plan {
 p := plan.Plan{Seed: int64(in.Seed)}
 for _, e := range in.Entities {
  var ent plan.EntitySpec
  ent.Name = e.Name
  ent.Count = int64(e.Count)
  for _, f := range e.Fields {
   ent.Fields = append(ent.Fields, plan.FieldSpec{
    Name:      f.Name,
    Kind:      string(f.Kind),
    Gen:       f.Gen,
    Config:    f.Config,
    Unique:    boolFromPtr(f.Unique),
    Nullable:  floatFromPtr(f.Nullable),
    DependsOn: f.DependsOn,
   })
  }
  p.Entities = append(p.Entities, ent)
 }
 if in.Output != nil {
  p.Output = plan.OutputSpec{
   Format: string(in.Output.Format),
   File:   in.Output.File,
   Sample: int(in.Output.Sample),
  }
 }
 return p
}

func boolFromPtr(v *bool) bool {
 if v == nil {
  return false
 }
 return *v
}

func floatFromPtr(v *float64) float64 {
 if v == nil {
  return 0
 }
 return *v
}
```

### 16. pkg/sdg/sdg.go

```go
package sdg

import (
 "context"

 "sdg/internal/plan"
 "sdg/internal/registry"
 "sdg/internal/runtime"
 "sdg/internal/writer"
)

type GenerateOptions struct {
 Writer writer.Writer
}

func Generate(ctx context.Context, p *plan.Plan, opts GenerateOptions) error {
 if err := plan.Validate(p); err != nil {
  return err
 }
 registry.RegisterBuiltins()
 w := opts.Writer
 ex := runtime.NewExecutor(p.Seed, w)
 return ex.Run(ctx, p)
}
```

---

## GraphQL Schema (graph/schema.graphqls)

```graphql
scalar JSON
scalar BigInt

enum OutputFormat {
  JSONL
  CSV
  PARQUET
  SQL_DUMP
  SFTJSONL
  DPO_PAIRS
  CHAT
  TOOL_TRACE
  RAG_TRIPLE
}

enum Cardinality {
  MANY_TO_ONE
  ONE_TO_MANY
  MANY_TO_MANY
}

enum Dist {
  UNIFORM
  WEIGHTED
}

enum FieldKind {
  STRING
  INT
  FLOAT
  BOOL
  TIME
  UUID
  JSONB
}

input FieldInput {
  name: String!
  kind: FieldKind!
  gen: String!
  config: JSON
  unique: Boolean
  nullable: Float
  dependsOn: [String!]
}

input RelationInput {
  field: String!
  target: String!
  cardinality: Cardinality!
  dist: Dist = UNIFORM
  weights: JSON
}

input EntityInput {
  name: String!
  count: BigInt!
  fields: [FieldInput!]!
  indexes: [[String!]]
  rels: [RelationInput!]
}

input OutputInput {
  format: OutputFormat = JSONL
  file: String
  sample: Int
  splits: JSON
}

input PlanInput {
  seed: BigInt!
  entities: [EntityInput!]!
  output: OutputInput
}

type Row {
  json: JSON!
}

type GenerateResult {
  ok: Boolean!
  sample: [[Row!]!]
  files: [String!]!
  stats: JSON
}

type DryRunResult {
  ok: Boolean!
  warnings: [String!]!
  estimates: JSON
}

type Query {
  generators: [String!]!
  catalogs: [String!]!
}

type Mutation {
  generate(plan: PlanInput!): GenerateResult!
  dryRun(plan: PlanInput!): DryRunResult!
}
```

---

## 12. MCP + Tooling Integration

### 12.1 MCP Resources

The SDG exposes MCP resources so assistants can mount the generator directly into their context window:

| Resource | URI | Description |
|----------|-----|-------------|
| `sdg://catalogs/*` | catalogs list/detail | Enumerates catalog metadata, sample rows, and provenance. |
| `sdg://plans/*` | plan templates | Ready-made plan snippets (retail, health, chat, RAG, etc.). |
| `sdg://outputs/*` | latest outputs | Signed URLs or file handles for generated datasets. |

Resources are backed by the same storage layer as writers. Every resource response includes `version`, `seed`, and `plan_hash` so LLM clients can ensure determinism.

### 12.2 MCP Tools

Besides the GraphQL API, MCP exposes structured tools aligned with the agent workflow:

* `sdg.generate_data(plan: JSON, format?: string, sample?: int)` — synchronous generation with optional streaming chunk responses.
* `sdg.compile_plan(prompt: string, context?: JSON)` — runs the NL plan compiler and returns both the plan and a confidence/explanation block.
* `sdg.validate_plan(plan: JSON)` — lightweight validation for speculative edits.
* `sdg.list_generators()` / `sdg.list_catalogs()` — mirrors GraphQL but optimized for small context windows.

Tools stream incremental status (`queued → compiling → running → writing → complete`). Errors always include `category` (validation/runtime/writer/network) for better agent recovery.

### 12.3 Session + Auth Model

* **API Tokens**: short-lived tokens with scopes (`generate`, `catalog:write`, `admin`).
* **MCP Sessions**: each agent session negotiates a capability set; generation runs in isolated sandboxes with configurable CPU/RAM limits.
* **Rate Limits**: per-token concurrency plus global backpressure; metadata returned in `Retry-After`.

### 12.4 Agent Patterns

1. Agent receives human prompt → calls `compile_plan`.
2. Agent presents plan summary to user, possibly editing via natural language.
3. Agent calls `validate_plan` until clean.
4. Agent triggers `generate_data` with `sample` for preview rows.
5. Once approved, agent requests full generation and fetches artifacts via MCP resource handles.

All responses are deliberately compact so they fit inside typical LLM context budgets.

---

## 13. Natural-Language Plan Compiler Implementation

### 13.1 Pipeline

1. **Intent Classification**: Distinguish dataset families (transactions, chat, telemetry, medical, etc.). Implemented via small fine-tuned model or heuristic classifier.
2. **Schema Retrieval**: For each intent, load schema templates from `/internal/compiler/templates`. Templates include entity graphs, field hints, and generator suggestions.
3. **Slot Filling**: Run an LLM (GPT-4o or local) with a structured prompt that fills JSON slots: entity counts, field descriptions, relations, catalogs, required distributions.
4. **Generator Selection**: Translate slot metadata into concrete generator specs using declarative rules (e.g., numeric + range → `uniform_int`; categorical + list → `pick`).
5. **Constraint Inference**: Detect uniqueness, foreign keys, and cardinality from natural language (“each user has many sessions”).
6. **Validation + Repair**: Call `plan.Validate`. For validation errors, emit `fixit` suggestions and optionally loop through an LLM patch step until clean or max retries reached.

### 13.2 Prompt Hints

* Provide generator catalog tables in the system prompt so the model references valid names.
* Include examples of high-quality plans plus commentary to bias toward deterministic configs.
* Use function calling to force the LLM to emit JSON; schema is defined in `compiler/prompts/plan.json`.

### 13.3 Confidence & Explanations

The compiler returns `{plan, reasoning, confidence}`:

* `reasoning` is a markdown bullet list of assumptions (e.g., “Assumed USD currency; adjust `currency` field if different”).
* `confidence` is derived from validation passes and LLM self-evaluation.

### 13.4 Offline Training Loop

* Log user edits vs. initial plan; feed pairs back into fine-tune corpus.
* Add automatic regression tests: run compiler on canonical prompts and diff generated plans (stored under `testdata/compiler/*.golden.json`).

---

## 14. Telemetry, Observability, and Governance

### 14.1 Metrics

Adopt OpenTelemetry:

* `sdg_generation_duration_seconds{entity,format}`
* `sdg_rows_generated_total{entity}`
* `sdg_generator_error_total{name,category}`
* `sdg_plan_validation_fail_total{reason}`

Metrics exported via Prometheus endpoint (`/metrics`) and aggregated per tenant.

### 14.2 Tracing

Wrap major phases (plan validation, catalog load, chunk execution, writer flush). Each span carries `seed` and `plan_hash`. This is crucial when investigating mismatched deterministic outputs.

### 14.3 Logging

Structured logs (JSON) with fields: `level`, `ts`, `component`, `seed`, `plan_hash`, `entity`, `chunk`. Sensitive catalog contents are redacted via hash digests.

### 14.4 Auditing

Keep an append-only audit trail for plan submissions and catalog mutations. Entries reference the authenticated user/agent, time, and diff summary. This allows enterprises to prove dataset lineage.

---

## 15. Deployment & DevOps

### 15.1 Binary + Container

* Build static Go binary (`GOOS=linux GOARCH=amd64`) for `/cmd/sdg`.
* Container image includes GraphQL server, MCP adapter, and CLI.
* Use environment variables for config (`SDG_HTTP_ADDR`, `SDG_STORAGE_PATH`, `SDG_MAX_WORKERS`, `SDG_MCP_ENABLED`).

### 15.2 Storage

* **Local Mode**: write outputs to configurable directory; serve via MCP resource URIs.
* **Cloud Mode**: plug a storage provider interface (S3/GCS/Azure). Writers stream directly to object storage using multipart uploads; URIs returned to clients.

### 15.3 Scaling

* Stateless API pods fronted by a load balancer.
* Long-running generations are delegated to a worker queue (e.g., NATS, Redis, or Postgres LISTEN/NOTIFY) so GraphQL requests stay responsive. Workers pull jobs, run the executor, and post status updates.
* Horizontal Pod Autoscaler keyed to CPU + queue depth.

### 15.4 Configuration Bundles

Provide Helm chart, Terraform modules, and Docker Compose templates under `/deploy/`. Standard observability sidecars (Prometheus, Loki) are included.

---

## 16. Testing & Quality Assurance

### 16.1 Determinism Tests

* Property tests that run the same plan twice with identical seeds; assert byte-for-byte equality of outputs.
* Concurrency stress tests that randomize worker counts/chunk sizes while comparing SHA-256 digests.

### 16.2 Generator Unit Tests

* Each primitive generator lives under `internal/registry/<name>_test.go`.
* Use statistical tests (chi-square, KS) for distributional correctness when applicable.

### 16.3 Integration Tests

* GraphQL golden tests (`graph/tests/*.graphql`) verifying queries, mutations, and error shapes.
* MCP end-to-end tests using a mock agent that compiles a plan, validates, and generates sample rows.

### 16.4 Catalog Tests

* Validate alias table build by comparing sampled frequencies against weights.
* Fuzz tests on catalog loaders to ensure malformed files/URLs are rejected gracefully.

### 16.5 Plan Compiler Regression

* Keep curated prompts and desired plan outputs in `testdata/compiler`.
* Run nightly pipeline that regenerates plans and diffs them; review deviations for drift.

---

## 17. Security, Privacy, and Compliance

* **Sandboxing**: Generation workers run in cgroups with restricted filesystem/network access; catalog uploads validated against allowlists to prevent SSRF.
* **Secrets Management**: External catalog fetchers use scoped credentials stored in Vault or AWS Secrets Manager.
* **Data Classification**: Catalog metadata tracks origin (`public`, `proprietary`, `llm-synthesized`). Policies enforce which tenants may access which catalogs.
* **PII Guardrails**: Because SDG can mimic real-world data, add automated scanners that flag catalog values matching real PII (hash-based detection, dictionary checks).
* **Compliance Hooks**: Audit logs integrate with SOC2 / ISO27001 evidence collection; provide signed plan/output manifest for reproducibility.

---

## 18. Performance & Scaling Guidance

* **Chunk Size Tuning**: default 50k rows; adapt dynamically based on field complexity (e.g., heavy expressions -> smaller chunks).
* **RNG Choice**: implement SplitMix64/PCG with per-chunk derivation to avoid contention and guarantee determinism regardless of worker scheduling.
* **Memory Layout**: Reuse buffers in writers, preallocate `record` maps using `make(map[string]any, len(fields))`.
* **Catalog Caching**: Keep hot catalogs in memory with ARC/LRU, but cap memory by evicting least-recently-used ones; cold catalogs stream from storage.
* **Benchmark Suite**: `benchmarks/*` includes Go benchmarks generating millions of rows for canonical schemas. Record baseline throughput (rows/sec) per schema and hardware tier.

---

## 19. Roadmap

1. **Phase 1 – Core MVP**
   * Complete RNG rewrite, full primitive set, CSV/JSONL writers, GraphQL + MCP parity.
2. **Phase 2 – Advanced Outputs**
   * Add SFT/chat/tool-trace writers, train/val/test splits, cloud storage connectors.
3. **Phase 3 – Intelligent Compiler**
   * Fine-tune plan compiler, add active learning loop, expose user-facing plan diff UI.
4. **Phase 4 – Ecosystem**
   * Publish SDKs (Go, TypeScript, Python), integrate with LangChain/LlamaIndex, release catalog marketplace.
5. **Phase 5 – Governance**
   * Add dataset watermarking, lineage explorer UI, enterprise RBAC.

Each phase ships with migration guides and deterministic regression suites to ensure existing workflows remain reproducible.

---

## Appendix A. Agent-First Spec Draft (Merged)

This section merges the previous agent-first spec into the main SDG spec.

### A.1 Product Vision

Apery is an agent-first synthetic data generator. It accepts explicit schema/plan specs and produces deterministic outputs. Agents (Codex, Claude, etc.) can call it via MCP or GraphQL to generate any data described by a plan.

**Core Promise:** Plan + Seed + Version = Identical Output

### A.2 Core Interface

#### A.2.1 Plan-First Contract

Agents provide a structured plan (no natural-language compilation in-core). A plan describes:

- Seed
- Entities (name, count)
- Fields (name, generator, config)
- Output options (format, splits)

#### A.2.2 GraphQL API

Minimum viable schema:

```graphql
type Query {
  generators: [String!]!
  catalogs: [String!]!
  validatePlan(plan: JSON!): ValidationResult!
}

type Mutation {
  runPlan(plan: JSON!, output: OutputSpec!): RunResult!
}

type OutputSpec {
  format: OutputFormat!
  splits: [SplitSpec!]
}

enum OutputFormat { JSONL CSV }

type SplitSpec {
  name: String!
  ratio: Float!
}

type RunResult {
  jobId: String!
  outputs: [OutputLocation!]!
}

type OutputLocation {
  name: String!
  uri: String!
}

type ValidationResult {
  ok: Boolean!
  errors: [String!]!
}
```

#### A.2.3 MCP Server

Minimum viable tools:

- `list_generators()`
- `list_catalogs()`
- `validate_plan(plan_json)`
- `run_plan(plan_json, output_spec)`

Responses return deterministic output locations or streamed data.

### A.3 Determinism and Execution

- Hierarchical seed derivation (root → entity → field → row).
- Chunk-based execution for scalable deterministic generation.
- Deterministic concurrency with per-chunk RNG derivation.
- Regression tests compare output digests across versions.

### A.4 Generator Surface

#### A.4.1 Scalars

- uniform_int(min,max)
- uniform_float(min,max)
- normal_float(mu,sigma)
- normal_int(mu,sigma,clamp)
- zipf(s,vmax)
- bool(p)
- regex(pattern) (supported subset; see Regex Generator Subset)
- time(start,end,tz)
- uuid_v4()
- ulid()
- seq(start,step)
- pick(values|file|url)
- pick(values|file|url, weights)
- const(value)

#### A.4.2 Composite

- object(fields)
- list(len|min_len+max_len, item)
- sample(values|file|url, n|min_n+max_n)
- one_of(gens,weights)
- switch(key,cases)
- template(tpl)

#### A.4.3 Relational

- rel_ref(target,field)
- m2m(target,meanDegree)

### A.5 Catalog Subsystem

- Bundled catalogs (names, companies, domains, cities, products, words).
- User catalogs via local file.
- Remote catalogs via allowlisted URL.
- Weighted alias tables, cached with ARC/LRU.

### A.6 Writers and Output Modes

- JSONL (streaming).
- CSV.
- Split modes: train/val/test.
- Optional cloud storage outputs (S3/GCS/Azure).

### A.7 Agent UX

Agents should:

1) Fetch available generators and catalogs.
2) Construct a plan from a provided schema/spec.
3) Validate the plan.
4) Run the plan and read streaming output or output URIs.

### A.8 Roadmap (Agent-First)

#### Phase 1 - Core MVP

- Deterministic execution engine
- Full primitive generator set
- JSONL + CSV writers
- GraphQL + MCP parity

#### Phase 2 - Advanced Outputs

- SFT/chat/tool-trace writers
- DPO/RLHF preference pairs
- Train/val/test splits + cloud connectors

#### Phase 3 - Ecosystem

- SDKs (Go, TypeScript, Python)
- LangChain/LlamaIndex integration
- Catalog marketplace

#### Phase 4 - Governance

- Dataset watermarking
- Lineage explorer UI
- Enterprise RBAC
