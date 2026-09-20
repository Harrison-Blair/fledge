# herdr API: addressing and IDs

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](README.md). Wire format: [protocol.md](protocol.md). Access model: [environment.md](environment.md).

herdr addresses topology with three kinds of opaque public ID — workspace, tab, and pane — plus live agent names that follow a pane's current occupant. This file defines the ID grammar, the opacity and stability guarantees, why closed IDs are never reused, how a moved pane is re-identified, the difference between a stable ID and the reorderable display `number`, how `--current` and injected caller context resolve a target, and the rules for targeting an agent by name. Every claim here is grounded in `raw/skill.md` (§"Use IDs and caller context") and corroborated by probe captures. Method-specific params/results are in the `api/*.md` files.

## ID scheme

Public IDs are opaque, stable string handles. Their shapes:

| entity | form | example | notes |
| --- | --- | --- | --- |
| workspace | `w<C>` | `w1`, `wA`, `wZ` | Top-level container. |
| tab | `w<C>:t<C>` | `w1:t1`, `w1:tA` | Workspace-qualified. A tab belongs to exactly one workspace and its ID names that workspace. |
| pane | `w<C>:p<C>` | `w1:p1`, `w1:pA` | Workspace-qualified. A pane's ID names the workspace it currently lives in. |

`<C>` is a per-workspace/per-session counter, but it is **not** a decimal integer: herdr 0.9.1 renders it in a Crockford-Base32-style alphabet — `1`-`9`, then `A,B,C,D,E,F,G,H,J,K,M,N,P,Q,R,S,T,V,W,X,Y,Z` (`I`, `L`, `O`, `U` are skipped to avoid look-alikes), then `0`, then two-character combinations (`11`, `12`, … `1Z`, `10`, `21`, …). IDs such as `w1:pA`, `wZ`, `w0`, `w11` are ordinary, not exceptional; a caller that validates or parses IDs with a decimal-only pattern such as `w\d+` or `w\d+:p\d+` will reject valid IDs. `raw/schema.json` places no `pattern` constraint on `workspace_id`/`tab_id`/`pane_id` at all, so this alphabet is a runtime fact, not something the schema documents either way. The counters for workspaces, tabs, and panes are independent, so `w2:p1` and `w1:p1` are distinct panes that merely share suffix `1`. Because tab and pane IDs embed the workspace, the same pane acquires a **different** ID when moved to another workspace (see below). IDs appear in every entity object as `workspace_id`, `tab_id`, `pane_id`, and in cross-references such as a pane's `tab_id`/`workspace_id`.

Validated 2026-09-19 against herdr 0.9.1.

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

  Validated 2026-09-19 against herdr 0.9.1.
- **A tab ID is stable across reordering within its workspace.** Probe `tab.move` reindexed tabs; each kept its `tab_id` (`w1:t1`, `w1:t2`, `w1:t3`, …) across the move. The tab's `number`, however, does **not** reflect the new order the way a workspace's does — see the next section for the corrected behaviour and an example.
- **IDs are scoped to one server.** New in 0.9.1: with saved SSH machines, two machines can
  both have a `w1:p1` or an agent named `reviewer`. Without the global `--machine` prefix a
  command keeps using the inherited session and socket context, and inherited local IDs and
  `--current` never identify remote panes; discover IDs on the target machine.

  Source: `raw/skill.md`, not live-validated (2026-09-19, herdr 0.9.1: requires a second saved SSH machine registered with herdr, out of scope for a single-host scratch session).
- **Labels are not identifiers.** `label` is display text set by rename and can collide (two workspaces both labeled `fledge` in `probes/workspace-list.json`). Never target by label. Renaming two different workspaces to the identical label succeeds for both with no error, and `workspace.get`/`tab.get` with a label string in the id field return a not-found error rather than resolving it — labels are never accepted as a selector.

Validated 2026-09-19 against herdr 0.9.1 (cross-machine ID scoping not exercised — see above).

## Display numbers vs stable IDs (public pane numbers)

There are two distinct "numbers" and they must not be conflated:

- The **counter inside a pane ID** (`p<C>` in `w1:p3`, `w1:pA`) is the stable, public pane number. It is assigned once, embedded in the handle, and — for the life of that pane in that workspace — never changes and never gets reused by a different pane (see next section). It is what you pass to `pane` methods. `PaneInfo` has no separate `number` field; the `pane_id` suffix *is* the pane's public number.
- The **`number` field** is a 1-based **display ordinal**, but `WorkspaceInfo` and `TabInfo` do not behave the same way on reorder. `WorkspaceInfo.number` *is* reassigned when workspaces are reordered (evidence above). `TabInfo.number` is **not**: it stays fixed at the tab's original creation-order position for the tab's whole life, even after `tab.move` changes the tab's position in the tab bar. Moving the third tab of a fresh workspace to the front leaves it at `number: 3` and leaves the untouched first and second tabs at `number: 1` and `number: 2` — only the ordering of entries in the `tab.move`/`tab.list` response reflects the new position. The same move also silently rewrote the first tab's label: it was never explicitly renamed and carried the auto-generated label `"1"`, but after the move — even though its `number` stayed `1` — its label changed to `"2"` to mirror its new *display* position, producing a tab whose `label` and `number` now disagree about where it is:

  ```json
  {"id":"tm1","method":"tab.move","params":{"tab_id":"w1:t3","insert_index":0}}
  {"id":"tm1","result":{"type":"tab_list","tabs":[
    {"tab_id":"w1:t3","workspace_id":"w1","number":3,"label":"tab-C",…},
    {"tab_id":"w1:t1","workspace_id":"w1","number":1,"label":"2",…},
    {"tab_id":"w1:t2","workspace_id":"w1","number":2,"label":"tab-B",…}]}}
  ```

  Do not target a workspace or tab by `number` in application logic; target by `workspace_id`/`tab_id`.
- **Undocumented numeric-selector fallback.** `workspace.get`/`workspace.rename` and `tab.get`/`tab.rename` — but not `pane.get`/`pane.rename` — silently accept the display `number` itself as an alternate selector in the id field. `workspace.get({"workspace_id":"2"})` resolves to whichever workspace currently shows `number: 2`, tolerating a leading `+` or zero-padding (`"+1"`, `"01"`) but not whitespace, `"1.0"`, or the real `w<C>` form. `tab.get`/`tab.rename` accept the analogous workspace-qualified form `"<workspace_id>:<number>"` (e.g. `"w1:1"`), resolved via the tab's *current* display position, not its `t<C>` suffix — this is exactly the moving target the previous bullet warns against, and it is not read-only: `workspace.rename({"workspace_id":"18", "label":"x"})` and `tab.rename({"tab_id":"w1:1", "label":"x"})` really mutate the entity that selector currently resolves to. `pane.get`/`pane.rename` have no such fallback — a bare number, `"p1"`, or a workspace-qualified numeric form all return `pane_not_found` — and all three entity types match IDs case-sensitively (`"W1:P1"`, `"wq:p1"` fail against the real `w1:p1`).

In short: the counter baked into a pane ID is stable; the standalone `number` field is a mutable display position for workspaces, a fixed creation-order value for tabs, and — though undocumented — a working alternate selector for workspace and tab targets, never for panes.

Validated 2026-09-19 against herdr 0.9.1.

## Non-reuse of closed IDs

Closed tab and pane IDs are **not reused**. skill.md: "Closed tab and pane IDs are not reused." Once `w1:p2` is closed, a later new pane in `w1` gets the next free number (`w1:p3`, …), never `w1:p2` again. Consequences for a client:

- A stale reference to a closed pane will not silently resolve to a different, newly created pane — it resolves to nothing. Targeting a closed ID yields a not-found style error rather than acting on an unrelated pane.
- You may safely cache an ID for the lifetime of the entity and detect closure by a failed lookup; you will never get a false positive from number recycling.

Validated 2026-09-19 against herdr 0.9.1.

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
| `move_result.focused_pane_id`, `move_result.changed`, `move_result.reason`, `source_layout`, `target_layout` | Focus outcome, whether anything changed, an optional reason (`PaneMoveReason` enum: `same_tab`, `zoomed_tab`; only `same_tab` observed live), and before/after layouts. |

`closed_tab_id`, `closed_workspace_id`, `reason`, `created_tab`, and `created_workspace` are declared nullable in the schema (e.g. `closed_tab_id: ["string", "null"]`), but on the wire herdr 0.9.1 omits each of these fields entirely when it does not apply, rather than serializing it as JSON `null`. A caller checking `"closed_tab_id" in result` and one checking `result.closed_tab_id is None` will disagree.

skill.md's rule reads: "After `pane move`, continue with `.result.move_result.pane.pane_id` or the live agent name. The old value is reported as `.result.move_result.previous_pane_id`; only the moved process's inherited caller context keeps resolving that old ID, so do not use it as a general agent target." **Measured behaviour on herdr 0.9.1 contradicts this claim.** A retired `pane_id` keeps resolving for *any* caller, not only the moved process, with no inherited context needed: `pane.get`, `pane.current`, and even a mutating call like `pane.rename` all succeed against a retired ID and act on the pane's current identity. This chains across multiple moves — after a pane is moved twice, both the original ID and the intermediate ID still resolve to its latest identity — and only stops once the pane is actually closed, at which point every ID in its history, retired or current, starts returning `pane_not_found`. Still switch to `move_result.pane.pane_id` for anything you write down: relying on a retired ID working is against the documented contract and could change without notice.

Example (an external, unrelated connection resolving a pane by an ID retired two moves ago):

```json
{"id":"pm1","method":"pane.move","params":{"pane_id":"w1:p2","destination":{"type":"tab","tab_id":"w2:t1","split":"right"}}}
{"id":"pm1","result":{"type":"pane_move","move_result":{"changed":true,
  "previous_pane_id":"w1:p2","previous_workspace_id":"w1","previous_tab_id":"w1:t1",
  "pane":{"pane_id":"w2:p2","workspace_id":"w2","tab_id":"w2:t1",…},
  "target_layout":{…}}}}
```

```json
{"id":"pg1","method":"pane.get","params":{"pane_id":"w1:p2"}}
{"id":"pg1","result":{"type":"pane_info","pane":{"pane_id":"w2:p2","workspace_id":"w2","tab_id":"w2:t1",…}}}
```

The second call used the *retired* ID `w1:p2` from a fresh connection with no caller context, and still resolved to the pane's current identity at `w2:p2` — it did not error, and a `pane.rename` through the same retired ID would have succeeded too.

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

In this capture the move stayed within `w1` and the pane's numeric suffix happened to be unchanged (`w1:p3` before and after, only the tab changed); the general rule still stands — always read the returned `pane` and continue with its `pane_id` rather than the ID you moved from, even though (see above) the old ID keeps working too.

Validated 2026-09-19 against herdr 0.9.1.

## Resolving --current and caller context

herdr injects the calling pane's identity into every managed pane as environment variables, and pane commands can target "the pane I am running in" without knowing its ID:

- Injected context (see [environment.md](environment.md)): `$HERDR_WORKSPACE_ID`, `$HERDR_TAB_ID`, `$HERDR_PANE_ID`.
- **Prefer `--current`** on a pane command to target the calling pane. skill.md: "An omitted `pane split` target uses the calling pane when `HERDR_PANE_ID` is available, otherwise the focused pane. Other commands may use the UI-focused pane, which can belong to the user or another client." So an omitted target is *not* a safe default for most pane commands — it can act on someone else's focused pane; `pane split` is the one documented exception, falling back to the calling pane via `$HERDR_PANE_ID` before the focused pane. A `$HERDR_PANE_ID` that no longer resolves is not treated as absent: `pane current --current` with a stale value returns `pane_not_found` rather than silently falling further back to the focused pane.
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

Validated 2026-09-19 against herdr 0.9.1.

## Agent-name targeting

Agent commands (`herdr agent …`, methods under `agent.*`) accept a target that is **either** a live agent name **or** the pane ID currently hosting that agent. They do **not** accept terminal IDs or bare agent-kind labels. Rules from skill.md §"Understand layout, panes, and agents":

- **Name grammar:** a name matches `[a-z][a-z0-9_-]{0,31}` — lowercase-first, then lowercase letters, digits, `_`, or `-`, up to 32 characters total.
- **Uniqueness:** a name must be unique among live agents. Uniqueness is only over *live* agents, so a name is reusable after its holder is gone. The `agent_name_taken` error additionally embeds diagnostic fields about the conflicting live agent (`terminal_id`, `pane_id`, `workspace_id`, `tab_id`, `cwd`, `status`) beyond the documented shape — useful for deciding what to do next, though not part of the documented error contract.
- **Lifetime / opacity:** "A name follows the current pane occupant and is cleared when that agent exits, is released, or is replaced." The name is a handle to whatever agent currently occupies the pane, not a durable identity of a process.
- **Pane ID as target:** you may instead pass the hosting pane's ID (e.g. `w2:p1`); after a `pane move`, the new `pane_id` (or the still-valid agent name) is the correct target — see the move section.
- **Assigning a name:** `agent start <name> --kind <kind> --pane <id>` names the agent it starts; `agent rename <target> <name>|--clear` changes or clears it. `agent rename` refuses to run — for both setting and clearing (`name: null`) — while the agent's launch is still pending (`launch_pending: true`), failing with error `agent_launch_pending` ("agent name cannot change while startup is pending"). This is undocumented in skill.md's unconditional description of `agent rename`, and it can persist well past the ~3.9s a launch usually takes to settle if the started process is itself blocked on interactive input (e.g. a harness's own first-run trust prompt).
- Agent identity fields in `AgentInfo`: `name` (the assignable unique name, nullable), `agent` (detected kind label such as `claude`/`codex`, nullable), and `agent_session` (`AgentSessionInfo`: `source`, `agent`, `kind` ∈ `{id,path}`, `value`) which carries the native session ref. Probe `probes/agent-get.json` resolves target `claude` to `pane_id: "w2:p1"` with `agent_session.value` a UUID. Target by `name`/pane-id; read `agent`/`agent_session` as attributes, not as targets.

Validated 2026-09-19 against herdr 0.9.1.
