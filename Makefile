.PHONY: audit build lint lintfix fmt fmt-check run test test-verbose test-coverage

check: lint
	go fmt ./...
	go vet ./...

build:
	go build -o bin cmd/launcher/main.go

lint:
	golangci-lint run

lintfix:
	golangci-lint run --fix

fmt:
	go fmt ./...

fmt-check:
	@if [ -n "$$(go fmt ./...)" ]; then \
		echo "Found unformatted Go files. Please run 'make fmt'"; \
		exit 1; \
	fi

run: build
	./bin/main javascript

test:
	go test ./...

test-verbose:
	go test -v ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html
