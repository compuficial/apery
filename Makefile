.PHONY: build install run clean fmt vet test bench

VERSION ?= dev

# Build the binary
build:
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/apery ./cmd/apery

# Install to ~/.local/bin
install: build
	mkdir -p $(HOME)/.local/bin
	cp bin/apery $(HOME)/.local/bin/apery

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
