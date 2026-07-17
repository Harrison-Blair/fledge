# Codex CLI — fledge adapter

fledge's agent-neutral workflow lives at `.fledge/skills/fledge-orchestrate/`. This file maps fledge's 6 orchestration primitives to OpenAI Codex CLI mechanisms.

## Derived tier

**Tier A** — provided: `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`. Not provided: `spawn-worker`, `message-peer`.

## Primitive map

| Primitive | Capability | Codex mechanism | Provided | Required for |
|---|---|---|---|---|
| `confirm-gate` | present material, get a structured Accept/Make-changes or option choice | chat | yes | A |
| `read-only-shell` | run read-only shell commands | shell (read-only) | yes | A |
| `write-file` | write a file | apply_patch / edit | yes | A |
| `run-fledge` | run any fledge CLI subcommand (incl. all spec mutation) | shell (fledge ...) | yes | A |
| `spawn-worker` | spawn a fresh, context-free, named, addressable sub-session returning one final message | — | no | B |
| `message-peer` | send an async by-name message; sender may idle, woken on reply | — | no | C |


## Notes

- Spec mutation goes through `run-fledge` (Codex shell running `fledge …`); never hand-edit spec frontmatter the CLI can write.
- `confirm-gate` (review): present the material under review — for a spec-body draft, a summary plus the on-disk file path to open (and a diff on each revision), never the pasted body — then ask the user in chat for a structured "Accept" / "Make changes" choice; loop on "Make changes" until "Accept".
- See `.fledge/skills/fledge-orchestrate/SKILL.md` for routing and ground rules. With this Tier A profile, planning foraging and implementation both run solo (in-session) per `planning.md` step 2 and `implementation.md` §2.
- Codex auto-loads `AGENTS.md` at the repo root; `fledge init` adds a one-line pointer there (additively — it never overwrites your existing `AGENTS.md`).
