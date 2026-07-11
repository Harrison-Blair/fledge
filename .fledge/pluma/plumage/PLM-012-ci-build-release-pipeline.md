---
id: PLM-012
title: "CI Build & Release Pipeline"
status: fledged
priority: P1
authored: 2026-07-11T05:25:19Z
agent: fledge-orchestrate/planning
fledge_version: 0.4.0
---

# PLM-012: CI Build & Release Pipeline

## Context
fledge is currently built and installed by hand (`go build`, `go install`, or `scripts/install.sh`) — there is no CI and no published releases. Every merge to `main` risks landing unformatted or vet-failing code, since nothing enforces `gofmt`/`go vet` today. This plumage adds GitHub Actions automation so (a) every pull request against `main` is required to pass a lint gate (formatting + vet) and the existing build/test suite before it can merge, and (b) merges to `main` that bump the `VERSION` file automatically cut a GitHub Release with cross-compiled binaries attached. It is a prerequisite for the planned `fledge update` command (a later plumage), which needs real GitHub Releases with predictably-named assets to check against and download from.

## User Stories
- As a maintainer, I want pull requests to `main` to fail automatically if the code isn't formatted or `go vet` reports a problem, so that improperly formatted code can never merge.
- As a maintainer, I want a GitHub Release with prebuilt binaries to be cut automatically whenever I bump the `VERSION` file and merge, so that I don't have to manually cross-compile and upload artifacts for every release.
- As a user installing fledge, I want each release to include binaries for my platform plus a checksum file, so that I can verify and use a prebuilt binary instead of building from source.

## Functional Criteria
1. FC-1: A GitHub Actions workflow runs on every pull request targeting `main` and executes, as required status checks: `gofmt -l` (fails if any tracked `.go` file is unformatted), `go vet ./...`, `go build ./...`, and `go test ./...`.
2. FC-2: Branch protection on `main` requires the PR check workflow's jobs to pass before a pull request can merge.
3. FC-3: A second GitHub Actions workflow runs on every push to `main` (i.e. after merge) and re-runs the same lint + build + test checks as a safety net.
4. FC-4: If the post-merge safety-net checks pass AND the `VERSION` file's contents differ from its value on the previous commit to `main`, the workflow cross-compiles the `fledge` binary for `linux/amd64`, `linux/arm64`, `darwin/amd64`, and `darwin/arm64`, using the same ldflag version-injection `scripts/install.sh` already uses. (`windows/amd64` was dropped on 2026-07-11: fledge is Unix-only — `internal/cli/brood.go`'s `pidAlive` uses `syscall.Kill`, which does not compile on Windows.)
5. FC-5: If the post-merge safety-net checks pass but `VERSION` did NOT change, no release is created (the build/test/lint re-run still occurs and must pass, but the pipeline stops there).
6. FC-6: The workflow creates a GitHub Release tagged `v<VERSION>` (e.g. `v0.4.0`), with GitHub's auto-generated release notes (`--generate-notes`), and attaches: one archive per platform named `fledge_<GOOS>_<GOARCH>[.exe-containing-archive]`, plus a `checksums.txt` covering all attached archives.
7. FC-7: If any job in the post-merge safety-net workflow fails (lint, build, or test), no release is created and no tag is pushed.

## Acceptance Criteria
- [x] AC-1: A pull request that introduces an unformatted `.go` file, a `go vet` violation, a build failure, or a failing test is blocked from merging by a required status check on `main`.
- [x] AC-2: A pull request with clean formatting, vet, build, and tests is allowed to merge.
- [x] AC-3: Merging a PR to `main` that does NOT change `VERSION` runs the post-merge workflow (lint/build/test) but produces no new GitHub Release or tag.
- [x] AC-4: Merging a PR to `main` that DOES change `VERSION` (e.g. `0.4.0` → `0.4.1`) produces a GitHub Release tagged `v0.4.1` with auto-generated notes and four binary archives (`linux_amd64`, `linux_arm64`, `darwin_amd64`, `darwin_arm64`) plus a `checksums.txt`, all attached to the release. (`windows_amd64` dropped 2026-07-11 — fledge is Unix-only.)
- [x] AC-5: Each attached binary, when downloaded and run on its target platform, reports the correct version via `fledge version` (ldflag-injected, matching the release's `VERSION`).
- [x] AC-6: If the post-merge safety-net lint/build/test re-run fails after a `VERSION`-changing merge, no release/tag is created.

## Out of Scope
- Homebrew tap, Docker image, or any package-manager (apt/scoop/etc.) publishing.
- Any registry push (e.g. Docker Hub, GHCR).
- The `fledge update` CLI command that consumes these releases (separate plumage).
- The local pre-commit hook mirroring this lint gate (separate plumage).
- Automatic `VERSION` bumping (bumping remains a manual, human decision per this plumage's Q1 answer).

## Open Questions
None — all decisions resolved during interrogation. Note: FC-2/AC-1 (branch protection enforcement) is a manual GitHub repository setting, not something a workflow file can configure by itself — carried forward as a note for feather authoring on how AC-1/AC-2 get verified.
