# Fledge agent commands — implementation and review plan

**File:** `reference/plan/agent-command.md`

**Status:** Implemented and independently verified, including the repaired live focus response. Git-aware formatting, vet, race tests, build, whitespace checks, rebuild, and installation passed. Live Claude/Codex launch and messaging, focused Claude launch, and agent listing passed.

**Review status:** Astra and [Claude Code Fable plan review](agent-command-fable-review.md) complete. [Claude Code Fable implementation review and triage](agent-command-fable-code-review.md) are complete: branch-namespace preflight, optional caller resolution, append-only ignore updates, and the observed `pane_info`/`pane` focus response are repaired and independently verified. Both original Fable critiques are preserved verbatim. Optional output/refactoring choices remain for discussion.

**Live follow-up:** immediate split-to-launch sometimes returned `agent_pane_busy`; the partial outcome preserved the new pane, and manual launch into that same pane succeeded after shell inspection. No automatic retry or readiness policy was added. This readiness race remains open. Live worktree behavior and all harness/model combinations were not verified.

## 1. Objective and design constraints

Add:

```text
fledge agent spawn
fledge agent list
fledge agent message
```

Use Herdr’s Unix socket API directly, following the repository’s Herdr reference and authoritative raw schema.

The design has received an independent Astra review. Incorporate its findings on targeting unnamed agents, source-workspace resolution, partial failures, directory defaults, and generated-worktree exclusion.

Follow existing project conventions:

- Fresh Cobra command constructors; parents register their children.
- Command packages contain wiring; internal packages contain behavior.
- Internal packages do not import Cobra.
- Explicit dependencies and small typed options/results.
- Test-first Go changes and independent verification.
- Develop on `dev` or a feature branch based on it.

Apply the Go design-pattern reference selectively: a small transport adapter and package boundaries are sufficient. Avoid builders, global clients, per-harness class hierarchies, a workflow framework, a full generated Herdr SDK, or a persistent agent registry.

## 2. CLI contract and behavior

### Spawn interface

```sh
fledge agent spawn \
  --name reviewer \
  --harness codex \
  [--model MODEL] \
  [placement and customization flags] \
  [--args=TOKEN ...] \
  [-- NATIVE_ARG...]
```

| Flag | Behavior |
|---|---|
| `--name` | Required unique live agent name; enforce Herdr’s name grammar. |
| `--harness` | Required Herdr agent kind. |
| `--model` | Optional native model-selection translation. |
| `--workspace` | Workspace name; destination ordinarily, source repository in worktree mode. |
| `--workspace-id` | Existing workspace ID; mutually exclusive with `--workspace`. |
| `--tab` | Destination tab name, scoped to the destination workspace. |
| `--tab-id` | Existing destination tab ID; mutually exclusive with `--tab`. |
| `--pane` | Existing shell pane ID; separate placement mode. |
| `--worktree` | `new` creates a checkout; a path opens an existing checkout. |
| `--branch` | New worktree branch; defaults to agent name. |
| `--base` | Optional starting ref for a new worktree. |
| `--cwd` | Explicit shell directory ordinarily; source repository directory in worktree mode. |
| `--env` | Repeatable `KEY=VALUE` for newly created ordinary shells. |
| `--focus` | Request focus; default is background creation. |
| `--label` | Label for the resulting agent pane. |
| `--direction` | Split direction: `right` or `down`; default `right`. |
| `--ratio` | Optional split ratio; omission delegates to Herdr. |
| `--timeout` | Startup duration, default `30s`; must satisfy Herdr’s bounds: greater than `3s`, at most `5m`. |
| `--args` | Repeatable exact native argument token. |
| `--json` | Structured operation outcome. |

Reject positional arguments before the native `--` separator. Do not interpret a shell command string.

**Default placement**

With no placement flags, create a new tab in the caller’s workspace, then start the agent in its root pane.

Resolve caller identity with `pane.current {caller_pane_id: HERDR_PANE_ID}` and use its returned workspace/tab/pane IDs instead of potentially stale environment IDs. Do not substitute the UI-focused workspace when caller context is missing.

**Named workspace/tab placement**

Resolve names exactly and case-sensitively:

- Workspace names are scoped to the session.
- Tab names are scoped to the selected workspace.
- No match: create the destination container.
- One match: reuse it.
- Multiple matches: fail and report matching IDs.

For example:

```sh
fledge agent spawn --name reviewer --harness codex \
  --workspace backend --tab reviews
```

Behavior:

- Neither exists: create `backend`, name its initial tab `reviews`, and use its initial pane.
- Workspace exists but tab does not: create `reviews` and use its root pane.
- Both exist: split a fresh pane into `reviews`.

When reusing a tab, split relative to its focused pane from the session snapshot’s per-tab layout data. Do not rely on global UI focus.

When only a workspace is selected, create a new tab there. If the workspace was just created, use its initial tab instead of creating redundant layout.

ID selectors never create containers. A tab ID can establish its owning workspace; an accompanying workspace selector must agree with that ownership.

Name resolution and creation are not atomic. Retain ambiguity errors and ID escape hatches; do not introduce a lock service.

**Explicit pane placement**

```sh
fledge agent spawn --name reviewer --harness codex --pane w2:p3
```

Use the existing pane directly. Herdr determines whether it can accept an interactive agent launch.

Reject workspace, tab, and worktree placement selectors with this mode. Also reject `--cwd`, `--env`, `--direction`, and `--ratio`, because this mode does not create a shell or split.

`--label` and `--focus` remain applicable. Apply labels first and focus the exact destination pane before `agent.start`. Direction and ratio apply only when splitting in other placement modes.

**Directory behavior**

For ordinary creation, omit the API’s `cwd` field unless `--cwd` was supplied. This honors Herdr’s configured policy, including `follow`, home, process directory, or a fixed directory.

Explicit IDs make placement deterministic; directory selection still follows the configured policy. Report the resulting cwd when available. Do not promise that omission always inherits Fledge’s working directory.

### Worktree placement

```sh
fledge agent spawn --name reviewer --harness codex \
  --workspace backend --worktree new --branch review-api
```

In this mode, `backend` is an **existing source workspace**. A missing source name is an error; it must never create a source workspace.

Follow Herdr’s source-resolution precedence:

1. Explicit source workspace name or ID.
2. Explicit `--cwd`.
3. For creation, Fledge’s current working directory; for opening, the absolute supplied checkout path.

Pass the resolved source explicitly on every relevant Herdr call. A workspace source excludes competing cwd parameters.

Document that an explicit source workspace takes precedence over `--cwd`.

Use `worktree.list` to resolve the repository. Its `source.repo_root` identifies the primary checkout, even when the caller is inside a linked worktree.

New checkout location:

```text
<primary-checkout>/.fledge/worktrees/<branch>
```

Branch slashes produce nested directories. For example:

```text
/projects/backend/.fledge/worktrees/feature/review-api
```

Pass this explicit path to `worktree.create`. Do not use Herdr’s default worktree storage directory or add an alternate creation-path flag.

Canonicalize the primary checkout. Reject existing symlink components beneath it that could redirect managed-directory, branch-parent, or ignore-file writes. Existing parents must be directories and the managed ignore file must be regular. This guards existing filesystem state without promising protection against concurrent replacement.

Before creating filesystem artifacts:

- Validate the exact branch name and derived destination.
- Reject branch shorthands and paths escaping the managed directory.
- Reject an existing branch or destination.
- Use argument-vector Git invocations where local Git validation is necessary; never invoke a shell.

A branch/path collision must suggest either choosing another branch or opening the existing checkout. Do not automatically reuse it or invent a suffix.

Create or update:

```text
<primary-checkout>/.fledge/worktrees/.gitignore
```

Ensure its final applicable rule excludes managed contents, using `*`. Preserve existing content and handle missing trailing newlines. Verify effective exclusion before creating the checkout. Do not change the project’s root `.gitignore`.

Opening an existing checkout:

```sh
fledge agent spawn --name reviewer --harness codex \
  --worktree /projects/backend/.fledge/worktrees/review-api
```

- Newly opened workspace: use its initial tab and pane.
- Already-open workspace: create a new tab.
- With `--tab NAME`: reuse/create that destination tab using the ordinary rules.
- Rename an initial tab only when newly created; never rename an existing tab merely because `worktree.open` returned it.

The agent’s directory is the worktree checkout, including when creating an additional tab or pane there.

Reject `--env` with worktree mode, as agreed. Reject `--pane` and `--tab-id` with worktree mode in this initial version. Document the tab-ID restriction as a Fledge limitation.

`--branch` and `--base` apply only to `--worktree new`.

### Harness and native-argument handling

Accept Herdr’s documented 24 kinds:

```text
pi claude codex gemini cursor devin agy cline omp mastracode
opencode copilot kimi kiro droid amp grok hermes kilo qodercli
qwen letta maki muse
```

Do not use `integration.list` as the allowlist for launching agents.

Use a centralized, small mapping:

| Harness group | `--model` translation |
|---|---|
| `pi`, `claude`, `codex`, `gemini`, `cursor`, `devin`, `agy`, `cline`, `omp`, `opencode`, `copilot`, `kimi`, `droid`, `grok`, `kilo`, `qwen`, `letta`, `maki` | `--model VALUE` |
| Hermes | `chat --model VALUE` |
| `kiro`, `amp`, `muse`, `mastracode`, `qodercli` | Reject `--model` pending verified interactive launch support; native arguments remain available. |

For Hermes, preserve a single leading `chat` if the caller already supplied it. Do not duplicate the subcommand. The documented model selector belongs to its chat command. [Hermes CLI reference](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/reference/cli-commands.md).

The [model-evidence appendix](#appendix-model-option-evidence) records primary documentation and installed-help evidence for the mapping. Mastracode evidence currently establishes headless support only; current Qoder documentation names `qoder`, not Herdr’s `qodercli`. Do not infer verified interactive translations from either.

Model identifiers remain opaque strings. Do not maintain a model catalog.

Compose repeated `--args` tokens in occurrence order, followed by trailing native arguments. Preserve token contents exactly, including commas and whitespace, using Cobra string-array binding.

When `--model` is supplied, reject recognized conflicting native model options rather than choosing precedence. Cover documented long/short aliases and Codex’s `-c/--config model=...` form. Stop interpreting native options after a native `--` separator; do not build a universal harness parser.

Return Herdr’s actual launched `argv`. Document that forwarding a native model option does not override every harness’s resume behavior; Letta explicitly documents limitations when resuming conversations. [Letta CLI reference](https://docs.letta.com/platform/cli/reference).

### List

```sh
fledge agent list [--json]
```

List all live agents recognized by Herdr, including unnamed agents and agents started outside Fledge.

Readable columns:

```text
NAME  HARNESS  STATUS  WORKSPACE  TAB  PANE  CWD
```

Always include pane IDs. Represent missing optional values consistently with `-`. Print a clear empty-state message when no agents exist.

Do not introduce filters, persistence, or a Fledge-only ownership concept.

### Message

```sh
fledge agent message \
  (--name reviewer | --pane w2:p3) \
  (--body "Review this change" | --file task.md) \
  [--json]
```

Require exactly one target and one content source.

- `--file -` reads stdin.
- Preserve multiline content and trailing newlines.
- Reject empty or invalid UTF-8 content before sending.
- Submit through `agent.prompt` without lifecycle waiting.
- Do not add `--wait` or `--until`.

Success means **submitted**: Herdr acknowledged writing the prompt and Enter. It does not mean the agent read it, began work, or completed the task.

Preserve `agent_blocked` and other Herdr errors. Do not answer approval dialogs automatically.

## 3. Implementation, transport, and outcomes

Keep three clear responsibilities:

- `cmd/agent` and its child command packages: flag binding, command registration, process streams.
- `internal/agent`: validation, placement, argument composition, message input, formatting, and operation outcomes.
- `internal/herdr`: socket transport and the typed API subset required by these commands.

Use package-local helpers and narrow consumer-owned interfaces only where substitution is needed for tests. Add `doc.go` where project conventions require it.

**Transport contract**

- Require `HERDR_ENV=1` and `HERDR_SOCKET_PATH`. Missing caller context prevents implicit placement but does not prevent fully explicit targets.
- Resolve ambient placement through caller pane context.
- Never fall back to another client’s UI focus.
- Use one connection per request.
- Send UTF-8 newline-delimited JSON with `id`, `method`, and `params`.
- Include `{}` for parameterless methods.
- Read complete response lines; do not impose the default scanner’s small token limit.
- Preserve structured server error codes and messages.
- Support cancellation and finite deadlines. Startup transport deadlines must allow the requested server timeout to complete.
- Attempt calls without a numeric protocol-version gate or a version-only probe. Validate response envelopes and required fields; return actual server errors.
- Do not automatically retry mutating requests.

Follow Herdr’s documented create/split → returned pane ID → `agent.start` sequence. Do not invent shell-readiness sleeps or terminal-content heuristics.

**Preflight and execution**

Perform local validation before creating resources. Use snapshot data to resolve selectors, check duplicate live agent names, and identify split anchors. Treat preflight as advisory under concurrency; Herdr remains authoritative.

Track confirmed resource effects as each step succeeds. Preserve resources following failure, including a created or updated managed ignore file.

**Output contract**

Readable output should report the agent target, pane, resolved directory, and relevant worktree path.

With `--json`, emit one operation object on stdout for success or failure. Use nonzero exit status for failure, and avoid duplicate Cobra error text.

Use a shared envelope:

```json
{
  "operation": "agent.spawn",
  "status": "success",
  "result": {},
  "effects": [],
  "error": null
}
```

- `result`: stable Fledge fields; unavailable values are null. Spawn includes requested name/harness, detected harness, agent status, workspace/tab/pane IDs, cwd, actual argv, worktree path, and whether placement split a pane. List includes `agents` with name, detected harness, agent status, IDs, and cwd. Message includes resolved agent fields and `submitted: true` after acknowledgement. Preserve known target context on failure without inventing status or argv.
- `effects`: known created, reused, or updated resources, with their kind and known ID/path. Reused resources alone do not count as mutations.
- `error`: code, message, and failing phase when unsuccessful.

Status meanings:

| Status | Meaning |
|---|---|
| `success` | Requested operation completed. |
| `rejected` | Failed with no mutation applied or outstanding uncertainty. |
| `partial` | Confirmed mutations preceded failure, or Herdr acknowledged failed startup, even in an existing pane. |
| `unknown` | A mutation may have applied, but its response was lost or unusable. |

Exit 0 for success, 2 for invalid arguments/input, and 1 for runtime failures. Server startup `agent_not_ready` and startup `timeout` are partial; readiness is not confirmed and a timed-out process may still be running. A client timeout or lost mutation response is unknown. Unknown outcome takes precedence over partial completion. Preserve known effects alongside the uncertain phase.

Never invent missing IDs, imply that a failed launch left no process, or recommend blindly retrying an uncertain message submission.

## 4. Test and acceptance plan

All Go changes follow the repository’s test-first rule: add a failing test, confirm the expected failure, then implement the minimum passing behavior.

Use fake Unix socket servers, temporary directories, and disposable local Git repositories. Automated tests must not contact the user’s live Herdr session or launch paid harness sessions.

Cover:

- Command registration, help, required flags, exclusions, and fresh command state.
- Default new-tab placement and caller-context resolution.
- Named container creation/reuse, duplicate labels, ID selectors, and ownership mismatches.
- Existing-tab splitting with explicit anchor IDs.
- Explicit-pane validation and absence of unnecessary creation calls.
- Source-workspace lookup-only behavior in worktree mode.
- Primary-checkout storage when invoked from a linked worktree.
- Branches containing slashes, collisions, invalid branches, and destination containment.
- Managed ignore-file creation/update, missing final newline, conflicting patterns, effective exclusion, and preserved local effects after failure.
- Worktree create/open and already-open behavior.
- Exact argument preservation, model mappings, Hermes subcommand composition, and recognized conflicts.
- Unnamed agents in list and message targeting by pane.
- Body, file, stdin, multiline preservation, invalid content, blocked submission, and absence of lifecycle waiting.
- NDJSON framing, large responses, server errors, differing protocol numbers, malformed envelopes, cancellation, and transport failures.
- Lost mutation responses, partial completion including failed startup in an existing pane, and accurate JSON outcomes and exit codes.
- Native composition tests establish generated argv, not actual harness acceptance.
- Filesystem symlink guards and explicit worktree source fields on the wire.
- Existing help/version behavior.

Replace recursive formatting checks with the [Git-aware formatting check](../../README.md#development), covering existing tracked and new nonignored Go files with NUL-safe filenames, skipping deleted files and ignored managed worktrees. Update contributor instructions, README, and CI consistently.

Before declaring implementation complete, run that formatting check and report:

```sh
go vet ./...
go test -race ./...
go build -o /tmp/fledge .
```

A separate verifier must inspect the current diff against this plan and the code-smell references, run relevant checks, and verify any subsequent repairs.

## 5. Delivery sequence and Claude Code review

Implement in bounded stages:

1. Transport, typed errors, and socket tests.
2. Agent options, model mapping, placement, and worktree behavior.
3. Message/list behavior, structured outcomes, and Cobra wiring.
4. Usage documentation and independent verification.

Delegate file edits under the active orchestrate workflow. Keep implementation and verification separate. Serialize shared-package edits unless file ownership is explicit.

Documentation must include representative commands, selector precedence, worktree source semantics, model-support gaps, cwd behavior, timeout units, and recovery from partial or unknown outcomes.

After implementation and independent verification, request a read-only code review through the existing Claude Code Fable session. Surface the critique and proposed follow-up decisions for discussion. Its review should check:

- Consistency between flag combinations and the chosen Herdr semantics.
- Accurate native argument composition without speculative parsing.
- Clear distinction between submission acknowledgment and task completion.
- Correct local-file effects and worktree exclusion.
- Minimal package/API design with no unnecessary indirection.
- Adequate tests for side effects and uncertain outcomes.

No migration, persistent configuration format, remote-session management, automatic cleanup, release change, or additional agent subcommands are included.

## Appendix: model-option evidence

This table preserves the planning audit's evidence, including the recorded
installed-help versions. Those help commands were checked in that earlier audit;
this documentation update did not rerun them. A dash means no short model alias
was verified for Fledge's conflict check, not proof that none can ever exist.
Model values are passed through unchanged. Documentation links identify the
upstream sources consulted; they are not guarantees about future CLI releases.

| Herdr kind | Generated native arguments | Known short model alias | Evidence |
|---|---|---|---|
| `pi` | `--model VALUE` | — | Recorded `pi 0.84.4` help; [Pi usage](https://pi.dev/docs/latest/usage). |
| `claude` | `--model VALUE` | — | Recorded Claude Code `2.1.275` help; [Claude Code CLI usage](https://docs.anthropic.com/en/docs/claude-code/cli-usage). |
| `codex` | `--model VALUE` | `-m` | Recorded `codex-cli 0.154.0` help; also documents `-c`/`--config` overrides, including the root `model` key. |
| `gemini` | `--model VALUE` | `-m` | [Gemini CLI configuration](https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/configuration.md). |
| `cursor` | `--model VALUE` | — | Recorded `cursor-agent 2026.08.25-3e8eec8` help. The audited version documents `--model`; older short-alias claims were not adopted. |
| `devin` | `--model VALUE` | — | [Devin CLI models](https://docs.devin.ai/cli/models) and [commands](https://docs.devin.ai/cli/reference/commands). |
| `agy` | `--model VALUE` | — | [Antigravity CLI settings](https://www.antigravity.google/docs/cli/settings): terminal-session model override and interactive `/config` behavior. No slug-only restriction is inferred from headless examples. |
| `cline` | `--model VALUE` | `-m` | [Cline CLI reference](https://github.com/cline/cline/blob/main/docs/cli/cli-reference.mdx). Provider selection is complementary, not a model conflict. |
| `omp` | `--model VALUE` | — | [OMP upstream README](https://github.com/unsigned-gg/omp). |
| `opencode` | `--model VALUE` | `-m` | Recorded OpenCode `1.18.25` global help; [OpenCode models](https://dev.opencode.ai/docs/models/). |
| `copilot` | `--model VALUE` | — | [GitHub Copilot CLI command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference). |
| `kimi` | `--model VALUE` | `-m` | [Kimi command reference](https://www.kimi.com/code/docs/en/kimi-code-cli/reference/kimi-command). |
| `droid` | `--model VALUE` | `-m` | [Droid CLI reference](https://docs.factory.ai/droid-cli/cli-reference). `--spec-model` selects a distinct role and is not treated as a conflict. |
| `grok` | `--model VALUE` | `-m` | [Grok Build getting started](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/01-getting-started.md) includes interactive model selection. |
| `hermes` | `chat --model VALUE` | `-m` | [Hermes CLI commands](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/reference/cli-commands.md). Preserve a supplied leading `chat` instead of duplicating it. |
| `kilo` | `--model VALUE` | `-m` | [Kilo CLI reference](https://kilo.ai/docs/code-with-ai/platforms/cli-reference). |
| `qwen` | `--model VALUE` | `-m` | [Qwen Code settings](https://github.com/QwenLM/qwen-code/blob/main/docs/users/configuration/settings.md). |
| `letta` | `--model VALUE` | `-m` | [Letta CLI reference](https://docs.letta.com/platform/cli/reference). Resume behavior may affect selection; Fledge does not insert `--new`. |
| `maki` | `--model VALUE` | `-m` | [Maki CLI documentation](https://maki.sh/docs/cli/), covering interactive and headless use. |

Five kinds remain launchable without Fledge's generated model option:

| Herdr kind | Why generated `--model` is unavailable |
|---|---|
| `kiro` | The audited [CLI commands](https://kiro.dev/docs/reference/cli-commands/) and [models documentation](https://kiro.dev/docs/models/) establish picker/persistent settings, not a verified per-launch model argument. Fledge does not mutate persistent settings. |
| `amp` | No interactive per-launch model option was verified in the audited [CLI documentation](https://ampcode.com/docs/cli) or [settings](https://ampcode.com/docs/cli/settings). This is an evidence gap, not proof of absence. |
| `muse` | No model-launch flag was verified from the audited [Muse announcement](https://research.meta.ai/blog/introducing-muse-code-and-muse-spark-1-2/). |
| `mastracode` | [Mastra Code headless documentation](https://code.mastra.ai/headless) establishes `--model`/`-m` for headless use; the audit did not establish acceptance in the interactive launch used here. |
| `qodercli` | The audited [Qoder CLI reference](https://docs.qoder.com/cli/cli-reference) documents `qoder --model`/`-m`. Herdr launches `qodercli`; equivalence of those executables was not verified. |

When Fledge's `--model` is present, conflict checking covers the documented long
option, the aliases above, and Codex root-model `-c`/`--config` overrides. It stops
at a native `--` and does not claim to parse every native CLI option. Native
arguments remain available for all kinds, including the five evidence gaps.

Automated tests verify argument composition and rejection behavior, not native
harness acceptance. They do not launch paid harness sessions. Future additions
to this mapping require evidence for the actual interactive executable Herdr
launches, plus corresponding composition and conflict tests.
