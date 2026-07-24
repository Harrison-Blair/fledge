#!/usr/bin/env bash
# Builds a local dev binary (version suffixed "-dev" via the dev build tag)
# and installs it onto PATH.
# Destination defaults to GOBIN, else GOPATH/bin. Override with BINDIR=...
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src="$(mktemp)"
trap 'rm -f "$src"' EXIT

(cd "$repo" && go build -tags dev -o "$src" ./cmd/fledge)

bindir="${BINDIR:-$(go env GOBIN)}"
if [[ -z "$bindir" ]]; then
	bindir="$(go env GOPATH)/bin"
fi

mkdir -p "$bindir"
install -m 0755 "$src" "$bindir/fledge"

echo "installed $bindir/fledge"

if [[ ":$PATH:" != *":$bindir:"* ]]; then
	echo "warning: $bindir is not on your PATH" >&2
fi

# A copy earlier on PATH would shadow what we just installed.
found="$(command -v fledge || true)"
if [[ -n "$found" && "$found" != "$bindir/fledge" ]]; then
	echo "warning: $found shadows $bindir/fledge on PATH" >&2
fi
