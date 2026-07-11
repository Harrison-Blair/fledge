# FTHR-018 Evidence

## AC-1

Grep-based before/after check across the 6 affected files for stale `pluma/plumage`/`pluma/feathers` path references (excluding already-correct `.fledge/pluma/...` occurrences).

### Before (captured pre-edit — FAILING / stale references present)

Command:
```sh
FILES="internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md internal/bootstrap/core/skills/fledge-orchestrate/implementation.md internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md internal/bootstrap/core/skills/fledge-orchestrate/templates/plumage.md internal/bootstrap/core/skills/fledge-orchestrate/templates/feather.md internal/spec/types.go"
grep -n -E 'pluma/(plumage|feathers)' $FILES | grep -v '\.fledge/pluma/'
```

Output (8 stale references found):
```
internal/spec/types.go:26:// Requirement is one pluma/plumage/PLM-###-<kebab>.md file.
internal/spec/types.go:40:// Task is one pluma/feathers/FTHR-###-<kebab>.md file.
internal/bootstrap/core/skills/fledge-orchestrate/templates/plumage.md:5:Plumages live at `pluma/plumage/PLM-###-<kebab-name>.md`. IDs are zero-padded and next-sequential within the folder. Plumages capture the WHAT and WHY at feature level — never implementation details (no file paths, no function names, no technology choices unless they are themselves the plumage).
internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md:8:You are running the fledge orchestration workflow. Fledge is a spec-driven development tool: repository knowledge lives in `.fledge/nest/`, feature intent lives in `pluma/plumage/` (the plumages, i.e. requirements), and implementable work lives in `pluma/feathers/` (the feathers, i.e. tasks).
internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md:41:- **Author-to-draft-then-gate.** When a phase authors a spec file and then gates on it, draft the full file content (frontmatter + body) in a buffer first and run the gate *before* committing the file via the CLI. You may note the prospective next ID by listing the relevant folder (`pluma/plumage/` or `pluma/feathers/`). On "Accept", create the file with `fledge new …` (it allocates the real ID) and write the body. On "Make changes", no spec mutation has occurred — revise the draft and re-gate. A refusal never leaves an un-gated file on disk.
internal/bootstrap/core/skills/fledge-orchestrate/templates/feather.md:5:Feathers live at `pluma/feathers/FTHR-###-<kebab-name>.md`. IDs are zero-padded and next-sequential within the folder. Every feather links to exactly one plumage. `depends_on` forms blocking relationships: a feather is `pipping` only when every feather in `depends_on` is `fledged`.
internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md:89:A review request from a brooder gives: feather ID, the feather spec path (`pluma/feathers/FTHR-###-<kebab>.md`), worktree path, branch, the evidence-file path (`.fledge/molt/FTHR-###.md` in the worktree), change summary, test commands, and an AC-by-AC self-check pointing at the evidence sections. If any of these are missing, return the request without reviewing. The spec path is needed because the checks below read the spec's Tests, Approach, acceptance criteria, and Affected Modules sections.
internal/bootstrap/core/skills/fledge-orchestrate/implementation.md:3:Executes ready feathers from `pluma/feathers/`. This phase runs in the main session — you are the **orchestrator**: you dispatch, gate, merge, and triage. How much you delegate depends on which primitives your adapter provides (see §primitives).
```

### After (captured post-edit — zero stale references)

Command:
```sh
grep -n -E 'pluma/(plumage|feathers)' $FILES | grep -v '\.fledge/pluma/'
```

Output: (empty — zero matches)
```
count: 0
```

All 8 references now read `.fledge/pluma/plumage/...` or `.fledge/pluma/feathers/...`.

## AC-2

None of the 6 affected files contain a stale `pluma/plumage`/`pluma/feathers` path reference — confirmed by the zero-match "after" grep above, covering all 6 files:
- `internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md` (2 references updated)
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` (1 reference updated)
- `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` (1 reference updated)
- `internal/bootstrap/core/skills/fledge-orchestrate/templates/plumage.md` (1 reference updated)
- `internal/bootstrap/core/skills/fledge-orchestrate/templates/feather.md` (1 reference updated)
- `internal/spec/types.go` (2 doc comments updated, lines 26 and 40)

## AC-3

`go test ./internal/bootstrap` — run before and after the edit to confirm no structural regression (this feather changes prose/comments only, not Go logic).

### Before (baseline, unchanged code)

```sh
$ go test ./internal/bootstrap
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
```

### After (post-edit)

```sh
$ go test ./internal/bootstrap
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
```

Also confirmed `go build ./...` succeeds with no errors after the edits.
