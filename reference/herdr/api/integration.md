# herdr API: integration methods

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `integration` namespace installs and removes herdr's built-in integrations into
third-party agent tooling. Each method operates on a single `target`, an enum naming a
supported agent/CLI whose configuration herdr knows how to modify. `install` writes the
integration's configuration into the target's environment; `uninstall` removes it. Both
methods mutate on-disk agent configuration and return a `messages` array describing the
concrete changes performed. Because these operations edit user agent configs, the reference
examples below are constructed from the schema and were not live-executed.

The CLI also exposes an `integration status` subcommand (with an `--outdated-only` flag),
which corresponds to `integration.list` below.

3 methods:

| method | purpose |
| --- | --- |
| [integration.install](#integrationinstall) | Install a built-in agent integration into a target tool. |
| [integration.list](#integrationlist) | List all known integration targets and their installation state. |
| [integration.uninstall](#integrationuninstall) | Remove a previously installed integration from a target tool. |

## integration.install

Installs herdr's built-in integration into the named target agent/CLI. This mutates the
target's on-disk configuration; the returned `details.messages` list describes the changes
that were applied.

**Params** (`IntegrationInstallParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | `IntegrationTarget` (enum) | yes | — | The agent/tool to install the integration into. See [IntegrationTarget](#integrationtarget-enum). |

**Result** — `type: "integration_install"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | `"integration_install"` (const) | yes | Result discriminator. |
| `target` | `IntegrationTarget` (enum) | yes | Echoes the target that was installed. See [IntegrationTarget](#integrationtarget-enum). |
| `details` | `IntegrationInstallResult` object | yes | Details of the installation (see below). |

`IntegrationInstallResult`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `messages` | `string[]` | yes | Human-readable messages describing the changes applied during installation (inferred). |

**Errors**: No probe evidence (this method was not executed because it modifies user agent
configs). Codes possible.

**CLI**: `herdr integration install <TARGET>` where `<TARGET>` is one of the possible values
`pi, omp, claude, codex, copilot, devin, droid, kimi, opencode, kilo, hermes, qodercli, qwen,
cursor, mastracode, antigravity-cli, grok`. Note the CLI spells the antigravity target
`antigravity-cli` (hyphen) whereas the wire/schema enum value is `antigravity_cli`
(underscore).

**Example** — `Constructed from schema; not live-validated.`

```json
{"id":"1","method":"integration.install","params":{"target":"claude"}}
{"id":"1","result":{"type":"integration_install","target":"claude","details":{"messages":["Installed herdr integration for claude"]}}}
```

## integration.list

Lists every integration target herdr knows about, along with each target's display label,
the CLI command herdr looks for, whether that command is available on the system, and the
integration's current installation state. Read-only; performs no mutation.

**Params**: `EmptyParams` — an empty object. Send `{}`.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| _(none)_ | — | — | — | No parameters. `params` must be present and be `{}`. |

**Result** — `type: "integration_list"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | `"integration_list"` (const) | yes | Result discriminator. |
| `integrations` | `IntegrationInfo[]` | yes | One entry per known integration target (see below). |

`IntegrationInfo`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `target` | `IntegrationTarget` (enum) | yes | The integration target this entry describes. See [IntegrationTarget](#integrationtarget-enum). |
| `label` | `string` | yes | Human-readable display name for the target. |
| `command` | `string` | yes | The CLI command name herdr checks for on the system for this target. |
| `available` | `boolean` | yes | `true` if `command` was found on the system. |
| `state` | `IntegrationState` (enum) | yes | Installation state of the integration. One of: `not_installed`, `current`, `outdated`. |

**Errors**: none evidenced; other codes possible.

**CLI**: `herdr integration status [--outdated-only]`. Since the wire method takes
`EmptyParams`, `--outdated-only` is not sent as a request param; the CLI requests the full
list and filters entries whose `state` is `outdated` itself (inferred).

**Example**

```json
{"id":"2","method":"integration.list","params":{}}
{"id":"2","result":{"type":"integration_list","integrations":[{"target":"pi","label":"pi","command":"pi","available":true,"state":"outdated"},{"target":"omp","label":"omp","command":"omp","available":false,"state":"not_installed"},{"target":"claude","label":"claude","command":"claude","available":true,"state":"outdated"},{"target":"codex","label":"codex","command":"codex","available":true,"state":"current"},{"target":"copilot","label":"copilot","command":"copilot","available":false,"state":"not_installed"},{"target":"devin","label":"devin","command":"devin","available":false,"state":"not_installed"},{"target":"droid","label":"droid","command":"droid","available":false,"state":"not_installed"},{"target":"kimi","label":"kimi","command":"kimi","available":false,"state":"not_installed"},{"target":"opencode","label":"opencode","command":"opencode","available":true,"state":"outdated"},{"target":"kilo","label":"kilo","command":"kilo","available":false,"state":"not_installed"},{"target":"hermes","label":"hermes","command":"hermes","available":false,"state":"not_installed"},{"target":"qodercli","label":"qodercli","command":"qodercli","available":false,"state":"not_installed"},{"target":"qwen","label":"qwen","command":"qwen","available":false,"state":"not_installed"},{"target":"cursor","label":"cursor","command":"cursor-agent","available":true,"state":"current"},{"target":"mastracode","label":"mastracode","command":"mastracode","available":false,"state":"not_installed"},{"target":"antigravity_cli","label":"antigravity-cli","command":"agy","available":false,"state":"not_installed"},{"target":"grok","label":"grok","command":"grok","available":false,"state":"not_installed"}]}}
```

Validated 2026-09-17 against herdr 0.9.1. (Captured against a scratch server; `state` and
`available` values reflect this machine's local agent-tool installs, not a fixed fixture.)

## integration.uninstall

Removes a previously installed herdr integration from the named target agent/CLI. This mutates
the target's on-disk configuration; the returned `details.messages` list describes the changes
that were applied.

**Params** (`IntegrationUninstallParams`):

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| `target` | `IntegrationTarget` (enum) | yes | — | The agent/tool to uninstall the integration from. See [IntegrationTarget](#integrationtarget-enum). |

**Result** — `type: "integration_uninstall"`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `type` | `"integration_uninstall"` (const) | yes | Result discriminator. |
| `target` | `IntegrationTarget` (enum) | yes | Echoes the target that was uninstalled. See [IntegrationTarget](#integrationtarget-enum). |
| `details` | `IntegrationUninstallResult` object | yes | Details of the uninstallation (see below). |

`IntegrationUninstallResult`:

| field | type | required | meaning |
| --- | --- | --- | --- |
| `messages` | `string[]` | yes | Human-readable messages describing the changes applied during uninstallation (inferred). |

**Errors**: No probe evidence (this method was not executed because it modifies user agent
configs). Codes possible.

**CLI**: `herdr integration uninstall <TARGET>` where `<TARGET>` is one of the possible values
`pi, omp, claude, codex, copilot, devin, droid, kimi, opencode, kilo, hermes, qodercli, qwen,
cursor, mastracode, antigravity-cli, grok`. As with install, the CLI spells the antigravity
target `antigravity-cli` (hyphen) whereas the wire/schema enum value is `antigravity_cli`
(underscore).

**Example** — `Constructed from schema; not live-validated.`

```json
{"id":"1","method":"integration.uninstall","params":{"target":"claude"}}
{"id":"1","result":{"type":"integration_uninstall","target":"claude","details":{"messages":["Removed herdr integration for claude"]}}}
```

## IntegrationTarget (enum)

Shared enum naming the supported integration targets. Used as the `target` param of
`install`/`uninstall` (echoed back in each of their results), and as the `target` field of
each `IntegrationInfo` entry returned by `integration.list`. The wire/schema values are:

| value | tool (inferred) |
| --- | --- |
| `pi` | pi agent |
| `omp` | omp agent |
| `claude` | Claude Code |
| `codex` | Codex |
| `copilot` | GitHub Copilot |
| `devin` | Devin |
| `droid` | Droid |
| `kimi` | Kimi |
| `opencode` | OpenCode |
| `kilo` | Kilo |
| `hermes` | Hermes |
| `qodercli` | Qoder CLI |
| `qwen` | Qwen |
| `cursor` | Cursor |
| `mastracode` | Mastra Code |
| `antigravity_cli` | Antigravity CLI (CLI arg: `antigravity-cli`) |
| `grok` | Grok |

The enum is identical in both the request definition (`request/$defs/IntegrationTarget`) and
the response definition (`success_response/$defs/IntegrationTarget`). The CLI accepts these
values with hyphens substituted for underscores (`antigravity-cli`); on the wire the value
must be the exact enum string (`antigravity_cli`).
