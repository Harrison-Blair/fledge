---
id: FTHR-010
title: Version-mismatch warning on every command
plumage: PLM-009
status: hatching
priority: P1
depends_on: [FTHR-009]
authored: 2026-07-10T14:59:00Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# FTHR-010: Version-mismatch warning on every command

## Description
The at-a-glance staleness feedback: every fledge command except `init` and
`version`, when run anywhere inside a repo whose `.fledge/scaffold.json`
records a fledgeVersion different from the running binary, prints exactly one
line to stderr — `fledge: scaffold was written by fledge <stamp>, binary is
<binary> — run 'fledge init --refresh'` — and continues normally. stdout is
never touched, so `--json` consumers are unaffected. Repos with no stamp stay
silent (adoption is surfaced by preen in FTHR-011).

## Affected Modules
- **`internal/cli/cli.go`** — in `Run()`, after command lookup and before
  dispatch: skip for `init`/`version`; walk up from cwd looking for
  `.fledge/scaffold.json` (no git subprocess); `bootstrap.LoadStamp`; compare
  to `binaryVersion`; warn on stderr on mismatch. All errors (unreadable stamp,
  no repo) are silently ignored — the warning must never break a command.
  See `.fledge/nest/conventions.md` → command dispatch & exit codes.
- **`cmd/fledge/testdata/stamp_warning.txtar`** (new) — dedicated fixture; no
  other txtar files change (their hand-built .fledge dirs have no stamp and
  remain warning-free by design).

## Approach
- The check reads at most one small JSON file per invocation — sub-millisecond;
  no caching needed.
- Warning text is a single line, stable enough to grep in tests but not part of
  any machine contract (stderr only).
- Exit codes are unaffected; the warning is advisory.

## Tests
Written test-first: (1) write; (2) observe FAIL against unchanged code for the
expected reason; (3) implement until green.
- **`cmd/fledge/testdata/stamp_warning.txtar`**
  - init a repo, sed the stamp's fledgeVersion to 0.0.1 → `fledge preen` exits 0
    and stderr matches the mismatch line; `fledge preen --json` stdout parses as
    JSON (assert first byte `{` via grep) while stderr still warns.
  - `fledge version` and `fledge init` with the same mismatched stamp → no
    warning on stderr.
  - matching stamp → silent; deleted stamp → silent.
  - run from a subdirectory → warning still fires (upward walk).
- Whole `go test ./...` + `go vet ./...` green.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: On stamp/binary version mismatch, every command except `init` and `version` emits the one-line stderr warning naming both versions and suggesting `fledge init --refresh`, from any depth inside the repo (satisfies PLM-009 FC-2).
- [x] AC-3: stdout is byte-identical with and without the warning (`--json` output remains valid), exit codes are unchanged, and matching-stamp and no-stamp repos produce no warning.
- [x] AC-4: `go test ./...` and `go vet ./...` pass.
