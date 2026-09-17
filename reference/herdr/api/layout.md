# herdr API: layout methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `layout.*` methods read and reshape the pane tree of a tab. `layout.export` serializes
the current layout of a tab as a portable `LayoutNode` tree; `layout.apply` realizes such a
tree (creating panes and splits, optionally spawning commands) into a workspace or tab; and
`layout.set_split_ratio` adjusts the divider position of one split in-place. All three are
**API-only**: there is no `herdr layout` CLI group — `herdr` reports `unknown command:
layout` (probe `probes/scratch/layout-export.err`). The related CLI command `herdr pane
layout` belongs to the `pane.*` namespace and only *shows* layout information; it does not
invoke these methods. Each method returns a `LayoutDescription` snapshot of the affected
tab after the operation.

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
results (as the exported/current layout); the field set is identical in both directions.
Also catalogued in [../data-model.md](../data-model.md).

**`type: "pane"`** — a leaf terminal pane.

| field | type | required | default | meaning |
|---|---|---|---|---|
| `type` | `"pane"` (const) | yes | — | Discriminator. |
| `pane_id` | string \| null | no | null | Existing pane ID to reference. On `layout.apply` this is a hint, not a reservation — apply allocates fresh pane IDs (probe: requested `w1:p1` became `w1:p6`). On export it is the live pane ID. |
| `command` | array of string \| null | no | null | Command line (argv) to run in the pane when applied; null/absent means a default shell. |
| `cwd` | string \| null | no | null | Working directory for the pane. |
| `env` | object (string → string) | no | `{}` | Environment variables to set in the pane. |
| `label` | string \| null | no | null | Human-readable label for the pane. |

**`type: "split"`** — an interior node dividing space between two child nodes.

| field | type | required | default | meaning |
|---|---|---|---|---|
| `type` | `"split"` (const) | yes | — | Discriminator. |
| `direction` | `SplitDirection` enum: `right`, `down` | yes | — | Orientation of the split. `right` places `second` to the right of `first` (vertical divider); `down` places `second` below `first` (horizontal divider). |
| `ratio` | number (float) | yes | — | Fraction of the space given to `first`, in `0.0`–`1.0`. |
| `first` | `LayoutNode` | yes | — | The first (left/top) child subtree. |
| `second` | `LayoutNode` | yes | — | The second (right/bottom) child subtree. |

### LayoutDescription

The snapshot returned inside every `layout.*` result's `layout` field. All fields required.

| field | type | meaning |
|---|---|---|
| `workspace_id` | string | Workspace owning the tab (e.g. `w1`). |
| `tab_id` | string | Tab whose layout this describes (e.g. `w1:t1`). |
| `zoomed` | boolean | Whether a single pane is currently zoomed to fill the tab. |
| `focused_pane_id` | string | Pane ID that currently holds focus within the tab. |
| `root` | `LayoutNode` | Root of the tab's pane tree. |

## layout.apply

Realize a `LayoutNode` tree into a target, creating the panes and splits it describes and
optionally running each pane's `command`. When `workspace_id` is given without `tab_id`,
apply creates a **new tab** in that workspace to hold the layout (probe: applying into `w1`
produced tab `w1:t6` with a fresh pane `w1:p6`); `tab_id` targets an existing tab and
`tab_label` names the created/target tab. Pane IDs in the input `root` are hints only — apply
allocates new pane IDs. `focus` controls whether focus moves to the applied layout; it
defaults to `false`, matching herdr's convention of leaving the caller's focus undisturbed
(see skill.md). Returns the resulting tab layout.

**Params** — `LayoutApplyParams`:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `root` | `LayoutNode` | yes | — | Layout tree to realize. See [LayoutNode](#layoutnode). |
| `workspace_id` | string \| null | no | null | Workspace to apply into. With no `tab_id`, a new tab is created here. |
| `tab_id` | string \| null | no | null | Existing tab to apply into. |
| `tab_label` | string \| null | no | null | Label for the target/created tab. |
| `focus` | boolean | no | `false` | Whether to move focus to the applied layout. |

**Result** — `type: "layout_apply"`:

| field | type | meaning |
|---|---|---|
| `type` | `"layout_apply"` (const) | Result discriminator. |
| `layout` | `LayoutDescription` | Snapshot of the tab after applying. See [LayoutDescription](#layoutdescription). |

**Errors**: no error captured for this method. Other codes possible (e.g. an invalid
`workspace_id`/`tab_id` target).

**CLI**: API-only (no CLI subcommand).

**Example** — Validated 2026-08-19 against herdr 0.8.2.

```json
{"id":"l3","method":"layout.apply","params":{"workspace_id":"w1","root":{"type":"pane","pane_id":"w1:p1","cwd":"…/scratch-repo"}}}
{"id":"l3","result":{"type":"layout_apply","layout":{"workspace_id":"w1","tab_id":"w1:t6","zoomed":false,"focused_pane_id":"w1:p6","root":{"type":"pane","pane_id":"w1:p6","cwd":"…/scratch-repo"}}}}
```

## layout.export

Serialize the current layout of a tab as a `LayoutNode` tree, suitable for storing and later
replaying with `layout.apply`. Target the tab directly with `tab_id`, or with `pane_id` to
export the tab that owns that pane; with neither, the active tab is exported. Read-only — it
has no side effects. Returns a `LayoutDescription` for the exported tab.

**Params** — `LayoutExportParams`:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `tab_id` | string \| null | no | null | Tab to export. |
| `pane_id` | string \| null | no | null | Export the tab that owns this pane. |

> Schema discrepancy: the captured probe requests set `workspace_id` (e.g. `"w1"`), which is
> **not** a declared field of `LayoutExportParams` — the schema defines only `tab_id` and
> `pane_id`. The schema is authoritative; treat `workspace_id` here as an undocumented/ignored
> extra. To select a tab explicitly, use `tab_id`.

**Result** — `type: "layout_export"`:

| field | type | meaning |
|---|---|---|
| `type` | `"layout_export"` (const) | Result discriminator. |
| `layout` | `LayoutDescription` | The exported tab layout. See [LayoutDescription](#layoutdescription). |

**Errors**: no error captured for this method. Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Validated 2026-08-19 against herdr 0.8.2. (Request uses the undocumented
`workspace_id` field noted above; prefer `tab_id`.)

```json
{"id":"r1","method":"layout.export","params":{"workspace_id":"w1"}}
{"id":"r1","result":{"type":"layout_export","layout":{"workspace_id":"w1","tab_id":"w1:t1","zoomed":false,"focused_pane_id":"w1:p1","root":{"type":"pane","pane_id":"w1:p1","cwd":"…/scratch-repo"}}}}
```

## layout.set_split_ratio

Set the divider `ratio` of a single split node in a tab's layout, identified by a `path` of
boolean child selectors walked from the tab's root. Each element chooses a branch at a split:
`false` descends into `first`, `true` into `second`; the node reached at the end of the path
is the split whose ratio is updated. Target the tab with `tab_id`, or the pane's tab with
`pane_id`; with neither, the active tab is used. Returns the tab layout after the change.

If the `path` does not resolve to an existing split (for example, a tab that is a single
unsplit pane), the method fails with `split_not_found`.

**Params** — `LayoutSetSplitRatioParams`:

| field | type | required | default | meaning |
|---|---|---|---|---|
| `path` | array of boolean | yes | — | Child selectors from the root to the target split: `false` = `first`, `true` = `second`. |
| `ratio` | number (float) | yes | — | New fraction given to `first`, `0.0`–`1.0`. |
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

Other codes possible.

**CLI**: API-only (no CLI subcommand).

**Example** — Validated 2026-08-19 against herdr 0.8.2. (This capture targets a single-pane
tab, so the split path does not resolve and the server returns an error.)

```json
{"id":"l1","method":"layout.set_split_ratio","params":{"tab_id":"w1:t1","path":[false],"ratio":0.6}}
{"id":"l1","error":{"code":"split_not_found","message":"split path not found"}}
```
