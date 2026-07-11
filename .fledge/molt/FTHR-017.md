# FTHR-017 evidence

## AC-1

Tests were updated first, then run against the **unchanged** `internal/repo/repo.go`
and `internal/cli/init.go` (before any implementation change). Two categories:

1. `internal/spec/load_test.go`, `internal/spec/frontmatter_test.go`,
   `internal/check/check_test.go` — path-derived fixtures/assertions updated
   from `pluma/plumage`/`pluma/feathers` to `.fledge/pluma/plumage`/
   `.fledge/pluma/feathers`. These tests are self-contained (they build their
   own fixture paths and don't call `repo.Repo`), so they do not exercise
   `RequirementsDir()`/`TasksDir()` and pass regardless of the repo.go change.
2. A new test, `internal/repo/repo_test.go` (`TestRequirementsAndTasksDir`),
   was added — this is the actual FC-1 pinning test since no repo_test.go
   existed. It asserts `Repo.RequirementsDir()`/`TasksDir()` return paths
   under `.fledge/pluma/...`.

Command: `go test ./internal/spec ./internal/check ./internal/repo`

Captured output (against unchanged repo.go/init.go):

```
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
--- FAIL: TestRequirementsAndTasksDir (0.00s)
    repo_test.go:13: RequirementsDir() = "/some/root/pluma/plumage", want "/some/root/.fledge/pluma/plumage"
    repo_test.go:18: TasksDir() = "/some/root/pluma/feathers", want "/some/root/.fledge/pluma/feathers"
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/repo	0.001s
FAIL
```

`internal/spec` and `internal/check` passed as expected (they don't exercise
`repo.go`), confirming the new `internal/repo` test was the one pinning the
behavior change and it failed for the expected reason (old path still
root-relative, not `.fledge`-relative).

After implementing (a) `internal/repo/repo.go` and (b) `internal/cli/init.go`,
re-running the same command:

```
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/spec	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/check	(cached)
```

All green.

## AC-2

`internal/repo/repo.go`:

```go
func (r *Repo) RequirementsDir() string { return filepath.Join(r.FledgeDir(), "pluma", "plumage") }
func (r *Repo) TasksDir() string        { return filepath.Join(r.FledgeDir(), "pluma", "feathers") }
```

Verified by `TestRequirementsAndTasksDir` in `internal/repo/repo_test.go`
(see AC-1 passing output above) — `RequirementsDir()` returns
`<FledgeDir>/pluma/plumage` and `TasksDir()` returns
`<FledgeDir>/pluma/feathers`.

## AC-3

Built the binary to a scratch location (not `go install`, so the shared
`fledge` on PATH is untouched), then ran `init` inside a **fresh temp git
repo** (not this worktree):

```
go build -o /tmp/fthr017-fledge-bin ./cmd/fledge
TMPD=$(mktemp -d) && cd "$TMPD" && git init -q && git commit -q --allow-empty -m init
/tmp/fthr017-fledge-bin init
```

Captured output (relevant lines):

```
created .fledge/nest/raw/.gitkeep
created .fledge/broods/.gitkeep
created .fledgeignore
created .fledge/pluma/plumage/.gitkeep
created .fledge/pluma/feathers/.gitkeep
created .gitignore
...
created .fledge/scaffold.json
scaffolded agents: claude
```

Confirmed no root-level `pluma/` was created and `.fledge/pluma/...` was:

```
$ find .fledge/pluma pluma -maxdepth 3
bfs: error: pluma: No such file or directory.
.fledge/pluma
.fledge/pluma/feathers
.fledge/pluma/plumage
.fledge/pluma/feathers/.gitkeep
.fledge/pluma/plumage/.gitkeep
```

(`pluma: No such file or directory` confirms no root-level `pluma/` exists;
`.fledge/pluma/plumage/.gitkeep` and `.fledge/pluma/feathers/.gitkeep` exist.)

Note: an earlier attempt at this manual check was accidentally run with cwd
still inside this worktree instead of the fresh temp repo (a shell-quoting
slip in a `cd a && (cd b && ...)` chain). That polluted the worktree with an
untracked `.fledge/pluma/` and a modified `.fledge/scaffold.json`; both were
reverted (`rm -rf .fledge/pluma`, `git checkout -- .fledge/scaffold.json`)
before recording the run above, and `git status --porcelain` was confirmed
clean of that pollution prior to committing.

## AC-4

```
go build ./...
go test ./internal/repo ./internal/spec ./internal/check ./internal/cli
```

Output:

```
ok  	github.com/Harrison-Blair/fledge/internal/repo	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/spec	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/check	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/cli	(cached)
```

`go build ./...` produced no output/errors (success). All four packages pass.

Out of scope per feather spec (explicitly, not touched): `cmd/fledge/testdata/*.txtar`
acceptance suite (will break here, fixed by FTHR-020) and this repo's own
`pluma/` specs.
