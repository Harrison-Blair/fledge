# Claude Models & CLI Reference

Reference for (1) every Claude model version and its API model ID string, including how those IDs differ across hosting platforms, and (2) how to drive Claude Code from the command line — selecting a model, scripting non-interactive runs, and how "agent types" actually work.

## 1. Model catalog

### Current models

| Friendly name    | Model ID           | Context | Max output |
|-------------------|---------------------|---------|------------|
| Claude Fable 5     | `claude-fable-5`      | 1M      | 128K       |
| Claude Mythos 5 *(Project Glasswing only)* | `claude-mythos-5` | 1M | 128K |
| Claude Opus 5      | `claude-opus-5`       | 1M      | 128K       |
| Claude Opus 4.8    | `claude-opus-4-8`     | 1M      | 128K       |
| Claude Opus 4.7    | `claude-opus-4-7`     | 1M      | 128K       |
| Claude Opus 4.6    | `claude-opus-4-6`     | 1M      | 128K       |
| Claude Sonnet 5    | `claude-sonnet-5`     | 1M      | 128K       |
| Claude Sonnet 4.6  | `claude-sonnet-4-6`   | 1M      | 128K       |
| Claude Haiku 4.5   | `claude-haiku-4-5` (full: `claude-haiku-4-5-20251001`) | 200K | 64K |

### Legacy models (still active)

| Friendly name    | Model ID           | Full ID                     | Status |
|-------------------|---------------------|------------------------------|--------|
| Claude Opus 4.5    | `claude-opus-4-5`     | `claude-opus-4-5-20251101`  | Active |
| Claude Opus 4.1    | `claude-opus-4-1`     | `claude-opus-4-1-20250805`  | Deprecated — retires 2026-08-05, migrate to `claude-opus-5` |
| Claude Sonnet 4.5  | `claude-sonnet-4-5`   | `claude-sonnet-4-5-20250929`| Active |

### Deprecated models (retiring soon)

| Friendly name  | Model ID           | Full ID                   | Retires      |
|-----------------|---------------------|-----------------------------|--------------|
| Claude Sonnet 4 | `claude-sonnet-4-0`   | `claude-sonnet-4-20250514`| TBD          |
| Claude Opus 4   | `claude-opus-4-0`     | `claude-opus-4-20250514`  | TBD          |
| Claude Haiku 3  | —                    | `claude-3-haiku-20240307` | Apr 19, 2026 |

### Retired models (no longer callable — 404)

| Friendly name     | Full ID                       | Retired      | Replacement          |
|---------------------|--------------------------------|---------------|------------------------|
| Claude Sonnet 3.7  | `claude-3-7-sonnet-20250219`  | Feb 19, 2026  | `claude-sonnet-5`     |
| Claude Haiku 3.5   | `claude-3-5-haiku-20241022`   | Feb 19, 2026  | `claude-haiku-4-5`    |
| Claude Opus 3      | `claude-3-opus-20240229`      | Jan 5, 2026   | `claude-opus-4-8`     |
| Claude Sonnet 3.5  | `claude-3-5-sonnet-20241022`  | Oct 28, 2025  | `claude-sonnet-5`     |
| Claude Sonnet 3.5  | `claude-3-5-sonnet-20240620`  | Oct 28, 2025  | `claude-sonnet-5`     |
| Claude Sonnet 3    | `claude-3-sonnet-20240229`    | Jul 21, 2025  | `claude-sonnet-5`     |
| Claude 2.1         | `claude-2.1`                  | Jul 21, 2025  | `claude-sonnet-5`     |
| Claude 2.0         | `claude-2.0`                  | Jul 21, 2025  | `claude-sonnet-5`     |

### Alias lookup

| User says...                  | Model ID              |
|--------------------------------|------------------------|
| "fable" / "most capable model" | `claude-fable-5`         |
| "mythos" / "mythos 5"          | `claude-mythos-5` (Project Glasswing participants only) |
| "opus" / "opus 5"              | `claude-opus-5`            |
| "sonnet" / "sonnet 5" / "balanced" | `claude-sonnet-5`   |
| "haiku" / "fast" / "cheap"     | `claude-haiku-4-5`       |

> Only use exact model ID strings from these tables — never construct one by guessing or appending a date suffix to an alias. Wrong IDs 404.

## 2. Cross-platform model ID formats

The same model is addressed differently depending on which platform serves the request:

| Platform | ID format | Example |
|----------|-----------|---------|
| **Direct Anthropic API** | Bare ID | `claude-opus-5` |
| **Claude Platform on AWS** | Bare ID (same as direct API — Anthropic-operated, same-day parity) | `claude-opus-5` |
| **Amazon Bedrock** | `anthropic.`-prefixed, via the `AnthropicBedrockMantle` client | `anthropic.claude-opus-5` |
| **Google Vertex AI** | No prefix for current-gen models; dated/legacy snapshots use an `@` version separator (not a dash suffix) | `claude-opus-5`, or `claude-opus-4-5@20251101` for a dated snapshot |
| **Microsoft Foundry** | Bare ID, via the `AnthropicFoundry` client | `claude-opus-5` |

**Do not cross-apply formats.** A first-party bare ID sent to Bedrock, or an `anthropic.`-prefixed ID sent to the direct API, both 400. Claude Platform on AWS is not the same thing as Bedrock — it's Anthropic-operated infrastructure reached through AWS, using bare first-party IDs; Bedrock is AWS-operated and uses the `anthropic.` prefix.

## 3. CLI: selecting a model

```sh
claude --model claude-sonnet-5
claude --model sonnet     # alias — resolves to the latest version
```

- Accepted values: aliases (`sonnet`, `opus`, `haiku`, `fable`) for the latest version of that tier, or a full model ID from the tables above.
- Precedence (highest wins): `--model` flag → `model` key in `settings.json` → `ANTHROPIC_MODEL` environment variable.
- The interactive equivalent is the `/model` slash command inside a running session.

## 4. CLI: non-interactive / scripted invocation

```sh
claude -p "your query"
claude --print "your query"
```

Runs one query via the SDK and exits — the building block for scripting and automation. Useful companion flags:

| Flag | Purpose |
|------|---------|
| `--model <alias-or-id>` | Set the model for this run |
| `--output-format <json\|stream-json\|text>` | Control response format |
| `--json-schema <schema>` | Force structured output |
| `--max-turns <n>` | Cap agentic turns |
| `--max-budget-usd <n>` | Cap spend |
| `--verbose` | Turn-by-turn output |
| `--bare` | Skip loading hooks, skills, plugins, and MCP servers — for CI/scripts. Pass credentials via `ANTHROPIC_API_KEY` instead of OAuth. |

## 5. CLI: agent types — how they actually work

**There is no CLI flag to open a prompt with a specific agent type, and no flag to list all available agent types.** There is no `--agent <name>` flag and no global agent registry — agent selection is not a CLI-level concept.

Instead:

- **Agents are defined per-project** in `.claude/agents/*.md` files (frontmatter + instructions).
- **Agents are invoked within a running session**, not from the shell — via the Agent tool, or `/subtask` / `/fork`.
- **In `--bare` mode**, agent definitions can be injected programmatically with `--agents <json>`; otherwise they're loaded from `.claude/agents/` in the working directory.

Four broader multi-agent coordination mechanisms exist, none of which are a "pick an agent type" CLI flag:

| Mechanism | What it is |
|-----------|------------|
| **Subagents** | In-session delegated workers, spawned via the Agent tool |
| **Agent view** | `claude agents` — dispatch and monitor background sessions |
| **Agent teams** | Coordinated multi-session work sharing a task list |
| **Dynamic workflows** | Scripted multi-agent orchestration via the Workflow tool |

**Bottom line:** to run a specific agent type, define it in `.claude/agents/`, then invoke it from inside a session (interactively or via a script that drives the SDK) — not from a shell flag.
