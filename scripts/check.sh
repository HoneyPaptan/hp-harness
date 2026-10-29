#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

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
	[ -d internal/core ] || return 0
	local banned
	banned=$(go list -f '{{join .Imports "\n"}}' ./internal/core/... | grep -xE 'os|os/exec|net|net/http|syscall' || true)
	[ -z "$banned" ] || fail "internal/core imports $(echo $banned); use Host"
}

size_budget() {
	local out size budget
	out=$(mktemp)
	go build -trimpath -ldflags='-s -w' -o "$out" ./cmd/hp
	size=$(wc -c <"$out")
	rm -f "$out"
	budget=$(cat .size-budget)
	printf 'size: %s bytes, budget %s\n' "$size" "$budget"
	[ "$size" -le "$budget" ] || fail "binary over size budget"
}

formatting
go vet ./...
go run ./tools/nocomments
core_boundary
size_budget
