---
id: FTHR-025
title: "Local pre-commit lint hook + setup docs"
plumage: PLM-013
status: pipping
priority: P2
depends_on: []
authored: 2026-07-11T06:01:41Z
agent: fledge-orchestrate/planning
fledge_version: 0.4.0
---

# FTHR-025: Local pre-commit lint hook + setup docs

## Description
Add a `pre-commit` git hook script that runs `gofmt -l .` then `go vet ./...`, blocking the commit on any output from either (read-only — never rewrites files), and document the one-time `git config core.hooksPath scripts/hooks` step needed to activate it. This delivers all of PLM-013's functional and acceptance criteria in one self-contained slice: a single small shell script plus one doc addition, with no natural sub-boundary to split further.

## Affected Modules
Per `.fledge/nest/conventions.md` (no existing lint config or hook framework) and PLM-013's Context (mirrors PLM-012's CI lint commands exactly):
- New: `scripts/hooks/pre-commit` — the hook script.
- Modified: `CLAUDE.md` (and/or `README.md`) — add the one-time `git config core.hooksPath scripts/hooks` setup step.
- New: `internal/hooktest/precommit_test.go` (new test-only package) — drives a real temporary git repo via `os/exec` to exercise the hook end-to-end.

## Approach
1. Write `scripts/hooks/pre-commit` (POSIX shell, executable bit set): run `gofmt -l .`; if output is non-empty, print it with a note to run `gofmt -w`, and `exit 1`. Otherwise run `go vet ./...`; if it reports anything (non-zero exit or output), print its output and `exit 1`. Otherwise `exit 0`. The script never modifies any file — purely read-only checks.
2. Add a short "Local git hooks" section to `CLAUDE.md` (near the existing Build/test section) documenting: `git config core.hooksPath scripts/hooks` as a one-time, manual, per-clone setup step; note it is optional/opt-in, not run by `fledge init` or `scripts/install.sh`.
3. Write `internal/hooktest/precommit_test.go`: for each test, create a fresh `t.TempDir()`, `git init` it, copy `scripts/hooks/pre-commit` in, `git config core.hooksPath` to point at it, commit an initial clean file to have a valid HEAD, then exercise the scenarios below by writing a file, `git add`-ing it, and running `git commit` via `os/exec`, asserting on the resulting exit code and captured stdout/stderr.

## Tests
Written first against the not-yet-existing `scripts/hooks/pre-commit` (all fail: hook script missing, so `core.hooksPath` points at nothing and commits are never blocked), then passing once the script is authored:
- `TestPreCommitHook_BlocksUnformattedFile` — stage a `.go` file with bad `gofmt` formatting; commit must fail (non-zero exit) and the hook's stdout/stderr must name the offending file. Pins FC-3, AC-1.
- `TestPreCommitHook_BlocksVetViolation` — stage a `.go` file that's `gofmt`-clean but fails `go vet` (e.g. a `Printf` format-string mismatch); commit must fail and the hook's output must show the vet diagnostic. Pins FC-4, AC-2.
- `TestPreCommitHook_AllowsCleanCommit` — stage a fully clean `.go` file; commit must succeed, and the file's content on disk must be byte-identical before and after (hook never mutates anything). Pins FC-5, FC-6, AC-3.
- `TestPreCommitHook_NoOpWithoutHooksPathConfigured` — same bad-formatting file, but `core.hooksPath` is left at its default (unset); commit must succeed (hook never runs). Pins AC-4.
- `TestPreCommitHook_MatchesCICommands` — a lightweight assertion (string comparison or shared constant) that the hook's `gofmt -l` and `go vet ./...` invocations are textually identical to what PLM-012's `pr-check.yml`/`release.yml` run — pins AC-5. (If PLM-012's feathers haven't merged yet when this runs, this assertion can compare against the literal command strings specified in both plumages' FCs rather than reading the actual workflow files — no `depends_on` needed since it's a text/constant comparison, not a runtime dependency.)

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after.
- [ ] AC-2: With `core.hooksPath` set, attempting to commit a change that includes an unformatted `.go` file is blocked, and the hook's output names the unformatted file(s) (satisfies PLM-013 AC-1).
- [ ] AC-3: With `core.hooksPath` set, attempting to commit a change that introduces a `go vet` violation is blocked, and the hook's output shows `go vet`'s diagnostic (satisfies PLM-013 AC-2).
- [ ] AC-4: With `core.hooksPath` set, a commit whose tree is fully clean succeeds, and no file is modified by the hook (satisfies PLM-013 AC-3).
- [ ] AC-5: Without `core.hooksPath` configured, commits are unaffected by this hook (satisfies PLM-013 AC-4).
- [ ] AC-6: The hook's two commands are textually identical to PLM-012's CI lint commands (satisfies PLM-013 AC-5).
