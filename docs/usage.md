# Apery Usage Guide

## Install

```bash
make install    # builds and copies to ~/.local/bin
```

## Help

```bash
# Top-level help
apery --help

# Command-specific help
apery help generate
apery help validate
apery help list
apery help describe
```

## Discover Generators

```bash
# List all available generators
apery list generators

# Get full config schema + example for a generator
apery describe generator pick
apery describe generator rel_ref
apery describe generator object
apery describe generator switch
```

## Example Plans

### 1. Simple — Users

Create `examples/users.yaml`:

```yaml
seed: 42
entities:
  - name: User
    count: 20
    fields:
      - name: id
        gen: seq
      - name: name
        gen: pick
        config:
          values: [Alice, Bob, Carol, Dave, Eve, Frank]
      - name: email
        gen: regex
        config:
          pattern: "[a-z]{5,10}@(gmail|yahoo|outlook)\\.com"
      - name: age
        gen: int
        config:
          min: 18
          max: 65
      - name: active
        gen: bool
        config:
          probability: 0.8
```

```bash
# Validate
apery validate -f examples/users.yaml

# Generate to stdout (JSONL)
apery generate -f examples/users.yaml

# Generate CSV
apery generate -f examples/users.yaml -o csv

# Pipe through jq
apery generate -f examples/users.yaml | jq '.name'

# Count rows
apery generate -f examples/users.yaml | wc -l

# Write to file
apery generate -f examples/users.yaml --output-dir ./out
cat ./out/output.jsonl
```

### 2. Composite — Users with Addresses

Create `examples/composite.yaml`:

```yaml
seed: 7
entities:
  - name: User
    count: 10
    fields:
      - name: id
        gen: ulid
      - name: department
        gen: pick
        config:
          values: [engineering, sales, marketing, support]
          weights: [40, 30, 20, 10]
      - name: address
        gen: object
        config:
          fields:
            city:
              gen: pick
              config:
                values: [New York, Los Angeles, Chicago, Houston, Phoenix]
            zip:
              gen: int
              config:
                min: 10000
                max: 99999
            geo:
              gen: object
              config:
                fields:
                  lat:
                    gen: float
                    config:
                      min: 25.0
                      max: 48.0
                  lng:
                    gen: float
                    config:
                      min: -125.0
                      max: -70.0
      - name: tags
        gen: list
        config:
          min_len: 1
          max_len: 4
          item:
            gen: pick
            config:
              values: [admin, beta, premium, internal, vip]
      - name: skills
        gen: sample
        config:
          values: [Go, Python, Rust, TypeScript, Java, C++, Ruby, Kotlin]
          min_n: 2
          max_n: 5
      - name: greeting
        gen: template
        config:
          tpl: "Welcome to {department}!"
      - name: access_level
        gen: switch
        config:
          key: department
          cases:
            engineering:
              gen: const
              config:
                value: full
            sales:
              gen: const
              config:
                value: read-only
          default:
            gen: const
            config:
              value: standard
```

```bash
# Pretty-print one record
apery generate -f examples/composite.yaml | head -1 | jq .

# Check all access levels
apery generate -f examples/composite.yaml | jq -r '.access_level' | sort | uniq -c
```

### 3. Relational — E-commerce

Create `examples/ecommerce.yaml`:

```yaml
seed: 99
entities:
  - name: User
    count: 100
    fields:
      - name: id
        gen: seq
      - name: name
        gen: pick
        config:
          values: [Alice, Bob, Carol, Dave, Eve, Frank, Grace, Hank]
      - name: signup
        gen: time
        config:
          start: "2023-01-01"
          end: "2024-12-31"
          format: "2006-01-02"

  - name: Product
    count: 50
    fields:
      - name: id
        gen: seq
      - name: sku
        gen: regex
        config:
          pattern: "[A-Z]{2}-\\d{6}"
      - name: price
        gen: int
        config:
          min: 499
          max: 99999

  - name: Order
    driven_by:
      entity: User
      field: id
      as: user_id
      min: 1
      max: 5
    fields:
      - name: order_id
        gen: seq
      - name: product_id
        gen: rel_ref
        config:
          entity: Product
          field: id
      - name: quantity
        gen: int
        config:
          min: 1
          max: 10

  - name: Review
    count: 500
    fields:
      - name: user_id
        gen: rel_ref
        config:
          entity: User
          field: id
          distribution: zipf
          s: 1.5
      - name: product_id
        gen: rel_ref
        config:
          entity: Product
          field: id
      - name: rating
        gen: int
        config:
          min: 1
          max: 5
```

```bash
# Validate relational plan
apery validate -f examples/ecommerce.yaml

# Generate all entities to stdout
apery generate -f examples/ecommerce.yaml | wc -l

# Split into per-entity files
apery generate -f examples/ecommerce.yaml --output-dir ./out --split-entities
ls -la ./out/

# Inspect each entity
head -3 ./out/User.jsonl
head -3 ./out/Product.jsonl
head -3 ./out/Order.jsonl
head -3 ./out/Review.jsonl

# Check order count per user (driven_by 1-5)
cat ./out/Order.jsonl | jq -r '.user_id' | sort -n | uniq -c | sort -rn | head -10

# Check zipf distribution on reviews (some users get way more)
cat ./out/Review.jsonl | jq -r '.user_id' | sort -n | uniq -c | sort -rn | head -10

# CSV split
rm -rf ./out-csv
apery generate -f examples/ecommerce.yaml -o csv --output-dir ./out-csv --split-entities
head -5 ./out-csv/User.csv
head -5 ./out-csv/Order.csv
```

### 4. JSON Plan

Create `examples/simple.json`:

```json
{
  "seed": 1,
  "entities": [
    {
      "name": "Event",
      "count": 10,
      "fields": [
        {"name": "id", "gen": "uuid"},
        {"name": "timestamp", "gen": "time"},
        {"name": "severity", "gen": "pick", "config": {"values": ["info", "warn", "error"], "weights": [70, 20, 10]}},
        {"name": "code", "gen": "int", "config": {"min": 100, "max": 599}}
      ]
    }
  ]
}
```

```bash
apery generate -f examples/simple.json | jq .
```

## Determinism

```bash
# Same seed = identical output
apery generate -f examples/ecommerce.yaml --seed 42 | md5sum
apery generate -f examples/ecommerce.yaml --seed 42 | md5sum

# Different seed = different output
apery generate -f examples/ecommerce.yaml --seed 43 | md5sum
```

## Verbose & Debug Output

```bash
# Default: completely silent on stderr
apery generate -f examples/users.yaml > /dev/null

# Verbose: entity-level progress
apery generate -f examples/ecommerce.yaml --verbose > /dev/null
# Output:
#   level=INFO msg=run.start entities=4 seed=99
#   level=INFO msg=entity.start entity=User rows=100
#   level=INFO msg=entity.complete entity=User rows=100 duration=1ms
#   level=INFO msg=entity.start entity=Product rows=50
#   level=INFO msg=entity.complete entity=Product rows=50 duration=0s
#   level=INFO msg=entity.start entity=Order type=driven_by parent=User
#   level=INFO msg=entity.complete entity=Order rows=292 duration=1ms
#   level=INFO msg=run.complete entities=4 rows=942 duration=4ms

# Debug: full detail (includes verbose + seeds, chunks, layout)
apery generate -f examples/ecommerce.yaml --debug > /dev/null
# Additional output includes:
#   level=DEBUG msg=field.init entity=User field=id gen=seq seed=...
#   level=DEBUG msg=chunk.dispatch chunk=0 start=0 end=50000
#   level=DEBUG msg=driven_by.layout entity=Order parent=User total_children=292
```

## Performance

```bash
# Generate 1M rows with tuned parallelism
cat > /tmp/bench.yaml << 'EOF'
seed: 1
entities:
  - name: Row
    count: 1000000
    fields:
      - name: id
        gen: seq
      - name: value
        gen: int
        config:
          min: 0
          max: 1000000
      - name: label
        gen: pick
        config:
          values: [a, b, c, d, e]
EOF

time apery generate -f /tmp/bench.yaml --workers 16 --chunk-size 100000 > /dev/null

# With verbose progress
time apery generate -f /tmp/bench.yaml --workers 16 --verbose > /dev/null

# With debug (see chunk dispatch details)
time apery generate -f /tmp/bench.yaml --workers 16 --debug > /dev/null
```

## Error Handling

```bash
# Missing file
apery validate -f nonexistent.yaml; echo "exit: $?"

# Invalid YAML
echo "seed: [broken" > /tmp/bad.yaml
apery validate -f /tmp/bad.yaml; echo "exit: $?"

# Missing required field
echo '{"seed": 1, "entities": []}' > /tmp/empty.json
apery validate -f /tmp/empty.json; echo "exit: $?"

# Unknown generator
cat > /tmp/badgen.yaml << 'EOF'
seed: 1
entities:
  - name: X
    count: 1
    fields:
      - name: x
        gen: nonexistent
EOF
apery generate -f /tmp/badgen.yaml; echo "exit: $?"

# --split-entities without --output-dir
apery generate -f examples/users.yaml --split-entities; echo "exit: $?"
```
