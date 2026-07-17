---
id: FTHR-080
title: "Preen dev-awareness: no false drift on dev-linked scaffold"
plumage: PLM-031
status: fledged
priority: P1
depends_on: [FTHR-077]
authored: 2026-07-17T02:07:05Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-080: Preen dev-awareness: no false drift on dev-linked scaffold

## Description

Makes the scaffold health check correct in a dev-linked repository.

Without this, dev mode breaks `preen`. The drift check compares each expected file's
on-disk content hash against the **embedded shipped bytes**. A dev-linked path is a symlink
into a live source tree, so reading it returns whatever the developer has saved — which by
design differs from the shipped bytes. Every dev-linked file would therefore report
`modified` / "user-edited", i.e. `preen` would emit ~18 false warnings in exactly the repos
dev mode is meant to serve, and its real findings would drown in them.

This is a **consequence** of dev mode rather than a feature request — it was added as FC-10
during plumage interrogation, and the user explicitly declined the option to cut it and let
preen scream.

Scope is strictly "do not lie": `preen` is **not** where dev state is reported (that is
`fledge dev status`, FTHR-079, per interrogation Q4=d). This feather adds no dev reporting
to preen — it only stops the false positives.

The false positive is observable **today**, before any of this ships: this repo's
hand-made agent symlinks already cause
`preen` → `.claude/agents/fledge-brooder.md: scaffold file is user-edited`. That is the
same bug, arrived at by hand.

Parallel-safe with FTHR-078 (`internal/cli/init.go`) and FTHR-079 (`internal/cli/dev.go`):
this feather's change is contained to `internal/bootstrap/drift.go`.

Satisfies PLM-031 FC-10.

## Affected Modules

See `.fledge/nest/modules.md` → `internal/bootstrap`, `internal/check`;
`.fledge/nest/architecture.md` (scaffold drift detection);
`.fledge/nest/data-model.md` (`Stamp`, `StampEntry`, `Drift`).

- `internal/bootstrap/drift.go` — the whole change. `DriftReport` (`drift.go:49`) and
  `EditedOnRefresh` (`drift.go:99`).
- `internal/cli/preen.go` — **read only**; `scaffoldDrift` (`preen.go:91`) builds `expected`
  from the manifest and calls `DriftReport`. Expected to need **no change** (see Approach).

## Approach

**Root cause.** `scaffoldDrift` builds `expected` from `ExpectedFiles(m, commandOrder)` —
purely manifest-derived, so a dev-linked agent file still carries the shipped `Sha256` with
an empty `Target`. `DriftReport`'s switch (`drift.go:80-87`) dispatches on
`exp.Target != ""`, so it takes the `classifyContent` branch, reads *through* the symlink,
hashes live source content, matches neither `exp.Sha256` nor `stamp.Sha256` (empty under
dev), and returns `StatusModified` (`drift.go:156`).

**The fix, and where it must live.** `DriftReport` **already receives the stamp**
(`drift.go:49`), and FTHR-077 records dev state there: the dev source plus, per linked
entry, `Policy: symlink` and a `Target`. So dev-awareness needs no new parameter and no
call-site change: inside `DriftReport`, when the stamp marks the repo dev-linked and a
stamp entry for the path carries a symlink `Target`, **prefer the stamp's symlink
expectation over the manifest's content expectation** for that path, and route it to
`classifySymlink` (`drift.go:162`).

Do it this way deliberately. The alternative — threading a dev source into `ExpectedFiles`
— would change its signature and force an edit to `init.go`, which **FTHR-078 is editing in
parallel**; that would manufacture a merge conflict for no benefit. Keeping the change
inside `drift.go` is what preserves the parallel fan-out.

`classifySymlink` then does the right thing unmodified: link target matches the stamp's
`Target` → `StatusUpToDate`. It also already handles the two failure modes worth keeping:
a **missing** target (`drift.go:165`) and a **regular file where a link is expected**
(`drift.go:169` → `modified`, i.e. something clobbered a dev link — a true finding, not a
false one). Preserve both; do not blanket-suppress findings for dev-linked paths. The goal
is "no *false* drift", not "no drift".

**`EditedOnRefresh` follows for free** (`drift.go:99`) — it derives from `DriftReport`, so
once dev-linked files stop reporting `StatusModified` they stop being listed as
user-edited files a refresh would overwrite. That matters beyond preen: today it would make
`fledge init --refresh` prompt "refresh will overwrite 18 user-edited file(s)" in a
dev-linked repo. This feather removes the false prompt; **FTHR-081** separately makes
refresh actually preserve the links. Neither substitutes for the other.

**Idempotence (FC-10, second half).** `preen` is read-only, so this should hold by
construction — but AC-4 pins it, because "the health check does not alter the dev links" is
a promise the plumage makes and a regression here would be silent.

**Constraint.** A non-dev repo's drift output must not change at all. `preen`'s existing
behavior is load-bearing for other fixtures; the new branch must be reachable only when the
stamp says dev-linked.

## Tests

Unit tests in `internal/bootstrap/drift_test.go` (the natural home — this is a
`DriftReport` classification change, and the package already has focused unit tests per
`.fledge/nest/testing.md`), plus one acceptance-level check in
`cmd/fledge/testdata/dev_preen.txtar` proving the user-visible result.

Run: `go test ./internal/bootstrap -run TestDrift` and
`go test ./cmd/fledge -run TestScripts/dev_preen`.

- *dev-linked file whose source differs from shipped is up-to-date* — stamp marks the repo
  dev-linked with a symlink `Target`; the link resolves to content deliberately unlike the
  embedded bytes. `DriftReport` reports `StatusUpToDate`, **not** `StatusModified`. This is
  the core regression → AC-2.
- *dev-linked file with a dangling target reports missing* — target deleted →
  `StatusMissing`. Proves findings are not blanket-suppressed → AC-3.
- *a regular file where a dev link is expected reports modified* — something overwrote the
  link; still a true finding → AC-3.
- *non-dev repo drift is unchanged* — with no dev source in the stamp, classification is
  byte-for-byte what it is today, including a genuinely user-edited file still reporting
  `modified` → AC-5.
- *EditedOnRefresh omits dev-linked files* — a dev-linked repo yields no dev-linked path in
  `EditedOnRefresh`, so refresh raises no "will overwrite user-edited file(s)" prompt for
  them → AC-6.
- `dev_preen.txtar` / *preen is clean and idempotent on a dev-linked repo* — after
  `fledge init --dev=<src>` and an edit saved in the source, `fledge preen` reports no
  finding for any dev-linked path; running it twice gives identical output and every link
  still resolves to the same target afterwards → AC-2, AC-4, PLM-031 AC-10.

Test-first order is fixed: write these, observe them FAIL against FTHR-077's code for the
expected reason (dev-linked files classify as `modified`), then implement until they pass.

## Acceptance Criteria

- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: In a dev-linked repository whose source content differs from the shipped bytes,
      no dev-linked path is reported as modified, drifted, or user-edited. Satisfies
      PLM-031 FC-10, AC-10.
- [x] AC-3: Genuine problems with dev-linked paths are still reported — a dangling target
      reports missing, and a regular file where a link is expected reports modified.
      Findings are not blanket-suppressed for dev-linked paths.
- [x] AC-4: Running `fledge preen` twice in succession on a dev-linked repository produces
      identical output, and every dev link still resolves to the same target afterwards.
      Satisfies PLM-031 FC-10, AC-10.
- [x] AC-5: Drift classification in a non-dev repository is unchanged, including a
      genuinely user-edited scaffold file still reporting as modified.
- [x] AC-6: `EditedOnRefresh` does not list dev-linked paths, so a refresh raises no
      "will overwrite user-edited file(s)" prompt for them.
- [x] AC-7: `go test ./...` passes with existing fixtures unmodified.
