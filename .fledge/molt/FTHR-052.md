# FTHR-052 — evidence

Rewrite implementation.md §6 recovery step to use `fledge broods --stale`.

## AC-1
Test observed FAILING before implementation, PASSING after; captured verbatim.

Test: `cmd/fledge/testdata/init.txtar` — new assertions (§6 recovery-step
rewrite) added after the FTHR-055 roster block. Run against the CURRENT (old,
hand-correlation) scaffolded `implementation.md`:

```
$ go test ./cmd/fledge -run 'TestScripts/init$'
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/init (0.01s)
            FAIL: testdata/init.txtar:113: no match for `fledge broods --stale` found in .fledge/skills/fledge-orchestrate/implementation.md
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.014s
FAIL
```

The old §6 step 2 hand-correlates `git worktree list` against `fledge broods`
and contains no `fledge broods --stale`, `worktree_exists: true`, or
empty-path re-check caveat — so the first new assertion fails as expected.

(Passing post-implementation run recorded under AC-3 below, since it is the
same `go test ./cmd/fledge -run TestScripts` invocation.)

## AC-2
§6 recovery step instructs `fledge broods --stale` for classification and
states the legacy-empty-path re-check caveat (satisfies PLM-025 FC-4, AC-4).

Rewrote §6 step 2 of the embedded source
`internal/bootstrap/core/skills/fledge-orchestrate/implementation.md`.
FTHR-055's step 1 (`fledge roster` resume reconstruction) is untouched — only
the manual stale-lock/worktree classification changed.

Scaffolded §6 step 2 after `fledge init --refresh`:

```
$ grep -n 'fledge broods --stale\|worktree_exists: true\|predates FTHR-050\|before force-releasing\|has no surviving worktree' .fledge/skills/fledge-orchestrate/implementation.md
131:2. Inventory reality: `fledge broods --stale` classifies the held locks for you (it reports the per-lock `worktree` path and `worktree_exists`), plus `fledge broods` (owner, branch, pid-alive per held lock) and `fledge vee` for the fuller picture. Feathers with a held lock (equivalently `status: hatching`) and `worktree_exists: true` are the resume set. The locks `--stale` reports are the release candidates — release each with `fledge abandon FTHR-### --force`, then set its status explicitly (`fledge status FTHR-### pipping --force`) so it re-enters the ready set. Caveat for legacy records: a `--stale` entry whose `worktree` field is empty (a lock brooded before this repo adopted `--worktree`, i.e. predates FTHR-050) was classified stale by convention — an empty path, not a path checked and found gone — so re-check it against `git worktree list` before force-releasing; if that feather's worktree is in fact still present, treat it as a resume, not a release.
```

- Classifies via `fledge broods --stale` (no hand-correlation).
- Resume set = held lock + `worktree_exists: true`.
- Legacy empty-`worktree`-path caveat: re-check against `git worktree list`
  before force-releasing (predates FTHR-050).
- Old "…has no surviving worktree are stale" hand-correlation phrasing removed
  (the `! grep` assertion confirms).

`git worktree list` is retained only as the tool for the manual re-check case.

## AC-3
`fledge init --refresh` regenerated the scaffolded copy; full `cmd/fledge`
script suite (and full `go test ./...`) passes.

```
$ go install ./cmd/fledge && hash -r && fledge init --refresh && fledge preen
scaffolded agents: claude
spec clean: 26 plumages, 56 feathers     # preen exit 0 → scaffold healthy

$ go test ./cmd/fledge -run TestScripts
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.098s

$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap
...  (all packages ok)

$ gofmt -l .            # (no output)
$ go vet ./...          # (clean)
```
