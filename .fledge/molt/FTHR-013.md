# FTHR-013 Evidence

## AC-1

`internal/cli/version_test.go` pins `binaryVersion` == VERSION; observed
failing when only VERSION is bumped, passing when both are bumped.

### Pre-implementation: VERSION bumped to 0.3.0, binaryVersion still 0.2.1

```
$ go test ./internal/cli -run TestBinaryVersionMatchesVersionFile -v -count=1
=== RUN   TestBinaryVersionMatchesVersionFile
    version_test.go:18: binaryVersion = "0.2.1", VERSION file = "0.3.0" — bump internal/cli/version.go
--- FAIL: TestBinaryVersionMatchesVersionFile (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/cli	0.001s
FAIL
```

### Post-implementation: both bumped to 0.3.0

```
$ go test ./internal/cli -run TestBinaryVersionMatchesVersionFile -v -count=1
=== RUN   TestBinaryVersionMatchesVersionFile
--- PASS: TestBinaryVersionMatchesVersionFile (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.001s
```
