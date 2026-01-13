.PHONY: build run clean fmt vet test

# Build the binary
build:
	go build -o bin/apery main.go

# Run the program
run:
	go run main.go

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f output.jsonl

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
