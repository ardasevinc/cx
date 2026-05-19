set shell := ["zsh", "-eu", "-o", "pipefail", "-c"]

default:
    just --list

fmt:
    gofumpt -w .

test:
    go test ./...

vet:
    go vet ./...

lint:
    golangci-lint run ./...

vuln:
    govulncheck ./...

build:
    mkdir -p bin
    go build -o bin/cx ./cmd/cx

install:
    go install ./cmd/cx

