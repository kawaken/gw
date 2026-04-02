.PHONY: all build test lint fmt fmt-check check clean

BIN := ./bin/gw

all: build

build:
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
		echo "Files not formatted:"; echo "$$diff"; exit 1; \
	fi

check: lint test

clean:
	rm -f $(BIN)
