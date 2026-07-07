# Evidence: FTHR-002 — report widening: blocked detail, active locks, degraded data issues

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
`cmd/fledge/testdata/report.txtar` pins FC-4, FC-5, FC-7 (blocked detail, active
locks, degraded-data issues) alongside FTHR-001's assertions; suite green.

## AC-3
Commands: `go test ./... && go vet ./...` — green/clean at migration time.

## AC-4
`fledge colony` run in this repository reflects its own spec set (PLM-001 AC-3
self-hosting check), re-verified at migration time.
