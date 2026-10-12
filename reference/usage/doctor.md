# Doctor

[README](../../README.md)

`fledge doctor` diagnoses the local Fledge and Herdr environment with read-only checks and prints a per-check report.
It never mutates Herdr state, writes files, or reaches the network.

```sh
fledge doctor
fledge doctor --verbose
fledge doctor --json
```

It runs five checks, each independently:

- `herdr_connectivity` — `ping` reaches the Herdr socket (`fail` if not).
- `herdr_compatibility` — the pong `protocol` matches the pinned protocol (`22`).
  A mismatch is `warn`, not `fail`, and the detail reports `version`, `protocol`, and capabilities.
- `harness_installations` — the integration targets Herdr knows about with their `command`, `available`, and `state`.
  `warn` when none are available.
- `model_discovery` — local model discovery gated on availability, restricted to the harness kinds whose models come from local cache files (`pi`, `codex`, `claude`).
  To stay strictly read-only, doctor never executes a harness command, so command-only kinds (`opencode`, `cursor`) are not checked here.
  Use `fledge agent models` for those.
  A missing harness is `warn` (nothing to diagnose).
  An installed harness whose cache is unreadable is `fail`.
  An installed harness with no models, including one that has not created its cache yet, is `warn`.
- `configuration` — `HERDR_ENV`, `HERDR_SOCKET_PATH` and its file mode, `HERDR_PANE_ID`, `HERDR_SESSION`, and the working directory.

Each check reports exactly one status: `ok`, `warn`, or `fail`.
Human output groups checks into blocks.
Each block starts with the padded check name and its status, followed by indented detail.
Blank lines separate the blocks.

A `N ok · N warn · N fail` summary ends the report.
Human output shows paths under your home directory with a leading `~`, such as `~/.config/herdr/herdr.sock`.
`--json` keeps absolute paths.

By default the long detail is hidden to keep the report scannable:

- `herdr_compatibility` shows `version … · protocol … (expected …)`.
- `harness_installations` shows `N targets, M available`.
- `model_discovery` shows the short per-harness counts (`pi N · codex N · claude N`).
- `configuration` always shows its labeled `HERDR_ENV` / `socket` / `pane` / `session` / `cwd` block with an aligned key column.

`--verbose` reveals long detail.
`herdr_compatibility` adds a `capabilities:` line.
Compatibility `warn`/`fail` reports also show this line when capabilities are known.
`harness_installations` adds an `available:` line listing target names.

These lists wrap at their `, ` separators to stay within 80 columns, with continuation lines indented and tokens never split.
`warn` and `fail` checks always show their diagnostic detail, with or without `--verbose`, because it is actionable.
`--verbose` never affects `--json`.

`--json` emits one document with stable per-check names, per-check `data`, and a summary.
The exit code is `1` if any check is `fail`, else `0`.
Warnings never fail.
Outside Herdr, a missing `HERDR_ENV=1` or unreachable socket produces `fail` rather than a crash.
