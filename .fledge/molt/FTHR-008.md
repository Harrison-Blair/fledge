# FTHR-008 evidence

## AC-1 — tests observed failing before, passing after
Test-first: the new Claude CLAUDE.md assertions in
`cmd/fledge/testdata/init_agents.txtar` were run against the **unchanged**
`adapters/claude/manifest.yaml` (no CLAUDE.md entry) and observed FAILING for the
expected reason — no CLAUDE.md is created, so `exists CLAUDE.md` fails:

```
$ go test ./cmd/fledge -run TestScripts/init_agents
...
            scaffolded agents: claude
            > stdout 'scaffolded agents: claude'
            > exists CLAUDE.md
            FAIL: testdata/init_agents.txtar:71: $WORK/claude-pointer-repo/CLAUDE.md does not exist
FAIL
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.018s
```

After adding the `append_if_missing` entry to the Claude manifest, the same
assertions pass:

```
$ go test ./cmd/fledge -run TestScripts/init_agents
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.039s
```

## AC-2 — created when absent (PLM-004 FC-1)
The `claude-pointer-repo` scenario asserts `! exists CLAUDE.md` before init, then
after `fledge init --agent claude`: `exists CLAUDE.md` and
`grep 'fledge-orchestrate/SKILL.md' CLAUDE.md`. Dogfood confirmation — the refresh
in this worktree created the pointer in this repo's own CLAUDE.md:

```
$ git diff CLAUDE.md
+> fledge: load and follow .fledge/skills/fledge-orchestrate/SKILL.md — primitive map at .claude/fledge-adapter.md
```

## AC-3 — additive + idempotent (PLM-004 FC-2)
`claude-existing-repo` seeds a CLAUDE.md with sentinel prose (`my existing claude
prose`); after init both `grep 'my existing claude prose'` and
`grep 'fledge-orchestrate/SKILL.md'` pass (existing content preserved, pointer
appended). The `claude-pointer-repo` scenario re-runs init and asserts
`grep -count=1 'fledge-orchestrate/SKILL.md' CLAUDE.md` (exactly one copy).

## AC-4 — wording matches Codex verbatim except adapter path (PLM-004 FC-3)
Exact-line `grep` asserts the full pointer line. Manifest parity:

```
claude: > fledge: load and follow .fledge/skills/fledge-orchestrate/SKILL.md — primitive map at .claude/fledge-adapter.md
codex:  > fledge: load and follow .fledge/skills/fledge-orchestrate/SKILL.md — primitive map at .codex/fledge-adapter.md
```

Differ only in `.claude/` vs `.codex/`.

## AC-5 — suite green, no Go source changed (PLM-004 AC-4)
```
$ go build ./...   # build OK
$ go vet ./...     # vet OK
$ go test ./...    # all packages ok
$ git diff --name-only main -- 'internal/**/*.go' 'cmd/**/*.go' | grep -v _test.go | grep -v testdata
(none — only manifest.yaml, txtar fixtures, CLAUDE.md, scaffold.json, and this evidence file changed)
```
