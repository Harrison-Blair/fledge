# herdr API: worktree methods

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `worktree.*` namespace manages Git-worktree-backed workspaces: it enumerates the
worktrees of the repository owning a given checkout, creates a new linked worktree and
opens it as a herdr workspace in one step, opens an existing worktree as a workspace, and
removes a linked worktree checkout. Every method resolves its target repository from a
working directory (`cwd`) or an existing `workspace_id`; when neither is supplied the
server resolves from the *focused* workspace, and fails `invalid_request`
(`workspace_id or cwd is required when no workspace is active`) when nothing is focused.
`worktree.create` and `worktree.open` additionally require the resolved source to be the
repository's parent (non-linked) checkout: a linked worktree reached through `cwd`, through
`workspace_id`, or as the focused-workspace fallback fails `linked_worktree_source`
(`New and open worktree actions start from the repo parent workspace.`). `worktree.list`
has no such restriction — it resolves through to the parent checkout and succeeds. Every
`cwd` and `path` must be absolute (`~` is expanded; a relative or empty string fails
`invalid_request` with `worktree path must be absolute`). A worktree becomes a herdr
*workspace* when opened — the create and open results therefore return the full
workspace/tab/pane topology (see [data-model.md](../data-model.md)) in addition to the
`WorktreeInfo`. `trust_repository` is honoured per request only: a trusted call does not
make later untrusted calls succeed and writes nothing to the user's global Git config.
Create, open and remove also publish `worktree_created`, `worktree_opened` and
`worktree_removed` subscription events; delivery is paced on a ~100 ms tick, so a frame can
arrive after the result it describes — see [events.md](../events.md).

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
server uses the focused workspace, which — like any explicit source — must be the
repository's parent checkout. If `branch` is omitted the server derives one of the form
`worktree/<adjective>-<noun>-<4 hex>` (observed: `worktree/green-river-c8c3`,
`worktree/silver-valley-39ff`); `base` selects the ref the new branch/worktree is created
from; `path` overrides the checkout location (default is under herdr's managed worktree
directory, `~/.herdr/worktrees/<repo>/<branch>`, where `<branch>` is the branch name with
`/` flattened to `-`: branch `rvwt/slash-a` lands in `…/worktrees/repo/rvwt-slash-a`).
`label` overrides the workspace label; when it is omitted the label is the **basename of
the checkout directory**, not the branch — branch `zebra` created at `…/wt/xyzdir` yields
workspace label `xyzdir`. The two coincide whenever the managed default path is used.
`focus` controls whether the new workspace is focused after creation. `trust_repository`
grants per-request Git trust for the operation; use it only after the user has verified the
repository, not as a routine retry for a failed worktree command.

The first `worktree.create` in a repository herdr has not seen before opens **two**
workspaces: one adopting the repository's parent checkout and one for the new worktree.

**Params** (`WorktreeCreateParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `workspace_id` | string \| null | no | null | Existing workspace whose repository is the source; resolves the target repo. Must not be a linked-worktree workspace. |
| `cwd` | string \| null | no | null | Absolute working directory used to resolve the source repository when `workspace_id` is absent. Must not be inside a linked worktree. |
| `branch` | string \| null | no | null | Branch name for the new worktree; derived as `worktree/<adjective>-<noun>-<4 hex>` when null. An empty string fails `invalid_request` (`branch is required`). |
| `base` | string \| null | no | null | Git ref the new branch/worktree is based on (inferred). |
| `path` | string \| null | no | null | Explicit absolute checkout path; server-managed location when null. |
| `label` | string \| null | no | null | Workspace label; the checkout directory's basename when null. An empty string is accepted and produces an empty label. |
| `focus` | boolean | no | `false` | Focus the created workspace after opening. |
| `trust_repository` | boolean | no | `false` | Grants per-request Git trust for the operation, bypassing Git's repository-ownership safety check. |

**Result** — `type: "worktree_created"`:

| field | type | meaning |
|---|---|---|
| `type` | string const `"worktree_created"` | Result discriminator. |
| `workspace` | WorkspaceInfo | The created workspace. See [data-model.md](../data-model.md#workspaceinfo). |
| `tab` | TabInfo | The workspace's initial tab. See [data-model.md](../data-model.md#tabinfo). |
| `root_pane` | PaneInfo | The root pane of the initial tab. See [data-model.md](../data-model.md#paneinfo). |
| `worktree` | WorktreeInfo | The created worktree checkout (see [WorktreeInfo](#worktreeinfo) below). |

**Errors**: `worktree_create_failed` (git refused — branch already checked out in another
worktree, target path occupied, invalid branch name, unknown `base`, a concurrent create of
the same branch, or an untrusted repository), `linked_worktree_source` (the resolved source
checkout is itself a linked worktree, including the focused-workspace fallback),
`not_git_worktree` (`cwd` is not inside a Git work tree), `workspace_not_found` (unknown
`workspace_id`), `invalid_request` (empty `branch`, non-absolute `cwd` or `path`). A CLI
invocation with an unknown flag exits status 2 (`unknown option: …` on stderr). Other codes
possible — see [errors.md](../errors.md).

**CLI**: `herdr worktree create [--workspace <ID> | --cwd <PATH>] [--branch <NAME>] [--base <REF>] [--path <PATH>] [--label <TEXT>] [--focus] [--no-focus] [--trust-repository]`

`--workspace` and `--cwd` are mutually exclusive: passing both exits 2 and prints that
usage line. `--focus --no-focus` together is accepted and leaves the workspace unfocused.

**Example**

```json
{"id":"wc1","method":"worktree.create","params":{"cwd":"…/wt-probe","branch":"docs-probe"}}
{"id":"wc1","result":{"type":"worktree_created","workspace":{"workspace_id":"w2","number":2,"label":"docs-probe","focused":false,"pane_count":1,"tab_count":1,"active_tab_id":"w2:t1","agent_status":"unknown","worktree":{"repo_key":"…/wt-probe/.git","repo_name":"wt-probe","repo_root":"…/wt-probe","checkout_path":"/home/penguin/.herdr/worktrees/wt-probe/docs-probe","is_linked_worktree":true}},"tab":{"tab_id":"w2:t1","workspace_id":"w2","number":1,"label":"1","focused":false,"pane_count":1,"agent_status":"unknown"},"root_pane":{"pane_id":"w2:p1","terminal_id":"term_65bb4daf520052","workspace_id":"w2","tab_id":"w2:t1","focused":false,"cwd":"/home/penguin/.herdr/worktrees/wt-probe/docs-probe","foreground_cwd":"/home/penguin/.herdr/worktrees/wt-probe/docs-probe","agent_status":"unknown","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":0,"viewport_rows":40},"revision":0},"worktree":{"path":"/home/penguin/.herdr/worktrees/wt-probe/docs-probe","branch":"docs-probe","is_bare":false,"is_detached":false,"is_prunable":false,"is_linked_worktree":true,"open_workspace_id":"w2","label":"wt-probe"}}}
```

The root pane of a freshly created workspace omits `terminal_title` and
`terminal_title_stripped`; both appear once the pane has settled, as in the
[worktree.open](#worktreeopen) example below.

Validated 2026-09-19 against herdr 0.9.1. (Run against throwaway `git init` repos on a
scratch server; the untrusted-repository path was driven with git's own
`GIT_TEST_ASSUME_DIFFERENT_OWNER=1` rather than a genuinely foreign-owned repository.)

## worktree.list

Lists all worktrees of the Git repository that owns the resolved checkout, along with a
`source` block identifying that repository and the checkout the listing was resolved from.
The target repository is resolved from `workspace_id` or `cwd`; when both are omitted the
server uses the focused workspace. Unlike create and open, a linked worktree is an
acceptable source: the listing resolves through to the repository's parent checkout and
reports that checkout in `source`. Read-only. Each returned `WorktreeInfo` reports
whether it is currently open as a workspace via `open_workspace_id`. `trust_repository`
grants per-request Git trust for the operation; use it only after the user has verified
the repository, not as a routine retry for a failed worktree command.

**Params** (`WorktreeListParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `workspace_id` | string \| null | no | null | Existing workspace whose repository to list; resolves the target repo. |
| `cwd` | string \| null | no | null | Absolute working directory used to resolve the repository when `workspace_id` is absent. |
| `trust_repository` | boolean | no | `false` | Grants per-request Git trust for the operation, bypassing Git's repository-ownership safety check. |

**Result** — `type: "worktree_list"`:

| field | type | meaning |
|---|---|---|
| `type` | string const `"worktree_list"` | Result discriminator. |
| `source` | WorktreeSourceInfo | The repository and checkout the listing was resolved from (see [WorktreeSourceInfo](#worktreesourceinfo) below). |
| `worktrees` | array of WorktreeInfo | All worktrees of the repository (see [WorktreeInfo](#worktreeinfo) below). |

**Errors**: `not_git_worktree` (`cwd` is not inside a Git work tree, including a path that
does not exist and an unexpanded `~`), `workspace_not_found` (unknown or malformed
`workspace_id`), `invalid_request` (neither selector while no workspace is active; a
non-absolute or empty `cwd`; a param of the wrong type; a missing `params` member),
`worktree_list_failed` (git itself refused, e.g. dubious repository ownership). Other codes
possible — see [errors.md](../errors.md).

**CLI**: `herdr worktree list [--workspace <ID> | --cwd <PATH>] [--trust-repository]`

`--workspace` and `--cwd` are mutually exclusive: passing both exits 2 and prints that
usage line.

**Example**

```json
{"id":"wl1","method":"worktree.list","params":{"cwd":"…/wt-probe"}}
{"id":"wl1","result":{"type":"worktree_list","source":{"repo_key":"…/wt-probe/.git","repo_name":"wt-probe","repo_root":"…/wt-probe","source_checkout_path":"…/wt-probe"},"worktrees":[{"path":"…/wt-probe","branch":"main","is_bare":false,"is_detached":false,"is_prunable":false,"is_linked_worktree":false,"label":"wt-probe"}]}}
```

This capture is a repository no workspace has opened yet, which is why `source` has no
`source_workspace_id` and the entry has no `open_workspace_id`. Once the primary checkout
is open as a workspace, `source_workspace_id` appears even for a `cwd`-resolved listing.

Validated 2026-09-19 against herdr 0.9.1. (Run against throwaway `git init` repos on a
scratch server, including a bare clone and worktrees that are detached, prunable or open.)

## worktree.open

Opens an existing Git worktree as a herdr workspace. Like `worktree.create` the result
carries the `WorkspaceInfo`, `TabInfo`, root `PaneInfo`, and `WorktreeInfo`, plus an
`already_open` flag indicating whether that worktree already had an open workspace (in
which case the existing workspace is returned rather than a new one being created). The
worktree is selected by **exactly one** of `path` or `branch`: supplying neither or both
fails `invalid_request` (`exactly one of path or branch is required`), even when the two
name the same worktree. The schema types both as optional and nullable, so the rule is
enforced only by the server. The owning repository is resolved from `workspace_id` or
`cwd`, falling back to the focused workspace; as with `worktree.create` that source must be
the repository's parent checkout, so a `path`-only request works only while the parent
workspace is focused. `label` overrides the workspace label and `focus` controls focus
after opening; both also apply when the worktree is already open, so an open call with
`label` renames the live workspace and `focus: true` refocuses it — the call is not
read-only. `trust_repository` grants per-request Git trust for the operation; use it only
after the user has verified the repository, not as a routine retry for a failed worktree
command.

**Params** (`WorktreeOpenParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `workspace_id` | string \| null | no | null | Existing workspace whose repository owns the worktree; resolves the target repo. Must not be a linked-worktree workspace. |
| `cwd` | string \| null | no | null | Absolute working directory used to resolve the repository when `workspace_id` is absent. Must not be inside a linked worktree. |
| `path` | string \| null | one of | null | Absolute checkout path of the worktree to open. Exactly one of `path` or `branch` must be supplied. |
| `branch` | string \| null | one of | null | Branch name of the worktree to open. Exactly one of `path` or `branch` must be supplied. |
| `label` | string \| null | no | null | Workspace label; the checkout directory's basename when null. Renames the workspace when it is already open. |
| `focus` | boolean | no | `false` | Focus the workspace after opening. |
| `trust_repository` | boolean | no | `false` | Grants per-request Git trust for the operation, bypassing Git's repository-ownership safety check. |

**Result** — `type: "worktree_opened"`:

| field | type | meaning |
|---|---|---|
| `type` | string const `"worktree_opened"` | Result discriminator. |
| `workspace` | WorkspaceInfo | The opened (or already-open) workspace. See [data-model.md](../data-model.md#workspaceinfo). |
| `tab` | TabInfo | The workspace's active tab — for an already-open workspace this is whichever tab is active, not necessarily `:t1`. See [data-model.md](../data-model.md#tabinfo). |
| `root_pane` | PaneInfo | The root pane of the tab. See [data-model.md](../data-model.md#paneinfo). |
| `worktree` | WorktreeInfo | The opened worktree checkout (see [WorktreeInfo](#worktreeinfo) below). |
| `already_open` | boolean | True if the worktree already had an open workspace before this call. |

**Errors**: `invalid_request` (neither or both of `path`/`branch`), `worktree_not_found`
(`worktree path not found`, or `worktree branch not found` when no worktree has that branch
checked out), `not_git_worktree` (`cwd` is not inside a Git work tree),
`linked_worktree_source` (the resolved source checkout is itself a linked worktree,
including the focused-workspace fallback), `worktree_list_failed` (git itself refused —
open resolves the worktree through an internal listing, so Git failures surface under the
`list` code rather than an open-specific one). Other codes possible — see
[errors.md](../errors.md).

**CLI**: `herdr worktree open [--workspace <ID> | --cwd <PATH>] (--path <PATH> | --branch <NAME>) [--label <TEXT>] [--focus] [--no-focus] [--trust-repository]`

`--workspace` and `--cwd` are mutually exclusive, and exactly one of `--path`/`--branch` is
required: violating either exits 2 and prints that usage line.

**Example**

```json
{"id":"wo1","method":"worktree.open","params":{"cwd":"…/wt-probe","path":"/home/penguin/.herdr/worktrees/wt-probe/docs-probe"}}
{"id":"wo1","result":{"type":"worktree_opened","workspace":{"workspace_id":"w2","number":2,"label":"docs-probe","focused":false,"pane_count":1,"tab_count":1,"active_tab_id":"w2:t1","agent_status":"unknown","worktree":{"repo_key":"…/wt-probe/.git","repo_name":"wt-probe","repo_root":"…/wt-probe","checkout_path":"/home/penguin/.herdr/worktrees/wt-probe/docs-probe","is_linked_worktree":true}},"tab":{"tab_id":"w2:t1","workspace_id":"w2","number":1,"label":"1","focused":false,"pane_count":1,"agent_status":"unknown"},"root_pane":{"pane_id":"w2:p1","terminal_id":"term_65bb4daf520052","workspace_id":"w2","tab_id":"w2:t1","focused":false,"cwd":"/home/penguin/.herdr/worktrees/wt-probe/docs-probe","foreground_cwd":"/home/penguin/.herdr/worktrees/wt-probe/docs-probe","terminal_title":"penguin@iceberg:~/.herdr/worktrees/wt-probe/docs-probe","terminal_title_stripped":"penguin@iceberg:~/.herdr/worktrees/wt-probe/docs-probe","agent_status":"unknown","scroll":{"offset_from_bottom":0,"max_offset_from_bottom":0,"viewport_rows":40},"revision":1},"worktree":{"path":"/home/penguin/.herdr/worktrees/wt-probe/docs-probe","branch":"docs-probe","is_bare":false,"is_detached":false,"is_prunable":false,"is_linked_worktree":true,"open_workspace_id":"w2","label":"wt-probe"},"already_open":true}}
```

`already_open: true` because the worktree had just been created and opened as `w2` via
`worktree.create` in the same session. The `cwd` is required here unless the repository's
parent workspace happens to be focused: a `path`-only request run right after
`worktree.create` — when the new linked worktree is focused — fails
`linked_worktree_source`.

Validated 2026-09-19 against herdr 0.9.1. (Exercised by path and by branch, on already-open
worktrees, on a worktree created outside herdr with `git worktree add`, and on a detached
worktree.)

## worktree.remove

Removes a worktree checkout identified by the `workspace_id` of the workspace that has it
open. `workspace_id` is required. Set `force` to remove the checkout even when it has
uncommitted or otherwise dirty state that Git would normally refuse to discard.
`trust_repository` grants per-request Git trust for the operation; use it only after the
user has verified the repository, not as a routine retry for a failed worktree command.
The result echoes the removed checkout `path` and whether removal was `forced` — `forced`
reports the request flag, not whether force was needed, so `force: true` on a clean
worktree still comes back `forced: true`. Removal also **closes the holding workspace**
(emitting `workspace_closed`; a later `workspace.get` answers `workspace_not_found`) and
deletes the checkout directory, but **keeps the branch**: `git branch --list` still shows
it afterwards.

**Params** (`WorktreeRemoveParams`):

| field | type | required | default | meaning |
|---|---|---|---|---|
| `workspace_id` | string | **yes** | — | Workspace whose worktree checkout is removed. |
| `force` | boolean | no | `false` | Force removal despite dirty/uncommitted state. |
| `trust_repository` | boolean | no | `false` | Grants per-request Git trust for the operation, bypassing Git's repository-ownership safety check. |

**Result** — `type: "worktree_removed"`:

| field | type | meaning |
|---|---|---|
| `type` | string const `"worktree_removed"` | Result discriminator. |
| `workspace_id` | string | The workspace whose worktree was removed. |
| `path` | string | Filesystem path of the removed checkout. |
| `forced` | boolean | Whether the request asked for the force path. |

**Errors**: `dirty_worktree_requires_force` (modified or untracked files with `force`
unset), `not_linked_worktree` — two situations with different messages: `workspace is not a
linked worktree checkout` for the repository's primary checkout and `workspace is not a
Herdr-managed worktree checkout` for a workspace with no worktree at all —
`workspace_not_found` (unknown workspace, including one whose worktree was already
removed), `worktree_remove_failed` (git itself refused, e.g. dubious repository ownership),
`invalid_request` (missing or null `workspace_id`; these are deserialization failures, so
the response `id` comes back empty). A CLI invocation with an unknown flag exits status 2
(`unknown option: --path` on stderr — `remove` accepts only `--workspace`, `--force`, and
`--trust-repository`). Other codes possible — see [errors.md](../errors.md).

**CLI**: `herdr worktree remove --workspace <ID> [--force] [--trust-repository]`

Omitting `--workspace` exits 2 and prints that usage line.

**Example**

```json
{"id":"wr1","method":"worktree.remove","params":{"workspace_id":"w2","force":false}}
{"id":"wr1","result":{"type":"worktree_removed","workspace_id":"w2","path":"/home/penguin/.herdr/worktrees/wt-probe/docs-probe","forced":false}}
```

Validated 2026-09-19 against herdr 0.9.1. (Run against throwaway `git init` repos on a
scratch server, over clean, dirty, primary-checkout, worktree-less and untrusted targets.)

## Worktree domain types

These types appear only in `worktree.*` results and are expanded here. Shared entities
(`WorkspaceInfo`, `TabInfo`, `PaneInfo`) live in [data-model.md](../data-model.md).

herdr **omits** optional fields instead of serialising them as `null`, so a client must
test for a key's presence rather than compare it to `null`.

### WorktreeInfo

Describes a single Git worktree of a repository.

| field | type | required | meaning |
|---|---|---|---|
| `path` | string | yes | Checkout path of the worktree. |
| `is_bare` | boolean | yes | True if this is the bare repository entry. |
| `is_detached` | boolean | yes | True if the worktree HEAD is detached. |
| `is_prunable` | boolean | yes | True if Git considers the worktree prunable (stale). |
| `is_linked_worktree` | boolean | yes | True if this is a linked worktree (not the primary checkout). |
| `label` | string | yes | Display label for the worktree: the repository name, identical on every entry of a listing — never the branch or the checkout directory. |
| `branch` | string | no | Checked-out branch. Omitted entirely when the worktree is detached or is the bare entry; the schema types it `string \| null`, but a null is never sent. |
| `open_workspace_id` | string | no | ID of the workspace currently holding this worktree open. Omitted entirely when the worktree is not open; the schema types it `string \| null`, but a null is never sent. |

### WorktreeSourceInfo

Identifies the repository and checkout that a `worktree.list` was resolved from. For a bare
repository, `repo_key` and `repo_root` are both the bare directory (no `/.git` suffix) and
`repo_name` keeps the `.git` suffix, e.g. `bare.git`.

| field | type | required | meaning |
|---|---|---|---|
| `repo_key` | string | yes | Stable key for the repository (its `.git` path). |
| `repo_name` | string | yes | Repository name. |
| `repo_root` | string | yes | Filesystem root of the repository's primary checkout. |
| `source_checkout_path` | string | yes | Checkout path the listing was resolved from — the repository's parent checkout, even when the caller named a linked worktree. |
| `source_workspace_id` | string | no | The repository's **parent** workspace, whenever one is open — including for a `cwd`-resolved listing, and in place of a linked-worktree `workspace_id` the caller passed. Omitted entirely when no such workspace is open; the schema types it `string \| null`, but a null is never sent. |

### WorkspaceWorktreeInfo

The worktree summary embedded in a `WorkspaceInfo.worktree` field (present on
create/open results, and on the `workspace.list`/`workspace.get` entry of any
worktree-backed workspace). Distinct from `WorktreeInfo`.

| field | type | required | meaning |
|---|---|---|---|
| `repo_key` | string | yes | Stable key for the repository (its `.git` path). |
| `repo_name` | string | yes | Repository name. |
| `repo_root` | string | yes | Filesystem root of the repository's primary checkout. |
| `checkout_path` | string | yes | Checkout path of this workspace's worktree. |
| `is_linked_worktree` | boolean | yes | True if the workspace's checkout is a linked worktree. |

Validated 2026-09-19 against herdr 0.9.1. (All three types read from live `worktree.list`,
`worktree.create` and `worktree.open` responses, including bare, detached and prunable
entries.)
