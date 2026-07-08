# Evidence: FTHR-004 — wire fledge unfledged into the orchestration skill command surface

> **Reconstructed at closeout 2026-07-08.** FTHR-004 (a documentation feather,
> `oversight: merge`) was implemented, reviewed, and fledged in an earlier
> session that checked its acceptance-criteria boxes and merged the prose change
> but never wrote or committed this evidence file. The prose changes are present
> and correct on `main`; the sections below record verification of each criterion
> against the current tree, not a test-first capture from build time. Preserved
> here to restore the audit trail and satisfy `fledge preen`'s criteria-evidence
> rule.

## AC-1
Documentation feather — automated failure-first is N/A (no code path to break).
Per the criterion, verification is a presence check: `fledge unfledged` appears
0 times in the skill prose before the feather and ≥1 after. Current state on
`main`:
```
$ grep -c "fledge unfledged" internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
1
```
The single pre-implementation "0" observation cannot be re-derived now that the
prose is merged; the ≥1 post-state is confirmed above.

## AC-2
`SKILL.md`'s deterministic-ops inventory lists `fledge unfledged` alongside
`ready`/`vee`, with its `--plumage`/`--feathers`/`--json` surface:
```
$ grep -n "fledge unfledged" internal/bootstrap/core/skills/fledge-orchestrate/SKILL.md
36:- Deterministic spec operations go through the `fledge` CLI ... Readiness/structure:
   `fledge ready`, `fledge vee`, and `fledge unfledged` to survey all non-fledged
   plumage and feathers (`--plumage`/`--feathers` to scope, `--json` for a
   machine-readable contract). ...
```

## AC-3
Exactly one usage touchpoint references `fledge unfledged` (the planning-close
inventory), preserving the `ready` (dispatchable-now) vs `unfledged` (all
non-fledged) distinction. `implementation.md` adds no second touchpoint:
```
$ grep -rn "fledge unfledged" internal/bootstrap/core/skills/fledge-orchestrate/*.md
planning.md:37:... the ready-to-start feathers (`fledge ready`, the dispatchable-now
   subset), and the full remaining slate of non-fledged plumage and feathers
   (`fledge unfledged`, everything not yet complete). ...
SKILL.md:36:... (AC-2 inventory entry, not a usage touchpoint)
```
The `SKILL.md` hit is the AC-2 inventory listing; `planning.md:37` is the sole
usage touchpoint.

## AC-4
`go build ./...` and `go vet ./...` are clean, and the feather changed no Go
files (prose/fixtures only):
```
$ go build ./...   # exit 0
$ go vet ./...     # exit 0
```
