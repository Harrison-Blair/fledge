# herdr API: addressing and IDs

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](README.md). Wire format: [protocol.md](protocol.md). Access model: [environment.md](environment.md).

herdr addresses topology with three kinds of opaque public ID — workspace, tab, and pane — plus live agent names that follow a pane's current occupant. This file defines the ID grammar, the opacity and stability guarantees, why closed IDs are never reused, how a moved pane is re-identified, the difference between a stable ID and the reorderable display `number`, how `--current` and injected caller context resolve a target, and the rules for targeting an agent by name. Every claim here is grounded in `raw/skill.md` (§"Use IDs and caller context") and corroborated by probe captures. Method-specific params/results are in the `api/*.md` files.

## ID scheme

Public IDs are opaque, stable string handles. Their shapes:

| entity | form | example | notes |
| --- | --- | --- | --- |
| workspace | `w<N>` | `w1`, `w2` | Top-level container. |
| tab | `w<N>:t<M>` | `w1:t1`, `w2:t1` | Workspace-qualified. A tab belongs to exactly one workspace and its ID names that workspace. |
| pane | `w<N>:p<K>` | `w1:p1`, `w2:p1` | Workspace-qualified. A pane's ID names the workspace it currently lives in. |

The `w`, `t`, and `p` integers are drawn from independent per-workspace/per-session counters, so `w2:p1` and `w1:p1` are distinct panes that merely share suffix `1`. Because tab and pane IDs embed the workspace, the same pane acquires a **different** ID when moved to another workspace (see below). IDs appear in every entity object as `workspace_id`, `tab_id`, `pane_id`, and in cross-references such as a pane's `tab_id`/`workspace_id`.

## Opacity and stability

Treat IDs as opaque handles, not as structured data to compute over. The rules:

- **Do not derive an ID.** Read it from a JSON response (a create/split result, a list, `pane current`, a snapshot). skill.md: "Parse IDs from JSON responses. Do not derive them from sidebar order or examples."
- **A workspace ID is stable across reordering.** Moving a workspace changes only its display `number`, not its `workspace_id`. Probe `workspace.move` moved `w2` to index 0; afterward `w2` shows `number: 1` and `w1` shows `number: 2`, but the `workspace_id` values are unchanged.

  ```json
  {"request":{"id":"wm1","method":"workspace.move","params":{"workspace_id":"w2","insert_index":0}},
   "response":{"id":"wm1","result":{"type":"workspace_list","workspaces":[
     {"workspace_id":"w2","number":1,"label":"second-ws",…},
     {"workspace_id":"w1","number":2,"label":"--label docs-ws-renamed",…},
     {"workspace_id":"w3","number":3,"label":"docs-probe",…}]}}}
  ```

  Validated 2026-08-19 against herdr 0.8.2 (`probes/raw/workspace-move.json`).
- **A tab ID is stable across reordering within its workspace.** Probe `tab.move` reindexed tabs; each kept its `tab_id` (`w1:t1`, `w1:t2`, `w1:t3`, …) while `number` reflected the new order (`probes/raw/tab-move.json`).
- **Labels are not identifiers.** `label` is display text set by rename and can collide (two workspaces both labeled `fledge` in `probes/workspace-list.json`). Never target by label.

## Display numbers vs stable IDs (public pane numbers)

There are two distinct "numbers" and they must not be conflated:

- The **integer inside a pane ID** (`p<K>` in `w1:p3`) is the stable, public pane number. It is assigned once, embedded in the handle, and — for the life of that pane in that workspace — never changes and never gets reused by a different pane (see next section). It is what you pass to `pane` methods. `PaneInfo` has no separate `number` field; the `pane_id` suffix *is* the pane's public number.
- The **`number` field** on `WorkspaceInfo` and `TabInfo` is a 1-based **display ordinal** — the position shown in the sidebar/tab bar. It is reassigned on reorder (evidence above) and is not a durable handle. Do not target a workspace or tab by `number`; target by `workspace_id`/`tab_id`.

In short: the number baked into an ID string is stable; the standalone `number` field is a mutable display position.

## Non-reuse of closed IDs

Closed tab and pane IDs are **not reused**. skill.md: "Closed tab and pane IDs are not reused." Once `w1:p2` is closed, a later new pane in `w1` gets the next free number (`w1:p3`, …), never `w1:p2` again. Consequences for a client:

- A stale reference to a closed pane will not silently resolve to a different, newly created pane — it resolves to nothing. Targeting a closed ID yields a not-found style error rather than acting on an unrelated pane.
- You may safely cache an ID for the lifetime of the entity and detect closure by a failed lookup; you will never get a false positive from number recycling.

## Pane-move re-identification

Moving a pane into a different tab or workspace **re-identifies** it: because the pane ID embeds its container, the pane receives a new workspace-qualified ID and its old ID is retired. skill.md: "A pane moved into another workspace receives a new workspace-qualified pane ID."

`pane.move` returns a `pane_move` result whose `move_result` (`PaneMoveResult`) reports both the new and previous identities:

| field | meaning |
| --- | --- |
| `move_result.pane` | The moved pane's new `PaneInfo`, including its **new** `pane_id`. Continue with `move_result.pane.pane_id`. |
| `move_result.previous_pane_id` | The pane's ID **before** the move. Retired for general use. |
| `move_result.previous_tab_id`, `previous_workspace_id` | Prior container IDs. |
| `move_result.created_tab` / `created_workspace` | Populated when the move created a destination tab/workspace (`--new-tab` / `--new-workspace`). |
| `move_result.closed_tab_id` / `closed_workspace_id` | Populated when the move emptied and closed the source tab/workspace. |
| `move_result.focused_pane_id`, `move_result.changed`, `move_result.reason`, `source_layout`, `target_layout` | Focus outcome, whether anything changed, an optional reason, and before/after layouts. |

skill.md rule: "After `pane move`, continue with `.result.move_result.pane.pane_id` or the live agent name. The old value is reported as `.result.move_result.previous_pane_id`; only the moved process's inherited caller context keeps resolving that old ID, so do not use it as a general agent target." That is, the old ID still resolves *only* from inside the moved process (via its inherited `$HERDR_PANE_ID`), not for external callers — external callers must switch to the new `pane_id`.

Example (move into a freshly created tab; note the new tab and the previous/new IDs):

```json
{"id":"cli:pane:move","result":{"type":"pane_move","move_result":{
  "changed":true,
  "created_tab":{"tab_id":"w1:t3","workspace_id":"w1","number":3,"label":"moved-tab","pane_count":1,"focused":false,"agent_status":"unknown"},
  "pane":{"pane_id":"w1:p3","tab_id":"w1:t3","workspace_id":"w1","label":"--label docs-pane",…},
  "previous_pane_id":"w1:p3","previous_tab_id":"w1:t1","previous_workspace_id":"w1",
  "focused_pane_id":"w1:p3",
  "source_layout":{…},"target_layout":{…}}}}
```

Validated 2026-08-19 against herdr 0.8.2 (`probes/scratch/pane-move.json`). In this capture the move stayed within `w1` and the pane's numeric suffix happened to be unchanged (`w1:p3` before and after, only the tab changed); the general rule still stands — always read the returned `pane` and treat `previous_pane_id` as retired for external targeting.

## Resolving --current and caller context

herdr injects the calling pane's identity into every managed pane as environment variables, and pane commands can target "the pane I am running in" without knowing its ID:

- Injected context (see [environment.md](environment.md)): `$HERDR_WORKSPACE_ID`, `$HERDR_TAB_ID`, `$HERDR_PANE_ID`.
- **Prefer `--current`** on a pane command to target the calling pane. skill.md: "Omitting a target may use the UI-focused pane, which can belong to the user or another client." So an omitted target is *not* a safe default — it can act on someone else's focused pane.
- Equivalent explicit forms: pass `--pane "$HERDR_PANE_ID"`, or pass a concrete ID read from a prior response. Many pane subcommands accept `--pane <ID>` and `--current` interchangeably (`pane current`, `pane layout`, `pane split`, `pane neighbor`, `pane edges`, `pane focus`, `pane resize`, `pane zoom`, `pane input`, `pane swap`).
- `pane current --current` resolves and returns the calling pane's full `PaneInfo`, the reliable way to learn your own `pane_id`, `tab_id`, `workspace_id`, and current agent occupant. Probe `probes/pane-current.json` returns `pane_id: "w2:p1"` with its `tab_id`/`workspace_id`.

Discovery calls that resolve live state from caller context:

```text
herdr workspace list
herdr tab list --workspace "$HERDR_WORKSPACE_ID"
herdr pane current --current
herdr pane list --workspace "$HERDR_WORKSPACE_ID"
herdr agent list
```

## Agent-name targeting

Agent commands (`herdr agent …`, methods under `agent.*`) accept a target that is **either** a live agent name **or** the pane ID currently hosting that agent. They do **not** accept terminal IDs or bare agent-kind labels. Rules from skill.md §"Understand layout, panes, and agents":

- **Name grammar:** a name matches `[a-z][a-z0-9_-]{0,31}` — lowercase-first, then lowercase letters, digits, `_`, or `-`, up to 32 characters total.
- **Uniqueness:** a name must be unique among live agents. Uniqueness is only over *live* agents, so a name is reusable after its holder is gone.
- **Lifetime / opacity:** "A name follows the current pane occupant and is cleared when that agent exits, is released, or is replaced." The name is a handle to whatever agent currently occupies the pane, not a durable identity of a process.
- **Pane ID as target:** you may instead pass the hosting pane's ID (e.g. `w2:p1`); after a `pane move`, the new `pane_id` (or the still-valid agent name) is the correct target — see the move section.
- **Assigning a name:** `agent start <name> --kind <kind> --pane <id>` names the agent it starts; `agent rename <target> <name>|--clear` changes or clears it.
- Agent identity fields in `AgentInfo`: `name` (the assignable unique name, nullable), `agent` (detected kind label such as `claude`/`codex`, nullable), and `agent_session` (`AgentSessionInfo`: `source`, `agent`, `kind` ∈ `{id,path}`, `value`) which carries the native session ref. Probe `probes/agent-get.json` resolves target `claude` to `pane_id: "w2:p1"` with `agent_session.value` a UUID. Target by `name`/pane-id; read `agent`/`agent_session` as attributes, not as targets.
