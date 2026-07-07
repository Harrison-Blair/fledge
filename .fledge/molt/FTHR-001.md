# Evidence: FTHR-001 — report command: tracer, status counts, per-REQ completion

Recorded retroactively on 2026-07-06 during the migration to checkbox acceptance
criteria: this task was implemented and merged (commit b701cf5) before evidence
files existed. Criteria re-verified against the current tree at migration time.

## AC-1
Test-first evidence predates this file; the pinning suite exists and passes:
Commands: `go test ./cmd/fledge -run TestScripts/report`
```
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.019s
```

## AC-2
`cmd/fledge/testdata/report.txtar` pins FC-1, FC-2, FC-3, FC-6, FC-8, FC-9 for
both `fledge colony` and `fledge colony --json`; suite green (see AC-1 output).

## AC-3
Commands: `go test ./... && go vet ./...` — green/clean at migration time.

## AC-4
`report` is present in `commandOrder` (internal/cli/cli.go) and appears in the
usage listing.
