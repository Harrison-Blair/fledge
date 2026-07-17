---
id: PLM-041
title: "Provider adapters: Codex and Claude work-subprocess drivers behind a common interface"
status: hatched
priority: P1
authored: 2026-07-17T21:45:12Z
agent: fledge-orchestrate/planning
fledge_version: 0.6.10
---

# PLM-041: Provider adapters: Codex and Claude work-subprocess drivers behind a common interface

## Context
Second plumage of the multi-harness migration program. It sits directly on
**PLM-040 (Process Runner)** and gives fledge the ability to run *specific
providers* — Codex and Claude Code — as non-interactive work subprocesses,
behind one provider-neutral interface, so roles (planner, implementer, reviewer)
can be mapped to providers and the rest of the system never depends on
provider-specific CLI shapes.

**Terminology settled in interrogation — two distinct "adapter" concepts:**
- **Harness adapters** (existing, unchanged): the format-only scaffold under
  `internal/bootstrap/adapters/<harness>` (`.claude`/`.codex`/`.agents`) that
  teaches an *interactive* LLM harness (layer 1) how to drive fledge. Not touched
  by this plumage.
- **Provider adapters** (new, this plumage): the drivers that teach fledge how to
  launch a *non-interactive provider CLI* (layer 3 — `codex exec`, `claude -p`) as
  a work subprocess via the Process Runner, parse its structured output, and report
  a result. Plain name, deliberately no bird metaphor.

**Realization — hybrid** (settled): a Go `ProviderAdapter` interface with **two
thin built-in implementations, Codex and Claude**, each reading a **declarative
provider profile**. Built-in default profiles for Codex and Claude are **embedded
in the binary** (as fledge already embeds its scaffold) and are **overridable by a
user config file**, so the tool works out of the box and swapping an executable
path, flag, or variant is pure config with zero Go. The profile declares the
*invocation shape and where things live*; the Go implementation owns the
*parsing mechanics* (reading JSONL / streaming-JSON, applying the extraction rule).
Adding a fundamentally new output format is a small new Go implementation; every
lesser change is config. This honors fledge's "swap pieces agnostically /
config-driven where cheap" ethos without pretending format parsing is free.

**Provider-CLI facts are verified at implementation, not assumed here.** The two
repo research docs (`docs/research_prompt.md`, `docs/google_ai_mode_response.md`)
are unrelated model-routing material, and no repo doc describes these CLIs; the
integration points come from the migration brief (`codex exec`, JSONL + JSON-Schema
constrained final output; `claude -p`, JSON/streaming-JSON, `--allowedTools` /
`--disallowedTools`, max-turns, session continuation). Exact flag names and output
shapes are confirmed against the official documented CLIs at feather-authoring time
and captured in the provider profiles.

**Interface surface — the core subset now** (settled): `startTask`, `cancelTask`,
`getResult`, `getCapabilities`, and `validateAuthentication` (needed by
`providers doctor`). A **streaming hook** surfaces the Process Runner's framed
output, but semantic event *normalization* is P3's job. `resumeTask` is **designed
into the interface** (adapters capture provider session ids when offered), but the
initial workflow **defaults to fresh contexts**, so real resume *usage* is settled
in the plan/implement and review/fix phases. `estimateAvailability` /
`normalizeUsage` are deferred to a later phase.

**Boundaries carried in from interrogation:**
- **Artifact-agnostic result** (P2 vs P3): `getResult` returns a **thin structured
  result** — the extracted final payload (or a path to it), the captured-output
  location, the session id (if any), and the exit/why-ended status from the Process
  Runner. It does **not** emit typed artifacts; the Artifact Protocol
  (`schema_version`/`run_id`/`producer`/…) is owned by P3, which wraps this result.
- **Output-schema: mechanism here, contents later.** `startTask` accepts an
  **optional output-schema parameter** the adapter passes through to providers that
  support schema-constrained output; `getCapabilities` reports whether a provider
  supports it. The **schema contents** (the shape of plan.json, review.json, …) are
  defined in P3/P4.
- **Permission→flag translation: mechanism here, policy in P6.** The adapter
  translates a supplied permission/capability spec into provider flags
  (`--allowedTools`/`--disallowedTools`, sandbox/read-only settings, worktree
  working directory via the Process Runner's cwd + scrubbed-env mechanism). The
  per-role **policy** — what a planner/implementer/reviewer may do — is owned by P6.

**Diagnostics:** a `fledge providers doctor` command reports, per configured
provider, pass/warn/fail on (1) the executable being present on PATH and runnable
(e.g. `--version`) and (2) a provider-declared **lightweight auth check** that
verifies credentials are present/valid **without printing any secret** (the Process
Runner's redaction choke point applies). A token-costing real round-trip is opt-in
(`--probe`), off by default. `--json` is supported, consistent with existing fledge
commands.

The change is net-new Go: a provider-adapter package with the interface + Codex and
Claude implementations, embedded default profiles + a user-config override loader,
and the `providers doctor` CLI command. It consumes PLM-040's runner and does not
touch the existing spec/ledger commands.

## User Stories
- As the fledge harness controller, I want to launch a role's work as either Codex
  or Claude through one common interface, so that I can map planner/implementer/
  reviewer to providers without embedding provider-specific CLI knowledge anywhere
  but the adapter.
- As a fledge user, I want Codex and Claude to work out of the box yet let me
  override the executable path, flags, or output handling in a config file, so that
  I can point fledge at my installed CLIs or swap a provider variant without a
  rebuild.
- As a fledge user, I want `fledge providers doctor` to tell me whether each
  provider's CLI is installed and authenticated — without ever printing a secret —
  so that I can diagnose setup problems before starting a run.
- As a fledge maintainer, I want the adapter to hand back a thin structured result
  and leave typed artifacts to a later layer, so that the artifact schema can evolve
  without reworking the provider drivers.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: A Go **`ProviderAdapter` interface** exists exposing the core subset —
   `startTask`, `cancelTask`, `getResult`, `getCapabilities`, `validateAuthentication`
   — plus a `resumeTask` designed into the interface, and a streaming hook that
   surfaces the Process Runner's framed output. All task execution goes through the
   PLM-040 runner; the interface itself carries no provider specifics.
2. FC-2: Two built-in implementations — **Codex** (`codex exec`, JSONL / schema-
   constrained final output) and **Claude** (`claude -p`, JSON/streaming-JSON,
   `--allowedTools`/`--disallowedTools`, max-turns) — implement the interface, each
   thin and driven by a declarative provider profile.
3. FC-3: A **provider profile** is a declarative record with at least: `id`,
   `executable`, `output_mode`, `base_args`, `prompt_delivery` (argv | stdin),
   flag-mappings for read-only/sandbox + allowed/disallowed-tools + max-turns,
   `session_capture` (where a session id appears in output), `result_extraction`
   (where the final payload lives), and `auth_check` (command + success condition,
   no secret output). The profile declares invocation shape + where things live; the
   Go implementation owns parsing.
4. FC-4: **Default profiles for Codex and Claude are embedded in the binary** and are
   **overridable by a user config file**; with no user config the built-ins are used.
   Overriding an executable path, flag, or variant requires config only, no Go change.
5. FC-5: `startTask` accepts a task request (role, prompt/input, working directory,
   permission/capability spec, and an **optional output-schema**). The adapter
   translates the permission spec into provider flags and the working directory into
   the runner's cwd, delivers the prompt via argv or stdin per the profile, and
   passes the output-schema through to providers that support it. It does not decide
   permission *policy* (that is P6) and does not define schema *contents* (P3/P4).
6. FC-6: `getResult` returns a **thin, artifact-agnostic structured result**: the
   extracted final payload (or a path to it), the captured-output location, the
   session id (if any), and the exit/why-ended status from the runner. It emits no
   typed artifact; P3 wraps it.
7. FC-7: `getCapabilities` returns a capability record enumerating at least:
   supports-resume, supports-output-schema, supports-streaming, read-only mode,
   allowed/disallowed-tools control, and max-turns — so the controller can negotiate
   behavior per provider.
8. FC-8: `cancelTask` cancels an in-flight task by driving the runner's cancellation
   (graceful-signal → grace → SIGKILL, process-group scoped), so provider
   subprocesses never orphan.
9. FC-9: `validateAuthentication` runs the profile's lightweight auth check and
   reports credentials present/valid or not **without printing any secret**, relying
   on the runner's redaction choke point.
10. FC-10: A **`fledge providers doctor`** command reports per configured provider
    pass/warn/fail on executable-present-and-runnable and the lightweight auth check;
    a real token-costing round-trip is opt-in via `--probe` (off by default);
    `--json` is supported and no secret is ever printed.
11. FC-11: `resumeTask` is present in the interface and adapters capture provider
    session ids when the provider offers them, but the initial workflow default is
    **fresh contexts**; this plumage does not wire resume into any workflow (that is
    P4/P5).
12. FC-12: The provider adapters depend on PLM-040's runner for all process
    execution and do not launch subprocesses by any other means; they do not read or
    write the existing fledge ledger.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [ ] AC-1: A `ProviderAdapter` Go interface exists with `startTask`, `cancelTask`, `getResult`, `getCapabilities`, `validateAuthentication`, `resumeTask`, and a streaming hook; a unit test asserts both built-in impls satisfy it and that execution is routed through the PLM-040 runner (verified with a fake command, no real provider CLI).
- [ ] AC-2: Codex and Claude implementations exist, each driven by a declarative profile; unit tests exercise both against **scripted fake commands** that emulate JSONL and streaming-JSON output, asserting correct result extraction with no real provider CLI or network.
- [ ] AC-3: A profile schema is defined and loaded; a test asserts embedded default profiles for Codex and Claude are present and that a user config file overrides selected fields (e.g. executable path, a flag) without code changes.
- [ ] AC-4: A test asserts `startTask` translates a permission/capability spec into the expected provider flags and sets the runner cwd, delivers the prompt via argv or stdin per the profile, and passes an optional output-schema through when the profile/capabilities declare support (and omits it otherwise).
- [ ] AC-5: A test asserts `getResult` returns a thin structured result (payload-or-path, captured-output location, session id, exit/why-ended) and emits no typed artifact.
- [ ] AC-6: A test asserts `getCapabilities` reports the enumerated capabilities for each built-in provider, and that a capability difference (e.g. one supports output-schema, one does not) is reflected.
- [ ] AC-7: A test asserts `cancelTask` terminates an in-flight fake task via the runner's escalation path with no orphaned process, and that `validateAuthentication` reports present/absent credentials without emitting any secret value.
- [ ] AC-8: A txtar acceptance test exercises `fledge providers doctor` end-to-end against scripted fake provider executables — reporting per-provider pass/warn/fail for executable presence and auth check, honoring `--json`, keeping `--probe` off by default, and printing no secret.
- [ ] AC-9: A test/import assertion confirms the provider-adapter package executes only via the PLM-040 runner and does not depend on `internal/ledger`.
- [ ] AC-10: `fledge preen` passes and `go test ./...` (new adapter/profile/doctor tests plus any updated fixtures) is green after the changes.

## Out of Scope
- **The typed Artifact Protocol** (plan.json/review.json envelopes, schema_version/
  run_id/producer/consumer/status, referencing large outputs by path) and the
  **normalized event model / `events.jsonl`** — owned by P3. P2 returns a thin
  result and surfaces a streaming hook; it does not define artifacts or normalize
  events.
- **The run state machine** and any reconciliation with the existing fledge ledger —
  P3.
- **Output-schema contents** (the actual shape of plan/review/fix artifacts) — P3/P4;
  P2 only wires the pass-through mechanism.
- **Per-role permission policy** (what planner/implementer/reviewer may do, network
  gating, command allowlists) and the **redaction ruleset** — P6. P2 does the
  mechanism (flag translation, cwd/env via the runner), not the policy.
- **Actual resume usage in a workflow**, and `estimateAvailability` /
  `normalizeUsage` — deferred (P4/P5 and later). `resumeTask` is only designed-in here.
- **The plan→implement→review/fix workflow, worktrees, human approval, validation
  pipeline** — later phases (P4/P5/P6).
- **Team/hosted/production concerns** (multi-user credential brokering, shared
  provider accounts, hosted concurrency) — flagged, not built; this is a single-user
  personal tool for now.

## Open Questions
- Exact package name and the concrete Go shapes of the task request, result, and
  capability records — settled at feather time.
- The precise profile config format (embedded default location + user-config path
  and file format) and the exact documented flag names / output-record shapes for
  `codex exec` and `claude -p` — verified against the official CLIs and captured at
  feather time.
- The exact lightweight auth-check per provider that verifies credentials without a
  token-costing call or any secret output — confirmed against each CLI at feather time.
