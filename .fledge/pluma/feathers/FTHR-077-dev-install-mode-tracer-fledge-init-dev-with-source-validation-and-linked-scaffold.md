---
id: FTHR-077
title: "Dev install mode tracer: fledge init --dev with source validation and linked scaffold"
plumage: PLM-031
status: fledged
priority: P1
depends_on: []
authored: 2026-07-17T01:56:17Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-077: Dev install mode tracer: fledge init --dev with source validation and linked scaffold

## Description

The tracer slice for PLM-031: `fledge init --dev <path>` in a consuming repository, end
to end. Parses the flag, validates that `<path>` is a fledge source tree, writes the
copy-type scaffold (agent definitions + core skill documents) as symlinks into that tree
instead of embedded copies, and records the dev source in the scaffold stamp.

This is deliberately thin but real: on completion, an edit saved in the source tree is
visible through the consuming repo's scaffold with no rebuild, no reinstall, and no
refresh — the headline behavior of the plumage (PLM-031 AC-1).

It is the root of the dependency graph because it establishes the **stamp representation
of dev state**, which FTHR-078 (rails), FTHR-079 (`dev status`), FTHR-080 (preen) and
FTHR-081 (refresh) all read. Getting that shape right here is what lets the other four
proceed in parallel.

Out of this feather (each covered elsewhere): bare `--dev`, `.gitignore` handling, and
version-skew reporting (FTHR-078); status reporting (FTHR-079); preen behavior
(FTHR-080); refresh behavior (FTHR-081). Until FTHR-078 lands, `--dev` will leave the
linked paths git-visible — acceptable inside this feather's slice, not shippable without
FTHR-078.

Satisfies PLM-031 FC-1, FC-2, FC-4, FC-5.

## Affected Modules

See `.fledge/nest/modules.md` → `internal/bootstrap`, `internal/cli`; and
`.fledge/nest/architecture.md` (manifest write policies, stamp/drift model).

- `internal/cli/init.go` — flag registration and wiring; passes dev source into `WriteOpts`.
- `internal/bootstrap/registry.go` — the write-policy switch that must render copy-type
  files as symlinks under dev mode. Reuses the existing `makeSymlink` helper
  (`registry.go:489`) and the existing `symlink:` policy path rather than adding a new
  mechanism.
- `internal/bootstrap/bootstrap.go` — `WriteCore` (core skills) must honor dev mode.
- `internal/bootstrap/stamp.go` — `Stamp` gains the dev source; `StampEntry.Target`
  (`stamp.go:34`) already exists and already models a symlink target, so entries need no
  new field.
- `cmd/fledge/testdata/dev.txtar` — new acceptance script.

## Approach

**Source validation (FC-2, FC-4).** Add a validator that reports whether a path is a
fledge source tree by reading `<path>/go.mod` and requiring the module line to be
`github.com/Harrison-Blair/fledge` (interrogation Q2 = go.mod identity). Validate
**before any write** — the failure mode AC-3/PLM-031 AC-3 pins down is that a bad path
leaves the scaffold untouched. Resolve `<path>` to an absolute path once, at validation
time, and thread that value through; symlink targets must not depend on the process's
working directory.

**Policy override (FC-1, FC-5).** Dev mode is a *policy override on copy-type files
only*. In the file-writing switch, when dev mode is active and a file's policy is the
default copy (no `generate`, no `primitive_map`, no `append_if_missing`, no existing
`symlink`), write a symlink to `<devSource>/<src>` instead of the embedded bytes.
`generate`/`primitive_map`/`append_if_missing` files keep their current behavior
untouched — they are rendered or merged and cannot be links (PLM-031 AC-6). The existing
`symlink:` manifest entries (`.claude/skills/*`) already point into `.fledge/skills/`,
which is itself dev-linked; leave them as-is so they resolve transitively.

Carry dev source on `WriteOpts` (alongside `Refresh`) so `WriteCore` and `WriteAdapter`
share one path. Symlinks must be **absolute** here: the consuming repo and the source
tree are unrelated trees, so a relative target has no stable meaning. Replacing an
existing regular file with a link requires removing it first — `os.Symlink` fails on an
existing path; make this idempotent so re-running `--dev` is a no-op rather than an error.

**Stamp (FC-1, and the seam the other feathers read).** Add a dev-source field to
`Stamp`, written when dev mode is active and absent otherwise, so a reader can distinguish
a dev-linked repo from a normal one and recover the source path. Populate each linked
entry's existing `Target` with the absolute link target and set its policy to the symlink
form, so the drift comparison at `drift.go:132` (which already compares symlink targets)
sees what it expects rather than a content hash. **This field is the contract for
FTHR-079/080/081** — name it deliberately.

**Constraint.** Do not alter non-dev init behavior. Every existing txtar fixture must
pass unchanged; `init.txtar` in particular asserts the current copy behavior byte-for-byte.

## Tests

New acceptance script `cmd/fledge/testdata/dev.txtar`, run via
`go test ./cmd/fledge -run TestScripts/dev`. Per interrogation Q4, symlink-ness is
asserted with `exec readlink <path>` + `stdout <target>` — no harness change, and note
**no existing txtar asserts symlink-ness at all** (`exists` follows links), so this is new
ground. Each script fabricates a minimal fake source tree (a `go.mod` with the fledge
module line, plus `internal/bootstrap/core/skills/...` and
`internal/bootstrap/adapters/claude/agents/...` files with known content).

- `dev.txtar` / *links copy-type scaffold to source* — after `fledge init --dev <src>`,
  `readlink .claude/agents/fledge-brooder.md` reports `<src>/internal/bootstrap/adapters/claude/agents/fledge-brooder.md`,
  and the same for a core skill document under `.fledge/skills/`. Pins FC-1, FC-5 → AC-2.
- `dev.txtar` / *source edits are live* — write new content into the fake source file,
  then read it back through the consuming repo's scaffold path and see the new content,
  with no intervening `fledge` command. Pins the headline behavior → AC-3, PLM-031 AC-1.
- `dev.txtar` / *rendered files are not links* — `.claude/fledge-adapter.md` and
  `.claude/settings.local.json` exist, contain rendered content, and `readlink` fails on
  them. Pins FC-5 → AC-4, PLM-031 AC-6.
- `dev.txtar` / *invalid source is rejected before any write* — `fledge init --dev <tmp>`
  where `<tmp>` has no fledge `go.mod` exits non-zero, prints an error naming the path,
  and every scaffold file is byte-identical to before (compare against a pre-run copy).
  Pins FC-4 → AC-5, PLM-031 AC-3.
- `dev.txtar` / *dev state recorded in stamp* — `.fledge/scaffold.json` contains the dev
  source path and the linked entries carry their symlink targets. Pins the FTHR-079/080/081
  contract → AC-6.
- `dev.txtar` / *re-running --dev is idempotent* — a second identical invocation succeeds
  and leaves the links unchanged. Pins AC-7.
- Existing `cmd/fledge/testdata/init.txtar` must pass **unmodified**, proving non-dev init
  is untouched → AC-8.

Test-first order is fixed: write these, run them against unchanged code and observe them
FAIL for the expected reason (unknown flag `--dev`; `readlink` on a regular file), then
implement until they pass.

## Acceptance Criteria

- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `fledge init --dev <path>` writes the agent definitions and core skill
      documents as symlinks into `<path>`, and `readlink` reports the expected absolute
      target for each. Satisfies PLM-031 FC-1, FC-5.
- [x] AC-3: An edit saved in the source tree is readable through the consuming repo's
      scaffold path with no rebuild, reinstall, or `fledge` command in between. Satisfies
      PLM-031 FC-1 and demonstrates PLM-031 AC-1.
- [x] AC-4: Rendered and merged files (`.claude/fledge-adapter.md`,
      `.claude/settings.local.json`, the `CLAUDE.md` line) are produced as they are today
      and are not symlinks. Satisfies PLM-031 FC-5, AC-6.
- [x] AC-5: `--dev` against a path whose `go.mod` does not declare the fledge module exits
      non-zero with an error naming the path, and no scaffold file is modified. Satisfies
      PLM-031 FC-2, FC-4, AC-3.
- [x] AC-6: The scaffold stamp records the absolute dev source path, and each linked entry
      records its symlink target rather than a content hash.
- [x] AC-7: Re-running the same `fledge init --dev <path>` succeeds and leaves every dev
      link unchanged.
- [x] AC-8: `go test ./...` passes, with `cmd/fledge/testdata/init.txtar` unmodified —
      non-dev init behavior is unchanged.
