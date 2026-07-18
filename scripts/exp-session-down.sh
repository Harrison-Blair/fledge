#!/usr/bin/env bash
# exp-session-down.sh — tear down the throwaway Herdr session `fledge-exp`
# started by scripts/exp-session-up.sh.
#
# RE-VERIFY: the exact server-stop invocation is version-specific (the socket
# method is `server.stop`); adjust for your Herdr version if the check below
# still sees the session, and note it in docs/DECISIONS.md.
set -euo pipefail

SESSION="fledge-exp"

if ! command -v herdr >/dev/null 2>&1; then
    echo "error: herdr not found on PATH" >&2
    exit 1
fi

if ! HERDR_SESSION="$SESSION" herdr api snapshot >/dev/null 2>&1; then
    echo "session '$SESSION' is not running; nothing to do."
    exit 0
fi

echo "stopping throwaway Herdr session '$SESSION'..."
HERDR_SESSION="$SESSION" herdr server stop >/dev/null 2>&1 \
    || herdr --session "$SESSION" server stop >/dev/null 2>&1 \
    || true

if HERDR_SESSION="$SESSION" herdr api snapshot >/dev/null 2>&1; then
    echo "error: session '$SESSION' still responds; stop it manually" >&2
    echo "(socket method server.stop) and adjust this script for your version." >&2
    exit 1
fi
echo "session '$SESSION' is down."
