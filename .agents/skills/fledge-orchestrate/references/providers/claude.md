# Claude Code

Read this note when the root harness is Claude Code or before spawning a Claude worker. The portable workflow in `SKILL.md` remains authoritative.

## Model map

| Tier | Model | Effort |
| --- | --- | --- |
| strongest | `claude-fable-5` | `high` |
| decent | `claude-opus-4-8` | `xhigh` |
| mid-tier | `claude-sonnet-5` | `medium` |
| cheap | `claude-haiku-4-5` | `low` |

Spawn with Fledge's model field and pass effort after the argument separator:

```sh
fledge agent spawn <name> --kind claude --model <model> -- --effort <effort>
```

For example:

```sh
fledge agent spawn review-1 --kind claude --model claude-fable-5 -- --effort high
```

Use the versioned model slugs above rather than moving aliases. Do not use Claude's native `Agent`, `SendMessage`, or `TaskStop` tools.

## Callback behavior

Claude Code queues terminal messages that arrive while it is working and exposes them to the model after active tool calls or in the next turn. Children still send exactly one Fledge callback without `--wait`; do not add native message or completion mechanisms.

Claude has no provider metadata sidecar that can disable implicit invocation for this shared portable skill. The description and opening instruction in `SKILL.md` enforce explicit-only use at the instruction level.
