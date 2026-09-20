# herdr API: integration methods

> herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-17
> Part of the fledge herdr reference. Index: [README.md](../README.md). Wire format: [protocol.md](../protocol.md).

The `integration` namespace installs and removes herdr's built-in integrations into
third-party agent tooling. Each method operates on a single `target`, an enum naming a
supported agent/CLI whose configuration herdr knows how to modify. `install` writes the
integration's configuration into the target's environment; `uninstall` removes it. Both
methods mutate on-disk agent configuration and return a `messages` array describing the
concrete changes performed. Because these operations edit user agent configs, the
`install` and `uninstall` examples below are constructed from the schema and were not
live-executed; the `integration.list` example is a live capture.

The `herdr integration` CLI subcommands do not use this namespace. `herdr integration
status`, `install` and `uninstall` each run entirely in-process against `$HOME` and never
open the herdr socket: against a fake socket server that records every byte, all five
`herdr integration …` invocations captured zero request lines, while control runs of
`herdr tab list` and `herdr agent list` each captured
`{"id":"api-client:status","method":"ping","params":{}}`. With `HERDR_SOCKET_PATH` pointed
at a path that does not exist, `herdr integration status` still exits 0 with its full
output, where `herdr tab list` fails with `server_not_running`. These wire methods
therefore have no first-party CLI caller in 0.9.1, and the **CLI** line in each section
below names the nearest subcommand, not a request that the CLI sends.

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

**Errors**: `invalid_request` is evidenced for request validation, which fails while the
request is being deserialized and therefore never reaches the handler and writes nothing.
`params: {}` returns ``invalid request: missing field `target` at line 1 column 59``; an
out-of-enum value returns ``invalid request: unknown variant `zzz_not_a_real_target`,
expected one of `pi`, `omp`, … `grok` ``; and `{"target":5}` returns ``invalid request:
invalid type: integer `5`, expected string or map``. All three come back with `"id":""` —
the server does not echo the request id on request-validation failures. The handler's own
error codes are unprobed, because reaching the handler requires a well-formed call that
edits user agent configs.

**Events**: none possible — the schema's `Subscription` enum has no `integration.*` variant.

**CLI**: `herdr integration install <TARGET>`, which performs the install in-process and
does not call this method. `herdr integration install --help` lists 18 possible values, one
more than the wire enum: `pi, omp, claude, codex, copilot, devin, droid, kimi, opencode,
kilo, hermes, qodercli, qwen, cursor, mastracode, antigravity-cli, grok, letta`. `letta` has
no wire equivalent; see [IntegrationTarget](#integrationtarget-enum). The CLI also spells
the antigravity target `antigravity-cli` (hyphen) whereas the wire/schema enum value is
`antigravity_cli` (underscore), and the wire rejects the hyphenated spelling as an unknown
variant.

**Example**

```json
{"id":"1","method":"integration.install","params":{"target":"claude"}}
{"id":"1","result":{"type":"integration_install","target":"claude","details":{"messages":["Installed herdr integration for claude"]}}}
```

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: a well-formed call
writes herdr hook files into agent config directories under `$HOME`, an effect that escapes
the scratch session, so only structurally invalid requests — which fail during request
deserialization and never reach the handler — were sent; the params table, the accepted enum
values and the `invalid_request` responses above are live-validated, while the result shape,
`details.messages` and the handler's error codes remain schema-derived).

## integration.list

Lists every integration target herdr knows about on the wire, along with each target's
display label, the CLI command herdr looks for, whether that command is available on the
system, and the integration's current installation state. Read-only; performs no mutation.
The CLI knows one target more than this method returns; see
[IntegrationTarget](#integrationtarget-enum).

**Params**: `EmptyParams` — send `{}`. The key must be present, but the server does not
check its contents strictly.

| field | type | required | default | meaning |
| --- | --- | --- | --- | --- |
| _(none)_ | — | — | — | No parameters are read. `params` must be present and be either a JSON object (any keys, all ignored) or an empty array. |

`EmptyParams` does not deny unknown fields: `{}`, `{"outdated_only":true}` and
`{"a":1,"b":[1,2],"c":{"d":null}}` each returned the full 17-entry list, as did `[]`. A
caller who mistypes a param name gets a success, not an error. Unknown top-level request
keys (`extra`, `jsonrpc`) are ignored too. The schema describes `EmptyParams` only as an
object with no properties.

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

All 17 entries carried exactly these five keys, with no nulls. `command` tracks a `PATH`
lookup exactly: every target whose `command` was on `PATH` reported `available: true` (`pi`,
`claude`, `codex`, `opencode`, `cursor-agent`) and every other reported `false`, with no
exceptions. `cursor` (`cursor-agent`) and `antigravity_cli` (`agy`) are the only targets
whose `command` differs from the target name. Of the `state` values, `current` and
`not_installed` were observed live; `outdated` is in the schema enum and was captured on
this machine on 2026-09-17, but could not be provoked on 2026-09-19 without installing a
stale hook.

**Errors**: `invalid_request` is evidenced for request validation. Omitting `params`
returns ``invalid request: missing field `params` at line 1 column 41``; `params` as `null`,
`""`, `true` or `5` returns `invalid request: invalid type: <value>, expected struct
EmptyParams`; and `params: [1,2]` returns `invalid request: invalid length 2, expected 0
elements in sequence`. These also come back with `"id":""`. No handler-level error could be
provoked; other codes possible.

**Events**: none. A subscriber holding open all 24 subscription types that need no
`pane_id` saw zero events across three `integration.list` calls, and the schema's
`Subscription` enum has no `integration.*` variant.

**CLI**: `herdr integration status [--outdated-only]` is the nearest subcommand, but it does
not call this method: it opens no socket and recomputes everything from `$HOME` in-process,
printing identical output against a real server, against a fake socket server whose fixture
never appeared, and against a socket path that does not exist. It reports a different set of
fields from `IntegrationInfo` — target, state, the installed hook version and the hook file
path, e.g. `pi: current (v9) (/home/penguin/.pi/agent/extensions/herdr-agent-state.ts)` —
omitting `available` and `command`, and adding an 18th line for `letta`. Neither the hook
version nor the hook path is reachable through `integration.list`. `--outdated-only` is
likewise a local filter, not a request param.

**Example**

```json
{"id":"2","method":"integration.list","params":{}}
{"id":"2","result":{"type":"integration_list","integrations":[{"target":"pi","label":"pi","command":"pi","available":true,"state":"current"},{"target":"omp","label":"omp","command":"omp","available":false,"state":"not_installed"},{"target":"claude","label":"claude","command":"claude","available":true,"state":"current"},{"target":"codex","label":"codex","command":"codex","available":true,"state":"current"},{"target":"copilot","label":"copilot","command":"copilot","available":false,"state":"not_installed"},{"target":"devin","label":"devin","command":"devin","available":false,"state":"not_installed"},{"target":"droid","label":"droid","command":"droid","available":false,"state":"not_installed"},{"target":"kimi","label":"kimi","command":"kimi","available":false,"state":"not_installed"},{"target":"opencode","label":"opencode","command":"opencode","available":true,"state":"current"},{"target":"kilo","label":"kilo","command":"kilo","available":false,"state":"not_installed"},{"target":"hermes","label":"hermes","command":"hermes","available":false,"state":"not_installed"},{"target":"qodercli","label":"qodercli","command":"qodercli","available":false,"state":"not_installed"},{"target":"qwen","label":"qwen","command":"qwen","available":false,"state":"not_installed"},{"target":"cursor","label":"cursor","command":"cursor-agent","available":true,"state":"current"},{"target":"mastracode","label":"mastracode","command":"mastracode","available":false,"state":"not_installed"},{"target":"antigravity_cli","label":"antigravity-cli","command":"agy","available":false,"state":"not_installed"},{"target":"grok","label":"grok","command":"grok","available":false,"state":"not_installed"}]}}
```

Validated 2026-09-19 against herdr 0.9.1. (Captured against a scratch server; `state` and
`available` values reflect this machine's local agent-tool installs, not a fixed fixture,
and the `outdated` state was not exercised. Read-only was checked by 50 consecutive calls,
after which the md5 of every installed herdr hook file was unchanged; two back-to-back calls
returned byte-identical results, and target order was identical across all 80+ calls made
during the probe.)

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

**Errors**: as with `install`, `invalid_request` is evidenced for request validation, which
fails during deserialization and removes nothing. `params: {}` returns ``invalid request:
missing field `target` at line 1 column 61``; an out-of-enum value returns ``invalid
request: unknown variant `zzz_not_a_real_target`, expected one of `pi`, `omp`, … `grok` ``,
naming the same 17 values as `install`. Both come back with `"id":""`. The handler's own
error codes are unprobed.

**Events**: none possible — the schema's `Subscription` enum has no `integration.*` variant.

**CLI**: `herdr integration uninstall <TARGET>`, which performs the removal in-process and
does not call this method. `herdr integration uninstall --help` lists the same 18 possible
values as `install`, including `letta`, which has no wire equivalent. As with install, the
CLI spells the antigravity target `antigravity-cli` (hyphen) whereas the wire/schema enum
value is `antigravity_cli` (underscore).

**Example**

```json
{"id":"1","method":"integration.uninstall","params":{"target":"claude"}}
{"id":"1","result":{"type":"integration_uninstall","target":"claude","details":{"messages":["Removed herdr integration for claude"]}}}
```

Constructed from schema; not live-validated (2026-09-19, herdr 0.9.1: a well-formed call
removes hook files and hook entries from agent config under `$HOME`, an effect that escapes
the scratch session, so only structurally invalid requests — which fail during request
deserialization and never reach the handler — were sent; the params table, the accepted enum
values and the `invalid_request` responses above are live-validated, while the result shape,
`details.messages` and the handler's error codes remain schema-derived).

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
the response definition (`success_response/$defs/IntegrationTarget`). On the wire the value
must be the exact enum string: `{"target":"antigravity-cli"}` is rejected with ``invalid
request: unknown variant `antigravity-cli` ``. The CLI spells the same target
`antigravity-cli`, but it never puts a target on the wire, so the hyphen-to-underscore
substitution is a fact about CLI argument spelling only and not an observable translation.

The CLI knows one target that the wire does not. `herdr integration install|uninstall
--help` lists an 18th possible value, `letta`, and `herdr integration status` prints
`letta (experimental): not installed (/home/penguin/.letta/hooks/herdr-agent-session.sh)`
as an 18th line, but `letta` is absent from the schema enum, absent from the 17 entries
`integration.list` returns, and absent from the server's own unknown-variant error list. A
caller restricted to the wire API cannot reach the letta integration at all.

Validated 2026-09-19 against herdr 0.9.1. (The 17 values and their order match
`schema.json`, the server's own unknown-variant error and the order of `integration.list`
entries; the `tool (inferred)` column was not verified beyond the `label` and `command`
values the server returns.)
