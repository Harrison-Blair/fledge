---
id: FTHR-038
title: Auto-resolve tmux precondition
plumage: PLM-019
status: pipping
priority: P2
depends_on: []
authored: 2026-07-15T19:00:49Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-038: Auto-resolve tmux precondition

## Description
Rewrite `team-loop.md` § Teammate display (tmux) and `implementation.md` §1's harness-piping-preconditions bullet so the tmux precondition auto-resolves with no `confirm-gate`: degraded (in-process teammates) when tmux is unavailable, panes when it is, reported as one line of non-blocking run narration either way. The permission-mode precondition and every other `team-loop.md`/`implementation.md` section stay untouched. Satisfies PLM-019 FC-1 through FC-6.

## Affected Modules
- `internal/bootstrap/adapters/claude/team-loop.md` — only the `## Teammate display (tmux)` section changes. See `.fledge/nest/architecture.md` → "Team-loop mechanics (Claude Tier C)".
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` — only the "Tier C only — harness piping preconditions" bullet in §1 changes.
- Do NOT touch `team-loop.md`'s permission-mode paragraph (same `## Spawning and addressing teammates` section, the sentence beginning "Teammates inherit your permission mode at spawn..."), any other `team-loop.md` section (orchestrator name, spawning, shutdown, planning delegation, team task list, skill loading, recovery), or any other part of `implementation.md`. Do NOT run `go install`/`fledge init --refresh` in this feather — that resync is FTHR-D, which depends on this one.

## Approach
1. In `team-loop.md` § Teammate display (tmux), replace the "**Precondition:**" paragraph. Current: "the session is inside tmux (`test -n \"$TMUX\"`). If not, split-pane teammate display is unavailable. `implementation.md` §1 surfaces this via a `confirm-gate`: stop and restart inside tmux (recommended), or proceed degraded with in-process teammates (no panes; teammates still run, you just can't watch them in split view)." Replace with wording stating: the precondition auto-resolves with no `confirm-gate` — tmux present → proceed with panes; tmux absent → proceed degraded with in-process teammates (no panes; teammates still run) — and report which path was taken as one line of plain, non-blocking run narration (not a gate, no wait for a response), giving an example line for each path (e.g. "tmux detected — spawning teammates in panes" / "tmux not detected — proceeding degraded, in-process teammates").
2. In `implementation.md` §1, replace the "Tier C only — harness piping preconditions" bullet's fallback/gating sentence. Current: "If a precondition is unmet, your piping file states the fallback (commonly: proceed degraded with in-process teammates, or stop and restart). Never silently proceed past a precondition your piping file says to surface." Replace with wording that: (a) states some preconditions auto-resolve without a gate — the piping file says which and what each resolved path is; (b) keeps "never silently proceed" scoped only to preconditions the piping file says to gate (i.e. still applies to permission-mode, no longer implies it applies to tmux).
3. Leave `team-loop.md`'s permission-mode paragraph and every other section of both files byte-for-byte unchanged (PLM-019 FC-5, FC-6) — these are verified, not rewritten.

No Go code changes; no adapter/manifest changes; no rebuild/refresh (that's FTHR-D).

## Tests
New file `internal/bootstrap/tmux_autodefault_test.go`, package `bootstrap`, reading `adapters/claude/team-loop.md` and `core/skills/fledge-orchestrate/implementation.md` via the package's embedded `FS` (same pattern as `TestCorePrimitivesReferenced`/`TestCoreNeutral` in `registry_test.go`):

- `TestTmuxPreconditionAutoResolves` — asserts the old gating language is **absent** from `team-loop.md`'s tmux section ("surfaces this via a `confirm-gate`", "stop and restart inside tmux (recommended)"), and that replacement language is **present**: no-gate auto-resolve wording plus both example notice lines (or equivalent asserted phrases) for the panes and degraded paths.
- `TestImplementationPreconditionCarveOut` — asserts `implementation.md` §1's bullet no longer states a blanket "never silently proceed past a precondition" without qualification (i.e. the old exact sentence is absent) and that it now states some preconditions auto-resolve without a gate, while the instruction to never silently proceed past a *gated* precondition is still present in some form.
- `TestPermissionModeUnchanged` — asserts a stable sentence from `team-loop.md`'s permission-mode paragraph ("Teammates inherit your permission mode at spawn") and its confirm-gate reference ("`implementation.md` §1 surfaces the current mode via a `confirm-gate`") are both present verbatim — guarding FC-5 against incidental changes.

Implementation order: write all three tests first, run `go test ./internal/bootstrap -run "TestTmux|TestImplementationPrecondition|TestPermissionModeUnchanged"`, capture them **failing** against the unmodified files (the "absent" assertions pass, but the "present"-new-wording assertions fail — expected reason: new language doesn't exist yet), then rewrite the two sections per Approach until all three pass.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation (new-language assertions) and pass after.
- [x] AC-2: `team-loop.md` § Teammate display (tmux) states the auto-resolve behavior with no confirm-gate language remaining for this precondition (satisfies PLM-019 FC-1, FC-2) — `TestTmuxPreconditionAutoResolves` passes.
- [x] AC-3: `team-loop.md` documents the non-blocking notice for both paths (satisfies PLM-019 FC-3) — covered by `TestTmuxPreconditionAutoResolves`.
- [x] AC-4: `implementation.md` §1's precondition bullet is scoped so "never silently proceed" no longer applies to the tmux precondition (satisfies PLM-019 FC-4) — `TestImplementationPreconditionCarveOut` passes.
- [x] AC-5: The permission-mode paragraph and confirm-gate wording are unchanged verbatim (satisfies PLM-019 FC-5), and no other section of either file changed (satisfies PLM-019 FC-6, verified via `git diff` scoped to the two edited paragraphs only) — `TestPermissionModeUnchanged` passes.
- [x] AC-6: `go vet ./...` and `go test ./internal/bootstrap/...` pass with the new tests included.
