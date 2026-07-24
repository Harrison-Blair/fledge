#!/usr/bin/env bash
# Builds the fledge binary into bin/ at the repo root.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="$repo/bin/fledge"

cd "$repo"
go build -o "$out" ./cmd/fledge

echo "built $out"
