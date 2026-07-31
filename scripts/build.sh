#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/.." && pwd -P)"

cd "${repo_root}"

embedded_version="$(tr -d '[:space:]' < internal/buildinfo/VERSION)"
if exact_tag="$(git describe --tags --exact-match HEAD 2>/dev/null)"; then
  if [[ "${exact_tag}" != "${embedded_version}" ]]; then
    printf 'error: release tag %s does not match embedded version %s\n' \
      "${exact_tag}" "${embedded_version}" >&2
    exit 1
  fi
fi

go test -trimpath -buildvcs=true -race ./...
go vet ./...
mkdir -p bin
go build -trimpath -buildvcs=true -o bin/fledge ./cmd/fledge
