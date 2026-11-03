VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
TAGS := nethttpomithttp2

build:
	go build -tags $(TAGS) -trimpath -ldflags '$(LDFLAGS)' -o hp ./cmd/hp

test:
	go test ./...

check:
	./scripts/check.sh

bench:
	go run ./tools/bench

.PHONY: build test check bench
