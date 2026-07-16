# FTHR-045 — Preen criteria-evidence diagnostic names required heading form

## AC-1
The new unit test `TestCriteriaEvidenceLabeledHeadingMessage` in
`internal/check/check_test.go` was written first and run against the
**unchanged** message format. It failed for the expected reason: the emitted
message did not name the required bare `## AC-N` heading form.

Command:

```
go test ./internal/check/ -run TestCriteriaEvidenceLabeledHeadingMessage -v
```

Failing output (pre-implementation, captured verbatim):

```
=== RUN   TestCriteriaEvidenceLabeledHeadingMessage
    check_test.go:351: message should name the required bare heading form "\"## AC-N\"", got: checked criteria missing evidence sections in /tmp/TestCriteriaEvidenceLabeledHeadingMessage2585540610/001/FTHR-001.md: AC-1
--- FAIL: TestCriteriaEvidenceLabeledHeadingMessage (0.00s)
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/check	0.001s
FAIL
```

After updating the `criteria-evidence` message in
`checkCriteriaEvidence` (`internal/check/check.go`) to name the required
form, the same test passes:

```
go test ./internal/check/ -run TestCriteriaEvidenceLabeledHeadingMessage -v
```

```
=== RUN   TestCriteriaEvidenceLabeledHeadingMessage
--- PASS: TestCriteriaEvidenceLabeledHeadingMessage (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
```

## AC-2
`checkCriteriaEvidence`'s emitted message now names the exact required
`## AC-N` heading form. The message format string in
`internal/check/check.go` is:

```
checked criteria missing evidence sections in %s: %s (heading must be the bare form "## AC-N", not "## AC-N: <label>")
```

The test asserts the message contains the literal `"## AC-N"`, which passes
(see AC-1 passing run). Satisfies PLM-023 FC-4, AC-4.

## AC-3
`go test ./internal/check/...` passes:

```
go test ./internal/check/...
```

```
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
```
