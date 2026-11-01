#!/bin/sh
# Install the latest hp release for linux or macOS, for the current user.
set -eu

repo="HoneyPaptan/hp-harness"
base="https://github.com/$repo/releases/latest/download"

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
	fetch() { curl -fL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -q "$1" -O "$2"; }
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

line=$(grep " $asset\$" "$tmp/checksums.txt") || {
	printf 'install.sh: no checksum for %s\n' "$asset" >&2
	exit 1
}
if command -v sha256sum >/dev/null 2>&1; then
	printf '%s\n' "$line" | sha256sum -c - >/dev/null
elif command -v shasum >/dev/null 2>&1; then
	printf '%s\n' "$line" | shasum -a 256 -c - >/dev/null
else
	printf 'install.sh: need sha256sum or shasum\n' >&2
	exit 1
fi

tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$HOME/.local/bin"
mv "$tmp/hp" "$HOME/.local/bin/hp"
chmod +x "$HOME/.local/bin/hp"
printf 'installed %s/hp\n' "$(cd "$HOME/.local/bin" && pwd)"
case ":$PATH:" in
	*":$HOME/.local/bin:"*) ;;
	*) printf 'add %s/.local/bin to your PATH\n' "$HOME" >&2 ;;
esac
