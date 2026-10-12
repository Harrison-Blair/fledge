# Migrating from earlier versions

[README](../../README.md)

This release removes several interfaces without a compatibility layer.

## Spawn placement

Fledge removed the interactive wizard, split placement, `--tab-id`, `--direction`, and `--ratio`.
Spawn's JSON no longer has `split`. The `tab_id`, `pane_id`, and `workspace_id` fields remain.
Every spawn without `--pane` opens a new tab. `--tab` only labels it.
Pass `--pane` to place an agent in a specific existing shell.
See the [agent guide](agents.md).

## Profiles

Profiles are Markdown role instructions in `.fledge/profiles/NAME.md`.
Fledge rejects TOML profiles. It never loads or converts them.
For each `.fledge/profiles/NAME.toml`, migrate in this order:

1. Copy its role instructions into `NAME.md`. This replaces a same-name built-in entirely.
2. Move `harness`, `model`, and `args` to spawn's command line (`--harness`, `--model`, and native arguments after `--`).
3. Delete the TOML file.

There are no `extends`, `[sections]`, `reads`, or `protocol` keys.
Fledge always adds the shared protocol and project memory to a selected profile.
Built-ins no longer choose a harness or model. Pass these on the command line.

Spawn supplies the permission defaults described below.
`agent profiles --json` drops `harness`, `model`, `args`, `reads`, `protocol`, `base`, and `sections`.
Spawn's `profile` object drops `base`.
See [profiles](agents.md#profiles).

## Spawn permissions

Codex defaults to `--yolo`, and Claude to `--permission-mode bypassPermissions`, with or without a profile.
Explicit native permission options take precedence.
Use `--no-permission-bypass` to leave permissions to native arguments and harness configuration.

## Tasks

Briefs are free text. The six-heading template is optional.
Fledge removed `--freeform`. Remove the flag from your commands.
`complete` and `verify` no longer record usage snapshots. Use `fledge agent usage` instead.
Completion notices no longer include the report. Read it with `fledge task get`.
See the [task guide](tasks.md).
