---
id: FTHR-081
title: Refresh preserves dev links instead of resetting them to copies
plumage: PLM-031
status: egg
priority: P1
depends_on: [FTHR-077, FTHR-078]
authored: 2026-07-17T02:11:04Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-081: Refresh preserves dev links instead of resetting them to copies

## Description

Makes `fledge init --refresh` preserve dev mode instead of destroying it.

This closes the loop that would otherwise make PLM-031 self-defeating. Dev mode cannot
cover the rendered files (`.claude/fledge-adapter.md`, `.claude/settings.local.json`) or
the appended `CLAUDE.md` line, so a developer **still has to run `--refresh`** in a
dev-linked repo whenever those templates change. But refresh is a reset-to-shipped: as it
stands it would overwrite every dev symlink with an embedded copy, silently un-dev-ing the
repo. The developer would then be editing source that no longer reaches their agents, and
would discover it only after tokens had been spent on stale behavior — which is precisely
the failure this plumage exists to eliminate, reintroduced by the plumage itself.

The user resolved this during feather interrogation (Q1=a) after declining to front-load
it at the plumage gate: refresh must preserve dev links, and PLM-031 gained FC-11 to say so.

**Depends on FTHR-078, not just FTHR-077.** Both this feather and FTHR-078 edit
`internal/cli/init.go`; sequencing them avoids a merge conflict on a shared file rather
than a true ordering need (`.fledge/skills/fledge-orchestrate/planning.md` §4.3). It is the
last feather in the plumage.

Satisfies PLM-031 FC-11.

## Affected Modules

See `.fledge/nest/modules.md` → `internal/cli`, `internal/bootstrap`;
`.fledge/nest/architecture.md` (refresh as reset-to-shipped, prune pass, the stamp).

- `internal/cli/init.go` — `runInit`'s refresh path: recover dev state from the old stamp,
  and carry it into the new stamp.
- `internal/bootstrap/stamp.go` — read/write of the dev-source field FTHR-077 defines.
  Consumer only; do not reshape it here.
- `cmd/fledge/testdata/dev_refresh.txtar` — new acceptance script.

## Approach

**Recover dev state from the stamp, not from flags.** The realistic invocation is a bare
`fledge init --refresh` (the developer is refreshing rendered files; they are not thinking
about dev mode). `runInit` already loads the old stamp up front for exactly this class of
need — `oldStamp, _ := bootstrap.LoadStamp(r.Root)` (`init.go:67`), with the comment
"refresh detection and the stamp's agents union need it". Read the dev source from
`oldStamp` and apply dev mode for the run when present.

**Carry the dev source forward into the new stamp.** This is the crux, and it has a
precedent to copy: the agents field is already unioned across runs (`init.go:206-220`,
"agents = this run's adapters ∪ old stamp's agents") specifically so a later run cannot
silently narrow earlier state. Dev source needs the same treatment — if `--refresh` writes
a stamp without it, the repo is no longer dev-linked as far as `dev status`, `preen`, and
the *next* refresh are concerned, and dev mode evaporates one refresh later even if the
links on disk survived. Precedence: an explicit `--dev=<path>` on this run wins (it
re-points the repo); otherwise inherit the old stamp's dev source.

**Validate the recovered source before writing** (FC-4's ethos). Reuse FTHR-077's `go.mod`
validator against the recovered path. If the recorded source no longer validates — the
realistic case being a moved or deleted `~/source/fledge` — **fail with `ExitFail`**,
naming the recorded path and the fact that it is no longer a fledge source tree, and
suggesting `--dev=<path>` to re-point. Do **not** silently fall back to writing copies:
that is the exact silent-un-dev this feather exists to prevent, and it would be
indistinguishable from the feature working.

> **This failure behavior is a user decision, made explicitly during feather
> interrogation.** The alternative (warn and fall back to copies) was put to the user
> alongside it, with the downside stated: because PLM-031 has no documented way to leave
> dev mode, a moved source means refresh errors until the developer re-points it with
> `--dev=<path>`. They chose fail-loudly with that cost in view. Do not soften it in code.
> There is deliberately no off-switch to name in the error message — state the problem and
> the re-point remedy only. See PLM-031's Open Questions: a sanctioned exit from dev mode
> is a known gap and the likely next plumage.

**Rendered files still refresh (FC-11, AC-12).** Dev mode is a policy override on
copy-type files only; `generate`/`primitive_map`/`append_if_missing` files must refresh
exactly as they do today. This is the entire reason a developer runs `--refresh` in a
dev-linked repo, so AC-4 pins it explicitly. The prune pass (`init.go:245-271`) must also
keep working: an obsolete dev-linked path should still be removed, and `PruneObsolete`
already understands symlink entries via the stamp's `Target`.

**Interaction with the edited-files confirmation.** `EditedOnRefresh` (`init.go:105`)
drives the "refresh will overwrite N user-edited file(s)" prompt. **FTHR-080** stops
dev-linked paths from being classified as modified, so they should not appear there — this
feather must not re-introduce them. If FTHR-080 has not merged when this is implemented,
expect that prompt to fire spuriously in tests; that is FTHR-080's bug, not this one's.

**Idempotence.** Re-running `--refresh` in a dev-linked repo must leave links pointing at
the same targets. Relinking an existing correct symlink should be a no-op, matching the
byte-idempotent spirit of `writeIfChanged` (`registry.go:504`) that the txtar fixtures
depend on.

## Tests

New acceptance script `cmd/fledge/testdata/dev_refresh.txtar`, run via
`go test ./cmd/fledge -run TestScripts/dev_refresh`. Symlink assertions use
`exec readlink` + `stdout` (Q4). Reach a dev-linked state with `fledge init --dev=<src>`
against a fabricated fledge source tree, then refresh.

- *bare --refresh preserves dev links* — `fledge init --refresh` (no `--dev`) in a
  dev-linked repo: every dev-linked path is still a symlink to the same target afterwards.
  The core regression → AC-2, PLM-031 AC-11.
- *source edits are still live after a refresh* — refresh, then save new content in the
  fake source, then read it through the repo's scaffold path and see the new content.
  Proves the links are functional, not merely present → AC-2, PLM-031 AC-11.
- *dev source survives in the new stamp* — after `--refresh`, `.fledge/scaffold.json` still
  records the dev source, and `fledge dev status` still reports the repo dev-linked. Guards
  the evaporate-one-refresh-later failure → AC-3.
- *rendered files still refresh under dev mode* — change what a generated file would render
  to (e.g. via the adapter's inputs), refresh, and observe the rendered file updated while
  links are untouched → AC-4, PLM-031 AC-12.
- *refresh with a vanished dev source fails loudly* — move the fake source tree away, then
  `fledge init --refresh`: exits non-zero, names the recorded source path, and **does not**
  replace links with copies (the paths are either still links or untouched — never
  silently reverted) → AC-5.
- *explicit --dev on a refresh re-points* — `fledge init --refresh --dev=<other-src>` in an
  already-dev-linked repo re-links to `<other-src>` and records it in the stamp → AC-6.
- *refresh is idempotent under dev mode* — two successive `--refresh` runs leave identical
  link targets → AC-7.
- `go test ./...` green, `init.txtar` unmodified (non-dev refresh unchanged) → AC-8.

Test-first order is fixed: write these, observe them FAIL against FTHR-078's code for the
expected reason (refresh replaces links with copies; the new stamp drops the dev source),
then implement until they pass.

## Acceptance Criteria

- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `fledge init --refresh` with no `--dev` flag in a dev-linked repository leaves
      every dev-linked path a symlink to the same source, and an edit saved in the source
      afterwards is still visible through the repository's scaffold. Satisfies PLM-031
      FC-11, AC-11.
- [x] AC-3: After a refresh, the scaffold stamp still records the dev source and
      `fledge dev status` still reports the repository as dev-linked.
- [x] AC-4: A refresh of a dev-linked repository still updates the files dev mode does not
      cover (rendered and appended files). Satisfies PLM-031 FC-11, AC-12.
- [x] AC-5: A refresh whose recorded dev source no longer validates exits non-zero naming
      that path, and does not silently replace dev links with copies.
- [x] AC-6: `fledge init --refresh --dev=<path>` re-points an already-dev-linked repository
      at `<path>` and records it in the stamp.
- [x] AC-7: Two successive refreshes of a dev-linked repository leave identical link
      targets.
- [x] AC-8: `go test ./...` passes with `init.txtar` unmodified — non-dev refresh behavior
      is unchanged.
