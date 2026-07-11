---
id: FTHR-023
title: "Release workflow — single-platform tracer slice (linux/amd64)"
plumage: PLM-012
status: hatching
priority: P1
depends_on: []
oversight: merge
authored: 2026-07-11T05:57:03Z
agent: fledge-orchestrate/planning
fledge_version: 0.4.0
---

# FTHR-023: Release workflow — single-platform tracer slice (linux/amd64)

## Description
Add a GitHub Actions workflow that runs on every push to `main`: it re-runs the lint+build+test safety net (identical commands to FTHR-022's PR check, as a defense against a direct push or admin-merge bypassing PR checks), then detects whether the `VERSION` file changed versus the previous commit on `main`. If it changed, it cross-compiles the `fledge` binary for `linux/amd64` only (deliberately narrow — this feather proves the entire release mechanism end-to-end; FTHR-c widens it to all 5 platforms), tags the release `v<VERSION>`, creates a GitHub Release with auto-generated notes (`--generate-notes`), and attaches the single binary archive plus a `checksums.txt` covering it. This is PLM-012's release-mechanism tracer bullet: independent of FTHR-022 (different workflow file, no shared code), it proves FC-3 through FC-7 end-to-end on the narrowest possible build matrix.

## Affected Modules
Per `.fledge/nest/entry-points.md` (`VERSION` file, ldflag version injection pattern already used by `scripts/install.sh`) and PLM-012's frontmatter/Context (release notes via `--generate-notes`, tag format `v<VERSION>`):
- New: `.github/workflows/release.yml` — the workflow file itself.
- New: `internal/ciconfig/release_workflow_test.go` (same new `ciconfig` package as FTHR-022) — structural validation of `release.yml`.
- `VERSION` (root, read-only reference — this feather never modifies it, only reads its current and previous-commit values to detect a change).
- GitHub state: a release/tag is created on the real repo as part of evidence-gathering — not a file in the tree, verified as evidence.

## Approach
1. Author `.github/workflows/release.yml`: `on: push: branches: [main]`.
2. First job: re-run `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...` (the safety net) — must pass before anything else proceeds.
3. Detect `VERSION` change: compare the `VERSION` file's content at `HEAD` against its content at `HEAD^` (e.g. `git show HEAD^:VERSION` vs. `cat VERSION`); expose as a job output/step condition gating the release job.
4. If changed: build for `GOOS=linux GOARCH=amd64` using the same ldflag version-injection as `scripts/install.sh` (`-X .../internal/cli.binaryVersion=$VERSION`), package it into an archive (e.g. `fledge_linux_amd64.tar.gz`), compute its SHA-256 into `checksums.txt`, then use `gh release create v$VERSION --generate-notes <archive> checksums.txt`.
5. If the safety-net checks fail, or `VERSION` did not change, no release/tag is created (the workflow simply completes without the release job running).
6. Author `internal/ciconfig/release_workflow_test.go`: parse `release.yml` with `goccy/go-yaml`, assert the `push`/`main` trigger, assert the safety-net commands appear, and assert the workflow contains a `linux`/`amd64` build step and a `gh release create` (or equivalent release-creation) step referencing `--generate-notes`.
7. Evidence gathering (real GitHub state, captured in `.fledge/molt/FTHR-023.md`): (a) merge a scratch change that does NOT bump `VERSION` — confirm no new release/tag appears; this path has no side effects and may use any throwaway scratch commit. (b) The VERSION-change evidence path must NOT use a throwaway/reverted tag — a published GitHub Release and tag cannot be cleanly undone. Instead, gather this evidence using the actual next intended release bump (e.g. the real next `VERSION` increment the project genuinely intends to ship), timed/coordinated with the maintainer: merge that real bump to `main`, confirm a real GitHub Release is created at `v<VERSION>` with auto-generated notes, one `linux_amd64` archive, and a `checksums.txt`; download the archive, verify its checksum matches, run it, and confirm `fledge version` reports the expected version. This is the one and only release cut during this feather's evidence-gathering, and it is a real, intended release.

## Tests
- `internal/ciconfig/release_workflow_test.go` (new): `TestReleaseWorkflow_TriggersOnPushToMain`, `TestReleaseWorkflow_RunsSafetyNet`, `TestReleaseWorkflow_BuildsLinuxAmd64AndReleases` — written first against the not-yet-existing `release.yml` (fails: file not found), then passing once authored. Pins FC-3, FC-4 (single-platform slice), FC-6.
- Real-world evidence (per the agreed CI test strategy): a no-VERSION-change scratch merge showing no release appears (pins FC-5, AC-3; no side effects, any throwaway commit is fine), and the actual next intended real VERSION-bump merge showing a real release with correct tag, notes, one archive, and checksums.txt, with the downloaded binary verified to run and report the right version (pins FC-4, FC-6, FC-7, AC-4 (single-platform slice), AC-6) — this real bump is the only release cut during evidence-gathering, never a disposable test tag.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation (workflow file missing) and pass after.
- [ ] AC-2: A real merge to `main` that does not change `VERSION` produces no new GitHub Release or tag (satisfies PLM-012 AC-3).
- [ ] AC-3: The actual next intended real merge to `main` that changes `VERSION` produces a GitHub Release tagged `v<VERSION>` with auto-generated notes and one `linux_amd64` binary archive plus a `checksums.txt` (partially satisfies PLM-012 AC-4 — full 5-platform coverage is FTHR-c's job).
- [ ] AC-4: The released `linux_amd64` binary, downloaded and run, reports the correct version via `fledge version` (partially satisfies PLM-012 AC-5).
- [ ] AC-5: If the safety-net lint/build/test re-run fails, no release/tag is created even if `VERSION` changed (satisfies PLM-012 AC-6, FC-7).
