# Worktrees

[README](../../README.md)

Worktree commands run inside Herdr and use the same `--json` outcome envelope as agent commands.
They act on the repository containing `--cwd`.
That defaults to the current directory, which may be a linked checkout.

```sh
fledge worktree list
fledge worktree list --cwd ~/src/project --json
fledge worktree create --branch feature/task --base dev
fledge worktree remove --branch feature/task
fledge worktree remove --path .fledge/worktrees/feature/task --force --json
```

## List and create

`list` shows every checkout, primary first.
It reports the branch or detached state, open Herdr workspace, dirtiness including untracked files, merge status, and managed status under `.fledge/worktrees`.
Merged means the branch head (or detached HEAD) is an ancestor of the [integration branch](#integration-branch).
Squash-merged branches therefore count as unmerged. Dirtiness and merge checks report `unknown` when Git cannot answer.

The `OWNER` column names the earliest registered live agent whose spawn created or opened that checkout, as `name (id)`.
It adds `+N` for N other live agents sharing the checkout, or shows `-` when no owner exists.
JSON rows carry `owner` (`{id, name, pane}` of that earliest agent, or null) and `owner_count`.
Records of agents no longer in Herdr do not count. A repository without a state store shows no owners.

Owner reporting is best effort.
If Herdr's agent list is unavailable, every row shows no owner.
The same applies if any list entry lacks a required field, such as its terminal ID.

`create` makes a managed checkout at `.fledge/worktrees/<branch>` on a new branch.
It opens a workspace without starting an agent, then writes the checkout's managed [`.fledge/.gitignore`](agents.md#profiles).
Fledge refuses an existing branch.

## Remove

`remove` takes exactly one of `--path` or `--branch` and keeps the branch.
Fledge removes an open checkout through Herdr, which also closes its workspace.
It removes a closed checkout through `git worktree remove`.
It never removes the primary checkout.

Removal refuses checkouts in use by live agents, whether or not the workspace is open.
It checks again immediately before removal. `--force` does not override this guard.
An agent uses a checkout when any of these conditions holds:

- Its pane is in the checkout's workspace.
- Its working directory is the checkout or inside it, from any workspace.
- Its record names a worktree at or inside the checkout.

For example, `fledge agent spawn --cwd .fledge/worktrees/feat` in another tab counts.
Fledge resolves symlinks before comparing paths. A sibling such as `feature` is not inside `feat`.
The guard sees only agents in the connected Herdr session.
Other sessions and direct Herdr actions are outside it.

Records of agents no longer in Herdr do not count.
An unreadable state store refuses removal until you fix or remove the bad record under `.fledge/state`.
A malformed Herdr agent list refuses removal with `protocol_error` if any entry lacks a required field, such as its terminal ID.

Removal refuses dirty or unmerged checkouts, or unknown results from either check, unless you pass `--force`.
It judges the merge against the [integration branch](#integration-branch).

## Integration branch

`worktree list` reports `MERGED` and `worktree remove` guards removal against one integration branch per repository.
`agent cleanup` instead uses each checkout's recorded creation base (see [Cleanup](agents.md#cleanup)).
The integration branch uses this order:

1. The local branch named by Git configuration `fledge.baseBranch`, resolved as `refs/heads/<name>`.
2. Otherwise, the branch `origin/HEAD` points to, usually the remote default such as `origin/main`.
3. Otherwise, the local `main` branch.
4. Otherwise, none: every checkout reports merged `unknown`.

Set it when work integrates into a branch other than the remote default.
Fledge develops on `dev` and reaches `main` only through pull requests.
Contributors must set it in each clone because Git does not copy local configuration:

```sh
git config fledge.baseBranch dev
```

Fledge reads `fledge.baseBranch` from ordinary Git configuration scopes, like `git config fledge.baseBranch`.
The repository value applies to every linked checkout and takes precedence over the global value.
`git config --global fledge.baseBranch dev` applies to every repository without its own setting.
If a repository lacks that branch, it reports `does not exist` as described below, without a fallback.

Unset it with `git config --unset fledge.baseBranch`. Add `--global` for the global value.
If Git cannot read its configuration, merged is `unknown` with Git's error as the reason.
Give a local branch name: `dev`, rather than `origin/dev` or `refs/heads/dev`.

If the configured branch does not exist, Fledge does not fall back to `origin/HEAD` or `main`.
Every checkout reports merged `unknown`.
`worktree list` ends with `MERGED is unknown: ...` and the reason.
JSON sets `default_branch` to null and puts the reason in `default_branch_error`.

`worktree remove` refuses without `--force`, naming the reason.
JSON `default_branch` is the chosen branch's short name, such as `dev` or `origin/main`.
