# Re-syncing these docs after a herdr update

> Docs describe herdr 0.8.2 · protocol 20 · schema_version 1 · captured 2026-08-19

herdr self-updates (`herdr update`, channel via `herdr channel set <stable|preview>`), so
the installed binary will drift ahead of these docs. The raw artifacts in `raw/` are the
diff baseline — the procedure is: re-emit, diff, update only what changed, re-stamp.

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

3. **Update the affected pages.** Added/removed/changed methods map to their namespace
   file in `api/` (see the README table); event changes go to `events.md`; envelope or
   `$defs` changes to `protocol.md` / `data-model.md`; new CLI subcommands to
   `cli-mapping.md` (re-sweep with `herdr <group> <cmd> --help`).

4. **Re-validate changed examples.** Read-only methods may be probed against the live
   session. Mutating methods must only ever be probed against an isolated scratch server:

   ```sh
   herdr --session docs-scratch server &            # isolated; socket at
   export HERDR_SOCKET_PATH=~/.config/herdr/sessions/docs-scratch/herdr.sock
   # ... probe ...
   herdr server stop                                # stops only the scratch server
   ```

   Never send mutating calls to the socket of a session you did not create.

5. **Replace `raw/` with the re-emitted files, re-fetch** `https://herdr.dev/agent-guide.md`
   and `https://herdr.dev/llms.txt`, and **update the stamp line**
   (`herdr X.Y.Z · protocol N · schema_version M · captured YYYY-MM-DD`) in **every**
   changed file plus `README.md`.

6. **Run the completeness check** — every method in the schema must appear as a `## `
   heading in exactly one reference page:

   ```sh
   jq -r '.schemas.request.oneOf[].properties.method.const' raw/schema.json | sort > /tmp/methods
   grep -rhoE '^## [a-z_.]+' api/ events.md | sed 's/^## //' | sort | uniq > /tmp/documented
   diff /tmp/methods /tmp/documented   # must be empty
   ```
