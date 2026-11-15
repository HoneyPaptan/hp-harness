VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
TAGS := nethttpomithttp2

build:
	go build -tags $(TAGS) -trimpath -ldflags '$(LDFLAGS)' -o hp ./cmd/hp

wasm:
	mkdir -p dist/web
	GOOS=js GOARCH=wasm go build -tags $(TAGS) -trimpath -ldflags '$(LDFLAGS)' -o dist/web/hp.wasm ./cmd/hp-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" dist/web/

test:
	go test ./...

check:
	./scripts/check.sh

bench:
	go run ./tools/bench

.PHONY: build wasm test check bench
