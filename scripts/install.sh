#!/usr/bin/env bash
# Build, install, and verify the fledge binary.
#
# This repo dogfoods fledge, so the `fledge` on your PATH must match the source
# you're editing. Run this after changing CLI or internal/bootstrap/... code to
# refresh the install. Pass --refresh to also re-sync this repo's scaffolded
# output (.fledge/skills/ and the .claude/ adapter) after installing.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

refresh=0
for arg in "$@"; do
	case "$arg" in
		--refresh) refresh=1 ;;
		-h|--help)
			echo "usage: scripts/install.sh [--refresh]"
			echo "  --refresh  re-sync .fledge/skills/ and the .claude/ adapter after install"
			exit 0
			;;
		*) echo "unknown argument: $arg" >&2; exit 2 ;;
	esac
done

want="$(cat VERSION)"
ldflags="-X github.com/Harrison-Blair/fledge/internal/cli.binaryVersion=$want"

echo "==> go build ./..."
go build ./...

echo "==> go install -ldflags ... ./cmd/fledge  (version $want)"
go install -ldflags "$ldflags" ./cmd/fledge

hash -r  # drop the shell's cached path to any old binary

bin="$(command -v fledge || true)"
if [[ -z "$bin" ]]; then
	echo "error: fledge not found on PATH after install" >&2
	echo "       add \"$(go env GOPATH)/bin\" to your PATH" >&2
	exit 1
fi
echo "==> installed: $bin"

got="$(fledge version)"
if [[ "$got" != *"$want"* ]]; then
	echo "error: version mismatch — VERSION=$want but 'fledge version' reports: $got" >&2
	echo "       the installed binary may be stale or shadowed by another copy" >&2
	exit 1
fi
echo "==> fledge version: $got (matches VERSION)"

if [[ "$refresh" -eq 1 ]]; then
	echo "==> fledge init --refresh"
	fledge init --refresh
	echo "==> done. review changes with: git status"
fi
