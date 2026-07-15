---
id: PLM-017
title: Acceptance-test coverage for stateful CLI command output
status: hatched
priority: P2
authored: 2026-07-15T15:23:05Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# PLM-017: Acceptance-test coverage for stateful CLI command output

## Context
Two agent-facing behaviors of the stateful CLI commands are entirely untested, so a
regression in either would ship green:

- **Stale-claim (`pid_alive`) detection.** `fledge broods` reports whether each claim's
  holder process is still alive — the `(pid not alive)` text suffix and the `pid_alive`
  JSON field are how agents spot claims abandoned by dead workers. Every existing test seeds
  a claim with the live test process's own PID, so the liveness check always returns true
  and the stale-detection branch never executes. An inverted check or a dropped field would
  pass the whole suite.
- **`--json` output shape of the mutation/report commands.** The project contract is that
  every command supports `--json`, but the machine-readable output of `brood`, `abandon`,
  `broods`, `status`, and `set` is never asserted. `abandon --json` in particular has a real
  branch whose `status` key is `null` without `--fledged` and a string with it. A renamed
  key, dropped field, or flipped nil/string branch would ship silently — and these are the
  outputs agents parse to drive the workflow.

These commands are exercised today only for their human-readable output via the
testscript/txtar harness. This plumage extends that same harness to pin the stale-detection
behavior and the `--json` shapes, so the machine contract agents depend on cannot regress
unnoticed. It adds test coverage only — no production behavior changes.

## User Stories
- As an agent scanning for abandoned work, I want the `broods` stale-claim detection
  (`pid_alive` false / `(pid not alive)`) to be guarded by a test, so that a regression in
  the liveness check is caught before release.
- As an agent parsing command output, I want each stateful command's `--json` shape pinned
  by a test — including `abandon`'s null-vs-string `status` branch — so that a changed key
  or field can't silently break my parsing.

## Functional Criteria
1. FC-1: A test seeds a claim whose holder PID is not alive and asserts `broods` renders the
   stale marker in text (`(pid not alive)`) and `pid_alive: false` in `--json`; the
   live-PID case still asserts alive.
2. FC-2: A test asserts the `--json` output shape (keys and value types/branches) of
   `brood`, `abandon`, `broods`, `status`, and `set`, including `abandon --json`'s `status`
   being null without `--fledged` and the terminal state with it.

## Acceptance Criteria
- [ ] AC-1: A test drives `broods`/`broods --json` against a claim with a not-alive PID and
  asserts both the `(pid not alive)` text and `"pid_alive": false`; verified failing against
  the current fixtures (which never seed a dead PID) in the sense that no such assertion
  exists, and passing once added.
- [ ] AC-2: Tests assert the documented `--json` shape of `brood`, `abandon`, `broods`,
  `status`, and `set`, including `abandon`'s null-vs-string `status` branch; each assertion
  would fail if the corresponding key/field/branch were changed.
- [ ] AC-3: A deliberate perturbation confirms the tests bite — e.g. temporarily inverting
  the liveness check or renaming a JSON key makes the new tests fail (recorded as evidence),
  then reverted.
- [ ] AC-4: `fledge preen` passes and the full test suite is green.

## Out of Scope
- Any change to the commands' behavior or output shape — this plumage pins existing
  behavior, it does not modify it.
- Raising the measured `internal/cli` coverage percentage via in-process unit tests; the
  chosen approach is txtar acceptance tests (black-box, real invocation), which assert
  behavior without instrumenting the subprocess.
- Coverage for commands already asserted via `--json` (colony, vee, ready, etc.).

## Open Questions
None.
