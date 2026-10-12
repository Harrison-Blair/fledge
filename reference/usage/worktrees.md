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

1. The branch that `base_branch` in `.fledge/config.toml` names, read from the primary checkout's working tree.
   Fledge uses the local branch `refs/heads/<name>` when it exists, else `refs/remotes/origin/<name>`, as in a fresh clone.
2. Otherwise, the branch `origin/HEAD` points to, usually the remote default such as `origin/main`.
3. Otherwise, the local `main` branch.
4. Otherwise, none: every checkout reports merged `unknown`.

Set it when work integrates into a branch other than the remote default.
[`fledge task land`](tasks.md#landing) merges a verified task's branch into this branch, or into the task's own base branch, and never removes a checkout.
Fledge develops on `dev` and reaches `main` only through pull requests, so this repository commits the file:

```toml
schema_version = 1
base_branch = "dev"
```

`schema_version = 1` is required.
`base_branch` is optional: without it, or without the file, Fledge continues with `origin/HEAD`.
Keys are case-sensitive, and an unknown key or a wrong type makes the file invalid.
Give a branch name: `dev`, rather than `origin/dev` or `refs/heads/dev`.
The name must be a valid branch name on one line and must not start with `-`.

The file accepts two more optional settings.
`task land` and the supervision daemon read them once those ship:

```toml
delivery_mode = "local"

[inbox]
ack_timeout = "5m"
```

`delivery_mode` must be the string `local`, `pr`, or `none`.
`[inbox]` is the only table, and `ack_timeout` is its only key.
`ack_timeout` must be a Go duration string, such as `90s` or `5m`, from `10s` to `1h` inclusive.

There is no command to edit the file. Edit it by hand and commit it.
The working-tree copy in the primary checkout applies, uncommitted edits included, and Fledge ignores the copies in linked checkouts.

Fledge no longer reads `fledge.baseBranch` from Git configuration in any scope, repository or `--global`.
If you set it, create `.fledge/config.toml` instead, and remove the old setting with `git config --unset fledge.baseBranch`.

The managed `.fledge/.gitignore` allows `config.toml`.
After you update Fledge, run one command that writes Fledge state in the repository: `fledge task create`, `fledge task import` without `--dry-run`, `fledge memory add`, `fledge agent adopt`, `fledge agent spawn`, or `fledge worktree create`.
That command appends `!/config.toml` to the primary checkout's `.fledge/.gitignore`.
Read-only commands such as `fledge memory list`, `fledge agent current`, and `fledge task list` do not.
No command upgrades a linked checkout: it keeps the ignore file that its creation wrote.
For a linked checkout that an older binary created, append `!/config.toml` to its `.fledge/.gitignore` by hand, or recreate the checkout.
An older Fledge binary does not know the rule: it appends its own block, which ignores the file again until a current binary runs.

If the configured branch exists neither locally nor on `origin`, Fledge does not fall back to `origin/HEAD` or `main`.
An unreadable or invalid file has the same result, with its own reason.
Every checkout reports merged `unknown`.
`worktree list` ends with `MERGED is unknown: ...` and the reason.
JSON sets `default_branch` to null and puts the reason in `default_branch_error`.

`worktree remove` refuses without `--force`, naming the reason.
JSON `default_branch` is the chosen branch's short name, such as `dev` or `origin/main`.
