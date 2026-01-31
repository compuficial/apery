.PHONY: build run clean fmt vet test bench

# Build the binary
build:
	go build -o bin/apery ./cmd/apery

# Run the program
run:
	go run ./cmd/apery

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f output.*

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Run tests
test:
	go test ./...

# Run benchmarks
bench:
	go test -run '^$$' -bench BenchmarkExecutor -benchmem ./internal/runtime

# Build and run
all: build
	./bin/apery
