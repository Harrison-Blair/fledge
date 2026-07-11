---
id: PLM-013
title: Local Lint Pre-Commit Hook
status: hatched
priority: P2
authored: 2026-07-11T05:36:31Z
agent: fledge-orchestrate/planning
fledge_version: 0.4.0
---

# PLM-013: Local Lint Pre-Commit Hook

## Context
PLM-012 adds a CI-enforced lint gate (`gofmt -l` + `go vet`) as a required PR check, but that feedback only arrives after a developer pushes and opens/updates a PR. This plumage adds a local `pre-commit` git hook that runs the identical checks before a commit is even created, so a developer learns about formatting or vet problems immediately, without waiting on CI. It is a defense-in-depth companion to PLM-012, not a replacement: the CI gate remains the enforcement backstop for anyone who hasn't installed the hook, or who bypasses it with `git commit --no-verify`.

## User Stories
- As a contributor, I want to be told immediately, at commit time, if my changes aren't `gofmt`-clean or fail `go vet`, so that I don't have to wait for CI to find out and push a fixup commit.
- As a maintainer, I want the local check to run the exact same commands as CI, so that "passes locally" and "passes in CI" never disagree.

## Functional Criteria
1. FC-1: A `pre-commit` git hook script lives at `scripts/hooks/pre-commit`, tracked in version control.
2. FC-2: Running `git config core.hooksPath scripts/hooks` (a one-time, manual, documented setup step) makes Git invoke this script before every commit in that clone.
3. FC-3: On invocation, the hook runs `gofmt -l .`; if it lists any file, the hook prints the list and exits non-zero, aborting the commit.
4. FC-4: If `gofmt -l .` reports nothing, the hook runs `go vet ./...`; if it reports any issue, the hook prints `go vet`'s output and exits non-zero, aborting the commit.
5. FC-5: If both checks report nothing, the hook exits zero and the commit proceeds normally.
6. FC-6: The hook is read-only — it never modifies, formats, or re-stages any file. A developer whose commit is blocked runs `gofmt -w` (or their editor's formatter) themselves and re-commits.
7. FC-7: The hook's checks are scoped to the whole working tree (`gofmt -l .`, `go vet ./...`), identical in command and scope to PLM-012's CI lint job — not limited to staged files.
8. FC-8: Setup (`git config core.hooksPath scripts/hooks`) is documented (in `CLAUDE.md` and/or `README.md`) as a manual, opt-in step for contributors — it is not auto-run by `fledge init`, `scripts/install.sh`, or any other automation.

## Acceptance Criteria
- [ ] AC-1: With `core.hooksPath` set to `scripts/hooks`, attempting to commit a change that includes an unformatted `.go` file is blocked, and the hook's output names the unformatted file(s).
- [ ] AC-2: With `core.hooksPath` set, attempting to commit a change that introduces a `go vet` violation is blocked, and the hook's output shows `go vet`'s diagnostic.
- [ ] AC-3: With `core.hooksPath` set, a commit whose tree is fully `gofmt`-clean and `go vet`-clean succeeds, and no file is modified by the hook.
- [ ] AC-4: Without `core.hooksPath` configured (default clone state), commits are unaffected by this hook (it does not run) — confirming setup is truly opt-in.
- [ ] AC-5: The hook's two commands (`gofmt -l .`, `go vet ./...`) are textually identical to the commands PLM-012's CI lint job runs, confirmed by inspecting both the workflow file and the hook script.

## Out of Scope
- Running `go test ./...` locally in this hook (already a required PR check in PLM-012; kept out to keep commits fast).
- A `pre-push` hook or any other git hook stage.
- Any hook-management framework (`pre-commit`, `lefthook`, `husky`, etc.) — a plain committed script + `core.hooksPath` is sufficient and adds no dependency.
- Auto-fixing/auto-formatting behavior (`gofmt -w`) — considered and explicitly rejected during interrogation once the partial-staging risk (rewriting a file with unstaged hunks and silently sweeping them into the commit on re-stage) was surfaced; the hook stays read-only and blocks instead.
- Automating the `core.hooksPath` setup step (e.g. wiring it into `scripts/install.sh` or `fledge init`) — stays a manual, documented step.

## Open Questions
None — all decisions resolved during interrogation.
