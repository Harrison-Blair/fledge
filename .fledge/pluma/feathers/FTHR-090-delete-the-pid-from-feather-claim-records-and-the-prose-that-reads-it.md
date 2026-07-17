---
id: FTHR-090
title: Delete the PID from feather-claim records and the prose that reads it
plumage: PLM-035
status: fledged
priority: P1
depends_on: []
authored: 2026-07-17T07:59:05Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.7
---

# FTHR-090: Delete the PID from feather-claim records and the prose that reads it

## Description
Removes the PID from the feather-claim (brood) record and from everything that reports it: `fledge broods`' printed `(pid not alive)` annotation and its `pid_alive` JSON field. Corrects, in the same change, the one piece of orchestration prose that tells agents to read that field — because the prose exists *because* the field does, and shipping one without the other would leave a doc pointing at nothing.

This is the **root** of PLM-035's defect, not a side-cleanup. `internal/lock` recorded the CLI's own PID first; PLM-030 copied the pattern into the ledger and promoted it from an annotation into a decisive verdict. FTHR-089 fixes the promoted copy; this feather removes the original, so the pattern cannot be copied a third time.

**Runs fully in parallel with FTHR-089** — different package, different record shape, no shared files. `depends_on` is deliberately empty.

Satisfies PLM-035 FC-2 (feather-claim half) and FC-11.

## Affected Modules
See `.fledge/nest/architecture.md` (two-layer split: CLI vs. bootstrap/adapter system) and `.fledge/nest/modules.md` → `internal/lock`.

- `internal/lock/lock.go` — `Record.PID` (~line 17) is deleted. The field has no other consumer in the package: `internal/lock` never checks PID liveness itself, which is exactly why the defect was harmless here and fatal once copied.
- `internal/cli/brood.go` — `lockOut.PIDAlive` and its `json:"pid_alive"` tag (~line 180) go; the `pidAlive(rec.PID)` call (~line 188) and the `(pid not alive)` annotation (~line 199) go; `fledge brood` stops populating `PID` (~line 76); the then-dead package-private `pidAlive` helper (~line 222) and any orphaned `syscall`/`errors` imports go with them.
  **This `pidAlive` is `internal/cli`-local and distinct from the one in `internal/ledger`** that FTHR-089 removes. Two copies of the same helper — itself a small piece of evidence for how far the pattern spread.
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` (~line 135) — the recovery procedure currently names "`fledge broods` (owner, branch, **pid-alive** per held lock)" as part of the picture for reconstructing a run. The pid-alive clause is removed. **Source of truth only** — never this repo's scaffolded `.fledge/skills/` copy (root `CLAUDE.md`).
- `internal/bootstrap/` — a guard test beside the ~15 existing ones (`.fledge/nest/testing.md` → `internal/bootstrap`), asserting no shipped prose references PID liveness.
- `cmd/fledge/testdata/broods_stale.txtar`, plus any fixture asserting `pid_alive` — updated to assert its absence.
- **Not touched:** `internal/ledger`, `internal/cli/heartbeat.go` (FTHR-089's surface); the rest of `implementation.md`'s handoff prose (FTHR-075's surface, ordered behind this feather).

## Approach
- **Scope discipline on `implementation.md`.** Touch **only** the pid-alive clause. FTHR-075 rewrites this same file for ledger-based handoffs and is ordered behind this feather precisely so the two never collide; widening here would manufacture the conflict that ordering exists to prevent. Root `CLAUDE.md`'s surgical-changes rule applies at full strength: that clause and nothing adjacent.
- **The `--stale` path must not move.** `fledge broods --stale` filters on `worktreeExists(rec.Worktree)` (~line 185), **not** on `PIDAlive`. That path feeds `fledge abandon --force` during recovery — it is the one genuinely dangerous consumer, it was sound before this feather, and it must be sound after. `worktreeExists` and the `--worktree` field are untouched. AC-5 pins this rather than trusting it.
- **Deleting the field is the point, not tidiness** (PLM-035 FC-2). A `pid_alive` that is always `false` is a machine-readable contract field whose only possible value is a falsehood, and `implementation.md` already directs agents to read it. Keeping it "for debugging" preserves the invitation that produced this plumage.
- Removing a JSON field is a **breaking change to `broods --json`'s contract**, taken deliberately: PLM-035 establishes that no stored corpus exists to migrate and that the field never carried a true value, so nothing downstream can be relying on it correctly.
- Remove what this change orphans (the helper, its imports) and nothing more (root `CLAUDE.md`).

## Tests
Test-first, with the failing-first observation **behavioral** (PLM-035 AC-2). This feather is less exposed to the build-break trap than FTHR-089 — it deletes a field rather than changing a signature — but the rule holds, and the txtar layer is the natural source since `broods`' output is the observable surface.

- `cmd/fledge/testdata/broods_stale.txtar` (extend) — **source of the AC-1/AC-2 behavioral evidence:**
  - `fledge broods --json` contains no `pid_alive` field → `! stdout 'pid_alive'`. Fails first: the field is currently present on every record.
  - `fledge broods` printed output contains no `(pid not alive)` → `! stdout 'pid not alive'`. Fails first — and fails for a claim *seconds old*, the exact absurdity PLM-035's Context captures.
  - The existing `worktree_exists` and `--stale` assertions must keep passing **unmodified** — that is AC-5's proof, not a new test.
- `internal/bootstrap/` guard test (new) — asserts by substring that no file under `core/skills/` mentions pid-alive. Fails first against the current `implementation.md`. Follows the established convention (`TestBrooderFixLoopInvariant`, `TestIncubatorDocDescribesScratchpadBatching`).
- `TestCoreNeutral` (existing) — must keep passing: no harness-specific paths introduced.
- Any existing `internal/lock` test referencing `Record.PID` — updated; the field is gone.
- Order: write the txtar and guard assertions first, capture their behavioral failures verbatim in `.fledge/molt/FTHR-090.md`, then implement.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: At least one failing-test observation is **behavioral** — captured from the txtar or guard-test layer, not a compilation error — and recorded verbatim in `.fledge/molt/FTHR-090.md` (satisfies PLM-035 AC-2).
- [x] AC-3: `lock.Record` carries no PID field, and `fledge brood` no longer records one (satisfies PLM-035 FC-2).
- [x] AC-4: `fledge broods` reports no PID in either its printed or `--json` output — no `pid_alive` field, no `(pid not alive)` annotation — proven by assertions on their absence (satisfies PLM-035 FC-2, and the feather-claim half of PLM-035 AC-4 — **FTHR-089 closes the status-record half; neither feather closes PLM-035 AC-4 alone**).
- [x] AC-5: `fledge broods --stale` continues to key on worktree existence and its behavior is unchanged, proven by the existing `--stale`/`worktree_exists` assertions passing unmodified — the force-release path feeding `fledge abandon --force` was sound before and remains so (satisfies PLM-035 FC-2, AC-10).
- [x] AC-6: No file under `internal/bootstrap/core/skills/` references PID liveness, proven by a guard test that fails against the current prose (satisfies PLM-035 FC-11, AC-12).
- [x] AC-7: `implementation.md` is otherwise unmodified — only the pid-alive clause changed — so FTHR-075's later rewrite of this file has no conflict to resolve.
- [x] AC-8: No orphaned `pidAlive` helper or unused import remains in `internal/cli` as a result of this change.
- [x] AC-9: `go test ./...` is green, `go vet ./...` and `gofmt -l .` are clean, and `fledge preen` reports no errors on the branch (satisfies PLM-035 AC-13).
