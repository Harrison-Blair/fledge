---
id: FTHR-078
title: "Dev mode init rails: bare --dev, git hygiene, and version skew reporting"
plumage: PLM-031
status: fledged
priority: P1
depends_on: [FTHR-077]
authored: 2026-07-17T01:59:26Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-078: Dev mode init rails: bare --dev, git hygiene, and version skew reporting

## Description

Completes the init-side surface of dev mode, on top of FTHR-077's linking tracer. Three
behaviors, deliberately in one feather because all three land in `internal/cli/init.go`
and would collide as parallel work (interrogation Q3 = merge):

1. **Bare `--dev`** (FC-3) — inside a fledge source checkout, `--dev` with no path links
   the checkout to its own source, relatively. Outside one, it fails with an actionable
   error.
2. **Git hygiene** (FC-6) — dev-linked paths are recorded in the repo's ignore rules, and
   `--dev` refuses outright if any of those paths is already tracked.
3. **Version skew reporting** (FC-7) — when the source tree's `VERSION` differs from the
   running binary's version, say so, naming both.

Together with FTHR-077 this makes dev mode **shippable**: FTHR-077 alone leaves linked
paths git-visible, which is exactly the "machine-specific junk in a tracked tree" hazard
the plumage exists to avoid. Item 2 closes that.

Satisfies PLM-031 FC-3, FC-6, FC-7.

## Affected Modules

See `.fledge/nest/modules.md` → `internal/cli`, `internal/repo`;
`.fledge/nest/conventions.md` (exit codes, git shelling).

- `internal/cli/init.go` — all three behaviors. Extends the `--dev` flag from FTHR-077
  (which requires a path) to make the path optional, and adds the pre-write checks.
- `internal/repo/repo.go` — `Version(fallback)` (`repo.go:48`) already reads a repo's
  `VERSION` file; reuse it against the source tree rather than re-reading by hand.

Reuse rather than reinvent:
- `ensureGitignore` (`init.go:461`) already appends a block of missing lines to
  `.gitignore` idempotently and is the model (or the direct vehicle) for the ignore write.
- `gitignoreLines` (`init.go:30`) is the existing ignore block; dev paths are a **separate,
  conditional** block — do not fold them into that unconditional list.
- `cli.go:67` already warns on a stamp-vs-binary version mismatch; match its phrasing and
  stderr-note idiom for FC-7 so the CLI has one voice for version skew.
- Git shelling idiom: `exec.Command("git", "-C", root, ...)` as in `scan.go:78` and
  `brood.go:70`.

## Approach

**Bare `--dev` (FC-3).** The flag becomes path-optional. Go's `flag` package cannot express
an optional-value flag directly — `fs.String("dev", "", ...)` makes `--dev` alone consume
the next argument. Use a custom `flag.Value` whose `IsBoolFlag() bool { return true }`
allows both `--dev` and `--dev=<path>`, or register `--dev` as a bool alongside a separate
path mechanism. **Prefer the `IsBoolFlag` route** so `--dev=<path>` and bare `--dev` are
one flag; note this means the space form `--dev <path>` will not bind — if that is
unacceptable, `--dev=<path>` must be documented as the required form. Surface whichever
shape you land on in the usage string (`init.go:23`).

With no path: verify the **current repo** is a fledge source checkout using FTHR-077's
`go.mod` validator against the repo root. If it is not, fail with `ExitUsage` and an error
that says a path is required outside a fledge source checkout. If it is, dev-link the repo
to itself.

**Reject stray positional arguments (the equals-form footgun).** Because `--dev` becomes
an `IsBoolFlag`, the natural-looking `fledge init --dev ~/source/fledge` parses as *bare
`--dev` plus an ignored positional* — and `runInit` currently never inspects `fs.Args()`,
so the path would be **silently discarded**. Inside a fledge source checkout that means
linking to the cwd rather than the named source: a silently wrong target, which is exactly
the "applied incorrectly, burns tokens" failure PLM-031 exists to eliminate. (Outside a
checkout it fails safe on the FC-3 error, but with a message that misleadingly claims no
path was given when the user plainly typed one.)

Therefore: after `fs.Parse`, if `fs.NArg() > 0`, fail with `ExitUsage`. Follow the existing
`fs.NArg()` precedent at `vee.go:38`. When `--dev` was given bare and a stray positional is
present, the error must name the likely cause and the fix — that `--dev` takes its path with
an equals sign, e.g. `--dev=<path>`. This is the one place a generic "unexpected argument"
would waste the user's time.

Note this makes `fledge init` reject positionals generally, not just under `--dev`. That is
a deliberate tightening of an existing silent-ignore; no current fixture passes positionals
to `init`, so it should be invisible to the suite (AC-7 proves it).

Self-links must be **relative** (Q1 = b), unlike FTHR-077's cross-repo absolute links: the
target lives in the same tree, so a relative link survives the tree being moved or cloned.
This matches the links already present in this repo by hand
(`.claude/agents/fledge-brooder.md -> ../../internal/bootstrap/adapters/claude/agents/fledge-brooder.md`).
Compute targets with `filepath.Rel` from each link's parent directory. Note the practical
effect in this repo: `.claude/agents/*` already point where bare `--dev` would point, so
they should end up unchanged — while `.fledge/skills/` (today real copies) becomes linked.

**Git hygiene (FC-6).** Before writing anything, ask git which of the to-be-linked paths
are tracked: `git -C <root> ls-files -- <paths>` and read the hits (simpler to parse than
`--error-unmatch`). If any are tracked, fail with `ExitFail`, list them, and print the
remedy (`git rm --cached <paths>`) — **without modifying anything**, since the whole point
is that a refusal leaves the tree exactly as found. If none are tracked, append the
dev-linked paths to `.gitignore` as their own labeled block.

The `.gitignore` write is the one intended git-visible effect of dev mode (PLM-031 FC-6,
as reworded). Write **path patterns**, not machine-specific locations, so the block is
valid for any clone. Not-a-git-repo is not an error state for fledge generally; degrade
gracefully rather than crashing if git is unavailable.

**Version skew (FC-7).** Compare the source tree's `VERSION` against `binaryVersion`. On
mismatch, print a note to **stderr** naming both versions and stating the consequence
(linked prose may reference CLI behavior this binary lacks). This is a **report, not a
gate** — PLM-031 leaves "should a mismatch ever block?" an explicit Open Question, so do
not add a refusal here. Say nothing when they match (AC-6 pins the silence).

**Ordering constraint.** All checks precede all writes. AC-3/AC-4 both assert that a
refusal leaves the working tree untouched.

## Tests

Extends `cmd/fledge/testdata/dev.txtar` from FTHR-077, or adds
`cmd/fledge/testdata/dev_rails.txtar` if that file is getting long — implementer's call.
Symlink assertions use `exec readlink` + `stdout` (Q4). Run:
`go test ./cmd/fledge -run TestScripts/dev`.

- *bare --dev inside a source checkout self-links relatively* — in a fabricated fledge
  source checkout, `fledge init --dev` (no path) succeeds and `readlink` on a linked
  scaffold path reports a **relative** target resolving into that same tree. Pins FC-3 →
  AC-2.
- *bare --dev outside a source checkout fails* — in a plain repo, `fledge init --dev`
  exits non-zero, and stderr says a path is required. No scaffold file is modified. Pins
  FC-3 → AC-3, PLM-031 AC-2.
- *--dev refuses when a target path is tracked* — `git add` a scaffold path, then
  `fledge init --dev=<src>`: exits non-zero, names the tracked path, prints the
  `git rm --cached` remedy, and leaves the working tree unmodified (`git status --porcelain`
  reports no change; the path is still a regular file, `readlink` fails). Pins FC-6 →
  AC-4, PLM-031 AC-4.
- *--dev writes ignore rules and leaves no git-visible links* — after a successful
  `--dev` in an untracked-scaffold repo, `.gitignore` contains the dev block and
  `git status --porcelain` reports no dev-linked path as a change (only the `.gitignore`
  edit itself). Pins FC-6 → AC-5, PLM-031 AC-5.
- *version skew is reported* — fake source `VERSION` differing from the binary's: stderr
  names both versions; exit code is still success (a report, not a gate). Pins FC-7 →
  AC-6, PLM-031 AC-7.
- *matching versions report nothing* — source `VERSION` equal to the binary's produces no
  skew note. Pins the negative half of FC-7 → AC-6, PLM-031 AC-7.
- *space form is rejected, not silently mis-linked* — inside a fabricated fledge source
  checkout (the dangerous case, where bare `--dev` would otherwise succeed against the
  wrong target), run `fledge init --dev <src>` with a **space**. It exits non-zero, the
  error mentions `--dev=` as the correct form, and **no** scaffold path becomes a link
  (`readlink` fails on each). Pins the footgun → AC-8.
- *stray positional is rejected outside a checkout too* — `fledge init somearg` exits
  non-zero with a usage error. Pins the general tightening → AC-8.
- Existing `init.txtar` and FTHR-077's dev tests must still pass → AC-7.

Test-first order is fixed: write these, observe them FAIL against FTHR-077's code for the
expected reason (bare `--dev` demands a path; no ignore block; no skew note), then
implement until they pass.

## Acceptance Criteria

- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: Bare `fledge init --dev` inside a fledge source checkout links that checkout
      to its own source using relative symlink targets. Satisfies PLM-031 FC-3.
- [x] AC-3: Bare `fledge init --dev` outside a fledge source checkout exits non-zero with
      an error stating a source path is required, and modifies no scaffold file. Satisfies
      PLM-031 FC-3, AC-2.
- [x] AC-4: `--dev` refuses when any to-be-linked path is tracked by git, naming the
      tracked paths and the `git rm --cached` remedy, and leaves the working tree
      unmodified. Satisfies PLM-031 FC-6, AC-4.
- [x] AC-5: After a successful `--dev`, the repo's ignore rules cover every dev-linked
      path and git reports no dev-linked path as a change. Satisfies PLM-031 FC-6, AC-5.
- [x] AC-6: A source/binary `VERSION` mismatch prints a note to stderr naming both
      versions without failing the command; matching versions print no such note.
      Satisfies PLM-031 FC-7, AC-7.
- [x] AC-7: `go test ./...` passes, including `init.txtar` unmodified and FTHR-077's dev
      tests.
- [x] AC-8: `fledge init --dev <path>` written with a space exits non-zero with an error
      naming `--dev=<path>` as the correct form, and creates no links — including inside a
      fledge source checkout, where bare `--dev` would otherwise silently link to the cwd
      instead of the named path. `fledge init` rejects stray positional arguments
      generally. Guards PLM-031 FC-3 against a silently-wrong link target.
