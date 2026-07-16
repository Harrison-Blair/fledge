---
id: FTHR-072
title: Ledger package and heartbeat command
plumage: PLM-030
status: pipping
priority: P1
depends_on: []
authored: 2026-07-16T22:20:15Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-072: Ledger package and heartbeat command

## Description
Introduces `internal/ledger`, the new package underlying PLM-030's whole handoff ledger: `status`, `verdict`, and `escalation` record types, addressed by `(subject, kind)` and written/read atomically with latest-value semantics (one file per subject+kind, no history). Also delivers the first CLI command built on it, `fledge heartbeat <name> [--note "<text>"]`, which writes/refreshes a worker's `status` record, and the stalled-vs-busy classification helper (PID-alive check OR stale lease, 5-minute fixed TTL) that later feathers (`fledge await`, orchestration prose) depend on for their liveness story. This is the tracer-bullet feather: it is the only one with no `depends_on`, and it proves the full architecture end to end (package → CLI → JSON → classification) on a single handoff type before FTHR-073/074 widen to `await` and `verdict`/`escalation`.

## Affected Modules
- New package `internal/ledger` (`internal/ledger/ledger.go` or split as needed) — see `.fledge/nest/data-model.md` → Lock/claim types (`internal/lock/lock.go`) for the atomic temp-file+`os.Link` pattern to reuse verbatim (same technique, new package — this feather does not modify `internal/lock`).
- New CLI command file `internal/cli/heartbeat.go`, following the command-registry pattern (`init()` → `register(name, run, usage)`) described in `.fledge/nest/conventions.md` → Go code conventions, and `internal/cli/cli.go`'s `commandOrder`/`ExitOK|Fail|Usage|Env` constants.
- `internal/repo` — add a `.fledge/ledger/` path accessor alongside the existing `LocksDir()`/`RosterDir()` etc. (see `.fledge/nest/data-model.md` → Roster/repo types, `repo.Repo`).
- No changes to `internal/lock`, `internal/spec`, or any existing command file.

## Approach
- `ledger.Record` interface or a shared envelope struct (e.g. `{Subject, Kind, Timestamp string; ...kind-specific fields}`) with three concrete kind payloads: `StatusRecord{PID int, Note string, UpdatedAt string}`, `VerdictRecord{Result string, Note string}` (defined now for the type but not yet wired to a CLI command — that's FTHR-074), `EscalationRecord{Message string}` (same). Keep the kind-specific fields minimal per PLM-030 FC-1.
- `Write(dir, subject, kind, payload)` — marshals JSON, writes via temp-file + `os.Link` into `.fledge/ledger/<subject>.<kind>.json`, replacing any existing file for that (subject, kind) atomically (unlike brood's `Acquire`, this must succeed even when the file already exists — it's an overwrite, not an exclusive claim, so this cannot reuse `lock.Acquire`'s EEXIST-as-conflict behavior; use rename over an existing link target, or link-to-temp-name-then-rename).
- `Read(dir, subject, kind)` — unmarshals one record; returns a typed not-found error when absent (needed by `fledge await`'s first-appearance case in FTHR-073) and a corrupt-file error (mirroring `lock.List`'s skip-and-report tolerance in `.fledge/nest/conventions.md`) rather than panicking.
- `ClassifyLiveness(pid int, lastUpdated time.Time, now time.Time) (stalled bool, reason string)` — pure function, no I/O, so it's trivially unit-testable against both failure directions named in PLM-030 AC-4: dead PID + fresh lease → not stalled call is moot (PID gone is decisive → stalled); live PID + lease older than 5 minutes → stalled; live PID + fresh lease → not stalled.
- `fledge heartbeat <name> [--note "..."]`: resolves the caller's own PID (`os.Getpid()`), writes a `StatusRecord{PID: os.Getpid(), Note: note, UpdatedAt: now}` for subject=`<name>`, kind=`status`. Supports `--json` (emit the written record) per FC-6.
- Timestamps: RFC3339, matching `internal/nest.Doc`'s existing convention (`.fledge/nest/data-model.md`).

## Tests
- `internal/ledger/ledger_test.go`:
  - `TestWriteReadRoundtrip` — write a `status` record, read it back, fields match.
  - `TestWriteOverwritesPriorRecord` — two writes to the same (subject, kind); read returns only the latest (pins the latest-value-only semantics from PLM-030's addressing decision).
  - `TestReadMissingRecord` — read on a (subject, kind) never written returns the not-found error, not a panic or generic I/O error.
  - `TestReadCorruptRecord` — a hand-written malformed JSON file at the expected path returns a corrupt-record error, doesn't panic (mirrors `lock`'s corrupt-brood-file tolerance).
  - `TestConcurrentWrites` — N goroutines writing to the same (subject, kind) concurrently; final state is exactly one of the written values, never a torn/partial file (pins FC-2's atomicity guarantee; use the same style as `internal/lock`'s existing 16-way contention test per `.fledge/nest/testing.md`).
  - `TestClassifyLiveness` — table-driven over the three cases: dead PID (stalled regardless of lease), live PID + fresh lease (not stalled), live PID + stale lease (stalled).
- `cmd/fledge/testdata/heartbeat.txtar`:
  - Happy path: `fledge heartbeat fledge-brooder-adelie --note "running tests"` succeeds, `--json` output contains the subject, note, and a PID.
  - Repeated heartbeat: second call updates the same record file (no duplicate files created).
  - Malformed input: missing required `<name>` argument exits `ExitUsage`.
- Implementation order fixed per template: write all tests above first, run `go test ./internal/ledger/... ./cmd/fledge/...` and confirm they fail for the expected reason (package/command don't exist yet), then implement until green.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after.
- [ ] AC-2: `internal/ledger` provides atomic `Write`/`Read` for `status`, `verdict`, and `escalation` record kinds with latest-value-only semantics, satisfying PLM-030 FC-1 and FC-2.
- [ ] AC-3: `fledge heartbeat <name> [--note]` writes/refreshes a `status` record and supports `--json`, satisfying PLM-030 FC-3 and FC-6.
- [ ] AC-4: `ClassifyLiveness` correctly classifies both failure directions (dead PID; live PID + stale lease) against a fixed 5-minute TTL, satisfying PLM-030 FC-4 and AC-4.
- [ ] AC-5: `go test ./internal/ledger/... ./cmd/fledge/...` passes with no data races (`-race`).
