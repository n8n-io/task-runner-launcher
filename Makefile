build:
	go build -o bin cmd/launcher/main.go
	@echo "Binary built at: $(shell pwd)/bin/main"

check: lint
	go fmt ./...
	go vet ./...

lintfix:
	golangci-lint run --fix

fmt:
	go fmt ./...

fmt-check:
	@if [ -n "$$(go fmt ./...)" ]; then \
		echo "Found unformatted Go files. Please run 'make fmt'"; \
		exit 1; \
	fi

lint:
	golangci-lint run

run: build
	./bin/main javascript

run-all: build
	./bin/main javascript python

test:
	go test -race ./...

test-verbose:
	go test -race -v ./...

test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

.PHONY: build check lint lintfix fmt fmt-check run run-many test test-verbose test-coverage
