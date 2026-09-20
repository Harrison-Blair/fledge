# Re-syncing these docs after a herdr update

> Docs describe herdr 0.9.1 · protocol 22 · schema_version 1 · captured 2026-09-19

herdr self-updates (`herdr update`, channel via `herdr channel set <stable|preview>`), so
the installed binary will drift ahead of these docs. The raw artifacts in `raw/` are the
diff baseline, but a schema diff alone is not enough — the procedure is: re-emit, diff,
behaviourally re-probe every page, update what changed, re-stamp.

## Procedure

1. **Check the installed version and protocol.**

   ```sh
   herdr --version
   herdr api schema | head -4   # shows protocol + schema_version
   ```

   If both match the stamp above, stop — the docs are current.

2. **Re-emit the raw artifacts into a scratch directory and diff.**

   ```sh
   herdr api schema --json > /tmp/herdr-schema.json
   herdr --skill            > /tmp/herdr-skill.md
   herdr --default-config   > /tmp/herdr-default-config.toml
   diff raw/schema.json /tmp/herdr-schema.json
   diff raw/skill.md /tmp/herdr-skill.md
   diff raw/default-config.toml /tmp/herdr-default-config.toml
   ```

   For a readable schema diff, compare the method inventory first:

   ```sh
   jq -r '.schemas.request.oneOf[].properties.method.const' raw/schema.json | sort > /tmp/old-methods
   jq -r '.schemas.request.oneOf[].properties.method.const' /tmp/herdr-schema.json | sort > /tmp/new-methods
   diff /tmp/old-methods /tmp/new-methods
   ```

   Same idea for event kinds (`.schemas.event."$defs".EventKind.enum[]`) and result types
   (`.schemas.success_response."$defs".ResponseResult.oneOf[].properties.type.const`).
   `jq` is only for these local file diffs — it is not available for talking to the
   socket (see step 4).

3. **Behaviourally re-probe every page on a version bump — not only schema-diffed
   ones.** A method's request/response *shape* can be unchanged while its *behavior*
   drifts: timing, validation order, error-code precedence, the exact event name on the
   wire, an undocumented side effect, a default that silently clamps or truncates. None
   of that shows up in the schema diff from step 2. On any version bump, re-exercise
   every documented claim on every page — read the page, re-run each example, re-check
   each stated Errors/Events entry — and only then move to step 5 to update what the
   probes actually show changed. Do not skip a page just because its methods are absent
   from the schema diff.

4. **Probe safely: mutating calls go to your own scratch server, never the default
   socket.** The default socket (`$HERDR_SOCKET_PATH`, normally
   `~/.config/herdr/herdr.sock`) may be hosting a live session — only read-only methods
   (`*.get`, `*.list`, `session.snapshot`, `pane.read` on your own panes) may ever be
   sent there. Start an isolated scratch server under its own session name for every
   mutating probe:

   ```sh
   env -u HERDR_SOCKET_PATH herdr --session <name> server &   # e.g. <name> = rv-agent
   # socket at ~/.config/herdr/sessions/<name>/herdr.sock
   ```

   Pick a session name scoped to what you're probing, and never touch a session
   directory you did not create. The socket speaks newline-delimited JSON, one request
   per connection: connect, send one line `{"id":"1","method":"...","params":{...}}`,
   read one line back. `events.subscribe` / `*.wait`-style calls stay open and need
   their own connection. There is no `herdr api call` subcommand and no `jq` on this
   path — script the socket directly with `python3`'s `socket` module. When done, stop
   the scratch server and confirm the process is gone:

   ```sh
   HERDR_SOCKET_PATH=~/.config/herdr/sessions/<name>/herdr.sock herdr server stop
   ```

   Some methods must never be probed this way because their effects escape the scratch
   session entirely — they write under the home directory outside
   `~/.config/herdr/sessions/<name>/` (for example `integration.install` /
   `integration.uninstall`, and any `plugin.*` call that writes to a config root shared
   across sessions rather than the scratch session's own directory). Record these as
   **not probed**, with the reason, instead of guessing at their live behavior.
   `server.stop` and `server.live_handoff` may only ever be sent to a scratch server you
   started yourself.

5. **Update the affected pages.** Added/removed/changed methods map to their namespace
   file in `api/` (see the README table); event changes go to `events.md`; envelope or
   `$defs` changes to `protocol.md` / `data-model.md`; new CLI subcommands to
   `cli-mapping.md` (re-sweep with `herdr <group> <cmd> --help`). Behavioural findings
   from step 3 land on the same pages even when nothing in the schema moved.

6. **Re-stamp.** Two stamp forms are used on individual claims and examples, and they
   are not interchangeable:

   - **Validated YYYY-MM-DD against herdr X.Y.Z** — the claim was actually exercised
     this pass, per the step-4 rules (a live session for read-only methods, a scratch
     server for mutating ones).
   - **Constructed from schema; not live-validated (YYYY-MM-DD, herdr X.Y.Z)** — the
     claim comes only from `raw/schema.json`'s shapes, including anything marked *not
     probed* in step 4.

   A section may say Validated only for the parts of it actually exercised this pass;
   leave everything else Constructed. Then replace `raw/` with the re-emitted files,
   re-fetch `https://herdr.dev/agent-guide.md` and `https://herdr.dev/llms.txt`, and
   update the page-level stamp line (`herdr X.Y.Z · protocol N · schema_version M ·
   captured YYYY-MM-DD`) in **every** page that was behaviourally re-probed this pass,
   plus `README.md`.

7. **Run the completeness check** — every method in the schema must appear as a `## `
   heading in exactly one reference page:

   ```sh
   jq -r '.schemas.request.oneOf[].properties.method.const' raw/schema.json | sort > /tmp/methods
   grep -rhoE '^## [a-z_.]+' api/ events.md | sed 's/^## //' | sort | uniq > /tmp/documented
   diff /tmp/methods /tmp/documented   # must be empty
   ```
