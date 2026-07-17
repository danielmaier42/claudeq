# claudeq developer tasks. These mirror the CI quality gates (see AGENTS.md §1).

GO      ?= go
GOBIN   := $(shell $(GO) env GOPATH)/bin

.PHONY: all check fmt fmt-check vet lint test build tidy tools

all: check

## check: run every quality gate (format check, vet, lint, race tests, build)
check: fmt-check vet lint test build

## fmt: format the code (gofmt + goimports)
fmt:
	gofmt -w .
	$(GOBIN)/goimports -w -local github.com/danielmaier42/claudeq .

## fmt-check: fail if any file is not formatted
fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

## vet: run go vet
vet:
	$(GO) vet ./...

## lint: run golangci-lint
lint:
	golangci-lint run

## test: run the test suite with the race detector
test:
	$(GO) test ./... -race

## build: build all packages
build:
	$(GO) build ./...

## tidy: tidy go.mod / go.sum
tidy:
	$(GO) mod tidy
