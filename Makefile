.PHONY: all build test lint fmt fmt-check clean

all: fmt lint test

build:
	go build -o bin/feistel ./...

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint run --fix ./...

# Check formatting only (non-destructive)
fmt-check:
	@echo "Checking formatting..."
	@output=$$(goimports -l .); \
	if [ -n "$$output" ]; then \
		echo "Files not formatted:"; \
		echo "$$output"; \
		exit 1; \
	else \
		echo "All files formatted properly."; \
	fi

clean:
	rm -rf bin
