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
4. **Validate first** — `apery validate -f plan.yaml`. Fix any errors before generating.
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
- `seed` is required at the top level. The user can override it at run time with `--seed`.

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

# CSV
apery generate -f plan.yaml -o csv

# Write to directory (single output.jsonl)
apery generate -f plan.yaml --output-dir ./out

# Per-entity files (User.jsonl, Order.jsonl, ...)
apery generate -f plan.yaml --output-dir ./out --split-entities

# Validate only
apery validate -f plan.yaml
apery generate -f plan.yaml --dry-run

# Override seed / tune parallelism
apery generate -f plan.yaml --seed 123 --workers 16 --chunk-size 100000

# Progress on stderr
apery generate -f plan.yaml --verbose          # entity-level
apery generate -f plan.yaml --debug            # + seeds, chunks, layout

# Discovery
apery list generators
apery describe generator <name>
```

Exit codes: `0` success, `1` validation/plan error, `2` generation error, `3` I/O error.

## Pitfalls to avoid

- **Don't fabricate generator names or config keys.** Run `apery describe generator <name>` to confirm the schema before writing the field. The catalog is the source of truth, not your memory.
- **Declaration order matters for `rel_ref` / `driven_by`** — the referenced entity must appear earlier in the `entities` list. Validation will reject out-of-order references, but it's faster to write them right the first time.
- **Don't add `count:` to a `driven_by` entity** — they're mutually exclusive. The child count comes from `min`/`max`.
- **`rel_ref` with `unique: true` on a standalone (non-`driven_by`) entity forces single-threaded execution.** Fine for small data; flag it to the user if they're generating millions of rows.
- **Composite generators** (`object`, `list`, `sample`, `switch`, `one_of`, `template`) nest sub-generators under `config`. Run `apery describe generator object` for the exact shape — it's `config.fields.<name>.gen` / `config.fields.<name>.config`, not a flat list.
- **Output goes to stdout by default.** If the user expects a file, use `--output-dir`. Don't redirect with `>` and then forget that `--verbose` writes to stderr separately.
- **Determinism is the headline feature.** If the user wants different output, they need a different `seed`, not a different run. Surface this when they ask "why is it the same every time?"

## When to push back

- If the user's request is fundamentally one-shot ("just give me 3 sample users right now"), inline the data instead of building a plan — the plan-and-generate overhead isn't worth it.
- If the user wants data that depends on external lookups or real-world sources, apery isn't the right tool — it generates from a declarative plan only. (Fields computed from sibling columns ARE supported via `expr`/`date_offset` — don't reject those.)
- If the user wants a generator that doesn't exist, don't invent config syntax for it. Tell them what's in `apery list generators` and ask which one fits, or whether the use case warrants a new generator.
