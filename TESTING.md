# Go Testing Commands

## Run all tests in the project
```bash
go test ./...
```

## Run tests in a specific package
```bash
go test ./internal/registry
```

## Run a specific test file
```bash
# Go doesn't support running by filename directly, but you can run the package with a pattern:
go test ./internal/registry -run TestSeq
```

## Run a specific test function
```bash
# Use the -run flag with a regex pattern matching the test name
go test ./internal/registry -run TestSeqGenerator_Config
go test ./internal/registry -run TestBoolGenerator_Distribution
```

## Run with verbose output
```bash
# Shows all test output (useful to see what's passing)
go test -v ./internal/registry

# Combine with -run for a specific test
go test -v ./internal/registry -run TestSeqGenerator_Sequence
```

## Run and see coverage
```bash
go test -cover ./internal/registry
```

## Useful patterns

```bash
# Run all Seq tests
go test ./internal/registry -run Seq

# Run all Config tests across all generators
go test ./internal/registry -run Config

# Run all Determinism tests
go test ./internal/registry -run Determinism
```

## Quick reference for current work

```bash
# Test the seq generator
go test -v ./internal/registry -run Seq

# Test the pick generator
go test -v ./internal/registry -run Pick

# Test the int generator
go test -v ./internal/registry -run Int

# Test the float generator
go test -v ./internal/registry -run Float

# Test the uuid generator
go test -v ./internal/registry -run UUID

# Test the bool generator
go test -v ./internal/registry -run Bool
```
