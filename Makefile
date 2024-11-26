.PHONY: audit build lint lintfix run

check: lint
	go fmt ./...
	go vet ./...

build:
	go build -o bin cmd/launcher/main.go

lint:
	golangci-lint run

lintfix:
	golangci-lint run --fix

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
