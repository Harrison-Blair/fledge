---
id: PLM-027
title: Split worker-protocols into per-role files
status: hatched
priority: P1
authored: 2026-07-16T15:45:08Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# PLM-027: Split worker-protocols into per-role files

## Context
`worker-protocols.md` (`.fledge/skills/fledge-orchestrate/`) currently holds all three delegated-worker protocols — Incubator, Brooder, Skua — as sections of one file. A spawned worker only ever needs its own role's protocol, but today it (and every doc citing "worker-protocols.md §Incubator" etc.) must load the whole combined file, wasting context. This plumage splits the file into one protocol file per role, so a worker's context only carries the protocol it actually needs. It is the foundation for two follow-on plumages (batched-scratchpad interrogation, phase-digest compression) that both amend content landing inside the new `incubator.md` — this split must land first.

This is a pure mechanical reorganization: content moves verbatim into new files and every cross-reference is repointed; no protocol wording changes.

## User Stories
- As an incubator/brooder/skua worker, I want to read only my own role's protocol file, so that my spawn context doesn't carry two other roles' prose I'll never use.
- As a maintainer editing one role's protocol (e.g. tightening the skua review loop), I want that role's content isolated in its own file, so that my diff doesn't touch unrelated incubator/brooder prose and reviewers can see the change is scoped.

## Functional Criteria
1. FC-1: `.fledge/skills/fledge-orchestrate/incubator.md`, `brooder.md`, `skua.md` each exist as flat siblings of `planning.md`/`foraging.md`/`implementation.md`, each self-contained (own H1 heading, no dependency on reading the other two to make sense).
2. FC-2: Each new file's content is the corresponding role section moved verbatim from the current `worker-protocols.md` (no wording changes) — Incubator's content (including "Relay envelope", "Communication rules", "Drafting", "Lifecycle" subsections) into `incubator.md`; Brooder's ("Communication rules", "Protocol", "When stuck", "Lifecycle") into `brooder.md`; Skua's ("Communication rules", "Reviewing a feather", "Verdict", "Lifecycle") into `skua.md`.
3. FC-3: `worker-protocols.md` shrinks to a stub index: the shared spawn-prompt contract paragraph (what a spawn prompt is, that it's a worker's entire context, the fixed fields every spawn prompt carries) plus a short link to each of the three role files.
4. FC-4: Every existing reference to `worker-protocols.md §Incubator`/`§Brooder`/`§Skua` across core docs (`planning.md`, `implementation.md`, `foraging.md`), the Claude adapter agent definitions (`fledge-incubator.md`, `fledge-brooder.md`, `fledge-skua.md`), `CLAUDE.md`, and any other citing doc is repointed to the new per-role file.
5. FC-5: `internal/bootstrap/worker_protocols_test.go`'s coverage is preserved but reorganized into three test files (`incubator_test.go`, `brooder_test.go`, `skua_test.go`) mirroring the new doc structure, each reading its own embedded file directly rather than extracting a section by string search.
6. FC-6: `cmd/fledge/testdata/forager_contract.txtar` and `init_agents.txtar` (and any other txtar fixture asserting on `worker-protocols.md` presence/content) are updated to assert on the new file set instead.
7. FC-7: `fledge init --refresh` on a previously-scaffolded repo removes the obsolete monolithic content and writes the new files cleanly (drift classification treats this as an ordinary content change, not a special case).

## Acceptance Criteria
- [ ] AC-1: `incubator.md`, `brooder.md`, `skua.md` exist under `.fledge/skills/fledge-orchestrate/` (and their `internal/bootstrap/core/` source), each containing its role's content verbatim (diff against the pre-split section shows no wording changes beyond the heading level).
- [ ] AC-2: `worker-protocols.md` contains only the shared spawn-prompt contract paragraph and links to the three new files.
- [ ] AC-3: A repo-wide search for `worker-protocols.md#` / `worker-protocols.md §` finds zero remaining stale section references; every prior citation now points at the correct per-role file.
- [ ] AC-4: `internal/bootstrap/worker_protocols_test.go` is replaced by `incubator_test.go`, `brooder_test.go`, `skua_test.go`, each passing against the corresponding new embedded file.
- [ ] AC-5: `go test ./...` and `go test ./cmd/fledge -run TestScripts` pass, including updated `forager_contract.txtar` and `init_agents.txtar`.
- [ ] AC-6: `fledge preen` passes on this repo after the split and after running `fledge init --refresh`.

## Out of Scope
- Any wording, structure, or protocol-behavior change to the Incubator/Brooder/Skua content itself — pure move only. (Content changes are carried by PLM-028 and PLM-029, which amend `incubator.md` after this split lands.)
- Codex and pi adapters — they run Tier A (no `spawn-worker`/`message-peer`), never reference `worker-protocols.md`, and are unaffected by this split.
- Any change to `foraging.md` (the forager/scout protocol) — it is a sibling file, not part of `worker-protocols.md`, and out of scope here.

## Open Questions
None — the interrogation resolved file placement (flat siblings), priority (P1), scope (pure mechanical split), and test-file structure (split into three files) with the user directly.
