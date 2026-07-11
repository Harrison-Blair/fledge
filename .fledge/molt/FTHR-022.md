# FTHR-022 Evidence

## AC-1

Tests: `internal/ciconfig/workflow_test.go` — `TestPRCheckWorkflow_TriggersOnMainPRs`, `TestPRCheckWorkflow_RunsLintBuildTest`.

### Pre-implementation (FAILING) — `go test ./internal/ciconfig/... -v`

Run against the not-yet-created `.github/workflows/pr-check.yml`:

```
=== RUN   TestPRCheckWorkflow_TriggersOnMainPRs
    workflow_test.go:48: reading ../../.github/workflows/pr-check.yml: open ../../.github/workflows/pr-check.yml: no such file or directory
--- FAIL: TestPRCheckWorkflow_TriggersOnMainPRs (0.00s)
=== RUN   TestPRCheckWorkflow_RunsLintBuildTest
    workflow_test.go:63: reading ../../.github/workflows/pr-check.yml: open ../../.github/workflows/pr-check.yml: no such file or directory
--- FAIL: TestPRCheckWorkflow_RunsLintBuildTest (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/ciconfig	0.001s
FAIL
```

Both tests fail for the expected reason: the workflow file does not exist yet.

### Post-implementation (PASSING) — `go test ./internal/ciconfig/... -v`

After authoring `.github/workflows/pr-check.yml`:

```
=== RUN   TestPRCheckWorkflow_TriggersOnMainPRs
--- PASS: TestPRCheckWorkflow_TriggersOnMainPRs (0.00s)
=== RUN   TestPRCheckWorkflow_RunsLintBuildTest
--- PASS: TestPRCheckWorkflow_RunsLintBuildTest (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.001s
```

### Full suite sanity check — `go build ./... && go vet ./... && go test ./...`

```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.081s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.011s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.002s
```

## AC-2, AC-3, AC-4

**Not yet captured.** These require mutating the LIVE GitHub repo:

- AC-2/AC-3: pushing a scratch branch with a deliberately unformatted `.go`
  file, opening a real PR against `main`, and observing the check go red then
  green.
- AC-4: setting `main`'s branch protection via
  `gh api repos/:owner/:repo/branches/main/protection` to require this
  workflow's status check, then querying it back.

Per the brooder's spawn instructions, no live-GitHub mutation was performed
autonomously. This has been escalated to the orchestrator for user go-ahead
before proceeding.

## AC-2
Live: scratch PR #1, PR Check run 29142976458 → `lint-build-test` FAILED (gofmt step) in 9s on a deliberately unformatted `.go` file. Confirms the `pull_request` trigger fires and reports red. (PLM-012 AC-1.)

## AC-3
Live: same PR after `gofmt -w`, PR Check run 29143001019 → `lint-build-test` PASSED in 15s. Confirms green when formatting is clean. Scratch PR/branch closed and deleted afterward. (PLM-012 AC-2.)

## AC-4
WAIVED by maintainer 2026-07-11: branch-protection *enforcement* (PLM-012 FC-2) is intentionally out of scope for a single-maintainer repo — the PR trigger firing matters, not a merge-blocking rule. Enabling required status checks in GitHub settings remains an optional manual step, not a workflow deliverable. No evidence command applies; recorded as an explicit scope decision.
