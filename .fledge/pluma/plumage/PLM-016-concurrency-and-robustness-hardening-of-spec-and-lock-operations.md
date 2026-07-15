---
id: PLM-016
title: Concurrency and robustness hardening of spec and lock operations
status: hatched
priority: P1
authored: 2026-07-15T15:12:51Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# PLM-016: Concurrency and robustness hardening of spec and lock operations

## Context
fledge is built for multi-agent workflows in which several agents operate on the same repo
at once — the implementation phase explicitly dispatches concurrent workers. Three latent
defects surfaced by a codebase audit undermine that premise:

- **Duplicate IDs under concurrency.** Spec creation allocates an ID by scanning existing
  filenames, then creates the file with an exclusivity guard on the *full* filename
  (ID + title). Two concurrent creations with different titles compute the same next ID and
  both succeed, producing two specs that share one ID. The shadowed spec then silently
  disappears from readiness and dependency dispatch until validation eventually flags it.
- **One corrupt claim file breaks the whole listing.** Listing active feather claims aborts
  entirely if any single claim file is unparseable, hiding all the healthy claims. Such a
  partial file is reachable because claim files are written by streaming directly into the
  live path with no atomic temp-then-rename, so a crash mid-write leaves a truncated file.
- **Network operations can hang forever.** The self-update command's HTTP fetches use a
  client with no timeout, so a stalled or black-holed peer blocks the command indefinitely.

None of these appear in normal single-user operation, which is why they shipped; all three
are directly reachable by the concurrency and networking the tool itself relies on. This
plumage hardens each so concurrent use cannot corrupt or lose specs and network operations
cannot hang.

## User Stories
- As an agent (or user) creating specs concurrently with others, I want ID allocation to be
  atomic, so that two simultaneous creations can never share an ID or silently shadow each
  other.
- As an operator inspecting active feather claims, I want the listing to survive one corrupt
  claim file, so that a single bad file never hides every healthy claim.
- As the tool writing a claim, I want the write to be atomic, so that a crash mid-write can
  never leave a partial claim file behind.
- As a user running self-update on a flaky network, I want the fetch to time out, so that a
  stalled peer can't hang the command forever.

## Functional Criteria
1. FC-1: Concurrent spec creation never allocates the same ID to two specs; the
   allocate-then-create step is atomic with respect to other fledge processes.
2. FC-2: Listing active claims skips (and surfaces) an individual unparseable claim file
   and still returns every healthy claim, rather than failing the whole operation.
3. FC-3: Writing a claim file is atomic — an interrupted write never leaves a partial file
   in place of a valid one.
4. FC-4: The self-update HTTP operations fail within a bounded time when a peer accepts the
   connection but does not make progress, rather than blocking indefinitely.

## Acceptance Criteria
- [ ] AC-1: A test drives concurrent ID allocation and asserts all allocated IDs are
  distinct; it fails against the pre-fix code (observed duplicate) and passes after.
- [ ] AC-2: A test with one corrupt claim file alongside healthy ones asserts the listing
  returns the healthy claims (and signals the bad one) instead of erroring; fails before,
  passes after.
- [ ] AC-3: Claim-file writes are atomic (temp-then-rename), verified by a test; no partial
  file is observable in place of a valid claim.
- [ ] AC-4: A test with a deliberately stalled HTTP peer asserts the self-update fetch
  returns an error within a bounded time rather than hanging; fails before, passes after.
- [ ] AC-5: `fledge preen` passes and the full test suite is green after the changes.

## Out of Scope
- Broader locking of spec *mutation* (status/set/criteria) — this plumage covers ID
  *allocation* races and claim-file integrity only; the existing brood lock already guards
  per-feather work.
- Any change to the self-update download/checksum/swap logic beyond adding request
  timeouts.
- Retry/backoff policy for network operations — a bounded timeout is the goal, not a retry
  framework.

## Open Questions
None.
