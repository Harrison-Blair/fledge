---
id: FTHR-022
title: PR check workflow (lint/build/test) with required branch protection
plumage: PLM-012
status: pipping
priority: P1
depends_on: []
oversight: merge
authored: 2026-07-11T05:54:02Z
agent: fledge-orchestrate/planning
fledge_version: 0.4.0
---

# FTHR-022: PR check workflow (lint/build/test) with required branch protection

## Description
Add a GitHub Actions workflow that runs on every pull request targeting `main`, executing the lint gate (`gofmt -l .`, `go vet ./...`) plus the existing build and test suite (`go build ./...`, `go test ./...`). Configure `main`'s branch protection to require this workflow's checks before a PR can merge. This is PLM-012's PR-gating tracer bullet: a complete, independently verifiable slice proving pull requests are genuinely blocked by red checks, satisfying FC-1, FC-2, AC-1, and AC-2 on its own, with no dependency on the release-side work (FTHR-b/FTHR-c).

## Affected Modules
Per `.fledge/nest/entry-points.md` (build/test commands) and `.fledge/nest/conventions.md` (no existing lint config; documented checks are `go build ./...`, `go vet ./...`, `go test ./...`):
- New: `.github/workflows/pr-check.yml` — the workflow file itself.
- New: `internal/ciconfig/workflow_test.go` (new small package, test-only) — structural validation of the workflow YAML using the already-vendored `goccy/go-yaml` dependency (see `.fledge/nest/dependencies.md`).
- GitHub repository setting: `main` branch protection rule (via `gh api repos/:owner/:repo/branches/main/protection`), not a file in the tree — configured as part of this feather's Approach, verified as evidence rather than by a Go test.

## Approach
1. Author `.github/workflows/pr-check.yml`: `on: pull_request: branches: [main]`; a single job (or a few steps in one job — no need for a matrix here since this always runs on one Linux runner) running, in order: `gofmt -l .` (fail the step if output is non-empty — e.g. `test -z "$(gofmt -l .)"`), `go vet ./...`, `go build ./...`, `go test ./...`.
2. Author `internal/ciconfig/workflow_test.go` (package `ciconfig`, test-only — no production code needed since this validates a static repo file, not runtime behavior): unmarshal `.github/workflows/pr-check.yml` with `goccy/go-yaml` into a minimal anonymous struct capturing `on.pull_request.branches` and each job's `steps[].run`, then assert `main` is in the trigger branches and that `gofmt -l`, `go vet ./...`, `go build ./...`, and `go test ./...` each appear in some step's `run` string.
3. Using `gh api`, set `main`'s required status checks to include this workflow's job name(s), and enable "Require status checks to pass before merging."
4. Evidence gathering (real GitHub state, captured in `.fledge/molt/FTHR-022.md`): push a scratch branch with a deliberately unformatted `.go` file, open a PR, confirm the check reports red; fix the formatting, confirm it turns green; confirm `gh api repos/:owner/:repo/branches/main/protection --jq '.required_status_checks'` shows this workflow's check as required. Close/clean up the scratch PR/branch afterward.

## Tests
- `internal/ciconfig/workflow_test.go` (new): `TestPRCheckWorkflow_TriggersOnMainPRs` and `TestPRCheckWorkflow_RunsLintBuildTest` — written first against the not-yet-existing `pr-check.yml` (fails: file not found), then passing once the workflow is authored with the right trigger and step commands. Pins FC-1.
- Real-world evidence (not a `go test`, captured verbatim in the molt evidence file per the agreed CI test strategy): a scratch PR demonstrating the check going red on an unformatted file and green once fixed, plus the `gh api` branch-protection query showing the check is required. Pins FC-2, AC-1, AC-2.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation (workflow file missing) and pass after (workflow file present with correct trigger/steps).
- [ ] AC-2: A real scratch PR with an unformatted `.go` file is blocked from merging by this workflow's required status check (satisfies PLM-012 AC-1).
- [ ] AC-3: The same scratch PR, once its formatting is fixed (and assuming vet/build/test are clean), shows the check passing (satisfies PLM-012 AC-2).
- [ ] AC-4: `gh api repos/:owner/:repo/branches/main/protection` shows this workflow's job(s) listed under required status checks (satisfies PLM-012 FC-2).
