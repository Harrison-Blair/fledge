---
id: FTHR-041
title: Add force-terminate backstop for forager/incubator teardown
plumage: PLM-021
status: pipping
priority: P2
depends_on: []
authored: 2026-07-16T00:17:56Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-041: Add force-terminate backstop for forager/incubator teardown

## Description
Extend the TaskStop force-terminate teardown backstop — established for the brooder/skua pair by commit 96a3ac3 — to the forager and incubator roles across every teardown path in the embedded `core/`/`adapters/claude` prose, and restore a bullet dropped from the forager's Claude agent definition by a later regression. This is a prose-only change (no Go code) to five embedded source files, followed by regenerating this repo's own scaffold and updating the `cmd/fledge` txtar fixtures that assert on the scaffolded output.

Concretely:
1. Add "the commissioner will force-terminate you if you do not exit promptly" language to the Incubator Lifecycle section of `worker-protocols.md`, mirroring the existing Brooder/Skua Lifecycle sentences.
2. Add the same force-terminate language to `foraging.md`'s Forager Lifecycle section and its Commissioner "On the final message, verify and release" step.
3. Update `planning.md`'s incubator-release step (§0) and forager-release step (§2) to reference the force-terminate backstop rather than ending at "request shutdown by name."
4. Generalize `team-loop.md`'s "Shutting down teammates" section from "the brooder *and* its paired skua" to explicitly cover all four teammate roles (brooder, skua, forager, incubator).
5. Fix `team-loop.md`'s "confirmed shutdown" definition so it no longer assumes a tmux pane always exists: roster-absence alone is sufficient confirmation when no pane was ever created (degraded/no-tmux mode); the pane-closed clause applies only when a pane exists. This is a wording-only change to one shared sentence — no functional change to brooder/skua teardown.
6. Restore the dropped bullet in `internal/bootstrap/adapters/claude/agents/fledge-forager.md`: "Gate your final message on `fledge nest status`" (or equivalent phrasing matching the file's existing bullet style), placed between the `nest stamp` bullet and the teammate-exit bullet.
7. Run `go install ./cmd/fledge && fledge init --refresh` in this repo to regenerate the scaffolded `.fledge/` and `.claude/` copies from the edited embedded sources, and update the `cmd/fledge` txtar fixtures that pin this content.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` — Incubator Lifecycle section (see `.fledge/nest/architecture.md` → bootstrap/adapter system, `.fledge/nest/domain.md` → Worker roles: Incubator).
- `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` — Forager Lifecycle + Commissioner sections (`.fledge/nest/domain.md` → Worker roles: Forager, Commissioner).
- `internal/bootstrap/core/skills/fledge-orchestrate/planning.md` — §0 (incubator release) and §2 (forager release).
- `internal/bootstrap/adapters/claude/team-loop.md` — "Shutting down teammates" section (`.fledge/nest/architecture.md` → "adapters/<harness> is a thin format-only mapping per harness").
- `internal/bootstrap/adapters/claude/agents/fledge-forager.md` — the forager's Claude agent definition (regression fix).
- `cmd/fledge/testdata/*.txtar` — acceptance fixtures asserting on scaffolded content; per `.fledge/nest/testing.md`, these are "the authoritative behavioral spec" and must be updated alongside any core/adapters change. Likely touched: `init.txtar`, `init_agents.txtar`, `agents.txtar`, `forager_contract.txtar` (or a new dedicated fixture following that file's pattern of pinning specific prose via `grep`).
- This repo's own `.fledge/scaffold.json`, `.fledge/skills/`, and `.claude/` copies — regenerated via `fledge init --refresh` per `CLAUDE.md`'s rebuild/reinstall/verify instructions.

## Approach
Follow the existing brooder/skua backstop language (commit 96a3ac3) as the template for wording — do not invent new phrasing patterns. For each core-skill file, add the force-terminate sentence to the analogous Lifecycle-style section, keeping the surrounding prose's voice and structure intact (edit only the sentences that need to change; do not reflow or "improve" adjacent text).

For `team-loop.md`'s "Shutting down teammates" section: broaden "Procedure per worker (do this for the brooder *and* its paired skua at green teardown)" to name all four roles, and note that the incubator/forager teardown moments are the ones defined in `planning.md`/`foraging.md`/`worker-protocols.md` (phase close / nest-status verification) rather than "green teardown" (a brooder/skua-specific term) — reuse existing vocabulary, don't introduce a new one.

For the confirmed-shutdown definition fix: change "the teammate no longer appears in the team roster and its tmux pane has closed" to something like "the teammate no longer appears in the team roster and, if it was given a pane, that pane has closed (no-tmux/degraded sessions have no pane to check — roster absence alone suffices there)." Keep it to one clarifying clause; don't restructure the whole sentence.

For `fledge-forager.md`: insert the missing bullet at its original position (between the `nest stamp` bullet and the teammate-exit bullet), using phrasing consistent with the file's other Claude-runtime-specifics bullets.

After all prose edits: `go build -o fledge ./cmd/fledge && go install ./cmd/fledge && hash -r && fledge version` (confirm it matches `VERSION`), then `fledge init --refresh` in this repo and `git status` to review what regenerated. Update fixtures to match, then `fledge preen`.

## Tests
Written test-first as new/extended `grep`/`! grep` assertions in `cmd/fledge/testdata/*.txtar` (testscript acceptance tests — this codebase's test surface for scaffolded prose, per `.fledge/nest/testing.md` and the `forager_contract.txtar` precedent for pinning specific sentences):

- **T1 — incubator force-terminate backstop**: extend `init_agents.txtar` (or a new fixture) with `grep 'force-terminate' .fledge/skills/fledge-orchestrate/worker-protocols.md` scoped to the Incubator Lifecycle section's new sentence (pin the exact added phrase, not just the word "force-terminate" which already appears for Brooder/Skua). Maps to AC-2 (PLM-021 AC-1).
- **T2 — forager force-terminate backstop**: extend `forager_contract.txtar` (its existing subject is forager wait-contract prose) with a `grep` for the new force-terminate sentence in `foraging.md`. Maps to AC-3 (PLM-021 AC-2).
- **T3 — planning.md release-step wording**: extend `plan_delegation.txtar` (already pins planning.md delegation markers) with `grep` assertions for the updated §0/§2 release-step text referencing the backstop. Maps to AC-4 (PLM-021 AC-3).
- **T4 — team-loop.md generalized to four roles**: new assertions (in `init.txtar` or a new fixture) that the "Shutting down teammates" section text no longer reads "the brooder *and* its paired skua" alone but names all four roles — e.g. `grep` for each role name in that section, and `! grep` the old brooder-only framing sentence verbatim. Maps to AC-5 (PLM-021 AC-4).
- **T5 — degraded/no-tmux confirmed-shutdown wording**: `grep` for the new pane-conditional clause in `team-loop.md`. Maps to AC-6 (PLM-021 AC-5).
- **T6 — fledge-forager.md bullet restored**: extend `agents.txtar` with `grep '\`fledge nest status\`' .claude/agents/fledge-forager.md` (or the exact restored phrase). Maps to AC-7 (PLM-021 AC-6).

Order: (1) write T1–T6 as new assertions against the *current* (unedited) scaffolded output — run `go test ./cmd/fledge -run TestScripts` and confirm each new assertion FAILS for the expected reason (text not present); capture that output verbatim as AC-1 evidence. (2) Make the prose edits per Approach. (3) `fledge init --refresh` to regenerate. (4) Re-run `go test ./cmd/fledge -run TestScripts` until all pass, and run `fledge preen` clean.

## Acceptance Criteria
- [ ] AC-1: The tests listed above (T1–T6) were observed failing before implementation, for the expected reason (assertion text absent from current scaffolded output), and pass after implementation.
- [ ] AC-2: `worker-protocols.md`'s Incubator Lifecycle section documents the force-terminate backstop (satisfies PLM-021 FC-1 / AC-1).
- [ ] AC-3: `foraging.md`'s Forager Lifecycle/Commissioner sections document the force-terminate backstop (satisfies PLM-021 FC-2 / AC-2).
- [ ] AC-4: `planning.md`'s forager-release (§2) and incubator-release (§0) steps reference the backstop (satisfies PLM-021 FC-3 / AC-3).
- [ ] AC-5: `team-loop.md`'s "Shutting down teammates" section explicitly covers all four teammate roles (satisfies PLM-021 FC-4 / AC-4).
- [ ] AC-6: `team-loop.md`'s confirmed-shutdown definition explicitly handles the no-tmux/degraded case (satisfies PLM-021 FC-5 / AC-5).
- [ ] AC-7: `fledge-forager.md`'s dropped self-verification bullet is restored (satisfies PLM-021 FC-6 / AC-6).
- [ ] AC-8: the `cmd/fledge` txtar fixtures are updated and `go test ./cmd/fledge -run TestScripts` passes (satisfies PLM-021 AC-7).
- [ ] AC-9: `fledge init --refresh` regenerates this repo's own scaffold and `fledge preen` passes clean (satisfies PLM-021 AC-8).
