---
id: FTHR-026
title: "fledge update — version check, release notes, and confirm prompt"
plumage: PLM-014
status: fledged
priority: P1
depends_on: []
authored: 2026-07-11T06:04:51Z
agent: fledge-orchestrate/planning
fledge_version: 0.4.0
---

# FTHR-026: fledge update — version check, release notes, and confirm prompt

## Description
Register a new `update` CLI command that queries the GitHub Releases API for the latest release, compares it against the running binary's version, and either reports "already up to date" or shows the current/latest version plus the release notes and prompts the user to confirm. Also implement `--yes` (skip the prompt) and `--json` (structured dry-run output, never proceeds). This feather deliberately stops at the consent boundary — it performs no download and no binary replacement (that is FTHR-027/FTHR-ii) — proving the whole check/display/consent layer of PLM-014 end-to-end as a tracer bullet.

## Affected Modules
Per `.fledge/nest/entry-points.md` (17-command `commandOrder` self-registration pattern in `internal/cli/cli.go`) and `.fledge/nest/conventions.md` (`--json` convention, `fail()`/`usageErr()` helpers, `ExitOK/Fail/Usage/Env` codes):
- New: `internal/cli/update.go` — registers the `update` command following the existing `init() { register(...) }` pattern.
- New: `internal/cli/update_test.go`.
- No changes to any existing command file.

## Approach
1. Define a small `releaseChecker` seam: a function type / small interface taking a base URL (default `https://api.github.com`, overridable via an unexported package variable or constructor param for tests) that fetches `GET {baseURL}/repos/Harrison-Blair/fledge/releases/latest` and decodes JSON into a struct with `TagName`, `Body`, `Assets []{Name, BrowserDownloadURL}`. Using stdlib `net/http` + `encoding/json` only (no new dependency).
2. `update` command logic: call the checker; strip the `v` prefix from `TagName`; compare to the existing `binaryVersion` (reuse whatever `internal/cli/version.go` already exposes) via exact string equality.
   - Equal: print `fledge is already up to date (v<version>)`, return `ExitOK`. No prompt.
   - Not equal, `--json` set: print a JSON object `{current, latest, upToDate: false, notes}` to stdout, return `ExitOK`. No prompt, no further action.
   - Not equal, `--json` not set: print current version, latest version, and the notes (`Body`), then prompt `Update to v<latest>? [y/N]: ` reading from an injectable `io.Reader` (default `os.Stdin`) — default-deny (only an explicit `y`/`yes`, case-insensitive, proceeds).
   - `--yes` flag: skip the prompt, proceed as if confirmed.
   - On confirm (via prompt or `--yes`): for this feather, print a placeholder line (e.g. `(update mechanics not yet implemented)`) and return `ExitOK` — FTHR-027 replaces this placeholder with the real download/verify/swap. This keeps FTHR-026 self-contained and testable without needing FTHR-027's logic to exist.
   - On decline: print nothing further, return `ExitOK` (declining is not an error).
3. Wire stdout/stdin through injectable fields (matching how other commands already take `io.Writer`/args, if such a pattern exists in `internal/cli` — reuse it; otherwise add minimal params) so tests can supply fake input and capture output without touching the real terminal.

## Tests
`internal/cli/update_test.go`, using `httptest.NewServer` to serve a canned JSON response (no real network call), written first against the not-yet-existing `update` command (fails: `update` not a registered command), then passing once authored:
- `TestUpdate_AlreadyUpToDate` — server reports the same version as `binaryVersion`; expect the up-to-date message, `ExitOK`, no prompt. Pins FC-3.
- `TestUpdate_NewerAvailable_PromptsAndShowsNotes` — server reports a newer version with a notes body; expect current/latest/notes printed and a prompt written to stdout. Pins FC-1, FC-2, FC-4.
- `TestUpdate_ConfirmYes` — fake stdin `"y\n"`; expect the placeholder confirm-path output, `ExitOK`. Pins FC-4.
- `TestUpdate_ConfirmDefaultDeny` — fake stdin `"\n"` (bare Enter); expect no update action taken, `ExitOK`. Pins FC-4.
- `TestUpdate_YesFlagSkipsPrompt` — `--yes`, no stdin needed; expect the placeholder confirm-path output directly. Pins FC-5.
- `TestUpdate_JSONFlagIsDryRun` — `--json`; expect valid JSON with `current`, `latest`, `upToDate`, `notes` fields, and no prompt/action regardless of whether a newer version exists (run once up-to-date, once not). Pins FC-6.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: Running `update` when already on the latest release prints the up-to-date message and exits 0, with no prompt (satisfies PLM-014 AC-1).
- [x] AC-3: Running `update` when a newer release exists prints the current version, latest version, and release notes, then prompts for confirmation (satisfies PLM-014 AC-2).
- [x] AC-4: Answering the prompt with anything other than `y`/`yes` (including a bare Enter) does not proceed to the confirm-path action (satisfies PLM-014 AC-3, partial — the "no changes to installed binary" half of AC-3 is trivially true here since this feather never touches the binary at all).
- [x] AC-5: `--json` outputs valid JSON containing current version, latest version, up-to-date boolean, and release notes, and never prompts, whether or not an update is available (satisfies PLM-014 AC-6).
