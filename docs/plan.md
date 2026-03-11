# Spec Completion Checklist

This checklist captures all work needed to fully implement the `sdg-spec.md` spec.
Items are grouped by spec sections and roadmap phases.

## Phase 1 - Core MVP

- [ ] RNG rewrite and execution engine
  - [x] Replace RNG with PCG (`rand.NewPCG`) and `Seed`-based construction.
  - [x] Seed derivation helpers (`Derive`, `DeriveIndex`) for hierarchical determinism.
  - [x] Per-row RNG instantiation from derived seeds in the executor.
  - [x] RNG implements `io.Reader` for entropy consumers (e.g., ULID).
  - [x] Implement chunk-based parallelism (default chunk size 50k rows).
  - [x] Deterministic row generation independent of worker scheduling.
  - [ ] Concurrency stress tests that randomize worker counts/chunk sizes and compare digests.
  - [ ] Determinism regression suite keyed by (Plan + Seed + Version).
  - [ ] Cross-version RNG compatibility and migration notes.
  - [ ] Statistical sanity checks for RNG-dependent generators.
  - [ ] RNG hot-path benchmarks (seed derivation + instantiation cost).
  - [ ] Seed serialization/format stability guarantees.
  - [ ] Uniqueness enforcement (bounded retries, entropy estimation, Bloom prechecks).
  - [ ] Relation resolution (M:1 alias sampling, 1:M multinomial, M:N degree + dedupe).

- [ ] Full primitive generator set
  - [x] Scalar generators
    - [x] uniform_int(min,max)
    - [x] uniform_float(min,max)
    - [x] normal_float(mu,sigma)
    - [x] normal_int(mu,sigma,clamp)
    - [x] zipf(s,vmax)
    - [x] bool(p)
    - [x] regex(pattern)
      - [x] Enforce supported subset (anchor placement rules, no word boundaries) and document constraints in spec
    - [x] time(start,end,tz)
    - [x] uuid_v4()
    - [x] ulid()
    - [x] seq(start,step)
    - [x] pick(values|file|url)
  - [ ] Composite generators
    - [x] object(fields)
    - [ ] list(len,item)
    - [ ] pipe(g1,g2)
    - [ ] one_of(gens,weights)
    - [ ] switch(key,cases)
    - [ ] when(cond,cases)
    - [ ] map(items,fn)
    - [ ] expr(code)
  - [ ] Relational generators
    - [ ] rel_ref(target,field)
    - [ ] m2m(target,meanDegree)

- [ ] Writers
  - [x] JSONL writer (streaming).
  - [x] CSV writer.
  - [ ] Split modes: train/val/test.

- [ ] GraphQL + MCP parity (minimum viable)
  - [ ] GraphQL API with generator listing and plan execution.
  - [ ] MCP integration with plan execution and catalog access.

- [ ] Determinism regression suite
  - [ ] Golden outputs keyed by (Plan + Seed + Version).
  - [ ] Cross-version compatibility checks and migration notes.

## Phase 2 - Advanced Outputs

- [ ] Writers for AI workflows
  - [ ] SFT/chat/tool-trace writers.
  - [ ] DPO/RLHF preference pairs (chosen/rejected).
  - [ ] Chat message arrays.
  - [ ] Tool-call traces.
  - [ ] RAG evaluation triples.

- [ ] Dataset splits and storage
  - [ ] Train/val/test split logic for all writers.
  - [ ] Cloud storage connectors (S3/GCS/Azure).

## Phase 3 - Intelligent Compiler

- [ ] Natural language plan compiler
  - [ ] Prompt to structured plan.
  - [ ] Schema inference.
  - [ ] Generator selection and configuration.
  - [ ] Active learning loop for improvements.
  - [ ] Plan diff UI (user-facing).

## Phase 4 - Ecosystem

- [ ] SDKs
  - [ ] Go SDK.
  - [ ] TypeScript SDK.
  - [ ] Python SDK.

- [ ] Integrations and marketplace
  - [ ] LangChain integration.
  - [ ] LlamaIndex integration.
  - [ ] Catalog marketplace.

## Phase 5 - Governance

- [ ] Dataset watermarking.
- [ ] Lineage explorer UI.
- [ ] Enterprise RBAC.

## Cross-Cutting Requirements

- [ ] Catalog subsystem
  - [ ] Bundled catalogs (names, companies, domains, cities, products, words).
  - [ ] User catalogs via local file.
  - [ ] Remote catalogs via allowlisted URL.
  - [ ] Virtual (LLM-generated) catalogs.
  - [ ] Weighted alias table conversion and caching (ARC/LRU).

- [ ] Performance and scaling
  - [ ] Buffer reuse in writers.
  - [ ] Preallocation of record maps.
  - [x] Benchmarks generating millions of rows per schema.
