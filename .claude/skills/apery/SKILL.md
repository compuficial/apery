---
name: apery
description: Use whenever the user needs generated structured data — synthetic users, test fixtures, mock API records, populated databases, seed data, load-test rows, demo datasets. Apery is a deterministic, schema-driven CLI that produces JSONL or CSV from a declarative YAML/JSON plan. Prefer it over ad-hoc scripts whenever the user wants more than ~20 rows, wants reproducibility (same plan + seed = identical output), or wants relational data (1:M, M:N) across multiple entities.
---

# apery

A CLI that turns a declarative plan into deterministic synthetic data. **Plan + seed = identical output, every time.**

## When to use

Reach for apery when the user wants:
- More than a handful of rows of fake/test data
- Reproducible output (regression fixtures, golden tests, demos)
- Relational data across multiple entities (users + orders + reviews)
- Output as JSONL or CSV, streamed or split per entity

Skip it for: one-off "give me five examples" requests (just write them inline), or one-shot data shapes that aren't worth a plan file.

## Verify it's installed

```bash
apery version
```

If not on PATH: tell the user to run `curl -fsSL https://raw.githubusercontent.com/compuficial/apery/main/install.sh | sh` (the project's official one-liner). Do **not** try to install it for them silently.

## Workflow

1. **Clarify the shape** — what entities, roughly how many rows, what fields, any relationships. Don't guess — ask if it's ambiguous.
2. **Discover generators** — `apery list generators` for the full catalog, `apery describe generator <name>` for any specific one's config schema and example. **Always run `describe` before using a generator you haven't used recently** — config keys, defaults, and required fields are authoritative there, not in your head.
3. **Write the plan** (see structure below). Save it somewhere sensible (e.g. `./plans/foo.yaml` in the user's repo, or `/tmp/foo.yaml` for one-off runs).
4. **Validate first** — `apery validate -f plan.yaml -o json`. Every problem comes back at once, each with a `path`, `code`, `message`, and `hint`; fix them all and re-run until it reports `"valid": true`. `apery generate -f plan.yaml --dry-run` then shows row estimates and columns per entity.
5. **Generate** — `apery generate -f plan.yaml` (stdout JSONL) or `--output-dir ./out --split-entities` for per-entity files.

## Plan structure

```yaml
seed: 42                       # any int64; same seed = same output
entities:
  - name: User                 # entity (table/collection) name
    count: 100                 # number of rows
    fields:
      - name: id
        gen: seq               # generator name (see `apery list generators`)
      - name: email
        gen: regex
        config:
          pattern: "[a-z]{5,10}@example\\.com"
```

- Entities are processed **in declared order** — declare upstream entities before any that reference them via `rel_ref`.
- Every field has `name`, `gen`, and an optional `config: {...}` block whose shape depends on the generator (use `apery describe generator <name>`).
- Pick the distribution that matches the quantity: `lognormal` for prices, incomes, and durations (positive, long right tail; `mu`/`sigma` are in log space, so the median is e^mu), `exponential` for gaps between events (`mean`), `poisson` for counts per period (`lambda`), `normal_float`/`normal_int` for symmetric measurements, `zipf` for popularity skew. Uniform `int`/`float` rarely looks real.
- For event streams and logs, use `time_seq` (`start`, `step: 90s`, optional `jitter` below `step`): timestamps increase row by row, which `time` (random within a range) doesn't do. Per-parent timelines (each user's events starting at their signup) are `date_offset` over `{signup}` with `driven_by`'s `index_as`.
- For missing data, add `null_rate: 0.2` to a field (any generator). Nulls render as `null` in JSONL and an empty cell in CSV; `expr`/`date_offset` over a null give null, `template` renders it as empty text, and `rel_ref` never picks a null. Composite children take it too (`{gen: ..., config: ..., null_rate: 0.3}` nulls just that child). Validation rejects it on a `driven_by` parent key.
- Set `seed` explicitly at the top level (it defaults to 0 when omitted). The user can override it at run time with `--seed`.
- Plans are strict: every key must be one the schema or the generator declares. A typo fails validation with a "did you mean" suggestion.

### Relational data

**1:M (e.g., User has Orders)** — use `driven_by` instead of `count`. The executor generates Min–Max children per parent and auto-injects the parent key:

```yaml
- name: Order
  driven_by:
    entity: User      # parent entity (must already be declared above)
    field: id         # parent field to reference
    as: user_id       # field name to inject into each child row
    min: 1
    max: 5
  fields:
    - name: order_id
      gen: seq
    - name: amount
      gen: int
      config: { min: 100, max: 10000 }
```

**M:1 (e.g., Order references Product)** — use `rel_ref` to sample from an upstream entity's column:

```yaml
- name: product_id
  gen: rel_ref
  config:
    entity: Product   # must be declared earlier
    field: id
    # optional: distribution: zipf, s: 1.5  (skewed sampling)
    # optional: unique: true  (no dupes within parent batch — for M:N junctions)
```

**M:N** — compose: a junction entity with `driven_by` on the left side and `rel_ref` with `unique: true` on the right side.

**Cross-row dependent values (`expr`, `date_offset`)** — compute a field from sibling columns in the same row. Inside a `driven_by` entity, `expose` extra parent columns and `index_as` the child's 0-based position, then reference them with `{field}`:

```yaml
- name: Invoice
  driven_by:
    entity: Subscription
    field: id
    as: subscription_id
    min: 1
    max: 12
    expose:
      - field: start_date       # parent column injected into each child
    index_as: period            # 0-based child position within its parent
  fields:
    - name: billed_at
      gen: date_offset
      config: { base: "{start_date}", amount: "{period}", unit: months }
    - name: quantity
      gen: int
      config: { min: 1, max: 5 }
    - name: total_cents
      gen: expr
      config: { expr: "{quantity} * 250" }
```

Field order matters: `expr`/`date_offset` can only reference fields declared **before** them (or injected by `driven_by`).

## CLI quick reference

```bash
# Generate JSONL to stdout (pipe-friendly)
apery generate -f plan.yaml

# CSV (one entity per stream: multi-entity plans need --output-dir ./out --split-entities, or --entities)
apery generate -f plan.yaml -o csv

# One table only; entities it references are generated but not written, rows match a full run
apery generate -f plan.yaml --entities Order -o csv

# Write to directory (single output.jsonl)
apery generate -f plan.yaml --output-dir ./out

# Per-entity files (User.jsonl, Order.jsonl, ...)
apery generate -f plan.yaml --output-dir ./out --split-entities

# Validate only
apery validate -f plan.yaml -o json            # all issues, machine-readable
apery generate -f plan.yaml --dry-run          # + row estimates and columns

# Quick sample: at most N rows per entity
apery generate -f plan.yaml --limit 5

# Override seed / tune parallelism
apery generate -f plan.yaml --seed 123 --workers 8   # defaults use every core; fewer workers use less memory

# Progress on stderr
apery generate -f plan.yaml --verbose          # entity-level
apery generate -f plan.yaml --debug            # + seeds, chunks, layout

# Committed data still matches the plan? Same flags as the generate that made it; exit 4 = drifted
apery verify -f plan.yaml --output-dir ./fixtures --split-entities

# Discovery
apery list generators                          # -o json for full metadata
apery describe generator <name>                # -o json for machine-readable
apery describe plan-schema                     # JSON Schema for plan files
```

Exit codes: `0` success, `1` validation/plan error, `2` generation error, `3` I/O error, `4` `verify` found a difference, `130` interrupted. With `--output-dir`, files appear only when the run succeeds.

## Pitfalls to avoid

- **Don't fabricate generator names or config keys.** Run `apery describe generator <name>` to confirm the schema before writing the field. The catalog is the source of truth, not your memory.
- **Declaration order matters for `rel_ref` / `driven_by`** — the referenced entity must appear earlier in the `entities` list. Validation will reject out-of-order references, but it's faster to write them right the first time.
- **Don't add `count:` to a `driven_by` entity** — they're mutually exclusive. The child count comes from `min`/`max`.
- **`rel_ref` with `unique: true` on a standalone (non-`driven_by`) entity runs that entity on a single worker** (the whole entity is one uniqueness batch). Fine for small data; flag it to the user if they're generating millions of rows.
- **Composite generators** (`object`, `list`, `one_of`, `switch`) nest sub-generators under `config`. Run `apery describe generator object` for the exact shape — it's `config.fields.<name>.gen` / `config.fields.<name>.config`, not a flat list. Nest only pure value generators; keep `seq`, `rel_ref`, and row-aware generators (`template`, `expr`, `date_offset`) as top-level fields. `switch` cases may be row-aware.
- **Output goes to stdout by default.** If the user expects a file, use `--output-dir`. Don't redirect with `>` and then forget that `--verbose` writes to stderr separately.
- **Determinism is the headline feature.** If the user wants different output, they need a different `seed`, not a different run. Surface this when they ask "why is it the same every time?"

## When to push back

- If the user's request is fundamentally one-shot ("just give me 3 sample users right now"), inline the data instead of building a plan — the plan-and-generate overhead isn't worth it.
- If the user wants data that depends on external lookups or real-world sources, apery isn't the right tool — it generates from a declarative plan only. (Fields computed from sibling columns ARE supported via `expr`/`date_offset` — don't reject those.)
- If the user wants a generator that doesn't exist, don't invent config syntax for it. Tell them what's in `apery list generators` and ask which one fits, or whether the use case warrants a new generator.
