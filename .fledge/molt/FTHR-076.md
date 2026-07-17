# FTHR-076 evidence: Refresh scaffold and verify suite green

Verification-only feather (no source delta of its own). Run inline by the
orchestrator on `main` rather than in a worktree pair: FTHR-076 implements no
code, and its integration — rebuild + `fledge init --refresh` — had already
been performed during the run as routine dogfooding maintenance after
FTHR-089/090 and again after FTHR-075 merged. The checks below re-run the full
verification from a clean main and capture the result.

## AC-1
`fledge version` matches the `VERSION` file after reinstall.

```
$ cat VERSION
0.6.7
$ fledge version
fledge 0.6.7
```

## AC-2
`fledge init --refresh` is byte-idempotent and touches no tracked scaffold.

**Reconciled against the dev-link migration.** FTHR-076 was authored
(`fledge_version: 0.5.8`) when this repo *tracked* its scaffold, so its AC-2
anticipated a diff in the scaffold stamp plus the seven regenerated skill docs.
Commit `1f5224d` ("Untrack fledge-owned scaffold; dev-link it into
internal/bootstrap") changed that: the seven `.fledge/skills/fledge-orchestrate/*.md`
copies are now symlinks into `internal/bootstrap/core/skills/...` (so a refresh
writes the verbatim-copied core content straight back through the symlink onto
its own source — byte-identical, no diff), and `.fledge/scaffold.json` is
gitignored (`.gitignore:40`). The correct post-refresh observation in this repo
is therefore a **clean tracked git status**, not a set of tracked-file changes.
That is what "only the expected file set touched" now means here — nothing
unexpected, and nothing tracked drifts.

```
$ fledge init --refresh --force   # (settings.local.json backed up and restored around it — gitignored, not git-recoverable)
$ git status --short              # excluding the user-edited settings.local.json
(clean — no tracked scaffold drift)
```

## AC-3
`fledge preen` passes (exit 0). Two pre-existing warnings, no errors:
`.fledge/molt/FTHR-061.md`'s non-bare AC headings and the user-edited
`.claude/settings.local.json` — both predate this feather and this plumage.

```
$ fledge preen; echo $?
WARN  ...FTHR-061...: checked criteria missing evidence sections (heading form)
WARN  .claude/settings.local.json: scaffold file is user-edited
2 warning(s)
0
```

## AC-4
`go vet ./...` clean; `go test ./... -race` green (satisfies PLM-030 AC-6).
The race detector covers `internal/ledger`'s concurrent-write tests (FTHR-072).

```
$ go vet ./...          # clean
$ go test -race ./...; echo $?
ok  ... (all packages)
0
```

## AC-5
`go test ./cmd/fledge -run TestScripts` passes in full, including the ledger
txtar fixtures `heartbeat.txtar`, `await.txtar`, `verdict.txtar`,
`escalate.txtar`, `ledger-read.txtar`, and `pulse.txtar` (satisfies PLM-030
AC-2/AC-3/AC-5/AC-6).

```
$ go test ./cmd/fledge -run TestScripts; echo $?
ok  github.com/Harrison-Blair/fledge/cmd/fledge
0
```

## AC-6
All five PLM-030 commands resolve their usage on the freshly reinstalled
binary (`--help` prints usage and exits with the usage convention — resolution,
not a missing command). Verified alongside `fledge pulse` (FTHR-092):

```
$ fledge heartbeat --help    → Usage of heartbeat: -expect/-note/-json  (exit 2)
$ fledge await --help        → Usage of await                            (exit 2)
$ fledge verdict --help      → Usage of verdict                          (exit 2)
$ fledge escalate --help     → Usage of escalate                         (exit 2)
$ fledge ledger read --help  → Usage of ledger read: -json               (exit 0)
```
