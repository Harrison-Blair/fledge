#!/usr/bin/env bash
# gen-herdr-types.sh — regenerate/verify internal/herdrclient against the live
# Herdr binary. This is the one-liner in the Herdr upgrade runbook: run it on
# every Herdr upgrade. Idempotent; safe to re-run.
#
# What it does:
#   1. Confirms `herdr` is on PATH and prints its version vs the pinned one.
#   2. Dumps `herdr api schema --json` to internal/herdrclient/herdr-schema.json
#      (committed, so the repo builds and reviews without Herdr installed).
#   3. Verifies every method/event name used by internal/herdrclient/types.go
#      appears in the schema dump; fails loudly on drift.
#   4. Reports whether the schema documents pane.clear_agent_authority and
#      pane.release_agent (their semantics are otherwise sourced only from
#      Herdr's bundled SOCKET_API.md) — record the answer in docs/DECISIONS.md.
#
# NOTE: protocol-version bumps require a Herdr server restart (or live
# handoff) before clients can reattach — see docs/INTEGRATION-CONTRACTS.md.
set -euo pipefail

PINNED_VERSION="0.7.4"
PINNED_PROTOCOL="15"

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
schema_out="$repo_root/internal/herdrclient/herdr-schema.json"
types_file="$repo_root/internal/herdrclient/types.go"

if ! command -v herdr >/dev/null 2>&1; then
    echo "error: herdr not found on PATH." >&2
    echo "Stage 0 pinned Herdr v$PINNED_VERSION (protocol v$PINNED_PROTOCOL);" >&2
    echo "until this script has run against a live binary, the committed types" >&2
    echo "are hand-authored from the reference snapshot (see docs/DECISIONS.md)." >&2
    exit 1
fi

live_version="$(herdr --version 2>/dev/null | head -n1 || true)"
echo "herdr on PATH:  ${live_version:-unknown}"
echo "pinned version: v$PINNED_VERSION (protocol v$PINNED_PROTOCOL)"
case "$live_version" in
    *"$PINNED_VERSION"*) : ;;
    *)
        echo "warning: live Herdr does not match the pin. Proceeding — update the" >&2
        echo "pin in this script and re-verify docs/INTEGRATION-CONTRACTS.md." >&2
        ;;
esac

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
herdr api schema --json > "$tmp"
mv "$tmp" "$schema_out"
trap - EXIT
echo "schema dump written: $schema_out ($(wc -c < "$schema_out") bytes) — commit it."

# Every dot-notation name string in types.go must appear in the schema dump.
echo
echo "== typed-surface coverage check =="
missing=0
while IFS= read -r name; do
    if grep -q "$name" "$schema_out"; then
        printf '  ok      %s\n' "$name"
    else
        printf '  MISSING %s\n' "$name"
        missing=$((missing + 1))
    fi
done < <(grep -oE '"[a-z_]+(\.[a-z_]+)+"' "$types_file" | tr -d '"' | sort -u)

echo
echo "== SOCKET_API.md soft-spot check (record in docs/DECISIONS.md) =="
for m in pane.clear_agent_authority pane.release_agent; do
    if grep -q "$m" "$schema_out"; then
        echo "  $m: DOCUMENTED in live schema dump"
    else
        echo "  $m: NOT in live schema dump (semantics remain SOCKET_API.md-only)"
    fi
done

if [ "$missing" -gt 0 ]; then
    echo
    echo "error: $missing typed name(s) missing from the live schema." >&2
    echo "Update internal/herdrclient/types.go (and envelope assumptions in" >&2
    echo "client.go) to match the dump, then re-run until clean." >&2
    exit 1
fi

echo
echo "done. If the schema dump changed, commit it together with any type updates."
