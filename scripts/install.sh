#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
binary="$repo_root/bin/fledge"
install_dir="${FLEDGE_INSTALL_DIR:-}"
if [[ -z "$install_dir" ]]; then
    install_dir="$(go env GOBIN 2>/dev/null || true)"
fi
if [[ -z "$install_dir" ]]; then
    gopath="$(go env GOPATH 2>/dev/null || true)"
    if [[ -n "$gopath" ]]; then
        install_dir="$gopath/bin"
    fi
fi
install_dir="${install_dir:-$HOME/go/bin}"
destination="$install_dir/fledge"

if [[ ! -x "$binary" ]]; then
    echo "fledge binary not found at $binary; run scripts/build.sh first" >&2
    exit 1
fi

mkdir -p "$install_dir"
install -m 0755 "$binary" "$destination"

echo "Installed fledge to $destination"

resolved_binary="$(command -v fledge || true)"
if [[ -n "$resolved_binary" && ! "$resolved_binary" -ef "$destination" ]]; then
    resolved_dir="$(dirname -- "$resolved_binary")"
    echo "warning: your shell resolves fledge to $resolved_binary, not $destination" >&2
    printf 'To replace that binary, run: FLEDGE_INSTALL_DIR=%q %q\n' \
        "$resolved_dir" "$repo_root/scripts/reinstall.sh" >&2
fi
