#!/usr/bin/env bash
# exp-session-up.sh — start the named throwaway Herdr session `fledge-exp`.
#
# All Stage 0 experiments run ONLY in this session (docs/EXPERIMENTS.md §
# environment): they deliberately churn pane authority (report_agent,
# clear_agent_authority, release_agent) and the 32-source-cap question is
# unresolved, so none of that may touch a session where real work lives.
#
# Harnesses target it via HERDR_SESSION=fledge-exp (socket under
# sessions/fledge-exp/). Tear down with scripts/exp-session-down.sh.
#
# RE-VERIFY: the exact server-start subcommand is version-specific and was not
# confirmable against a live binary when this script was written; the
# readiness check below is the source of truth. Adjust the start command for
# your Herdr version if the check fails, and note it in docs/DECISIONS.md.
set -euo pipefail

SESSION="fledge-exp"

if ! command -v herdr >/dev/null 2>&1; then
    echo "error: herdr not found on PATH" >&2
    exit 1
fi

echo "starting throwaway Herdr session '$SESSION'..."
# Best-effort start; some versions auto-start the session server on first
# --session use. Failures here are tolerated as long as readiness passes.
herdr --session "$SESSION" server start >/dev/null 2>&1 \
    || herdr server start --session "$SESSION" >/dev/null 2>&1 \
    || true

for i in $(seq 1 20); do
    if HERDR_SESSION="$SESSION" herdr api snapshot >/dev/null 2>&1; then
        echo "session '$SESSION' is up."
        echo "target it with: HERDR_SESSION=$SESSION"
        exit 0
    fi
    sleep 0.5
done

echo "error: session '$SESSION' did not become ready." >&2
echo "Start it manually for your Herdr version (e.g. run 'herdr --session $SESSION')," >&2
echo "then confirm with: HERDR_SESSION=$SESSION herdr api snapshot" >&2
exit 1
