---
id: PLM-014
title: fledge update Command
status: fledged
priority: P1
authored: 2026-07-11T05:44:38Z
agent: fledge-orchestrate/planning
fledge_version: 0.4.0
---

# PLM-014: fledge update Command

## Context
PLM-012 makes fledge releases exist as versioned, checksummed GitHub Releases with predictably-named binary assets, but there is still no way to install a new release except manually (`go install ...@latest` or rebuilding from source). This plumage adds a `fledge update` CLI subcommand that checks GitHub for the latest release, shows the user what changed via the release notes, and — only on explicit confirmation — downloads, verifies, and atomically swaps in the new binary in place of the currently-running one. It is the first fledge command to make a network call, so it is scoped narrowly: user-invoked only, no background checking, no auth/proxy handling, latest-release-only (no version pinning or rollback).

## User Stories
- As a fledge user, I want to run `fledge update` and see whether a newer version exists and what changed in it, so that I can decide whether to update.
- As a fledge user, I want to explicitly confirm before my installed binary is replaced, so that an update never happens without my say-so.
- As a fledge user scripting an automated environment, I want a way to skip the interactive prompt, so that I can update non-interactively when I already intend to.

## Functional Criteria
1. FC-1: `fledge update` calls the GitHub Releases API (`GET /repos/Harrison-Blair/fledge/releases/latest` — unauthenticated, public-repo read access) to get the latest release's `tag_name`, `body` (release notes), and `assets[]`.
2. FC-2: `fledge update` compares the running binary's version (the same value `fledge version` reports) against the latest release's `tag_name` with its `v` prefix stripped, using exact string equality (no semver ordering/ranges).
3. FC-3: If the versions are equal, `fledge update` prints `fledge is already up to date (v<version>)` and exits 0 — no prompt, no download.
4. FC-4: If a newer version is available, `fledge update` prints the current version, the latest version, and the release's notes (`body`), then prompts `Update to v<version>? [y/N]:` on stdin — default-deny (anything other than an explicit `y`/`yes` answer aborts with no changes made).
5. FC-5: `fledge update --yes` skips the interactive prompt and proceeds as if the user confirmed.
6. FC-6: `fledge update --json` prints a JSON object with the current version, latest version, up-to-date boolean, and release notes, and performs no download, prompt, or replacement regardless of whether a newer version exists (dry-run / structured-output mode, consistent with `--json`'s existing meaning across the CLI).
7. FC-7: On confirmed update, `fledge update` downloads the release asset matching `runtime.GOOS`/`runtime.GOARCH` (asset naming `fledge_<GOOS>_<GOARCH>[.exe]` per PLM-012), extracts the binary, and verifies its SHA-256 against the matching line in that release's `checksums.txt` asset — on mismatch, aborts with an error and makes no changes to the installed binary.
8. FC-8: On successful verification, `fledge update` writes the new binary to a temp file in the same directory as `os.Executable()`'s resolved path, marks it executable, and atomically replaces the original via `os.Rename` — the original binary is left untouched unless and until this final rename succeeds.
9. FC-9: On any error before the final rename (network failure, checksum mismatch, missing asset for the current platform, etc.), `fledge update` exits non-zero with a descriptive error and the currently-installed binary is unchanged.

## Acceptance Criteria
- [x] AC-1: Running `fledge update` when already on the latest release prints the up-to-date message and exits 0, with no network write/download and no prompt.
- [x] AC-2: Running `fledge update` when a newer release exists prints the current version, latest version, and release notes, then prompts for confirmation.
- [x] AC-3: Answering the prompt with anything other than `y`/`yes` (including a bare Enter) aborts with no changes to the installed binary.
- [x] AC-4: Answering `y` (or running with `--yes`) downloads the correct platform asset, verifies its checksum against `checksums.txt`, and replaces the running binary such that a subsequent `fledge version` reports the new version.
- [x] AC-5: If the downloaded asset's SHA-256 does not match `checksums.txt`, the command aborts with an error and the pre-existing binary is provably unchanged (e.g. unchanged file mtime/checksum).
- [x] AC-6: `fledge update --json` outputs valid JSON containing current version, latest version, up-to-date boolean, and release notes, and never downloads or replaces the binary, whether or not an update is available.

## Out of Scope
- Any background/automatic update checking on other commands — `fledge update` only runs a network check when explicitly invoked.
- Network proxy or authentication configuration for restricted environments — assumes normal outbound HTTPS access, same baseline `go install` already requires.
- Installing a specific (non-latest) version, or rolling back/downgrading to a previous version.
- Self-uninstall.
- Refreshing the scaffolded `.fledge/skills/`/`.claude/` output as part of an update — that remains `fledge init --refresh`'s job, unrelated to swapping the binary.
- GitHub authentication/token handling — the Releases API's read endpoints are unauthenticated for this public repo.
- GitHub's unauthenticated API rate limit (60 requests/hour/IP) is not specially handled — a rate-limited response simply surfaces to the user as a plain API error, with no retry or backoff logic.

## Open Questions
None — all decisions resolved during interrogation.
