# FTHR-046 verification evidence

Regeneration performed by `fledge-forager-king` (fresh one-shot forager,
Commissioner protocol) against this repo at HEAD `154510f`, binary 0.5.8.
Structural checks below were run first against the **pre-regeneration** nest
(captured before `fledge nest scaffold`) to confirm they fail for the expected
reason, then against the **regenerated** nest to confirm they pass.

## AC-1

The structural checks were observed failing against the pre-regeneration nest
content and passing against the regenerated content.

**Pre-regeneration (FAIL — nest stated stale 18/23, no release-file list):**

```
$ grep -rniE '18 command|full 18-command|18 `internal/cli` command' .fledge/nest/*.md
.fledge/nest/index.md:31:... the full 18-command CLI surface ...
.fledge/nest/conventions.md:20:- **`--json` on every command**: all 18 `internal/cli` commands accept it ...
.fledge/nest/modules.md:72:## `internal/cli` (command dispatch + all 18 command implementations)
.fledge/nest/entry-points.md:37:## The 18 CLI commands ...

$ grep -rniE '23 .*(txtar|fixture)' .fledge/nest/*.md
.fledge/nest/dependencies.md:15:... all 23 `cmd/fledge/testdata/*.txtar` files.
.fledge/nest/testing.md:21:go test ./cmd/fledge -run TestScripts   # all 23 txtar acceptance tests

# conventions.md had no "Versioning & release" section naming the three files.
```

**Post-regeneration (PASS):** see AC-2, AC-3, AC-4, AC-5 below.

## AC-2

`entry-points.md`, `modules.md`, and `index.md` state 19 commands; no stale
"18 command" phrasing remains anywhere in the nest.

```
$ grep -rniE '\b19\b[^0-9]*command|19-command' .fledge/nest/entry-points.md .fledge/nest/modules.md .fledge/nest/index.md
.fledge/nest/modules.md:29:All 19 CLI commands and their handlers. ...
.fledge/nest/index.md:31:... the exact 19-command list ... (verified via `awk` over `commandOrder` in cli.go) ...

$ grep -rniE '\b18\b[^0-9]*command|18-command' .fledge/nest/*.md
(no matches)
```

Ground truth: `commandOrder` in `internal/cli/cli.go` has 19 entries
(init, agents, scan, new, nest, preen, ready, vee, colony, unfledged, status,
set, criteria, brood, abandon, broods, roster, version, update).

## AC-3

`testing.md`, `modules.md`, and `dependencies.md` state 25 txtar fixtures; no
stale "23 txtar" phrasing remains.

```
$ grep -rniE '\b25\b[^0-9]*(txtar|fixture)' .fledge/nest/testing.md .fledge/nest/modules.md .fledge/nest/dependencies.md
.fledge/nest/dependencies.md:17:... all 25 `.txtar` acceptance fixtures ...
.fledge/nest/modules.md:16:## cmd (27 files: ... 25 `testdata/*.txtar`)
.fledge/nest/modules.md:17:... all 25 acceptance-test fixtures (`ls cmd/fledge/testdata/*.txtar | wc -l` = 25) ...
.fledge/nest/testing.md:61:go test ./cmd/fledge -run TestScripts   # all 25 acceptance fixtures

$ grep -rniE '\b23\b[^0-9]*(txtar|fixture)' .fledge/nest/*.md
(no matches)
```

Ground truth: `ls cmd/fledge/testdata/*.txtar | wc -l` = 25.

## AC-4

`conventions.md`'s "Versioning & release" section names all three
must-move-together release files.

```
$ grep -niE 'stamp_warning|version\.go|VERSION|versioning' .fledge/nest/conventions.md
46:## Versioning & release
50:1. **`VERSION`** (repo root) — single-line plain-text source of truth; currently `0.5.8`.
51:2. **`internal/cli/version.go`** — the `binaryVersion` constant ...
52:3. **`cmd/fledge/testdata/stamp_warning.txtar`** — the acceptance fixture pinning ...
```

All three named, all at 0.5.8.

## AC-5

`fledge nest status` reports complete and stamped to HEAD after regeneration.

```
$ fledge nest status
nest complete: all concern docs synthesized, index stamped to HEAD
exit=0

$ git rev-parse HEAD
154510fc963e7071b2f09297ecfeba2b6710e85e
$ grep -m1 'commit:' .fledge/nest/index.md
commit: 154510fc963e7071b2f09297ecfeba2b6710e85e
```
