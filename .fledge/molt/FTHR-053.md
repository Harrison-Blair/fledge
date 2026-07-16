# FTHR-053 — Persisted roster allocator core: evidence

## AC-1

_Tests observed failing before implementation, passing after_

### Before implementation (test-first)

The tests in `internal/roster/roster_test.go` were written first, against a
package with no source file yet. Running them fails to compile for the expected
reason — every roster symbol (`Species`, `Assign`, `Release`, `List`) is
undefined:

```
$ go test ./internal/roster/...
# github.com/Harrison-Blair/fledge/internal/roster [github.com/Harrison-Blair/fledge/internal/roster.test]
internal/roster/roster_test.go:18:9: undefined: Species
internal/roster/roster_test.go:19:46: undefined: Species
internal/roster/roster_test.go:22:6: undefined: Species
internal/roster/roster_test.go:23:45: undefined: Species
internal/roster/roster_test.go:34:14: undefined: Assign
internal/roster/roster_test.go:42:13: undefined: Assign
internal/roster/roster_test.go:52:16: undefined: Assign
internal/roster/roster_test.go:57:13: undefined: Assign
internal/roster/roster_test.go:72:14: undefined: Assign
internal/roster/roster_test.go:80:12: undefined: Release
internal/roster/roster_test.go:80:12: too many errors
FAIL	github.com/Harrison-Blair/fledge/internal/roster [build failed]
FAIL
```

### After implementation

With `internal/roster/roster.go` implemented, every test passes:

```
$ go test -v ./internal/roster/...
=== RUN   TestSpeciesList
--- PASS: TestSpeciesList (0.00s)
=== RUN   TestAssignSequentialAndOverflow
--- PASS: TestAssignSequentialAndOverflow (0.00s)
=== RUN   TestPairReleaseFreesOnlyWhenBothReleased
--- PASS: TestPairReleaseFreesOnlyWhenBothReleased (0.00s)
=== RUN   TestListOmitsFullyReleased
--- PASS: TestListOmitsFullyReleased (0.00s)
=== RUN   TestConcurrentAssignYieldsDistinctSpecies
--- PASS: TestConcurrentAssignYieldsDistinctSpecies (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.006s
```

## AC-2

_Species holds the confirmed 18-species list in confirmed order_

`internal/roster/roster.go` declares `var Species = [18]string{...}` with the
exact confirmed order. `TestSpeciesList` pins both the length (18) and every
element by index against the confirmed list (adelie, emperor, gentoo, king,
chinstrap, little, african, humboldt, magellanic, galapagos, yelloweyed,
fiordland, snares, erectcrested, rockhopper, royal, macaroni,
northernrockhopper).

```
$ go test -run TestSpeciesList -v ./internal/roster/...
=== RUN   TestSpeciesList
--- PASS: TestSpeciesList (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.004s
```

## AC-3

_Assign/Release/List behave as specified (incl. suffix overflow + per-member release)_

- `TestAssignSequentialAndOverflow` — Assign hands out species in canonical
  order (adelie, then emperor), and once all 18 bases are in use the 19th call
  overflows to `adelie-2`.
- `TestPairReleaseFreesOnlyWhenBothReleased` — a pair reserves one species
  across two members; releasing one member leaves the species in use (the next
  Assign returns emperor, not adelie), and releasing the second frees it (a
  later Assign reuses adelie).
- `TestListOmitsFullyReleased` — List returns only live entries; a
  fully-released species is dropped.

```
$ go test -run 'TestAssignSequentialAndOverflow|TestPairReleaseFreesOnlyWhenBothReleased|TestListOmitsFullyReleased' -v ./internal/roster/...
=== RUN   TestAssignSequentialAndOverflow
--- PASS: TestAssignSequentialAndOverflow (0.00s)
=== RUN   TestPairReleaseFreesOnlyWhenBothReleased
--- PASS: TestPairReleaseFreesOnlyWhenBothReleased (0.00s)
=== RUN   TestListOmitsFullyReleased
--- PASS: TestListOmitsFullyReleased (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.004s
```

## AC-4

_Concurrency test demonstrates no double-allocation_

`TestConcurrentAssignYieldsDistinctSpecies` launches 18 goroutines that all
call `Assign` against one shared state dir behind a start barrier, over 5
rounds, and asserts all 18 species come back distinct. It passes normally and
under `-race`:

```
$ go test -race -run TestConcurrentAssignYieldsDistinctSpecies ./internal/roster/...
ok  	github.com/Harrison-Blair/fledge/internal/roster	1.026s
```

Mutation check (throwaway copy outside the worktree, never committed): with the
`syscall.Flock(LOCK_EX)` call removed from `lockRosterDir`, the same test
fails — proving it genuinely pins the locking behavior rather than passing
vacuously:

```
$ go test -run TestConcurrentAssignYieldsDistinctSpecies ./roster/   # flock disabled
--- FAIL: TestConcurrentAssignYieldsDistinctSpecies (0.00s)
    roster_test.go:171: round 0: goroutine 3: Assign: roster: corrupt state file: unexpected end of JSON input
FAIL
```

## AC-5

_`go test ./internal/roster/...` passes_

```
$ go test ./internal/roster/...
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.006s
```

Supporting hygiene (`go vet` clean, `gofmt` reports no files needing
formatting):

```
$ go vet ./internal/roster/...
$ gofmt -l internal/roster/
```
