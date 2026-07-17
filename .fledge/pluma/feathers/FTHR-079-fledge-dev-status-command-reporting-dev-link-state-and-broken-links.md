---
id: FTHR-079
title: fledge dev status command reporting dev-link state and broken links
plumage: PLM-031
status: fledged
priority: P1
depends_on: [FTHR-077]
authored: 2026-07-17T02:04:37Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-079: fledge dev status command reporting dev-link state and broken links

## Description

The observability half of PLM-031. Adds `fledge dev status`: a deterministic query that
answers, at a glance, "is this repo dev-linked, where does it point, and is any link
broken?"

This exists because the plumage's motivating pain is **uncertainty**, not keystrokes — a
propagation step that may have silently not applied is what costs tokens. A dev-linked
repo whose source moved away has links that resolve to nothing, and today nothing would
tell you until an agent read an empty file. This command is the thing you run to trust the
state instead of inspecting files by hand.

Interrogation Q4 chose a dedicated command over folding this into `preen` (`preen` is
separately made dev-aware by FTHR-080, but only so it does not false-positive — it is not
where dev state is *reported*).

Parallel-safe with FTHR-078 and FTHR-080: this feather's logic lives in a new file, and
its only shared touchpoint is one line in `commandOrder`.

Satisfies PLM-031 FC-8, FC-9.

## Affected Modules

See `.fledge/nest/modules.md` → `internal/cli`, `internal/bootstrap`;
`.fledge/nest/entry-points.md` (CLI command surface);
`.fledge/nest/conventions.md` (command-registry pattern, `--json` on every command).

- `internal/cli/dev.go` — **new file**. `register("dev", runDev, ...)` in an `init()`, with
  a `status` verb. Follow the verb-dispatch shape of `internal/cli/nest.go:16-39`
  (`runNest` → `runNestStatus`) rather than inventing a new one; `dev` is a noun-command
  with subverbs exactly as `nest` is.
- `internal/cli/cli.go` — add `"dev"` to `commandOrder` (`cli.go:105`).
- `internal/bootstrap/stamp.go` — read-only consumer of the dev-source field and per-entry
  `Target` that FTHR-077 defines. **Do not redefine that shape here**; if it proves
  insufficient, raise it rather than duplicating state.
- `cmd/fledge/testdata/dev_status.txtar` — new acceptance script.

## Approach

**Source of truth.** Read `.fledge/scaffold.json` via the existing `LoadStamp` and report
from it. Do not re-derive dev state by walking the tree and guessing from what happens to
be a symlink — the stamp is the record FTHR-077 writes, and a link fledge did not create is
not dev state. `LoadStamp` returns `(nil, nil)` when absent (`stamp.go:42`); a repo with no
stamp is "not dev-linked", not an error.

**Reported facts (FC-8).** Whether the repo is dev-linked; the absolute source path; the
count of linked files. Not dev-linked is a normal, successful answer — say so plainly and
exit `ExitOK`. A missing stamp and a present-but-non-dev stamp both mean "not dev-linked".

**Broken links (FC-9).** For each entry the stamp records as dev-linked, resolve the link
and report any whose target no longer exists, naming each path. Use `os.Stat` on the link
path (which follows the link) and treat `IsNotExist` as broken — this catches both a
deleted target and a moved source tree, which is the realistic failure (the source
directory gets renamed and every link dies at once). Distinguish "link is gone / is now a
regular file" from "link exists but dangles" if cheap; the former means something
overwrote a dev link, which is worth saying differently.

**Exit code.** Broken links are a **finding, not a crash**: report them and exit non-zero
so the command is usable as a check, mirroring how `preen` signals findings. Report all
broken links, not just the first — a moved source breaks every link, and listing one at a
time would be maddening.

**Output (Q6 = a).** Human-readable by default, `--json` for machines, matching every other
command (`.fledge/nest/conventions.md`). The JSON shape should carry at least: linked
(bool), source (string), file count (int), and the broken paths (list). Follow the
`emitJSON` helper (`cli.go:112`) and the `preen`/`nest status` JSON-struct idiom.

**Note on `commandOrder`.** It feeds the generated permission allow-list template
(`adapters/claude/settings.local.json` renders `Bash(fledge <cmd> *)` from
`.CommandOrder` — `registry.go:203`). Adding `"dev"` therefore changes generated scaffold
output. Verified: **no txtar fixture asserts on allow-list content**, so no fixture should
need updating — but the allow-list only regenerates under `fledge init --refresh`, so a
pre-existing repo will not permit `Bash(fledge dev *)` until refreshed. Do not add a
bespoke allow-list entry to compensate; the refresh path is the intended mechanism.

## Tests

New acceptance script `cmd/fledge/testdata/dev_status.txtar`, run via
`go test ./cmd/fledge -run TestScripts/dev_status`. Fabricate a fake fledge source tree
and `fledge init --dev=<src>` (FTHR-077's behavior) to reach a dev-linked state.

- *not dev-linked reports plainly* — after a normal `fledge init`, `fledge dev status`
  exits zero and says the repo is not dev-linked. Pins the negative case → AC-2.
- *no stamp at all is not an error* — in a repo with no `.fledge/scaffold.json`,
  `fledge dev status` exits zero and reports not dev-linked. Pins the `LoadStamp` nil path
  → AC-2.
- *dev-linked reports source and count* — after `fledge init --dev=<src>`,
  `fledge dev status` exits zero, names `<src>`, and reports the linked-file count. Pins
  FC-8 → AC-3, PLM-031 AC-8.
- *broken link is detected and named* — after `--dev=<src>`, delete a file from the fake
  source, then `fledge dev status`: exits non-zero and names the broken path. Pins FC-9 →
  AC-4, PLM-031 AC-9.
- *a moved source reports every broken link* — rename the whole fake source directory,
  then `fledge dev status`: names more than one broken path, not just the first. Pins the
  realistic failure → AC-4.
- *--json emits the documented shape* — `fledge dev status --json` on a dev-linked repo
  produces JSON carrying linked/source/count/broken. Pins Q6 → AC-5.
- `go test ./...` stays green, `init.txtar` unmodified → AC-6.

Test-first order is fixed: write these, observe them FAIL against FTHR-077's code for the
expected reason (`unknown command "dev"`), then implement until they pass.

## Acceptance Criteria

- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `fledge dev status` in a repo that is not dev-linked — including one with no
      scaffold stamp at all — exits zero and reports that plainly.
- [x] AC-3: `fledge dev status` in a dev-linked repo reports the absolute source path and
      the number of linked files. Satisfies PLM-031 FC-8, AC-8.
- [x] AC-4: With a dev link's target removed or the source tree moved, `fledge dev status`
      exits non-zero and names every broken link, not merely the first. Satisfies PLM-031
      FC-9, AC-9.
- [x] AC-5: `fledge dev status --json` emits machine-readable output carrying at least
      linked, source, file count, and broken paths, per the CLI's `--json` convention.
- [x] AC-6: `go test ./...` passes with `init.txtar` unmodified; `"dev"` appears in
      `commandOrder` and in `fledge` usage output.
