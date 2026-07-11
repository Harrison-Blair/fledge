# FTHR-023 evidence

## AC-1

Tests written first against the not-yet-existing `.github/workflows/release.yml`.

Command:

```
go test ./internal/ciconfig/... -run TestReleaseWorkflow -v
```

Pre-implementation output (FAILING, expected reason — workflow file missing):

```
=== RUN   TestReleaseWorkflow_TriggersOnPushToMain
    release_workflow_test.go:65: reading ../../.github/workflows/release.yml: open ../../.github/workflows/release.yml: no such file or directory
--- FAIL: TestReleaseWorkflow_TriggersOnPushToMain (0.00s)
=== RUN   TestReleaseWorkflow_RunsSafetyNet
    release_workflow_test.go:98: reading ../../.github/workflows/release.yml: open ../../.github/workflows/release.yml: no such file or directory
--- FAIL: TestReleaseWorkflow_RunsSafetyNet (0.00s)
=== RUN   TestReleaseWorkflow_BuildsLinuxAmd64AndReleases
    release_workflow_test.go:110: reading ../../.github/workflows/release.yml: open ../../.github/workflows/release.yml: no such file or directory
--- FAIL: TestReleaseWorkflow_BuildsLinuxAmd64AndReleases (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/ciconfig	0.001s
FAIL
```

Post-implementation output (PASSING), after authoring `.github/workflows/release.yml`:

```
=== RUN   TestReleaseWorkflow_TriggersOnPushToMain
--- PASS: TestReleaseWorkflow_TriggersOnPushToMain (0.00s)
=== RUN   TestReleaseWorkflow_RunsSafetyNet
--- PASS: TestReleaseWorkflow_RunsSafetyNet (0.00s)
=== RUN   TestReleaseWorkflow_BuildsLinuxAmd64AndReleases
--- PASS: TestReleaseWorkflow_BuildsLinuxAmd64AndReleases (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.002s
```

Full-repo safety net after the change (confirms no regressions):

```
$ go build ./... && go vet ./... && go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.144s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.010s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.005s
```

## AC-2 / AC-3 / AC-4

**Not yet gathered.** Per the feather spec and spawn instructions, these require cutting a real GitHub Release/tag on the live repo:

- AC-2 needs a real merge to `main` that does NOT change `VERSION`, confirmed to produce no new Release/tag (no side effects; any throwaway commit is fine for this half).
- AC-3 / AC-4 need the actual next intended real `VERSION` bump merged to `main` — never a throwaway/reverted tag — with the resulting Release, tag, notes, archive, and checksums verified, and the downloaded binary run to confirm `fledge version` reports the expected version.

This is a live-GitHub action outside this brooder's authority to perform autonomously. Awaiting maintainer coordination on the real version bump (see handoff message to team-lead).

## AC-5

**Not yet gathered** — depends on exercising the workflow's safety-net-failure path on the real repo (or is assessed by code review of the job dependency graph: `release` job `needs: detect-version`, which in turn `needs: safety-net`, so a safety-net failure short-circuits the whole chain before `detect-version` or `release` can run). Structural verification: see `.github/workflows/release.yml` job graph (`safety-net` → `detect-version` → `release`).

## Live verification (post-merge, orchestrator, 2026-07-11)
- AC-2 (no release when VERSION unchanged): push of merge to main, Release run 29142889316 → safety-net + detect-version green, Build-and-release SKIPPED, `gh release list` empty.
- AC-5 (safety-net failure blocks release): v0.5.0 push, run 29143033866 → `go test` FAILED (stamp_warning fixture), release NOT cut.
- AC-3 (release on VERSION change): v0.5.1 push, run 29143130884 → all three jobs green, release v0.5.1 created.
- AC-4 (binary reports correct version): downloaded fledge_linux_amd64.tar.gz from v0.5.1, checksums.txt verified OK, extracted binary reports `fledge 0.5.1`.
