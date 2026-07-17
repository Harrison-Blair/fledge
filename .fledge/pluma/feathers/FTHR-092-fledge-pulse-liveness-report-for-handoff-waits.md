---
id: FTHR-092
title: "fledge pulse: liveness report for handoff waits"
plumage: PLM-035
status: fledged
priority: P1
depends_on: [FTHR-089]
authored: 2026-07-17T07:59:11Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.7
---

# FTHR-092: fledge pulse: liveness report for handoff waits

## Description
Delivers `fledge pulse <name>`, the command that makes liveness classification **reachable from prose**. It reports whether a named worker is stalled, why, and — critically — the quiet period that worker declared alongside how much of it has elapsed.

This is the door PLM-030 never built. PLM-030 FC-4 defined a stalled-vs-busy rule and FC-7 required orchestration prose to describe handoffs in ledger terms — but the rule shipped as a Go function with no CLI caller, and prose cannot call a Go function. FTHR-089 makes that rule *correct*; this feather makes it *usable*. Without it, FTHR-075's timeout-recovery path has to reimplement the classification in English and do timestamp arithmetic by eye.

`pulse` is a **new top-level command**, deliberately: liveness is a handoff primitive and belongs alongside `heartbeat`/`await`/`verdict`/`escalate`, not nested under a ledger-file accessor.

Satisfies PLM-035 FC-7, FC-8, FC-9.

## Affected Modules
See `.fledge/nest/architecture.md` → "New since the last index: PLM-030 handoff ledger", and `.fledge/nest/entry-points.md` → `internal/cli.Run`'s registered subcommands.

- `internal/cli/pulse.go` (new) — the command. Registers via `init() { register("pulse", runPulse, usage) }`, per the pattern every command file follows (`.fledge/nest/architecture.md`).
- `internal/cli/cli.go` (~line 106) — `pulse` is added to `commandOrder`.
- `internal/cli/pulse_test.go` (new) — unit coverage for the output shape and the no-record path.
- `cmd/fledge/testdata/pulse.txtar` (new) — CLI acceptance coverage, consistent with `await`/`verdict`/`escalate`/`ledger-read`.
- `internal/ledger` — **read-only consumer**. `ledger.Read` and `ClassifyLiveness` are used as FTHR-089 leaves them; this feather changes nothing in that package.

**On the scaffold: no refresh is required, and this feather does not serialize against others.** `commandOrder` feeds exactly one template — `internal/bootstrap/adapters/claude/settings.local.json`, the only `{{range .CommandOrder}}` in the adapter tree — whose output `.claude/settings.local.json` is **gitignored** (`.gitignore:7`), as is `.fledge/scaffold.json` (`.gitignore:40`). The tracked `.claude/fledge-adapter.md` carries no command list. **So adding `pulse` to `commandOrder` changes no tracked file**, and no txtar compares the allow-list exactly (`init.txtar` only greps individual entries, which an added line cannot break). The older "scaffold-touching feathers must be dispatched alone" rule predates commit `1f5224d`, which untracked the scaffold; it no longer applies **in this repository**. It still applies in a fledge-managed repo that *tracks* its scaffold — check before assuming either way.

## Approach
- Read the `status` record with `ledger.Read(r.LedgerDir(), name, ledger.KindStatus)`; classify with `ledger.ClassifyLiveness` as FTHR-089 leaves it. **All decision logic stays in `internal/ledger`** — `internal/cli` is thin glue and never holds business logic (`.fledge/nest/architecture.md`). This feather must not reimplement the classification; the entire point is that one tested procedure has one home.
- **Report `stalled` + `reason` mirroring `ClassifyLiveness`'s return exactly, and exit `ExitOK`** (FC-9). Stalled-ness is **information, not a command failure** — the exit code must not encode it, or an agent loses the ability to distinguish "your peer is stalled" from "the command broke". Reserve the existing codes for their existing meanings: `ExitEnv` for repo problems, `ExitUsage` for a missing/invalid name, `ExitFail` for a corrupt record.
- **Report the declared period and the elapsed time against it** (FC-7). This is **load-bearing, not a convenience**: PLM-035 FC-6 caps nothing, so visibility is the *only* control on an implausible declaration — an agent that declares an hour is detectable solely because `pulse` shows it. It is also what lets a waiter wait out the *remaining* period instead of re-timing-out blindly, which is FTHR-075's recovery path. **Do not defer or drop this half; doing so silently removes the only check on `--expect 1h`.**
- **No status record is a distinct third state, never `stalled: true`** (FC-8). `ledger.Read` returns a `*ledger.NotFoundError`; report `stalled: false` with a `reason` explicitly naming the absence, and exit `ExitOK`. Rationale, from PLM-035: a worker 30 seconds into startup that hasn't reached its first heartbeat is *starting up*, not dead — and it is the most likely real case early in a handoff. Reporting it stalled would rebuild the false-stall this whole plumage exists to remove, in the very command built to fix it.
- `--json` support, per the convention that every `fledge` command has it (PLM-030 FC-6).
- Follow `runAwait`/`runLedgerRead` for flag parsing (`parseMixed`), repo resolution (`repo.Find` + `RequireFledge`), and `InvalidSubjectError` → `ExitUsage` mapping — a subject that would escape the ledger directory is rejected, not sanitized.
- Naming: the CLI verb is `pulse` while the function stays `ClassifyLiveness`. That divergence is the documented design, not an inconsistency — root `CLAUDE.md` describes exactly this mapping ("`check` (validation = `preen`)", "`graph` (dependency graph = `vee`)", "`lock` (feather claims = `brood`)"). Renaming the Go side is out of scope (PLM-035).

## Tests
Test-first, with the failing-first observation **behavioral** (PLM-035 AC-2). Natural here: an unregistered command fails at the CLI surface, so `pulse.txtar` fails first with an unknown-command error rather than a build break.

- `cmd/fledge/testdata/pulse.txtar` (new) — **source of the AC-1/AC-2 behavioral evidence:**
  - `fledge pulse w` on a fresh heartbeat → `stalled: false`. Fails first with an unknown-command error. **Behavioral.**
  - `--json` reports `stalled`, `reason`, the declared period, and elapsed time → pins FC-7.
  - A worker with **no** status record → not stalled, a reason naming the absence, **exit 0** → pins FC-8/FC-9.
  - A lease that declared 30m, aged past 5m → **not stalled** (impossible before PLM-035; the payoff of the whole plumage).
  - A lease that declared nothing, aged past 5m → stalled.
  - Missing `<name>` → `ExitUsage` (2). `fledge pulse ../escape` → rejected as invalid, consistent with `heartbeat`/`ledger read`.
  - **`fledge pulse` on a *stalled* worker still exits 0** → pins FC-9 directly: the classification rides in the output, not the exit code.
- `internal/cli/pulse_test.go` — output shape for stalled / not-stalled / no-record; the no-record path in particular, since it is the case with no `ClassifyLiveness` coverage by construction (it is outside that function's domain).
- Aging a lease deterministically: write the record with a back-dated `updated_at` rather than sleeping — the suite races no clocks (see `awaitClock` in `internal/cli/await_test.go` for the established injectable-time convention).
- Order: write `pulse.txtar` first, capture its verbatim behavioral failure in `.fledge/molt/FTHR-092.md`, then the unit tests, then implement.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: At least one failing-test observation is **behavioral** — captured from `pulse.txtar` (e.g. an unknown-command error), not a compilation error — and recorded verbatim in `.fledge/molt/FTHR-092.md` (satisfies PLM-035 AC-2).
- [x] AC-3: `fledge pulse <name>` reports `stalled` and `reason` mirroring `ClassifyLiveness`'s return, in both human-readable and `--json` output (satisfies PLM-035 FC-7).
- [x] AC-4: The output includes the declared quiet period **and** the elapsed time against it, proven by a test asserting both appear (satisfies PLM-035 FC-7, AC-8).
- [x] AC-5: A worker with no status record reports as a **distinct state** — not stalled, with a reason naming the absence — and exits `ExitOK` (satisfies PLM-035 FC-8, FC-9, AC-9).
- [x] AC-6: `fledge pulse` on a **stalled** worker exits `ExitOK`, proving the classification is carried in the output and not encoded in the exit code (satisfies PLM-035 FC-9).
- [x] AC-7: A worker whose lease declared a period longer than the default reports **not stalled** past the old five-minute threshold, proven by a test — the behavior that was impossible before PLM-035 (satisfies PLM-035 FC-5, FC-7).
- [x] AC-8: `pulse` contains no liveness logic of its own — the classification comes from `internal/ledger` — keeping one tested decision procedure with one home (satisfies PLM-035 FC-1).
- [x] AC-9: `--json` is supported, consistent with every other `fledge` command (satisfies PLM-030 FC-6).
- [x] AC-10: A missing name is `ExitUsage` (2), and a subject that would escape the ledger directory is rejected rather than sanitized, consistent with `heartbeat` and `ledger read`.
- [x] AC-11: `go test ./...` is green, `go vet ./...` and `gofmt -l .` are clean, and `fledge preen` reports no errors on the branch (satisfies PLM-035 AC-13).
