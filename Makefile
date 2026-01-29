.PHONY: build run clean fmt vet test

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

# Build and run
all: build
	./bin/apery
