# Evidence: FTHR-003 — fledge unfledged command: list incomplete plumage and feathers with type filters

## AC-1
Test-first discipline observed. The suite `cmd/fledge/testdata/unfledged.txtar`
was written first and run against the unchanged tree, failing at the first
assertion for the expected reason:
```
fledge: unknown command "unfledged"
FAIL github.com/Harrison-Blair/fledge/cmd/fledge
```
After implementing `internal/cli/unfledged.go` and registering the command it passes:
```
go test ./cmd/fledge -run TestScript/unfledged
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.015s
```
(An intermediate run caught a bad negative assertion — `PLM-002` legitimately
appears inside the `(plumage PLM-002)` feather link — which was corrected to
target the plumage entry, confirming the assertion actually fails when wrong.)

## AC-2
`cmd/fledge/testdata/unfledged.txtar` pins PLM-002 FC-1–FC-9:
- FC-1/FC-2: fledged PLM-001 and FTHR-001 absent; open plumage/feathers listed
  with `ID status priority title` (feathers add `(plumage PLM-###)`).
- FC-3: `cmp stdout expected-order.txt` fixes priority-then-ID ordering (P0 before P2).
- FC-5: `--plumage`, `--feathers`, and both-flags cases assert section scoping,
  including the neither-or-both rule.
- FC-6: `--json` asserts `plumage`/`feathers` arrays, feather `plumage` link and
  `path`, and `[]`-not-null empty sections.
- FC-7: an unterminated-frontmatter file appears under `Issues:` / `"issues"`
  while parsed specs still list, exit 0.
- FC-8: empty repo yields both sections with `(none)`, exit 0.
- FC-9: unknown flag → exit 2; outside a fledge repo → exit 3; no exit-1 path.

## AC-3
Commands: `go test ./...` (all packages ok) and `go vet ./...` (clean, no output).
Full suite unaffected by the new command and the `commandOrder` addition.

## AC-4
`unfledged` is present in `commandOrder` (internal/cli/cli.go) and appears in the
usage listing:
```
fledge unfledged [--plumage] [--feathers] [--json]
```
Run in this repository it lists PLM-002 (hatched), FTHR-003 (pipping), and
FTHR-004 (egg), with fledged specs absent — matching `fledge colony`.
