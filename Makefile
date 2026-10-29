.PHONY: all ui build test test-short check dev devcluster openapi docker clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/wankdanker/natsmin/internal/version.Version=$(VERSION) -X github.com/wankdanker/natsmin/internal/version.Commit=$(shell git rev-parse --short HEAD 2>/dev/null)

all: ui build

ui:
	cd ui && pnpm install --frozen-lockfile && pnpm run build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/natsmin ./cmd/natsmin
	CGO_ENABLED=0 go build -trimpath -o bin/natsmin-devcluster ./cmd/natsmin-devcluster

test:
	go test ./...

test-short:
	go test -short ./...

check:
	go vet ./...
	test -z "$$(gofmt -l .)"
	cd ui && pnpm run check

# Regenerate the OpenAPI document and the UI's typed client after API changes.
openapi:
	go run ./cmd/natsmin openapi > docs/openapi.json
	cd ui && pnpm run gen:api

# Local supercluster with demo traffic (east/west clusters + a leafnode).
devcluster:
	go run ./cmd/natsmin-devcluster -demo

# API server on :8080; run `cd ui && pnpm dev` alongside for the UI with hot reload on :5173.
dev:
	go run ./cmd/natsmin -data ./data -poll 2s

docker:
	docker build -t natsmin:$(VERSION) .

clean:
	rm -rf bin web/dist/_app web/dist/index.html web/dist/favicon.svg ui/.svelte-kit
