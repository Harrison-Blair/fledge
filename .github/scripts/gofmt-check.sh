#!/usr/bin/env bash
# Report Go files that gofmt would change. Run from the repository root. The
# check covers tracked and new nonignored files that exist, so it skips deleted
# files and ignored managed worktrees.
set -euo pipefail

unformatted="$(
  git ls-files --cached --others --exclude-standard --deduplicate -z -- '*.go' |
    while IFS= read -r -d '' file; do
      if [[ -f "$file" ]]; then
        printf '%s\0' "$file"
      fi
    done |
    xargs -0 -r gofmt -l --
)"
if [[ -n "$unformatted" ]]; then
  printf '%s\n' "$unformatted" >&2
  exit 1
fi
