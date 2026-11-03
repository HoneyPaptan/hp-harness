#!/bin/sh
set -eu

repo="HoneyPaptan/hp-harness"
base="https://github.com/$repo/releases/latest/download"
if [ -n "${HP_VERSION:-}" ]; then
	case "$HP_VERSION" in
		v[0-9]*) ;;
		*) printf 'install.sh: HP_VERSION must look like v1.2.3\n' >&2; exit 1 ;;
	esac
	case "$HP_VERSION" in
		*[!0-9A-Za-z.+-]*) printf 'install.sh: bad HP_VERSION %s\n' "$HP_VERSION" >&2; exit 1 ;;
	esac
	base="https://github.com/$repo/releases/download/$HP_VERSION"
fi

case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) printf 'install.sh: unsupported system %s\n' "$(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
	x86_64|amd64) arch=amd64 ;;
	aarch64|arm64) arch=arm64 ;;
	*) printf 'install.sh: unsupported architecture %s\n' "$(uname -m)" >&2; exit 1 ;;
esac

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fL --proto '=https' --tlsv1.2 "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -q --https-only "$1" -O "$2"; }
else
	printf 'install.sh: need curl or wget\n' >&2; exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

asset="hp-$os-$arch.tar.gz"
fetch "$base/$asset" "$tmp/$asset" || {
	printf 'install.sh: cannot download %s\n' "$asset" >&2
	exit 1
}
fetch "$base/checksums.txt" "$tmp/checksums.txt"

line=$(awk -v a="$asset" '$2 == a' "$tmp/checksums.txt") && [ -n "$line" ] || {
	printf 'install.sh: no checksum for %s\n' "$asset" >&2
	exit 1
}
if command -v sha256sum >/dev/null 2>&1; then
	(cd "$tmp" && printf '%s\n' "$line" | sha256sum -c - >/dev/null)
elif command -v shasum >/dev/null 2>&1; then
	(cd "$tmp" && printf '%s\n' "$line" | shasum -a 256 -c - >/dev/null)
else
	printf 'install.sh: need sha256sum or shasum\n' >&2
	exit 1
fi

tar -xzf "$tmp/$asset" -C "$tmp" hp
mkdir -p "$HOME/.local/bin"
cp "$tmp/hp" "$HOME/.local/bin/.hp.new"
chmod +x "$HOME/.local/bin/.hp.new"
mv -f "$HOME/.local/bin/.hp.new" "$HOME/.local/bin/hp"
printf 'installed %s/hp\n' "$(cd "$HOME/.local/bin" && pwd)"
case ":$PATH:" in
	*":$HOME/.local/bin:"*) ;;
	*) printf 'add %s/.local/bin to your PATH\n' "$HOME" >&2 ;;
esac
