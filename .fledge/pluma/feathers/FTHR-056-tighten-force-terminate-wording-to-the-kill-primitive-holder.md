---
id: FTHR-056
title: Tighten force-terminate wording to the kill-primitive holder
plumage: PLM-021
status: pipping
priority: P2
depends_on: []
authored: 2026-07-16T02:06:38Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# FTHR-056: Tighten force-terminate wording to the kill-primitive holder

## Description
A `/code-review med` pass (F9) found that `foraging.md`'s forager-lifecycle
prose assigns the force-terminate backstop to "the worker that commissioned
it (the incubator or the orchestrator)" — confirmed still live in both
`internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` and this
repo's scaffolded `.fledge/skills/...` copy. On Claude Code, the incubator is
a teammate with no `TaskStop`/kill primitive of its own — only the
orchestrator (`team-lead`) can force-terminate a teammate. The disjunction
phrasing can be misread as assigning a kill action to a worker that can't
perform it; worker-protocols.md's Incubator section already routes the
actual spawn/lifecycle-wait to the orchestrator on Claude Code, so a careful
implementer resolves it correctly today, but the wording itself is
imprecise. This is a follow-up to PLM-021 (already `fledged`, hence a new
feather rather than a spec edit) that tightens the wording to name the actual
kill-primitive holder — the orchestrator on Claude Code — instead of the
abstract "commissioner."

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` — the
  Forager Lifecycle / Commissioner "verify and release" language (~line 25,
  ~line 68) (see `.fledge/nest/modules.md` → bootstrap/core skills).
- `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` —
  Incubator Lifecycle section, if it uses the same "commissioner"
  disjunction for its own force-terminate backstop language (grep to
  confirm before editing).
- `cmd/fledge/testdata/` — whichever txtar fixture(s) assert on the
  scaffolded `foraging.md`/`worker-protocols.md` content (at least
  `init.txtar`, `init_agents.txtar`, `agents.txtar`, per PLM-021's own
  precedent for this file family).

## Approach
Retitle the "the worker that commissioned it (the incubator or the
orchestrator)" phrasing in `foraging.md` to name the actual holder of the
kill primitive on the current harness — concretely: "the orchestrator (on
Claude Code, `team-lead`; the party holding the `spawn-worker`/kill
primitive) force-terminates it if it does not exit promptly" — rather than
leaving it as an abstract disjunction resolved only by inference from
worker-protocols.md. Apply the same tightening to any matching phrasing in
`worker-protocols.md`'s Incubator Lifecycle section. Keep the behavioral
content identical (force-terminate on non-prompt exit, confirmed-shutdown
definition, species freed only after confirmation) — this is a wording-only
fix, no functional change, matching PLM-021's own "prose-only" framing for
this file family.

## Tests
- Update the `cmd/fledge` txtar fixture(s) that assert on the scaffolded
  `foraging.md`/`worker-protocols.md` content (identified above) to assert
  the tightened wording is present and the old "commissioner" disjunction
  phrasing is gone. Written first against the *current* scaffolded content
  (new wording absent) and confirmed to FAIL, then the prose is edited and
  `fledge init --refresh` regenerates the scaffold until the assertion
  passes (satisfies FC-1).

## Functional Criteria (informal, tracked directly as ACs since this is a
follow-up wording fix rather than a fresh plumage)
1. FC-1: `foraging.md`'s force-terminate language for the forager names the
   orchestrator (the actual kill-primitive holder on Claude Code) rather than
   the abstract "commissioner (incubator or orchestrator)" disjunction.

## Acceptance Criteria
Checkbox list, one `- [ ] AC-N: …` line per criterion — authored unchecked; checked only via `fledge criteria check`, with per-criterion evidence in `.fledge/molt/FTHR-056.md`. Reference the parent plumage's criteria where applicable (e.g. "satisfies PLM-021 FC-2"). AC-1 is always:
- [ ] AC-1: The test listed above was observed failing before implementation
  and passes after; evidence captured verbatim.
- [ ] AC-2: `foraging.md`'s force-terminate wording names the orchestrator as
  the kill-primitive holder instead of the "commissioner" disjunction
  (satisfies FC-1).
- [ ] AC-3: `worker-protocols.md`'s Incubator Lifecycle section is checked
  for the same phrasing and tightened to match if present.
- [ ] AC-4: `fledge init --refresh` regenerates this repo's scaffolded copies
  to match, and `go test ./cmd/fledge -run TestScripts` passes.
- [ ] AC-5: `go test ./...` passes and `fledge preen` reports the scaffold
  healthy after the refresh.
