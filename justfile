set shell := ["zsh", "-eu", "-o", "pipefail", "-c"]

default:
    just --list

gate:
    just fmt
    just test
    just vet
    just lint
    just vuln
    just build

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

bench-startup:
    just build
    for i in {1..5}; do /usr/bin/time -p ./bin/cx --list --limit 20 >/dev/null; done

bench-index:
    just build
    /usr/bin/time -p ./bin/cx index refresh --json >/tmp/cx-index-refresh.json
    ./bin/cx index status
