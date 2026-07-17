---
id: FTHR-091
title: Ignore the ledger directory in version control
plumage: PLM-035
status: pipping
priority: P1
depends_on: []
authored: 2026-07-17T07:59:05Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.7
---

# FTHR-091: Ignore the ledger directory in version control

## Description
Adds `.fledge/ledger/` to the gitignore block `fledge init` writes, so that ledger records — ephemeral, latest-value-only, per-run worker state — stop being candidates for commit in every fledge-managed repository.

Deliberately tiny and fully independent: one entry in a Go string slice, one assertion in an existing fixture. It runs in parallel with every other PLM-035 feather and shares no file with any of them.

**This is an omission from an established pattern, not a new policy.** `.gitignore`'s own block comment reads *"per-run intermediates — regenerable, not shared"*, and every other member of that class is already listed: `.fledge/nest/raw/`, `.fledge/broods/`, `.fledge/roster/`, `.fledge/scratch/`, `.alloc.lock`. The ledger is the only one missed. Nobody has hit it because `.fledge/ledger/` is created lazily on first write and does not yet exist in this repository — which is exactly why it is cheap to fix now and will not stay cheap.

Satisfies PLM-035 FC-10.

## Affected Modules
See `.fledge/nest/entry-points.md` → `fledge init`, and `.fledge/nest/architecture.md` on the bootstrap/adapter seam.

- `internal/cli/init.go` (~line 54) — `gitignoreLines`, the slice of entries appended as one block when any is missing. Currently `{".fledge/nest/raw/", ".fledge/broods/", ".fledge/roster/", ".fledge/scratch/", ".alloc.lock"}`. Gains `.fledge/ledger/`.
- `cmd/fledge/testdata/init.txtar` (~lines 26–30) — greps asserting each expected entry lands in a fresh repo's `.gitignore`. Gains one for the ledger, alongside the existing idempotency assertion at ~line 57 (`grep -count=1 '.fledge/broods/'`).
- **Not touched:** the bootstrap scaffold manifest. `.gitignore` is **not** a manifest-managed file — it is handled directly by `ensureGitignore` in `internal/cli/init.go`. This feather therefore does **not** edit `commandOrder`, does not regenerate any adapter template, and does not rewrite `.fledge/scaffold.json`. It shares no file with FTHR-092 (`pulse`), despite both being loosely "init-adjacent".

## Approach
- Add the one entry to `gitignoreLines`. `ensureGitignore` already handles the rest: it appends the block when any line is missing, and existing repos pick the new line up on their next `fledge init` (the `grep -count=1` fixture pins that re-running does not duplicate entries).
- **Match the established list exactly** — a trailing-slash directory entry (`.fledge/ledger/`), consistent with `.fledge/nest/raw/`, `.fledge/broods/`, `.fledge/roster/`, `.fledge/scratch/`. Not a glob, not a bare path.
- **Do not** retrofit `.gitignore` in this repository by hand. The point is that `fledge init` writes it; this repo's own copy updates when init next runs, like any other consumer's.
- Nothing else in `init.go` moves. Root `CLAUDE.md`'s surgical-changes rule: this is a one-line change and its diff should look like one.

## Tests
Test-first, with the failing-first observation **behavioral** (PLM-035 AC-2). Straightforward here: `init.txtar` drives the built binary against a fresh repo, so the assertion fails on observable output rather than on compilation.

- `cmd/fledge/testdata/init.txtar` (extend) — **source of the AC-1/AC-2 behavioral evidence:**
  - `grep '.fledge/ledger/' .gitignore` after `fledge init` in a fresh repo → fails first, because the line is not written today.
  - The existing entry assertions (~lines 26–30) must keep passing unmodified — the new line is additive, not a replacement.
  - Idempotency: re-running `fledge init` does not duplicate the ledger entry, mirroring the existing `grep -count=1 '.fledge/broods/'` assertion at ~line 57.
- Order: write the assertion, capture its verbatim behavioral failure in `.fledge/molt/FTHR-091.md`, then add the line.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: The failing-test observation is **behavioral** — captured from `init.txtar` running against a fresh repo, not a compilation error — and recorded verbatim in `.fledge/molt/FTHR-091.md` (satisfies PLM-035 AC-2).
- [x] AC-3: A freshly initialized repository's `.gitignore` contains `.fledge/ledger/`, proven by a test that fails against the current code (satisfies PLM-035 FC-10, AC-11).
- [x] AC-4: Re-running `fledge init` does not duplicate the ledger entry, proven by a count assertion mirroring the existing one for `.fledge/broods/`.
- [x] AC-5: Every gitignore entry written before this feather is still written, proven by the existing assertions passing unmodified.
- [x] AC-6: `go test ./...` is green, `go vet ./...` and `gofmt -l .` are clean, and `fledge preen` reports no errors on the branch (satisfies PLM-035 AC-13).
