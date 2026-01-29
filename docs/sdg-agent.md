# Agent-First Spec Draft

This draft reframes Apery as an agent extension: a deterministic data generator exposed via GraphQL and MCP, driven by explicit schema/plan inputs rather than natural-language compilation.

## 1. Product Vision

Apery is an agent-first synthetic data generator. It accepts explicit schema/plan specs and produces deterministic outputs. Agents (Codex, Claude, etc.) can call it via MCP or GraphQL to generate any data described by a plan.

**Core Promise:** Plan + Seed + Version = Identical Output

## 2. Core Interface

### 2.1 Plan-First Contract

Agents provide a structured plan (no natural-language compilation in-core). A plan describes:

- Seed
- Entities (name, count)
- Fields (name, generator, config)
- Output options (format, splits)

### 2.2 GraphQL API

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

### 2.3 MCP Server

Minimum viable tools:

- `list_generators()`
- `list_catalogs()`
- `validate_plan(plan_json)`
- `run_plan(plan_json, output_spec)`

Responses return deterministic output locations or streamed data.

## 3. Determinism and Execution

- Hierarchical seed derivation (root → entity → field → row).
- Chunk-based execution for scalable deterministic generation.
- Deterministic concurrency with per-chunk RNG derivation.
- Regression tests compare output digests across versions.

## 4. Generator Surface

### 4.1 Scalars

- uniform_int(min,max)
- uniform_float(min,max)
- normal_float(mu,sigma)
- normal_int(mu,sigma,clamp)
- zipf(s,vmax)
- bool(p)
- regex(pattern)
- time(start,end,tz)
- uuid_v4()
- ulid()
- seq(start,step)
- pick(values|file|url)

### 4.2 Composite

- object(fields)
- list(len,item)
- pipe(g1,g2)
- one_of(gens,weights)
- switch(key,cases)
- when(cond,cases)
- map(items,fn)
- expr(code)

### 4.3 Relational

- rel_ref(target,field)
- m2m(target,meanDegree)

## 5. Catalog Subsystem

- Bundled catalogs (names, companies, domains, cities, products, words).
- User catalogs via local file.
- Remote catalogs via allowlisted URL.
- Weighted alias tables, cached with ARC/LRU.

## 6. Writers and Output Modes

- JSONL (streaming).
- CSV.
- Split modes: train/val/test.
- Optional cloud storage outputs (S3/GCS/Azure).

## 7. Agent UX

Agents should:

1) Fetch available generators and catalogs.
2) Construct a plan from a provided schema/spec.
3) Validate the plan.
4) Run the plan and read streaming output or output URIs.

## 8. Roadmap (Agent-First)

### Phase 1 - Core MVP

- Deterministic execution engine
- Full primitive generator set
- JSONL + CSV writers
- GraphQL + MCP parity

### Phase 2 - Advanced Outputs

- SFT/chat/tool-trace writers
- DPO/RLHF preference pairs
- Train/val/test splits + cloud connectors

### Phase 3 - Ecosystem

- SDKs (Go, TypeScript, Python)
- LangChain/LlamaIndex integration
- Catalog marketplace

### Phase 4 - Governance

- Dataset watermarking
- Lineage explorer UI
- Enterprise RBAC

