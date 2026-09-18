# Fledge agent command plan — Fable review

**Reviewer:** Claude Code Fable 5.1

**Herdr agent:** `fable-review`, pane `wQ:p5`

**Status:** Review-only findings for discussion; these are not accepted plan revisions.

The completed reviewer response is reproduced verbatim below, including its completion marker.

---

## Verdict

The plan is implementable and its shape is right: three packages, one connection per request, snapshot-driven preflight, explicit effects in a JSON envelope, no lifecycle waiting in `message`. I found no flaw that invalidates the design. I did find one wire-level correctness gap that will misbehave outside a scratch server, two places where the plan contradicts itself, one unverified table that the whole `--model` feature rests on, and one consequence of a settled preference that hits this repository's own required checks. Those should be settled before stage 1 starts. The rest is simplification and specification gaps.

`★ Insight ─────────────────────────────────────`
The Herdr wire protocol carries no caller identity. "Caller context" exists only as environment variables the server injected into the pane, and the only method that accepts it on the wire is `pane.current` via `caller_pane_id`. Every other method that says "falls back to the caller's ambient context" is really saying "falls back to whatever the UI has focused". A direct-socket client like Fledge therefore has to make every resolution explicit, or it silently inherits UI focus. Several findings below are this one fact showing up in different places.
`─────────────────────────────────────────────────`

## Prioritized findings

**1. High, correctness. Repository resolution can fall back to UI focus on the wire.**
Evidence: plan L137-143 and L180-183; `reference/herdr/api/worktree.md` worktree.open (L120-125: "owning repository is resolved from `workspace_id` or `cwd`, falling back to the caller's ambient context"), worktree.create (L26-32), worktree.list (L79-82). The validated `worktree.open` example sends only `path` and succeeds, but it ran on a scratch server whose only workspace was that repo.
Example: user runs `fledge agent spawn --name r --harness codex --worktree /projects/backend/.fledge/worktrees/review-api` while the Herdr UI has an unrelated workspace focused. Fledge sends `{"path": ...}` only. The server resolves the owning repository from the focused workspace, not from the path, and either errors or opens the wrong repository.
Smallest correction: state that Fledge never omits both `workspace_id` and `cwd` on any `worktree.*` call. For `--worktree <path>`, send `cwd: <path>` alongside `path`. For precedence step 3, send Fledge's working directory explicitly as `cwd`. Same rule for `tab.create`: always send `workspace_id` (tab.md L122 says null means "default/current workspace"). Add a test asserting these fields are present on the wire.

**2. High, correctness. "Caller pane context" is undefined and the obvious implementation is stale after a pane move.**
Evidence: plan L77 and L278; `environment.md` L25 ("A moved process keeps its inherited `HERDR_PANE_ID`, so inside that process the old pane ID still resolves"); `api/pane.md` L253-260 (`pane.current` with `caller_pane_id`).
Example: the calling agent's pane was moved from `w1` to `w3`. Its environment still says `HERDR_WORKSPACE_ID=w1` and `HERDR_PANE_ID=w1:p2`. If Fledge trusts the workspace variable, the default new tab lands in `w1`, not the caller's actual workspace.
Smallest correction: define caller resolution as: read `HERDR_PANE_ID`, call `pane.current` with it as `caller_pane_id`, and take `workspace_id` and `cwd` from the returned pane. Ignore `HERDR_WORKSPACE_ID` and `HERDR_TAB_ID`. Also clarify L276 "require the documented Herdr environment": require only `HERDR_SOCKET_PATH`. Require a resolvable caller pane only for default placement, with an error that names `--workspace` and `--pane` as the fix. Otherwise `list`, `message`, and explicit placement are unusable from a shell outside Herdr even with the socket path set.

**3. High, correctness. The `--model` mapping asserts `--model VALUE` for twenty kinds with no evidence.**
Evidence: plan L207-215 cites sources only for Hermes and Letta. Nothing in `reference/herdr/` documents native flags for any kind; `AgentStartParams.kind` is a free string in the schema.
Example: a kind whose model selector is actually `-m` only, or a positional, or `--model provider/model`. Fledge creates the tab and pane, then `agent.start` launches a command that exits or shows a usage error, and the outcome is `partial` with an orphaned pane. The failure lands after mutations, which is the worst place.
Smallest correction: the mapping table must carry a citation per kind, the same way Hermes and Letta do, or the initial version supports `--model` only for kinds actually verified and rejects it for the rest with "pass the native flag via `--args`". Either is fine. A blanket claim is not.

**4. Medium, consequence of a settled preference. Worktrees under the primary checkout are inside the tree that tools walk.**
Evidence: plan L149-159; AGENTS.md requires `gofmt -l .`; plan L358-363 repeats it. Verified in a scratch repo: `gofmt -l .` lists `.fledge/worktrees/x/bad.go`, while `go vet ./...` and `go test ./...` skip dot directories. `git clean -dfx` in the primary deletes ignored paths, so it deletes every managed worktree.
Example: a reviewer worktree has an unformatted file. Every `gofmt -l .` run in the primary checkout reports it, and the project's own completion check fails for a file outside the project.
I am not asking to revisit the storage location. The concrete consequence just needs to be documented for the user of the feature, and this repository's verification commands should be written as `gofmt -l $(git ls-files '*.go')` or equivalent once the feature is in use here.

**5. Medium, internal contradiction. Omitting `cwd` under Herdr's `follow` policy is a UI-focus fallback.**
Evidence: plan L124-126 ("omit the API's `cwd` field") versus L77 and L278 ("never fall back to another client's UI focus"). `raw/default-config.toml` L49-52: `follow` inherits "the source pane/workspace". `api/workspace.md` L93: `source_workspace_id` is "Workspace whose focused pane supplies the `follow` cwd policy". `raw/skill.md` L117-121 tells agents to pass `--cwd "$PWD"` explicitly to preserve their directory.
Example: an agent in `/repo` runs the default `fledge agent spawn --name reviewer --harness codex`. The new tab's shell inherits the directory of whatever pane is focused in that workspace, which may be a `/tmp` scratch pane the human is using. The reviewer starts in the wrong repository, and nothing in the output warns.
This does touch the settled "Herdr cwd policy on ordinary omitted cwd" preference. Smallest correction that keeps the preference mostly intact: when a caller pane resolves (finding 2), default `cwd` to that pane's `cwd` from `pane.current`, which is Herdr's data, not Fledge's process directory. Omit `cwd` only when there is no caller pane. On `workspace.create`, pass `source_workspace_id` set to the caller's workspace so `follow` has a deterministic source. If the user keeps pure omission, the plan should at least drop the claim at L278 and document that default placement inherits focus-dependent directories.

**6. Medium, usability. `agent_not_ready` will be the common path in worktree mode, and Fledge has no recovery surface.**
Evidence: `api/agent.md` L379-381 (blocked during startup returns immediately; name stays usable for read and send-keys); `raw/skill.md` L137 ("Wait until the agent becomes idle before prompting"). Plan L294 and L320-329 define outcomes but never map this code.
Example: Claude Code or Codex launched in a freshly created worktree shows a folder-trust dialog. `agent.start` returns `agent_not_ready`. The agent is running and named, but Fledge's spawn reports failure.
Smallest correction: map `agent_not_ready` to `partial`, include the agent name, pane ID, and `agent_status` in `effects`, and print the exact `herdr agent read <name>` and `herdr agent send-keys <name> ...` recovery commands, since Fledge deliberately has no read or send-keys. Map startup `timeout` to `partial` with the pane and an explicit "a process may be running in this pane" note.

**7. Medium, decision needed. Strict protocol validation will break on every Herdr self-update.**
Evidence: plan L285; `protocol.md` "Protocol and version negotiation" (a wrong-protocol client "simply risks a method the server does not recognize, which surfaces as `invalid_request`"); `raw/skill.md` L211 ("A missing method is not permission to stop or upgrade a server").
Example: Herdr ships 0.9.2 with protocol 23 and unchanged `agent.*` methods. Fledge refuses every command until someone edits a constant.
Smallest correction: do not gate. Record the server `version` and `protocol` from the snapshot in the JSON result for diagnostics, and let `invalid_request` be the incompatibility signal. If a gate is kept, gate only on `protocol < 22`. Either way, drop the extra `ping` round trip before `message`; the `type` discriminant on the response already proves it is a Herdr server.

**8. Medium, simplification. Conflict detection for native model options is the universal parser the plan disclaims.**
Evidence: plan L221 ("Cover documented long/short aliases and Codex's `-c/--config model=...` form") versus L221 ("do not build a universal harness parser") and L32.
Example: to reject conflicts correctly you need per-kind knowledge of `-m`, `--model`, `--model=`, `-c model=`, `--config model=`, and each kind's `--` handling. That table will be wrong for some kind and will reject valid invocations.
Smallest correction: detect only the exact token sequence Fledge itself would inject for that kind (for most kinds `--model` and `--model=`), reject that, and document that anything else is passed through and the harness decides precedence. Or drop detection entirely. A caller passing both `--model` and a native model flag is doing so deliberately.

**9. Medium, usability. The JSON `result` shape for spawn is undefined and tied to Herdr's `AgentInfo`.**
Evidence: plan L314 ("the final Herdr result when available"), L298 lists what readable output reports but nothing equivalent for JSON.
Example: an AI caller wants the pane ID and worktree path after spawn. It must know that `result.agent.pane_id` comes from Herdr's `agent_started` and that the worktree path is somewhere in `effects`. When Herdr adds or renames fields, Fledge's contract moves.
Smallest correction: define a fixed spawn result: `name`, `kind`, `workspace_id`, `tab_id`, `pane_id`, `cwd`, `worktree_path` (nullable), `argv`, `agent_status`, `server_version`. Nest the raw Herdr payload under `herdr` if wanted. Do the same for `message` (`pane_id`, `agent_status` after submission) and `list` (an array of the same row shape the table prints).

**10. Low, wording that matters. The `rejected` definition is ambiguous.**
Evidence: plan L323: "Failed with no mutation applied or outstanding uncertainty."
As written it can be read as "rejected with outstanding uncertainty", which is the opposite of the intent and collides with `unknown`. Change to "Failed before any mutation, with no uncertainty." Also add exit codes: 2 for local validation and flag errors, 1 for Herdr or transport failures, mirroring Herdr's own convention so callers can branch without parsing.

**11. Low, specification gaps in flag combinations.**
- L98 "name its initial tab `reviews`": `workspace.create` has no tab label parameter, so this is a `tab.rename` on the returned tab. Same for `--tab` with `--worktree new`. State it, since it adds a mutation step and an effect entry.
- L64 `--label`: this is `pane.rename`. Specify it runs before `agent.start`, so a blocked or timed-out agent still has the label.
- L120 `--focus` in `--pane` mode: no create call carries focus, so it needs `pane.focus` or `agent.focus` after start. Specify which and when.
- L65-66 `--direction` and `--ratio` when the destination turns out to be a new tab: whether a split happens is only known at runtime, so the only workable rule is "ignored unless a split occurs". Document that and report `split: true|false` in the result.
- L180-183 `--worktree <path>` with `--workspace NAME`: not addressed. Either reject or define it as the source workspace for repository resolution.
- L102 "explicit `focused_pane_id` from the session snapshot": that field lives on `layouts[]` (per tab), not on `TabInfo` (`api/session.md`, PaneLayoutSnapshot). Name the source so the implementer does not look for it on the tab.

**12. Low, simplification. Two guards protect against Fledge's own bugs.**
Evidence: plan L165 (containment check) and L176 ("Verify effective exclusion before creating the checkout").
`git check-ref-format --branch` already forbids `..`, leading `/`, `\`, `~`, `:`, and components starting with `.`, so a validated branch cannot escape the directory or collide with the managed `.gitignore`. Git also gives the deepest `.gitignore` precedence, so a `*` in `.fledge/worktrees/.gitignore` cannot be overridden by the root file, `info/exclude`, or a global excludes file. Verified: `git check-ignore` on a not-yet-existing path returns the `*` rule, and the managed `.gitignore` ignores itself. Keep the branch validation and the collision checks. Replace the runtime exclusion verification and the containment check with unit tests, per the "no error handling for impossible scenarios" tenet.

**13. Low, test plan gaps.**
- Unix socket paths are limited to 108 bytes on Linux. `t.TempDir()` under a deep working directory can exceed that and fail with `bind: invalid argument`. Use a short directory under the system temp root for fake sockets.
- Add: duplicate live-name preflight failure issues no `workspace.create`, `tab.create`, or `pane.split`. L342 covers explicit-pane mode only.
- Add: every `worktree.*` and `tab.create` request carries explicit repository or workspace resolution (from finding 1).
- Add: caller resolution uses `pane.current` and ignores `HERDR_WORKSPACE_ID` (from finding 2).
- L349 "model mappings": the test can only assert Fledge's composition, not that the harness accepts it. Say so, and require one manual smoke launch per supported kind before `--model` is documented as supported for it.

**14. Low, decision. The hardcoded 24-kind list drifts with Herdr.**
Evidence: plan L197-205. The list is documentation-only; the schema has no enum. Local validation is still worth keeping because an unknown kind would otherwise fail at `agent.start`, after the tab and pane exist. Keep the list, document that it tracks Herdr 0.9.1, and make the rejection message say so.

## Remaining user decisions

1. **Default `cwd` when a caller pane resolves.** Keep pure omission (Herdr policy, focus-dependent), or default to the caller pane's `cwd` from `pane.current` and omit only without caller context. The second is deterministic and matches Herdr's own skill guidance. My recommendation is the second.
2. **Protocol gate.** None (recommended), `< 22` only, or strict equality.
3. **`--model` scope.** Verified subset with rejection elsewhere, or blanket table with per-kind citations added before implementation.
4. **Native model-conflict detection.** Drop, narrow to Fledge's own injected token (recommended), or keep the alias table.
5. **Message target flags.** Two flags as settled, or one `--target` since names cannot contain `:` and a pane ID always does. Pure taste. The settled form is fine.

## What is sound and should stay simple

- Three packages with the transport adapter as the only interface seam. No builder, no registry, no SDK.
- One connection per request, no automatic retry of mutations, finite deadlines sized to the requested startup timeout.
- Snapshot-driven preflight for names, duplicates, and split anchors, with Herdr authoritative. This is what keeps orphan panes rare.
- `message` returns on submission acknowledgment with no `--wait`, and the plan states plainly that acknowledgment is not a started turn.
- Effects list plus `success`, `rejected`, `partial`, `unknown`, with `unknown` winning over `partial`.
- Agent name grammar doubling as the default branch name. Every valid name is a valid branch.
- `git check-ref-format` via argv, no shell, no `trust_repository`, no model catalog, no list filters.
- Explicit `path` on `worktree.create`, existing-branch and existing-path rejection with a suggested recovery rather than auto-suffixing.
- Managed `.fledge/worktrees/.gitignore` containing `*`. It ignores itself, git will not descend into the checkouts, and no root `.gitignore` edit is needed.

`★ Insight ─────────────────────────────────────`
Two of the highest-value corrections here cost almost nothing in code: always send `cwd` or `workspace_id` on worktree calls, and resolve the caller through `pane.current`. Both replace an implicit server-side fallback with an explicit field. When a protocol offers a "fallback to ambient" path, a non-interactive client should treat it as a bug to avoid, not a convenience to rely on.
`─────────────────────────────────────────────────`

FABLE_REVIEW_COMPLETE
