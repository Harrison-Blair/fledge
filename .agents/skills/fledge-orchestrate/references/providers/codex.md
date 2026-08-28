# Codex

Read this note when the root harness is Codex or before spawning a Codex worker. The portable workflow in `SKILL.md` remains authoritative.

## Model map

| Tier | Model | Reasoning effort |
| --- | --- | --- |
| strongest | `gpt-5.6-sol` | `xhigh` |
| decent | `gpt-5.6-luna` | `xhigh` |
| mid-tier | `gpt-5.6-luna` | `medium` |
| cheap | `gpt-5.6-luna` | `low` |

Spawn with Fledge's model field and pass reasoning effort after the argument separator:

```sh
fledge agent spawn <name> --kind codex --model <model> -- -c 'model_reasoning_effort="<effort>"'
```

For example:

```sh
fledge agent spawn impl-1 --kind codex --model gpt-5.6-luna -- -c 'model_reasoning_effort="xhigh"'
```

Do not combine model and effort into one `--model` value. Do not use native Codex sub-agent or collaboration tools.

## Callback behavior

Fledge submits terminal input with Enter. When a Codex root is idle, a callback starts a new turn; while it is working, the callback steers the active turn. Treat the complete callback as new orchestration input and incorporate it without dropping the work already in progress.

Fledge cannot invoke Codex's separate next-turn queue behavior. Do not work around this with native Codex queue or collaboration commands. A blocked approval or question UI can reject delivery; the child then follows the portable callback-failure rule and remains intact.
