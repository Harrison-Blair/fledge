# Memory

[README](../../README.md)

Memories are durable, non-obvious facts about the project, Fledge, or Herdr that agents share across sessions.
Each is one Markdown file in `.fledge/memories/` under the repository's primary checkout, so every linked worktree reads and writes the same memories.
`.fledge/.gitignore` keeps them untracked.
Memory commands never contact Herdr, so they work outside a Herdr pane.

```sh
fledge memory add --name herdr-socket --type project \
  --description "Herdr commands need socket access in sandboxes" --file note.md
fledge memory list                  # one index line per memory, by name
fledge memory list --type feedback
fledge memory get --name herdr-socket
fledge memory remove --name herdr-socket
```

A memory file has `name`, `description`, and `type` frontmatter, then a nonempty Markdown body:

```markdown
---
name: herdr-socket
description: Herdr commands need socket access in sandboxes
type: project
---

Every `fledge agent` command except `models`, `profiles`, and `capabilities`
without `--live` connects to Herdr's socket.
```

The name is a kebab-case slug of lowercase letters and digits, at most 64 characters, and is the file's basename.
The description is one line.
The type is `user`, `feedback`, `project`, or `reference`.
`add` takes the body from `--body`, or from `--file` (`-` for stdin).

Invalid input fails with `invalid_input` (exit 2) before anything is written, and an existing name fails with `memory_exists` rather than being replaced: remove it first.
`get` prints the file.
`get` and `remove` fail with `memory_not_found` for an unknown name.

`MEMORY.md` beside the memories is the index, one `- [name](name.md) — description` line per memory ordered by name, with no frontmatter.
`add` and `remove` regenerate it from the memory files under the state lock, so concurrent writers never lose a line.
Never edit it by hand.
`list` prints the same lines, or `No memories.`.

A malformed memory file fails `list`, and fails `add` and `remove` before they change any file, naming the file.
Fix or delete it to continue.
All four commands accept `--json`.
`list` returns `memories` (`name`, `description`, `type`) and `get` the memory with its `body`.

Profile spawns inject the index into the worker's brief (see [Profiles](agents.md#profiles)).
