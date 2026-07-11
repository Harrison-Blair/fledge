---
id: FTHR-024
title: "Release workflow — widen to full 5-platform build matrix"
plumage: PLM-012
status: hatching
priority: P2
depends_on: [FTHR-023]
authored: 2026-07-11T05:58:44Z
agent: fledge-orchestrate/planning
fledge_version: 0.4.0
---

# FTHR-024: Release workflow — widen to full 5-platform build matrix

## Description
Widen FTHR-023's proven single-platform (`linux/amd64`) release mechanism in `.github/workflows/release.yml` to build and attach all 5 target platforms: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`. Each is packaged as an archive named `fledge_<GOOS>_<GOARCH>[.exe]`, and `checksums.txt` is extended to cover all 5. This is a same-mechanism widening (no new release logic — just a build matrix over the already-proven single-platform path from FTHR-023), fully satisfying PLM-012 AC-4 and AC-5.

## Affected Modules
Same as FTHR-023: `.github/workflows/release.yml` (extends the existing build step into a `strategy: matrix` over the 5 `GOOS`/`GOARCH` pairs), `internal/ciconfig/release_workflow_test.go` (extends the existing structural test).

## Approach
1. In `.github/workflows/release.yml`'s release job, replace the single `GOOS=linux GOARCH=amd64` build step with a `strategy: matrix: include: [{goos: linux, goarch: amd64}, {goos: linux, goarch: arm64}, {goos: darwin, goarch: amd64}, {goos: darwin, goarch: arm64}, {goos: windows, goarch: amd64}]` (or equivalent), building each with the same ldflag version injection, naming each archive `fledge_<goos>_<goarch>[.exe]` (Windows binaries get a `.exe` inside the archive; archive itself can still be `.zip` for Windows, `.tar.gz` for the rest — pick per platform convention).
2. Aggregate all 5 archives' SHA-256 sums into one `checksums.txt` (a job needs to collect all matrix outputs before creating the release — e.g. build job produces artifacts, a separate release job downloads all of them, computes/merges `checksums.txt`, and calls `gh release create` once with all 5 archives + the combined checksums file).
3. Extend `internal/ciconfig/release_workflow_test.go`: assert all 5 `goos`/`goarch` matrix entries are present in the workflow's build step.

## Tests
- Extended `internal/ciconfig/release_workflow_test.go`: `TestReleaseWorkflow_BuildsAllFivePlatforms` — written first against the not-yet-widened `release.yml` (fails: only `linux/amd64` present), then passing once all 5 entries are added. Pins PLM-012 FC-4 (full matrix).
- Real-world evidence: since FTHR-023 already establishes the discipline of only cutting real, intended releases (never throwaway tags), this feather's evidence is gathered from the next real `VERSION`-bumping release after this feather merges — inspect that release's assets and confirm all 5 expected archives plus a `checksums.txt` covering all 5 are attached, and spot-check at least the `linux_amd64` and one other platform's binary (e.g. `darwin_arm64`, if a Mac is available, or verify via `file`/checksum inspection alone if not) reports the correct version.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation (matrix incomplete) and pass after (all 5 entries present).
- [ ] AC-2: The next real `VERSION`-bumping release after this feather merges attaches exactly 5 binary archives (`linux_amd64`, `linux_arm64`, `darwin_amd64`, `darwin_arm64`, `windows_amd64`) plus one `checksums.txt` covering all 5 (satisfies PLM-012 AC-4 fully).
- [ ] AC-3: Each archive's checksum in `checksums.txt` matches its actual downloaded content (satisfies PLM-012 AC-5, at least for the platforms practically runnable during review).
