---
id: FTHR-085
title: Phase-close roost step in orchestration prose
plumage: PLM-032
status: egg
priority: P2
depends_on: [FTHR-083]
authored: 2026-07-17T03:32:42Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-085: Phase-close roost step in orchestration prose

## Description

Adds "run `fledge roost`" as a closing step to the planning and implementation phases, so
tidying happens automatically at a point where no merge is in flight.

This is the other half of the drift answer. The explicit-command design (PLM-032 FC-2) keeps
file renames out of the merge window, at the cost of tidiness drifting until someone runs
the command. FTHR-084 makes that drift visible; this feather makes it stop happening.
Phase close is the safe moment: every branch is merged, no brooder holds a worktree, and
the orchestrator is already mutating spec state.

Depends on FTHR-083 because prose must not instruct agents to run a command that does not
exist. Ordering is the whole reason for the edge.

Parallel-safe with FTHR-084 (`internal/cli/preen.go`) and FTHR-086 (`.fledge/pluma/**`):
this feather's changes are confined to `internal/bootstrap/core/**` and the regenerated
scaffold.

Satisfies PLM-032 FC-13.

## Affected Modules

See `.fledge/nest/modules.md` → `internal/bootstrap`;
`.fledge/nest/architecture.md` (embedded `core/` tree is the agent-neutral source of truth;
`.fledge/skills/` is generated output).

- `internal/bootstrap/core/skills/fledge-orchestrate/planning.md` — closing step in step 4.7.
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` — closing step in
  its phase-close section.
- `internal/bootstrap/` prose-invariant test files — see Tests.
- `.fledge/skills/**` + `.fledge/scaffold.json` — **regenerated**, not hand-edited (see
  Approach).

## Approach

**Edit the source, never the generated copy.** `internal/bootstrap/core/` is the source of
truth; this repository's `.fledge/skills/` is generated output of it. Edit `core/`, then run
`fledge init --refresh` to regenerate. Hand-editing `.fledge/skills/` would be overwritten
by the next refresh and would leave the stamp inconsistent. This is the single most common
way to get this repo's scaffold wrong — `CLAUDE.md` calls it out explicitly.

**Where the step goes.** Both phases already have a close-out sequence that runs
`fledge preen`, reports the remaining slate, and writes a digest to `.fledge/scratch/`.
Add the roost step to that existing sequence rather than inventing a new section. Place it
**after** the merges and status flips are done and **before** the digest is written, so the
digest describes the tidied state. Keep it brief and imperative, matching the surrounding
prose register.

**Say why, briefly.** The prose should note that roosting happens at phase close
specifically because no merge is in flight — otherwise a future editor may "simplify" it to
a status-change hook and reintroduce the rename-during-merge hazard the whole design avoids.
One clause is enough; this is prose, not a rationale document.

**Scaffold stamp — serialization matters.** Changing `core/` content changes the hash of
every file copied from it, so `fledge init --refresh` rewrites `.fledge/scaffold.json`. This
is the **second** of three feathers that rewrite that stamp (after FTHR-083, before
FTHR-087). They are serialized by `depends_on` so they never collide at merge. Do not
dispatch concurrently with either.

**Constraint: prose only.** No Go behavior changes here. The command already exists
(FTHR-083); this feather only tells agents to run it.

## Tests

Prose-invariant unit tests in `internal/bootstrap`, matching the established pattern:
`.fledge/nest/testing.md` documents `planning_digest_test.go`, `implementation_digest_test.go`,
and `foraging_digest_test.go` as tests that assert the core prose contains its required
instructions. This feather's test is the same shape, in a **new file**
(`internal/bootstrap/roost_phase_close_test.go`) so it stays merge-disjoint from the other
feathers' test edits.

Run: `go test ./internal/bootstrap -run TestRoostPhaseClose`,
`go test ./cmd/fledge -run TestScripts/init`, then `go test ./...`.

- *planning phase-close instructs roosting* — `core/.../planning.md`'s close-out section
  names the roost step. Substring/section extraction per the existing digest tests, tolerant
  of prose edits, not a snapshot → AC-2.
- *implementation phase-close instructs roosting* — same for `implementation.md` → AC-2.
- *the step is ordered before the digest write* — in both files the roost instruction
  precedes the digest instruction, so the digest describes the tidied state. Pins ordering,
  not just presence → AC-3.
- *the safe-moment rationale survives* — both files state that roosting happens at phase
  close because no merge is in flight. This is what stops a future editor turning it back
  into a status hook → AC-4.
- *scaffold regenerates cleanly* — after `fledge init --refresh`, `fledge preen` reports the
  scaffold healthy, and a second refresh is byte-idempotent (`writeIfChanged` behavior the
  txtar tests already depend on) → AC-5.

Test-first order is fixed: write these, run them against the unchanged prose and observe
them FAIL for the expected reason (no roost instruction present), then edit `core/` until
they pass.

## Acceptance Criteria

- [ ] AC-1: The tests listed above were observed failing before implementation and pass
      after.
- [ ] AC-2: Both the planning and implementation phase-close sequences in
      `internal/bootstrap/core/` instruct the orchestrator to run `fledge roost`. Satisfies
      PLM-032 FC-13, AC-11.
- [ ] AC-3: In both files the roost step is ordered after merges/status flips and before the
      digest write, so the digest describes the tidied state.
- [ ] AC-4: Both files record that roosting happens at phase close because no merge is in
      flight.
- [ ] AC-5: `fledge init --refresh` has been run, `.fledge/skills/**` and
      `.fledge/scaffold.json` are regenerated and committed, `fledge preen` reports the
      scaffold healthy, and a repeat refresh changes nothing.
- [ ] AC-6: No Go behavior changed; `go test ./...` passes with existing fixtures updated
      only where they assert on the regenerated scaffold content.
