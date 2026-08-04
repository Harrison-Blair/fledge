#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$repo_root"
go vet ./...
go test -trimpath -buildvcs=true -race ./...
mkdir -p bin
go build -trimpath -buildvcs=true -o bin/fledge .
