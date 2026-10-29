VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o hp ./cmd/hp

test:
	go test ./...

check:
	./scripts/check.sh

.PHONY: build test check
