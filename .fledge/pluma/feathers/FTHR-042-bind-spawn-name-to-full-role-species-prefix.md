---
id: FTHR-042
title: Bind spawn name to full role-species prefix
plumage: PLM-022
status: egg
priority: P2
depends_on: [FTHR-041]
authored: 2026-07-16T00:19:18Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-042: Bind spawn name to full role-species prefix

## Description
Fix `team-loop.md`'s "Spawning and addressing teammates" section so the spawn tool's `name` argument is explicitly bound to the complete `fledge-<role>-<species>` string, closing the ambiguity that lets an orchestrator drop the role prefix and pass just the bare species (the confirmed live symptom: teammates showing up as `emperor`/`adelie` instead of `fledge-forager-emperor`/`fledge-brooder-adelie`). Depends on FTHR-041 because both feathers edit `team-loop.md` (different sections) and both regenerate this repo's own `.fledge/scaffold.json` via `fledge init --refresh` — sequencing avoids the scaffold-refresh collision this repo has hit before when refresh-driven feathers run concurrently.

## Affected Modules
- `internal/bootstrap/adapters/claude/team-loop.md` — "Spawning and addressing teammates" section (the explicit binding + example) and "Planning delegation" section (the incubator/forager spawn callouts), per `.fledge/nest/architecture.md` ("adapters/<harness> is a thin format-only mapping per harness, driven entirely by that harness's manifest.yaml") and `.fledge/nest/domain.md`'s Naming mechanics entry.
- `cmd/fledge/testdata/*.txtar` — acceptance fixtures asserting on `team-loop.md`'s scaffolded content (`.fledge/nest/testing.md`: "the authoritative behavioral spec — update alongside any core/adapters change"). Likely: `init.txtar`, `init_agents.txtar`, or a new dedicated fixture following `plan_delegation.txtar`'s/`forager_contract.txtar`'s pattern of pinning specific prose via `grep`.
- This repo's own `.fledge/scaffold.json` and `.claude/team-loop.md` copy — regenerated via `fledge init --refresh`.
- No change to `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` §3.1 or `worker-protocols.md` — confirmed during planning interrogation that the agent-neutral naming scheme already states the full pattern correctly (per PLM-022's Context/Out of Scope).

## Approach
In `team-loop.md`'s "Spawning and addressing teammates" section, replace the current first bullet — "Spawn a teammate of a given agent type (e.g. `fledge-brooder`) named per the penguin-species scheme in `implementation.md` §3.1." — with wording that states explicitly: the value passed to the spawn tool's `name` parameter must be the complete `fledge-<role>-<species>` string, and gives a concrete correct-vs-incorrect example, e.g.:

> Pass the full `fledge-<role>-<species>` string as the spawn tool's `name` argument — e.g. `name: "fledge-brooder-adelie"`, never just `name: "adelie"`. The species scheme in `implementation.md` §3.1 governs which species token to append; the role prefix is fixed by which kind of worker you are spawning.

Then add one line after spawning (still in this section, or as a new short bullet immediately following): after the spawn call returns, confirm the teammate now appears in the team roster under that exact full name before proceeding — a cheap self-check that catches a dropped prefix immediately rather than downstream (e.g. in a later `SendMessage` failing to resolve, or a task-list entry with the wrong name).

In the "Planning delegation" section, the existing text already shows full names inline (`fledge-incubator-<species>`, `fledge-forager-<species>`) — verify (don't necessarily rewrite) that these read consistently with the newly explicit binding above; add a one-line cross-reference to the "Spawning and addressing teammates" binding if it reads ambiguously in isolation, but do not duplicate the full explanation.

Only edit the sentences/bullets identified above — match the file's existing style and do not reflow unrelated prose. After edits: `fledge init --refresh` in this repo, review `git status`, update fixtures, `fledge preen`.

## Tests
Written test-first as new `grep`/`! grep` assertions in `cmd/fledge/testdata/*.txtar`:

- **T1 — explicit name-argument binding present**: `grep` for the new binding sentence (or its distinctive substring, e.g. `'must be the complete'` combined with `'fledge-<role>-<species>'`) in `.claude/team-loop.md`, and `grep` for the concrete example pair (`'name: "fledge-brooder-adelie"'` and the negative example `'name: "adelie"'` framed as incorrect). Maps to AC-2 (PLM-022 AC-1).
- **T2 — old ambiguous phrasing removed**: `! grep` the current bare phrasing "named per the penguin-species scheme in \`implementation.md\` §3.1." (as it stands today, with no inline example) to confirm it was actually replaced, not merely supplemented. Maps to AC-2 (PLM-022 AC-1).
- **T3 — consistency across spawn callouts**: `grep` assertions confirming the "Planning delegation" section's incubator/forager spawn callouts remain consistent with (or explicitly cross-reference) the binding statement. Maps to AC-3 (PLM-022 AC-2).
- **T4 — post-spawn roster self-check present**: `grep` for the new self-check sentence in `.claude/team-loop.md`. Maps to AC-4 (PLM-022 AC-3).

Order: (1) write T1–T4 against the *current* scaffolded `.claude/team-loop.md` (post FTHR-041's merge, pre this feather's edits) and confirm each new assertion FAILS for the expected reason (new text absent / old text still present); capture that output verbatim as AC-1 evidence. (2) Make the prose edits per Approach. (3) `fledge init --refresh`. (4) Re-run `go test ./cmd/fledge -run TestScripts` until all pass, and run `fledge preen` clean.

## Acceptance Criteria
- [ ] AC-1: The tests listed above (T1–T4) were observed failing before implementation, for the expected reason, and pass after implementation.
- [ ] AC-2: `team-loop.md`'s "Spawning and addressing teammates" section states the full-string requirement for the spawn tool's `name` argument with a correct/incorrect example, replacing the old ambiguous phrasing (satisfies PLM-022 FC-1 / AC-1).
- [ ] AC-3: the "Planning delegation" incubator/forager spawn callouts remain consistent with that explicit binding (satisfies PLM-022 FC-2 / AC-2).
- [ ] AC-4: `team-loop.md` states the post-spawn roster self-check (satisfies PLM-022 FC-3 / AC-3).
- [ ] AC-5: the `cmd/fledge` txtar fixtures are updated and `go test ./cmd/fledge -run TestScripts` passes (satisfies PLM-022 AC-4).
- [ ] AC-6: `fledge init --refresh` regenerates this repo's own scaffold and `fledge preen` passes clean (satisfies PLM-022 AC-5).
