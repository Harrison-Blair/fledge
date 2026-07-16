# FTHR-064 evidence

## AC-1

Pre-implementation run of the new test against the unchanged
`internal/bootstrap/core/skills/fledge-interrogate/SKILL.md` — fails for the
expected reason (no batching exception present yet):

```
$ go test ./internal/bootstrap -run TestInterrogateSkillDocumentsBatchingException
--- FAIL: TestInterrogateSkillDocumentsBatchingException (0.00s)
    interrogate_batching_test.go:33: the paragraph containing the one-at-a-time sentence must reference incubator.md; got ""
    interrogate_batching_test.go:36: the paragraph containing the one-at-a-time sentence must mention batching via the scratchpad mechanism; got ""
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

Post-implementation run of the same test — passes:

```
$ go test ./internal/bootstrap -run TestInterrogateSkillDocumentsBatchingException
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

## AC-2

The exception sentence was added immediately after the original one-at-a-time
sentence on line 8 of `internal/bootstrap/core/skills/fledge-interrogate/SKILL.md`;
no other line changed:

```
$ git diff --stat
 internal/bootstrap/core/skills/fledge-interrogate/SKILL.md | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)
```

Line 8 now reads (original sentence intact, exception appended):

```
Ask the questions one at a time, waiting for feedback on each question before continuing. Asking multiple questions at once is bewildering. Exception: a delegated incubator may batch multiple resolved, independent questions into one scratchpad file per `incubator.md`'s scratchpad-batching rule — this changes only how resolvable answers are delivered, not the question-generation approach below.
```

The test also pins both halves: it fatals if the original sentence is missing
or altered, and errors if the same paragraph lacks `incubator.md`, `batch`, or
`scratchpad`.

## AC-3

```
$ go test ./internal/bootstrap/...
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
```

Full suite also run for safety (txtar tests included):

```
$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.101s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	(cached)
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.010s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.126s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.006s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/roster	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s
```
