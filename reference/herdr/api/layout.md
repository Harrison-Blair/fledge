# herdr API: layout methods

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `layout.*` methods read and reshape the pane tree of a tab. `layout.export` serializes
the current layout of a tab as a `LayoutNode` tree; the tree round-trips structurally with
`layout.apply`, but is not fully portable — a pane's `env` takes effect on apply but never
comes back in any `layout.apply` or `layout.export` result, so exporting and re-applying a
layout silently drops every pane's environment (see [LayoutNode](#layoutnode)).
`layout.apply` realizes such a tree (creating panes and splits, optionally spawning
commands) into a workspace or tab; and `layout.set_split_ratio` adjusts the divider position
of one split in-place. All three are **API-only**: there is no `herdr layout` CLI group —
`herdr` reports `unknown command: layout` and exits 2. The related CLI command `herdr pane
layout` belongs to the `pane.*` namespace and only *shows* layout information; it does not
invoke these methods. Each method returns a `LayoutDescription` snapshot of the affected
tab after the operation — except `layout.apply` given a `tab_id`, where the returned tab did
not exist before the call (see [layout.apply](#layoutapply)).

3 methods:

| method | purpose |
|---|---|
| [layout.apply](#layoutapply) | Realize a `LayoutNode` tree into a workspace/tab, creating panes and splits. |
| [layout.export](#layoutexport) | Serialize the current layout of a tab (or the tab owning a pane) as a `LayoutNode` tree. |
| [layout.set_split_ratio](#layoutsetsplitratio) | Set the divider ratio of one split identified by a path of child selectors. |

## Shared types

### LayoutNode

A recursive union (`oneOf`) describing one node of a layout tree. The `type` discriminator
selects the variant. This type appears both in requests (as the layout to apply) and in
results (as the exported/current layout), but the field set is not identical in both
directions: `env` is accepted and takes effect on `layout.apply` but is never present in any
`layout.apply` or `layout.export` result (see the `env` row below). Also catalogued in
[../data-model.md](../data-model.md).

**`type: "pane"`** — a leaf terminal pane.

| field | type | required | default | meaning |
|---|---|---|---|---|
| `type` | `"pane"` (const) | yes | — | Discriminator. |
| `pane_id` | string \| null | no | null | Existing pane ID to reference. On `layout.apply` this is a hint, not a reservation — apply allocates fresh pane IDs (probe: requested `w1:p1` became `w1:p6`). On export it is the live pane ID. |
| `command` | array of string \| null | no | null | Command line (argv) to run in the pane when applied; null/absent means a default shell. An empty array (`[]`) is rejected with `invalid_layout` ("pane command must not be empty"). |
| `cwd` | string \| null | no | null | Working directory for the pane. A `cwd` that does not exist is not an error — herdr silently substitutes `$HOME`, and the result reports the substituted path. |
| `env` | object (string → string) | no | `{}` | Environment variables to set in the pane. Takes effect on `layout.apply` but is never echoed back — no result ever includes `env` (see above). |
| `label` | string \| null | no | null | Human-readable label for the pane. |

**`type: "split"`** — an interior node dividing space between two child nodes.

| field | type | required | default | meaning |
|---|---|---|---|---|
| `type` | `"split"` (const) | yes | — | Discriminator. |
| `direction` | `SplitDirection` enum: `right`, `down` | yes | — | Orientation of the split. `right` places `second` to the right of `first` (vertical divider); `down` places `second` below `first` (horizontal divider). |
| `ratio` | number (float) | yes | — | Fraction of the space given to `first`. Silently clamped to `0.1`–`0.9`: out-of-range values (`0.0`, `1.0`, negative, `>1`) are accepted without error, and it is the clamped value that takes effect and is returned. Stored as `f32` — precision beyond ~7 significant digits is not preserved (e.g. `0.123456789` comes back as `0.12345679`). |
| `first` | `LayoutNode` | yes | — | The first (left/top) child subtree. |
| `second` | `LayoutNode` | yes | — | The second (right/bottom) child subtree. |

### LayoutDescription

The snapshot returned inside every `layout.*` result's `layout` field. All fields required.

| field | type | meaning |
|---|---|---|
| `workspace_id` | string | Workspace owning the tab (e.g. `w1`). |
| `tab_id` | string | Tab whose layout this describes (e.g. `w1:t1`). |
| `zoomed` | boolean | Whether a single pane is currently zoomed to fill the tab. While `zoomed` is `true`, `root` still describes the full (unzoomed) split tree — nothing in this snapshot identifies which pane is zoomed. |
| `focused_pane_id` | string | Pane ID that currently holds focus within the tab. |
| `root` | `LayoutNode` | Root of the tab's pane tree. |

## layout.apply

Realize a `LayoutNode` tree into a target, creating the panes and splits it describes and
optionally running each pane's `command` (spawned non-blockingly: `layout.apply` does not
wait for it to exit, and a pane whose command exits immediately closes that pane, closing
the tab too if it was the last pane in it). The tree is limited to 16 levels of depth and 24
panes; larger trees fail with `invalid_layout`. With neither `workspace_id` nor `tab_id`,
apply creates a new tab in the currently focused workspace. When `workspace_id` is given
without `tab_id`, apply creates a **new tab** in that workspace to hold the layout; giving
both `workspace_id` and `tab_id` is rejected (`invalid_target`). `tab_id` does **not**
reshape the named tab in place — it closes that tab and builds a replacement with a new
`tab_id`, appended at the end of the workspace's tab order; the replacement inherits the
closed tab's label unless `tab_label` overrides it, and any pane IDs from the closed tab
become invalid. Pane IDs in the input `root` are hints only — apply always allocates new
pane IDs. `focus` controls whether focus moves to the applied layout; it defaults to
`false`, matching herdr's convention of leaving the caller's focus undisturbed (see
skill.md), though see the `focus` row below for two edge cases. Returns the resulting tab's
layout — for a `tab_id` call, the layout of the newly created replacement.

**Params** — `LayoutApplyParams`:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `root` | `LayoutNode` | yes | — | Layout tree to realize. Limited to 16 levels of depth and 24 panes; a deeper or larger tree fails with `invalid_layout`. See [LayoutNode](#layoutnode). |
| `workspace_id` | string \| null | no | null | Workspace to apply into. With no `tab_id`, a new tab is created here. With neither `workspace_id` nor `tab_id`, apply creates a new tab in the currently focused workspace. Giving both `workspace_id` and `tab_id` fails with `invalid_target`. |
| `tab_id` | string \| null | no | null | Existing tab to target — apply closes this tab and creates a replacement with a different `tab_id`, appended at the end of the tab order, rather than reshaping it in place (see above). |
| `tab_label` | string \| null | no | null | Label for the target/created tab. With `tab_id` and no `tab_label`, the replacement inherits the closed tab's label. |
| `focus` | boolean | no | `false` | Whether to move focus to the applied layout. Two cases override the `false` default's "leave focus undisturbed" behaviour: applying into the only tab of a workspace focuses the replacement anyway (the previous active tab no longer exists), and applying into the active tab of a non-focused workspace reassigns that workspace's active tab to a sibling tab rather than to the new one. |

**Result** — `type: "layout_apply"`:

| field | type | meaning |
|---|---|---|
| `type` | `"layout_apply"` (const) | Result discriminator. |
| `layout` | `LayoutDescription` | Snapshot of the tab after applying. See [LayoutDescription](#layoutdescription). |

**Errors**:

| code | when |
|---|---|
| `workspace_not_found` | Unknown `workspace_id`. |
| `tab_not_found` | Unknown `tab_id`. |
| `invalid_target` | Both `workspace_id` and `tab_id` were given. |
| `invalid_layout` | The tree is malformed in ways the schema alone can't catch: an empty pane `command` array, depth greater than 16, or more than 24 panes. |
| `invalid_request` | A malformed `LayoutNode` — unknown `type`, or a `split` missing `direction`, `ratio`, `first`, or `second`. |

Other codes possible.

**Events**: applying into a new tab emits `layout_updated`, `tab_created` (subscription type
`tab.created`), and one `pane_created`/`pane_updated` pair per pane (subscription types
`pane.created`/`pane.updated`); applying with an existing `tab_id` additionally emits
`tab_closed` (subscription type `tab.closed`) for the destroyed tab. The `layout_updated`
payload is a `PaneLayoutSnapshot` (area/panes/splits with rects), not the
`LayoutDescription` this method returns.

**CLI**: API-only (no CLI subcommand).

**Example** — Validated 2026-09-19 against herdr 0.9.1. (Same request replayed verbatim; a
pristine server yields `w1:t2`/`w1:p2` here, since the concrete IDs depend on prior session
state.)

```json
{"id":"l3","method":"layout.apply","params":{"workspace_id":"w1","root":{"type":"pane","pane_id":"w1:p1","cwd":"…/scratch-repo"}}}
{"id":"l3","result":{"type":"layout_apply","layout":{"workspace_id":"w1","tab_id":"w1:t6","zoomed":false,"focused_pane_id":"w1:p6","root":{"type":"pane","pane_id":"w1:p6","cwd":"…/scratch-repo"}}}}
```

## layout.export

Serialize the current layout of a tab as a `LayoutNode` tree. The tree round-trips
structurally with `layout.apply`, but not fully portably — see [LayoutNode](#layoutnode)
for the `env` gap. Target the tab directly with `tab_id`, or with `pane_id` to export the
tab that owns that pane — the two are mutually exclusive; giving both fails with
`layout_not_found`, even when the pane genuinely belongs to that tab. With neither, the
active tab (of the focused workspace) is exported. Read-only — it has no side effects.
Returns a `LayoutDescription` for the exported tab.

**Params** — `LayoutExportParams`:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `tab_id` | string \| null | no | null | Tab to export. |
| `pane_id` | string \| null | no | null | Export the tab that owns this pane. |

> Schema discrepancy: the captured probe requests set `workspace_id` (e.g. `"w1"`), which is
> **not** a declared field of `LayoutExportParams` — the schema defines only `tab_id` and
> `pane_id`. The schema is authoritative; treat `workspace_id` here as an undocumented/ignored
> extra. To select a tab explicitly, use `tab_id`. The same silent-ignore behaviour applies
> to unknown params sent to `layout.apply` and `layout.set_split_ratio` — a typo'd field
> name has no effect and produces no error.

**Result** — `type: "layout_export"`:

| field | type | meaning |
|---|---|---|
| `type` | `"layout_export"` (const) | Result discriminator. |
| `layout` | `LayoutDescription` | The exported tab layout. See [LayoutDescription](#layoutdescription). |

**Errors**:

| code | when |
|---|---|
| `layout_not_found` | The target could not be resolved: unknown `tab_id`, unknown `pane_id`, both given together, or nothing to export (e.g. a server with no workspaces at all). One opaque code covers all of these — it does not distinguish the cause. |
| `invalid_request` | Wrong field type, e.g. `tab_id` given as a number. |

Other codes possible.

**Events**: none — a subscriber held open across an export call, subscribed to every
`layout.*`/`tab.*`/`pane.*`/`workspace.*` event, saw nothing.

**CLI**: API-only (no CLI subcommand).

**Example** — Validated 2026-09-19 against herdr 0.9.1. (Replayed verbatim, byte-identical.
Request uses the undocumented `workspace_id` field noted above; prefer `tab_id`.)

```json
{"id":"r1","method":"layout.export","params":{"workspace_id":"w1"}}
{"id":"r1","result":{"type":"layout_export","layout":{"workspace_id":"w1","tab_id":"w1:t1","zoomed":false,"focused_pane_id":"w1:p1","root":{"type":"pane","pane_id":"w1:p1","cwd":"…/scratch-repo"}}}}
```

## layout.set_split_ratio

Set the divider `ratio` of a single split node in a tab's layout, identified by a `path` of
boolean child selectors walked from the tab's root; the empty path selects the tab's
top-level split. Each element chooses a branch at a split: `false` descends into `first`,
`true` into `second`; the node reached at the end of the path is the split whose ratio is
updated. Target the tab with `tab_id`, or the pane's tab with `pane_id` — the two are
mutually exclusive; giving both fails with `layout_not_found`, the same as `layout.export`.
With neither, the active tab is used. Returns the tab layout after the change.

If the `path` does not resolve to an existing split (for example, a tab that is a single
unsplit pane), the method fails with `split_not_found`.

**Params** — `LayoutSetSplitRatioParams`:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `path` | array of boolean | yes | — | Child selectors from the root to the target split: `false` = `first`, `true` = `second`. Empty selects the tab's top-level split. |
| `ratio` | number (float) | yes | — | New fraction given to `first`. Silently clamped to `0.1`–`0.9` (see [LayoutNode](#layoutnode) `split.ratio` for the exact clamp behaviour and the `f32` precision note); integers are accepted (`1` → `0.9`, `0` → `0.1`). |
| `tab_id` | string \| null | no | null | Tab whose split to adjust. |
| `pane_id` | string \| null | no | null | Adjust the split in the tab that owns this pane. |

**Result** — `type: "layout_split_ratio_set"`:

| field | type | meaning |
|---|---|---|
| `type` | `"layout_split_ratio_set"` (const) | Result discriminator. |
| `layout` | `LayoutDescription` | Snapshot of the tab after adjusting the split. See [LayoutDescription](#layoutdescription). |

**Errors**:

| code | when |
|---|---|
| `split_not_found` | The `path` does not resolve to an existing split in the target tab (probe: `path:[false]` against a single-pane tab). |
| `layout_not_found` | Unknown `tab_id`/`pane_id`, or both given together. |
| `invalid_request` | Malformed `path` (a non-boolean element) or `ratio` (wrong type). |

Other codes possible.

**Events**: emits exactly one `layout_updated` event to subscribers (subscription type
`layout.updated`), with the same `PaneLayoutSnapshot` payload described under
[layout.apply](#layoutapply).

**CLI**: API-only (no CLI subcommand).

**Example** — Validated 2026-09-19 against herdr 0.9.1. (Byte-identical replay; this capture
targets a single-pane tab, so the split path does not resolve and the server returns an
error — see below for a successful call.)

```json
{"id":"l1","method":"layout.set_split_ratio","params":{"tab_id":"w1:t1","path":[false],"ratio":0.6}}
{"id":"l1","error":{"code":"split_not_found","message":"split path not found"}}
```

A successful call, against a tab whose root is `split(right){A, split(down){B, C}}` — path
`[]` selects the root split:

```json
{"id":"l2","method":"layout.set_split_ratio","params":{"tab_id":"w1:t2","path":[],"ratio":0.8}}
{"id":"l2","result":{"type":"layout_split_ratio_set","layout":{"workspace_id":"w1","tab_id":"w1:t2","zoomed":false,"focused_pane_id":"w1:p1","root":{"type":"split","direction":"right","ratio":0.8,"first":{"type":"pane","pane_id":"w1:p1","label":"A","cwd":"…/scratch-repo"},"second":{"type":"split","direction":"down","ratio":0.25,"first":{"type":"pane","pane_id":"w1:p2","label":"B","cwd":"…/scratch-repo"},"second":{"type":"pane","pane_id":"w1:p3","label":"C","cwd":"…/scratch-repo"}}}}}}
```
