#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

BUILD_TAGS=nethttpomithttp2

fail() {
	printf 'check: %s\n' "$*" >&2
	exit 1
}

formatting() {
	local unformatted
	unformatted=$(gofmt -l cmd internal tools 2>/dev/null || true)
	[ -z "$unformatted" ] || fail "gofmt needed: $unformatted"
}

core_boundary() {
	local pkg banned
	for pkg in internal/core internal/permit; do
		[ -d "$pkg" ] || continue
		banned=$(go list -f '{{join .Imports "\n"}}' "./$pkg/..." | grep -xE 'os|os/exec|net|net/http|syscall' || true)
		[ -z "$banned" ] || fail "$pkg imports $(echo $banned); use Host"
	done
}

size_budget() {
	local out size budget
	out=$(mktemp)
	go build -tags "$BUILD_TAGS" -trimpath -ldflags='-s -w' -o "$out" ./cmd/hp
	size=$(wc -c <"$out")
	rm -f "$out"
	budget=$(cat .size-budget)
	printf 'size: %s bytes, budget %s\n' "$size" "$budget"
	[ "$size" -le "$budget" ] || fail "binary over size budget"
}

wasm_budget() {
	local out size budget
	out=$(mktemp)
	GOOS=js GOARCH=wasm go vet ./cmd/hp-wasm
	GOOS=js GOARCH=wasm go build -tags "$BUILD_TAGS" -trimpath -ldflags='-s -w' -o "$out" ./cmd/hp-wasm
	size=$(gzip -9 -c "$out" | wc -c)
	rm -f "$out"
	budget=$(cat .wasm-size-budget)
	printf 'wasm gzip size: %s bytes, budget %s\n' "$size" "$budget"
	[ "$size" -le "$budget" ] || fail "wasm over size budget"
}

formatting
go vet ./...
go run ./tools/nocomments
core_boundary
size_budget
wasm_budget
