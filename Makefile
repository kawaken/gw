.PHONY: all build test lint fmt fmt-check check clean

BIN := ./bin/gw
TMPDIR := $(CURDIR)/tmp
GOCACHE ?= $(TMPDIR)/gocache
GOLANGCI_LINT_CACHE ?= $(TMPDIR)/golangci-lint-cache

export GOCACHE
export GOLANGCI_LINT_CACHE

all: build check

build:
	mkdir -p $(dir $(BIN))
	go build -o $(BIN) ./cmd/gw/

test:
	go test ./... -v -race -count=1

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	@if command -v goimports > /dev/null 2>&1; then \
		goimports -w -local github.com/kawaken/gw .; \
	else \
		echo "goimports not found (install: go install golang.org/x/tools/cmd/goimports@latest)"; \
	fi

fmt-check:
	@diff=$$(gofmt -l .); \
	if [ -n "$$diff" ]; then \
		echo "Files not formatted (gofmt):"; echo "$$diff"; exit 1; \
	fi
	@if command -v goimports > /dev/null 2>&1; then \
		diff=$$(goimports -l -local github.com/kawaken/gw .); \
		if [ -n "$$diff" ]; then \
			echo "Files not formatted (goimports):"; echo "$$diff"; exit 1; \
		fi; \
	fi

check: fmt-check lint test

clean:
	rm -f $(BIN)
