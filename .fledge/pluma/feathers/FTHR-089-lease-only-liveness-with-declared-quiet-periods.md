---
id: FTHR-089
title: Lease-only liveness with declared quiet periods
plumage: PLM-035
status: hatching
priority: P1
depends_on: []
authored: 2026-07-17T07:59:05Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.7
---

# FTHR-089: Lease-only liveness with declared quiet periods

## Description
The tracer bullet for PLM-035: a thin but complete slice through the liveness contract, end to end. A worker declares how long it expects to be quiet, the status record stores that declaration, and the classifier honors it — with the PID input deleted rather than demoted, so the false-stall PLM-035's Context measures (a two-second-old lease classifying `stalled=true`) is gone at its root.

Deliberately scoped to the record shape, the classifier, and the one command that writes the record. The command that *reads* a classification (`fledge pulse`) is FTHR-092, which depends on this feather for the classifier's new signature and the stored declaration. That split keeps this feather to a single package plus one existing command, and leaves the new top-level command surface to its own feather.

Satisfies PLM-035 FC-1, FC-3, FC-4, FC-5, FC-6; supersedes PLM-030 FC-3 and the PID half of PLM-030 FC-4 for the status record.

## Affected Modules
Source of truth only — never this repo's scaffolded `.fledge/skills/` copy (root `CLAUDE.md`). See `.fledge/nest/architecture.md` → "New since the last index: PLM-030 handoff ledger", and `.fledge/nest/data-model.md` for the record shapes.

- `internal/ledger/ledger.go` — `StatusRecord` loses its `PID` field and gains the declared quiet period; `ClassifyLiveness` loses its `pid` parameter and its `pidAlive` short-circuit; the package-private `pidAlive` helper (and its `syscall` import) become dead and go with them. `StaleAfter` **stays** — it is no longer the only answer, but it remains the default for a lease that declares nothing (FC-5).
- `internal/cli/heartbeat.go` — gains the `--expect <duration>` flag; stops populating `PID`. When `--expect` is omitted the record declares the `StaleAfter` default, so every existing call site keeps its present behavior byte-for-byte in meaning (FC-3).
- `internal/ledger/ledger_test.go` — `TestClassifyLiveness`'s two dead-PID rows cease to exist (the direction is gone); the live-PID rows become plain lease rows. `TestStaleAfterIsFiveMinutes` survives unchanged in intent: five minutes is still the default, just no longer the only value. The `deadPID` test constant becomes dead if nothing else uses it.
- `cmd/fledge/testdata/heartbeat.txtar` — currently asserts `stdout '"pid": [0-9]+'`. That assertion is **deleted**, not adjusted: FC-2 requires no PID field to remain in any output.
- **Not touched:** `internal/lock`, `internal/cli/brood.go`, and the orchestration prose — those carry the *feather-claim* record's PID and belong to FTHR-090, which runs in parallel. Different package, different record shape, no shared files.

## Approach
- **Delete, don't demote.** `ClassifyLiveness`'s PID parameter and `StatusRecord.PID` are removed outright. PLM-035 FC-1/FC-2 are explicit that a permanently-false advisory is worse than none, and that a surviving field invites a future reader to trust it — which is exactly how the defect arrived here from `internal/lock`.
- **New classifier shape:** `ClassifyLiveness(lastUpdated time.Time, expect time.Duration, now time.Time) (stalled bool, reason string)`. Stalled iff `now.Sub(lastUpdated) > expect`. The `reason` string stays non-empty in both directions (the existing test asserts this) and should name the declared period, since it is now the thing the verdict turns on.
- **Store the declaration as a duration, not a deadline** (FC-4) — e.g. `Expect string` on `StatusRecord`, holding a `time.ParseDuration`-compatible value. `updated_at` stays the single time anchor. The record must remain self-describing: a reader sees *what was claimed* and *when*, not merely their sum. This is what lets FTHR-092 render "declared 12m, 3m elapsed" rather than a bare deadline.
- **The default is the seam that keeps this non-breaking** (FC-3): `--expect` omitted ⇒ the record declares `ledger.StaleAfter`. Existing callers, which pass nothing, behave exactly as today. Prefer writing the default explicitly into the record over leaving the field empty and defaulting at read time — an explicit value keeps the record self-describing for every reader, including a human diagnosing a stall.
- **No cap on the declared value** (FC-6). Reject the temptation to bound it "for safety": PLM-035 rules that out explicitly, because a cap reinstates the fixed-TTL defect at a different threshold. Visibility (FTHR-092) is the control, not a limit here.
- An invalid `--expect` value is a usage error (`ExitUsage`), matching how `await` treats an unparseable `--timeout` (`internal/cli/await.go`).
- Reuse `ledger.Write`/`ledger.Read` unchanged — this feather adds no persistence machinery and touches neither atomicity nor the record kinds (PLM-035 Out of Scope).

## Tests
Test-first, and **the failing-first observation must be behavioral, not a build break** (PLM-035 AC-2). This matters acutely here: this feather changes `ClassifyLiveness`'s signature *twice over* (a parameter removed, another added), so unit tests written first against the unchanged package will fail to **compile** — every test in the package will "fail" for a reason that proves nothing about behavior. That exact trap was hit by FTHR-088, accepted with a caveat on the record, and written into the previous run's digest as a lesson. The txtar layer is the way out: it drives the *built binary*, so a fixture written first fails on observable behavior.

- `cmd/fledge/testdata/heartbeat.txtar` (extend) — **this is the source of the AC-1/AC-2 behavioral evidence.**
  - `fledge heartbeat w --expect 12m --json` emits `"expect": "12m"` → fails first with `flag provided but not defined: -expect`. **Behavioral, not a compile error.**
  - `fledge heartbeat w --json` emits the 5m default → pins FC-3's non-breaking default.
  - `! stdout '"pid"'` → pins FC-2 for the status record; fails first because the field is currently present.
  - `fledge heartbeat w --expect nonsense` exits 2 with a usage error.
  - `fledge heartbeat w --expect 90m` succeeds → pins FC-6 (no cap).
- `internal/ledger/ledger_test.go` — `TestClassifyLiveness` rewritten to the new signature: fresh lease → not stalled; lease past its declared period → stalled; **lease at 6 minutes with a declared 30m → not stalled** (the case that is impossible today and is the whole point); lease with the 5m default at 6 minutes → stalled. `reason` non-empty in every row.
- `TestStaleAfterIsFiveMinutes` — kept; still pins the default.
- Order: write the txtar assertions first and capture their behavioral failures verbatim in `.fledge/molt/FTHR-089.md`; then the unit tests; then implement.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: At least one failing-test observation is **behavioral** — captured from the txtar layer (e.g. `flag provided but not defined: -expect`), not a compilation or arity error — and is recorded verbatim in `.fledge/molt/FTHR-089.md`. Evidence consisting solely of build breakage does not satisfy AC-1 (satisfies PLM-035 AC-2).
- [x] AC-3: A worker whose lease is fresh classifies as **not stalled**, proven by a test that fails against the current code — the condition misclassified today (satisfies PLM-035 FC-1, AC-3).
- [x] AC-4: `StatusRecord` carries no PID field, and `fledge heartbeat`'s human-readable and `--json` output contain no PID, proven by an assertion on its absence (satisfies PLM-035 FC-2, and the status-record half of PLM-035 AC-4 — **FTHR-090 closes the feather-claim half; neither feather closes PLM-035 AC-4 alone**).
- [x] AC-5: `ClassifyLiveness` takes no PID parameter, and no PID-liveness check remains in `internal/ledger` (satisfies PLM-035 FC-1).
- [x] AC-6: A lease declaring a period longer than five minutes classifies **not stalled** past the old five-minute threshold, and **stalled** once the declared period elapses — both directions proven (satisfies PLM-035 FC-3, FC-5, AC-5).
- [x] AC-7: A lease declaring nothing classifies stalled after five minutes, exactly as today — proving the default preserves existing behavior for every current call site (satisfies PLM-035 FC-3, AC-6).
- [x] AC-8: The declared period is stored as a **duration** alongside `updated_at`, and both are readable from the record; no absolute deadline field is introduced (satisfies PLM-035 FC-4, AC-7).
- [x] AC-9: A declared period well beyond any plausible default is accepted rather than rejected or clamped, proving no cap was introduced (satisfies PLM-035 FC-6).
- [x] AC-10: An unparseable `--expect` value is a usage error exiting `ExitUsage` (2) with a message naming the flag.
- [x] AC-11: `go test ./...` is green, `go vet ./...` and `gofmt -l .` are clean, and `fledge preen` reports no errors on the branch (satisfies PLM-035 AC-13).
