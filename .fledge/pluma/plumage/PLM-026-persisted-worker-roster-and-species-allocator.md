---
id: PLM-026
title: Persisted worker roster and species allocator
status: fledged
priority: P3
authored: 2026-07-16T01:44:09Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.5
---

# PLM-026: Persisted worker roster and species allocator

## Context
A `/code-review med` pass over recent commits found that worker naming — the
`<role>-<species>` scheme drawn from 18 penguin species that identifies every
spawned brooder/skua pair — is allocated, tracked, and freed entirely inside
the orchestrator's own context, with no CLI backing it
(`implementation.md` §3.1, §6):

- Assignment is a mental rule: "assign the first unused species... append a
  numeric suffix... if all 18 are in use." A species frees only once **every**
  worker bearing it is confirmed shut down — a lifecycle event the LLM must
  track itself, spanning from before the pair's brood claim exists to after
  it's released.
- The orchestrator is told to "keep the full name→feather mapping internally"
  — species reuse correctness depends on this in-context bookkeeping never
  slipping.
- On crash/resume (§6), the roster is explicitly discarded ("treat all
  remembered workers as gone; clear the roster") and rebuilt by guesswork from
  worktrees, branches, and `fledge broods` — there's no persisted record to
  read back.

With many concurrent pairs, a slip double-assigns a species (two live workers
answering to the same name) or frees one prematurely while a member is still
alive; post-crash reconstruction is heuristic rather than authoritative. It's
also a standing per-run context/token cost, independent of any failure — the
review's core interest.

Two design decisions were settled during interrogation:
- **A new `fledge roster` command**, not an extension of `fledge brood
  --owner`. Species assignment and release don't share `brood`'s lifecycle:
  a pair is spawned and named *before* any brood claim exists, and a species
  frees only *after* the feather's brood is already released (once both pair
  members are confirmed shut down) — outside the window `brood`'s per-feather
  `Record` covers. `brood` also only ever names the owner (brooder); it has no
  notion of a pair or a skua. Roster state is tracked separately, mirroring
  the existing `.alloc.lock`-guarded ID allocator (`internal/spec/ids.go`)
  rather than bolting a second state machine onto the lock file.
- **Priority P3** — the largest and least-defined of this review's four
  plumages (new command + persisted state + two rewritten workflow sections),
  whose failure mode only bites at higher concurrency; sequenced last behind
  PLM-023/024/025.

## User Stories
- As an orchestrator dispatching a new brooder/skua pair, I want to allocate
  their shared species name via a CLI call, so I don't have to mentally track
  which of the 18 species are in use or already reused with a numeric suffix.
- As an orchestrator tearing down a completed pair, I want to release their
  species via a CLI call once both members are confirmed shut down, so the
  species frees deterministically instead of depending on in-context
  bookkeeping surviving the whole feather's lifecycle.
- As an orchestrator recovering from a crash, I want to read the current
  name→feather roster from disk, so resume reconstruction is a CLI query
  instead of "clear the roster and guess from worktrees/branches."

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: A new `fledge roster assign --feather FTHR-### [--pair]` command
   allocates the next unused species (first unused of the 18; if all are in
   use, the first species with the lowest unused numeric suffix, e.g.
   `adelie-2`), persists the assignment (species, role name(s), feather ID) to
   a tracked state file, and prints/returns (`--json`) the allocated name(s)
   — one name for a solo worker, two (`<role>-<species>` for each of a stated
   pair, e.g. brooder+skua) when `--pair` is given.
2. FC-2: A new `fledge roster release <name>` command marks that named
   worker's member of its species assignment as confirmed-shut-down; the
   species becomes available for reallocation only once every member sharing
   it has been released.
3. FC-3: A new `fledge roster [--json]` command (no args) lists the current
   name→feather assignments (all live entries in the state file), replacing
   the "clear the roster... reconstruct from worktrees/branches/broods"
   resume step with a direct read.
4. FC-4: The state file is concurrency-safe under the same pattern as
   `internal/spec/ids.go`'s ID allocator (an exclusive flock guarding
   read-modify-write), so two concurrent `roster assign`/`release` calls never
   race to the same species.
5. FC-5: `implementation.md` §3.1 (pair spawn) is rewritten to call
   `fledge roster assign --feather FTHR-### --pair` for the species instead of
   the prose allocation rule, and §6 (crash/resume) is rewritten to call
   `fledge roster` to reconstruct the roster instead of discarding and
   guessing it; §3.5 (teardown) calls `fledge roster release <name>` for each
   confirmed-shutdown member instead of freeing the species in context.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [x] AC-1: `fledge roster assign --feather FTHR-### --pair` allocates the
  first unused species and returns both role names; a repeated call with all
  18 species in use returns a numeric-suffixed species (e.g. `adelie-2`);
  covered by a `cmd/fledge` txtar test (FC-1).
- [x] AC-2: `fledge roster release <name>` marks a member released; the
  species is unavailable for reallocation until every member sharing it is
  released, then becomes available again; covered by a txtar test exercising
  a pair where only one member is released first (FC-2).
- [x] AC-3: `fledge roster --json` lists current name→feather assignments and
  omits fully-released (freed) species; covered by a txtar test (FC-3).
- [x] AC-4: A unit test in the package implementing the allocator demonstrates
  two concurrent `assign` calls never allocate the same species (mirroring
  the existing `AllocateAndCreate` concurrency test pattern) (FC-4).
- [x] AC-5: `implementation.md`'s §3.1, §3.5, and §6 (core source in
  `internal/bootstrap/core/skills/fledge-orchestrate/`) are rewritten per
  FC-5; this repo's scaffolded copy is refreshed to match.
- [x] AC-6: `go test ./...` passes and `fledge preen` reports the scaffold
  healthy after `fledge init --refresh`.

## Out of Scope
- Extending `fledge brood --owner` to perform allocation — declined during
  interrogation; roster state is tracked separately from the lock/brood file.
- Any change to the 18-species naming scheme itself (the species list, the
  `<role>-<species>` format, or the numeric-suffix overflow rule) — this
  plumage makes the existing rules CLI-enforced, not different rules.
- Automatic reassignment or renaming of a worker already spawned under a
  species — `roster release` only marks a member released; it does not
  rename or restart anything.
- Any change to `fledge brood`/`fledge abandon` themselves.

## Open Questions
None — the design decision (new `roster` command vs. extending `brood`) and
priority were resolved during interrogation.
