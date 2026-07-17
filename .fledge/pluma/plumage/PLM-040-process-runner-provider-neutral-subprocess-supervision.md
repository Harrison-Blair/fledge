---
id: PLM-040
title: "Process Runner: provider-neutral subprocess supervision"
status: hatched
priority: P1
authored: 2026-07-17T21:36:02Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.10
---

# PLM-040: Process Runner: provider-neutral subprocess supervision

## Context
This is the **root of the multi-harness migration program** — the foundational
plumage every later phase (provider/worker adapters, run state machine, artifacts,
the plan→implement→review workflow, permissions/approval) is built on. It has no
program dependencies; everything else depends on it.

**Where it sits — the three-layer model** (settled during interrogation):
1. **Interactive LLM driver** — the human-facing interface (Claude Code or Codex
   used *interactively*, like the current orchestrator session). A human talks to
   it; it issues `fledge` CLI commands that advance the workflow. It is itself
   swappable, and is *not* built here.
2. **fledge CLI (Go) — the harness controller.** Owns the state machine,
   permissions, artifacts, and process lifecycle. This is where determinism lives.
   The Process Runner is a library inside this layer.
3. **Non-interactive provider WORK subprocesses** — `claude -p` / `codex exec`,
   launched per step by fledge, doing the actual planning/implementation/review
   work and emitting structured output. **The Process Runner launches and
   supervises these.**

Determinism comes from moving the real work into non-interactive provider CLI
invocations that fledge owns end-to-end. Grounded fact: **fledge launches no agent
subprocesses today** — the only `os/exec` calls in the repo are `git`
(`internal/repo`, `internal/scan`, `internal/cli/brood.go`, `internal/cli/init.go`).
This primitive is therefore net-new substrate, not a refactor of existing exec code.

**Execution model** (settled): synchronous, foreground, **per-step CLI
invocations — no daemon**. A `fledge` step launches a provider subprocess, blocks
until it exits (or times out / is cancelled), persists its output/exit result to a
caller-provided sink, and returns. Durable state lives on disk between steps; the
"controller" is a sequence of CLI invocations driving the state machine, not a
resident process. The runner is nonetheless built as a **reusable library** able to
supervise ≥1 concurrent child processes, so a future daemon and concurrent runs are
not precluded (deferred to a later phase). This is the durable groundwork the
**superseded PLM-038** (deterministic, harness-owned coordination) and **PLM-039**
(per-run process isolation, multiplexed supervision) pointed at: owning the
subprocess is what makes state a deterministic fact rather than something inferred
from an interactive agent's messages or idle notifications.

**Provider neutrality** (settled): the Process Runner knows **nothing** about
Codex or Claude. It launches an arbitrary command with arguments, optional stdin,
a working directory, and an explicit environment, and supervises it. All
provider-specific knowledge — CLI flags, JSONL vs streaming-JSON formats, JSON
Schema-constrained output, session/resume — lives in the provider/worker adapters
(the next phase). Because it is provider-neutral, it is fully testable with
scripted fake commands and needs no real provider CLI.

**Two clean splits** carried in from interrogation, both putting the *mechanism*
here and the *policy/semantics* in later phases:
- **Events:** the runner owns only the **transport level** — incremental
  line-framed reads, stdout/stderr separation, output-size limits, raw framed
  output to a caller-provided sink. Semantic parsing into a normalized event model
  and the per-run `events.jsonl` artifact belongs to the adapter/state-machine
  phases. **The runner does not touch the existing fledge ledger**
  (`status`/`verdict`/`escalation`/`signal`); the ledger is reconciled with the new
  run state machine in a later phase, not here.
- **Redaction:** the runner provides the redaction **mechanism** — a filter applied
  to all captured output before it is persisted, the single choke point where a
  secret can be scrubbed before it ever reaches a log or artifact. The redaction
  **ruleset** (patterns, credential-file paths, env values to mask) is declarative
  configuration owned by the later security/permissions phase.
- **Isolation:** the runner provides the **mechanism** to launch each child with a
  caller-specified working directory and an **explicit environment spec** (a scrubbed
  allowlist, not the full inherited environment) — the enforcement choke point for
  credential isolation ("no agent process sees another provider's credential
  files"). The per-role **policy** of what is permitted is owned by the later
  permissions phase.

The change is net-new Go in a focused package (e.g. `internal/proc` or
`internal/runner`) plus a thin CLI surface for exercising/observing it; it does not
alter existing spec/ledger commands.

## User Stories
- As the fledge harness controller, I want to launch a provider CLI as a supervised
  subprocess — with a set working directory, a controlled environment, optional
  piped input, enforced timeout, and captured framed output — so that the real work
  runs deterministically under fledge's control rather than inside an interactive
  agent's turn loop.
- As a fledge maintainer, I want the runner to be entirely provider-neutral and
  exercised by scripted fake commands, so that its supervision behavior (timeout,
  cancellation, exit handling, size limits, redaction) is tested deterministically
  without any real provider CLI or network.
- As a security-conscious single-user of fledge, I want every subprocess launched
  with a scrubbed environment and its output passed through a redaction choke point,
  so that one provider's credentials can never leak into another agent's environment,
  logs, or artifacts.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: A provider-neutral **Process Runner** library exists that launches an
   arbitrary executable with arguments and supervises it to completion. It has no
   knowledge of Codex, Claude, or any specific provider; all provider specifics are
   supplied by the caller.
2. FC-2: Execution is **synchronous/foreground**: a launch call blocks until the
   child exits, times out, or is cancelled, then returns a structured result
   (exit code, why-it-ended: exited / timed-out / cancelled / failed-to-start,
   and duration). No daemon or persistent supervisor process is introduced.
3. FC-3: The runner captures **stdout and stderr separately**, reading them
   **incrementally as line-framed chunks** and writing raw framed output to a
   **caller-provided sink** (an `io.Writer`-style interface), so output is streamed
   as it is produced rather than buffered whole. It performs **no semantic parsing**
   of that output.
4. FC-4: The runner enforces a configurable **per-task timeout** and a configurable
   **maximum captured-output size**; exceeding either terminates the child and is
   reported distinctly in the result (timed-out vs output-limit-exceeded).
5. FC-5: **Cancellation** is supported via a context/cancel signal. Termination
   escalates deterministically — a graceful signal (SIGTERM/SIGINT), a bounded grace
   period, then SIGKILL — and targets the child's **process group** so descendant
   processes are also terminated (no orphans).
6. FC-6: The runner accepts optional **stdin**: it writes caller-provided input
   bytes to the child's standard input and then closes it, so large prompts or
   artifact payloads can be piped rather than passed as arguments (avoiding argv
   size limits). It remains provider-neutral — it writes only the bytes it is handed.
7. FC-7: The runner launches each child with a **caller-specified working directory**
   and an **explicit environment specification** (a controlled allowlist/scrubbed
   set — not the full inherited process environment). This is the mechanism enabling
   worktree confinement and credential isolation; the per-role *policy* is out of
   scope (owned by the later permissions phase).
8. FC-8: The runner applies a **redaction filter** to all captured output before it
   reaches the caller's sink — the single choke point for scrubbing sensitive values.
   The runner owns the *mechanism* (a pluggable filter over the output stream); the
   redaction *ruleset* is supplied by the caller/configuration and is out of scope
   here. With no ruleset supplied, output passes through unmodified.
9. FC-9: The runner **does not read or write the existing fledge ledger** and does
   not depend on `internal/ledger`; it is a standalone supervision primitive.
   Integration with run state and events is performed by later phases.
10. FC-10: The runner library is structured to supervise **one or more concurrent
    child processes** from a single caller, so future concurrency and a potential
    daemon are not precluded; the current CLI usage may drive it one child at a time.
11. FC-11: A minimal CLI surface exists to launch and observe a supervised process
    for testing/diagnostics (provider-neutral — it runs whatever command it is
    given), supporting `--json` output of the structured result consistent with
    existing fledge command conventions.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [ ] AC-1: A provider-neutral runner package exists (e.g. `internal/proc`/`internal/runner`) that launches and supervises an arbitrary command; unit tests drive it with scripted fake commands only (no real provider CLI, no network).
- [ ] AC-2: A test asserts synchronous completion returns a structured result distinguishing exited(code), timed-out, cancelled, and failed-to-start, including duration; a test asserts no background/daemon process persists after the call returns.
- [ ] AC-3: Tests assert stdout and stderr are captured separately and delivered to a caller-provided sink incrementally (a fake command that emits interleaved streamed lines is observed as framed chunks, not one final buffer) with no semantic interpretation by the runner.
- [ ] AC-4: Tests assert timeout termination (a fake command that sleeps past the timeout is killed and reported timed-out) and output-size-limit termination (a fake command that emits past the cap is killed and reported limit-exceeded), as distinct outcomes.
- [ ] AC-5: Tests assert cancellation via context terminates the child, that termination escalates graceful-signal → grace period → SIGKILL, and that a spawned descendant process is also terminated (process-group kill; verified by a fake command that forks a child).
- [ ] AC-6: A test asserts caller-provided stdin is written to the child and closed (a fake command echoes its stdin back through the captured output).
- [ ] AC-7: Tests assert the child runs in the caller-specified working directory and sees only the explicitly provided environment (a fake command prints its cwd and environment; inherited/undeclared variables are absent).
- [ ] AC-8: A test asserts the redaction filter scrubs matching values from captured output before the sink receives them, and that with no ruleset output is unmodified.
- [ ] AC-9: A bootstrap/package test (or import-graph assertion) confirms the runner package does not depend on `internal/ledger`.
- [ ] AC-10: A txtar acceptance test exercises the minimal launch/observe CLI surface end-to-end against a scripted fake command, including `--json` output of the structured result.
- [ ] AC-11: `fledge preen` passes and `go test ./...` is green after the changes.

## Out of Scope
- **Any provider knowledge** — Codex/Claude CLI flags, `codex exec`/`claude -p`
  invocation shapes, JSONL vs streaming-JSON parsing, JSON-Schema-constrained
  output, session capture/resume. All owned by the next phase (provider/worker
  adapters). The runner only runs commands it is handed.
- **The normalized event model and `events.jsonl` artifact**, and semantic
  interpretation of subprocess output — owned by the artifacts/state-machine phase.
- **The run state machine**, artifact protocol, and any reconciliation with the
  existing fledge ledger — later phases; this plumage deliberately does not touch
  the ledger.
- **The redaction ruleset and the per-role permission/credential-isolation policy**
  — owned by the security/permissions phase; only the redaction and
  environment-scrubbing *mechanisms* are built here.
- **A long-lived controller/daemon, live event streaming to a UI, and actual
  concurrent-run orchestration** — deferred to the extensibility phase; the runner
  is only *built not to preclude* them.
- **Naming of the two adapter concepts** (interface/harness adapters vs
  provider/worker adapters) — settled in the next phase, where those adapters are
  actually introduced; this plumage references them only descriptively.
- **Team/hosted/production concerns.** This is a single-user personal tool for now;
  what multi-user or hosted operation would additionally require (e.g. per-user
  process sandboxing beyond OS defaults, resource quotas) is flagged, not built.

## Open Questions
- Exact package name and CLI verb(s) for the launch/observe surface, and the precise
  Go interface shape of the caller-provided sink and environment spec — settled at
  feather-authoring time.
- Default values for the per-task timeout, grace period, and max-output-size (and
  whether any are exposed as flags vs config keys now vs deferred to the config
  work) — baseline defaults chosen at feather time; the config *ownership* is the
  later phase.
- The exact redaction-filter interface (streaming line filter vs chunk filter) so it
  composes cleanly with the ruleset the security phase will supply — settled at
  feather time.
