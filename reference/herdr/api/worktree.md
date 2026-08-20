# herdr API: worktree methods

> herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `worktree.*` namespace manages Git-worktree-backed workspaces: it enumerates the
worktrees of the repository owning a given checkout, creates a new linked worktree and
opens it as a herdr workspace in one step, opens an existing worktree as a workspace, and
removes a linked worktree checkout. Every method resolves its target repository from a
working directory (`cwd`) or an existing `workspace_id`; when neither is supplied the
server falls back to the caller's ambient context. A worktree becomes a herdr *workspace*
when opened — the create and open results therefore return the full workspace/tab/pane
topology (see [data-model.md](../data-model.md)) in addition to the `WorktreeInfo`.

4 methods:

| method | purpose |
|---|---|
| [worktree.create](#worktreecreate) | Create a new linked Git worktree and open it as a workspace. |
| [worktree.list](#worktreelist) | List the worktrees of the repository owning a checkout. |
| [worktree.open](#worktreeopen) | Open an existing Git worktree as a workspace. |
| [worktree.remove](#worktreeremove) | Remove a worktree checkout by its open workspace ID. |

## worktree.create

Creates a new linked Git worktree for the target repository and opens it as a herdr
workspace in a single operation. The result carries the newly created `WorkspaceInfo`, its
initial `TabInfo` and root `PaneInfo`, plus the `WorktreeInfo` describing the checkout. The
target repository is resolved from `workspace_id` or `cwd`; when both are omitted the
server uses the caller's ambient context. If `branch` is omitted the server derives a
branch; `base` selects the ref the new branch/worktree is created from; `path` overrides
the checkout location (default is under herdr's managed worktree directory, e.g.
`~/.herdr/worktrees/<repo>/<branch>`); `label` overrides the workspace label. `focus`
controls whether the new workspace is focused after creation.

**Params** (`WorktreeCreateParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `workspace_id` | string \| null | no | null | Existing workspace whose repository is the source; resolves the target repo. |
| `cwd` | string \| null | no | null | Working directory used to resolve the source repository when `workspace_id` is absent. |
| `branch` | string \| null | no | null | Branch name for the new worktree; server-derived when null. |
| `base` | string \| null | no | null | Git ref the new branch/worktree is based on (inferred). |
| `path` | string \| null | no | null | Explicit checkout path; server-managed location when null. |
| `label` | string \| null | no | null | Workspace label; derived (typically from branch) when null. |
| `focus` | boolean | no | `false` | Focus the created workspace after opening. |

**Result** — `type: "worktree_created"`:

| field | type | meaning |
|---|---|---|
| `type` | string const `"worktree_created"` | Result discriminator. |
| `workspace` | WorkspaceInfo | The created workspace. See [data-model.md](../data-model.md#workspaceinfo). |
| `tab` | TabInfo | The workspace's initial tab. See [data-model.md](../data-model.md#tabinfo). |
| `root_pane` | PaneInfo | The root pane of the initial tab. See [data-model.md](../data-model.md#paneinfo). |
| `worktree` | WorktreeInfo | The created worktree checkout (see [WorktreeInfo](#worktreeinfo) below). |

**Errors**: no server error code observed live for this method; a CLI invocation with an
unknown flag exits status 2 (`unknown option: …` on stderr). Other codes possible — see
[errors.md](../errors.md).

**CLI**: `herdr worktree create [--workspace <ID>] [--cwd <PATH>] [--branch <NAME>] [--base <REF>] [--path <PATH>] [--label <TEXT>] [--focus | --no-focus]`

**Example**

```json
{"id":"cli:worktree:create","method":"worktree.create","params":{"branch":"docs-probe","path":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe"}}
{"id":"cli:worktree:create","result":{"type":"worktree_created","workspace":{"active_tab_id":"w3:t1","agent_status":"unknown","focused":false,"label":"docs-probe","number":3,"pane_count":1,"tab_count":1,"workspace_id":"w3","worktree":{"checkout_path":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe","is_linked_worktree":true,"repo_key":"…/scratch-repo/.git","repo_name":"scratch-repo","repo_root":"…/scratch-repo"}},"tab":{"agent_status":"unknown","focused":false,"label":"1","number":1,"pane_count":1,"tab_id":"w3:t1","workspace_id":"w3"},"root_pane":{"agent_status":"unknown","cwd":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe","focused":false,"foreground_cwd":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe","pane_id":"w3:p1","revision":0,"scroll":{"max_offset_from_bottom":0,"offset_from_bottom":0,"viewport_rows":39},"tab_id":"w3:t1","terminal_id":"term_65970c143f1165","workspace_id":"w3"},"worktree":{"branch":"docs-probe","is_bare":false,"is_detached":false,"is_linked_worktree":true,"is_prunable":false,"label":"scratch-repo","open_workspace_id":"w3","path":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe"}}}
```

Validated 2026-08-19 against herdr 0.8.2. (Request line reconstructed from the CLI flags;
the response is the captured `worktree_created` payload.)

## worktree.list

Lists all worktrees of the Git repository that owns the resolved checkout, along with a
`source` block identifying that repository and the checkout the listing was resolved from.
The target repository is resolved from `workspace_id` or `cwd`; when both are omitted the
server uses the caller's ambient context. Read-only. Each returned `WorktreeInfo` reports
whether it is currently open as a workspace via `open_workspace_id`.

**Params** (`WorktreeListParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `workspace_id` | string \| null | no | null | Existing workspace whose repository to list; resolves the target repo. |
| `cwd` | string \| null | no | null | Working directory used to resolve the repository when `workspace_id` is absent. |

**Result** — `type: "worktree_list"`:

| field | type | meaning |
|---|---|---|
| `type` | string const `"worktree_list"` | Result discriminator. |
| `source` | WorktreeSourceInfo | The repository and checkout the listing was resolved from (see [WorktreeSourceInfo](#worktreesourceinfo) below). |
| `worktrees` | array of WorktreeInfo | All worktrees of the repository (see [WorktreeInfo](#worktreeinfo) below). |

**Errors**: no server error code observed live. Other codes possible — see
[errors.md](../errors.md).

**CLI**: `herdr worktree list [--workspace <ID>] [--cwd <PATH>]`

**Example**

```json
{"id":"cli:worktree:list","method":"worktree.list","params":{}}
{"id":"cli:worktree:list","result":{"source":{"repo_key":"/home/penguin/source/fledge/.git","repo_name":"fledge","repo_root":"/home/penguin/source/fledge","source_checkout_path":"/home/penguin/source/fledge","source_workspace_id":"w1"},"type":"worktree_list","worktrees":[{"branch":"main","is_bare":false,"is_detached":false,"is_linked_worktree":false,"is_prunable":false,"label":"fledge","open_workspace_id":"w1","path":"/home/penguin/source/fledge"}]}}
```

Validated 2026-08-19 against herdr 0.8.2.

## worktree.open

Opens an existing Git worktree as a herdr workspace. Like `worktree.create` the result
carries the `WorkspaceInfo`, `TabInfo`, root `PaneInfo`, and `WorktreeInfo`, plus an
`already_open` flag indicating whether that worktree already had an open workspace (in
which case the existing workspace is returned rather than a new one being created). The
worktree is selected by `path` or `branch`; the owning repository is resolved from
`workspace_id` or `cwd`, falling back to the caller's ambient context. `label` overrides
the workspace label and `focus` controls focus after opening.

**Params** (`WorktreeOpenParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `workspace_id` | string \| null | no | null | Existing workspace whose repository owns the worktree; resolves the target repo. |
| `cwd` | string \| null | no | null | Working directory used to resolve the repository when `workspace_id` is absent. |
| `path` | string \| null | no | null | Checkout path of the worktree to open. |
| `branch` | string \| null | no | null | Branch name of the worktree to open (alternative to `path`). |
| `label` | string \| null | no | null | Workspace label; derived when null. |
| `focus` | boolean | no | `false` | Focus the workspace after opening. |

**Result** — `type: "worktree_opened"`:

| field | type | meaning |
|---|---|---|
| `type` | string const `"worktree_opened"` | Result discriminator. |
| `workspace` | WorkspaceInfo | The opened (or already-open) workspace. See [data-model.md](../data-model.md#workspaceinfo). |
| `tab` | TabInfo | The workspace's active tab. See [data-model.md](../data-model.md#tabinfo). |
| `root_pane` | PaneInfo | The root pane of the tab. See [data-model.md](../data-model.md#paneinfo). |
| `worktree` | WorktreeInfo | The opened worktree checkout (see [WorktreeInfo](#worktreeinfo) below). |
| `already_open` | boolean | True if the worktree already had an open workspace before this call. |

**Errors**: no server error code observed live. Other codes possible — see
[errors.md](../errors.md).

**CLI**: `herdr worktree open [--workspace <ID>] [--cwd <PATH>] [--path <PATH>] [--branch <NAME>] [--label <TEXT>] [--focus | --no-focus]`

**Example**

```json
{"id":"wt1","method":"worktree.open","params":{"path":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe"}}
{"id":"wt1","result":{"type":"worktree_opened","workspace":{"workspace_id":"w3","number":3,"label":"docs-probe","focused":false,"pane_count":1,"tab_count":1,"active_tab_id":"w3:t1","agent_status":"unknown","worktree":{"repo_key":"…/scratch-repo/.git","repo_name":"scratch-repo","repo_root":"…/scratch-repo","checkout_path":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe","is_linked_worktree":true}},"tab":{"tab_id":"w3:t1","workspace_id":"w3","number":1,"label":"1","focused":false,"pane_count":1,"agent_status":"unknown"},"root_pane":{"pane_id":"w3:p1","terminal_id":"term_65970c143f1165","workspace_id":"w3","tab_id":"w3:t1","focused":false,"cwd":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe","foreground_cwd":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe","terminal_title":"penguin@raft: ~/.herdr/worktrees/scratch-repo/docs-probe","terminal_title_stripped":"penguin@raft: ~/.herdr/worktrees/scratch-repo/docs-probe","agent_status":"unknown","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":0,"viewport_rows":39},"revision":1},"worktree":{"path":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe","branch":"docs-probe","is_bare":false,"is_detached":false,"is_prunable":false,"is_linked_worktree":true,"open_workspace_id":"w3","label":"scratch-repo"},"already_open":true}}
```

Validated 2026-08-19 against herdr 0.8.2. (`already_open: true` because the worktree had
just been created and opened as `w3`.)

## worktree.remove

Removes a worktree checkout identified by the `workspace_id` of the workspace that has it
open. `workspace_id` is required. Set `force` to remove the checkout even when it has
uncommitted or otherwise dirty state that Git would normally refuse to discard. The result
echoes the removed checkout `path` and whether removal was `forced`.

**Params** (`WorktreeRemoveParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `workspace_id` | string | **yes** | — | Workspace whose worktree checkout is removed. |
| `force` | boolean | no | `false` | Force removal despite dirty/uncommitted state. |

**Result** — `type: "worktree_removed"`:

| field | type | meaning |
|---|---|---|
| `type` | string const `"worktree_removed"` | Result discriminator. |
| `workspace_id` | string | The workspace whose worktree was removed. |
| `path` | string | Filesystem path of the removed checkout. |
| `forced` | boolean | Whether removal used the force path. |

**Errors**: no server error code observed live for a valid request; a CLI invocation with
an unknown flag exits status 2 (`unknown option: --path` on stderr — `remove` accepts only
`--workspace` and `--force`). Other codes possible — see [errors.md](../errors.md).

**CLI**: `herdr worktree remove --workspace <ID> [--force]`

**Example**

```json
{"id":"cli:worktree:remove","method":"worktree.remove","params":{"workspace_id":"w3","force":false}}
{"id":"cli:worktree:remove","result":{"type":"worktree_removed","workspace_id":"w3","path":"/home/penguin/.herdr/worktrees/scratch-repo/docs-probe","forced":false}}
```

Constructed from schema; not live-validated.

## Worktree domain types

These types appear only in `worktree.*` results and are expanded here. Shared entities
(`WorkspaceInfo`, `TabInfo`, `PaneInfo`) live in [data-model.md](../data-model.md).

### WorktreeInfo

Describes a single Git worktree of a repository.

| field | type | required | meaning |
|---|---|---|---|
| `path` | string | yes | Checkout path of the worktree. |
| `is_bare` | boolean | yes | True if this is the bare repository entry. |
| `is_detached` | boolean | yes | True if the worktree HEAD is detached. |
| `is_prunable` | boolean | yes | True if Git considers the worktree prunable (stale). |
| `is_linked_worktree` | boolean | yes | True if this is a linked worktree (not the primary checkout). |
| `label` | string | yes | Display label for the worktree (typically the repository name). |
| `branch` | string \| null | no | Checked-out branch, or null when detached/none. |
| `open_workspace_id` | string \| null | no | ID of the workspace currently holding this worktree open, or null if not open. |

### WorktreeSourceInfo

Identifies the repository and checkout that a `worktree.list` was resolved from.

| field | type | required | meaning |
|---|---|---|---|
| `repo_key` | string | yes | Stable key for the repository (its `.git` path). |
| `repo_name` | string | yes | Repository name. |
| `repo_root` | string | yes | Filesystem root of the repository's primary checkout. |
| `source_checkout_path` | string | yes | Checkout path the listing was resolved from. |
| `source_workspace_id` | string \| null | no | Workspace ID the listing was resolved from, or null when resolved from `cwd`/ambient context. |

### WorkspaceWorktreeInfo

The worktree summary embedded in a `WorkspaceInfo.worktree` field (present on
create/open results). Distinct from `WorktreeInfo`.

| field | type | required | meaning |
|---|---|---|---|
| `repo_key` | string | yes | Stable key for the repository (its `.git` path). |
| `repo_name` | string | yes | Repository name. |
| `repo_root` | string | yes | Filesystem root of the repository's primary checkout. |
| `checkout_path` | string | yes | Checkout path of this workspace's worktree. |
| `is_linked_worktree` | boolean | yes | True if the workspace's checkout is a linked worktree. |
