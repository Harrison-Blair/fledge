# FTHR-068 evidence

## AC-1

Test `TestForagingDocDescribesDigestWrite` (new, `internal/bootstrap/foraging_digest_test.go`) run against the unchanged code, from the worktree root:

```
$ go test ./internal/bootstrap -run TestForagingDocDescribesDigestWrite
--- FAIL: TestForagingDocDescribesDigestWrite (0.00s)
    foraging_digest_test.go:34: foraging.md verify-and-release paragraph missing digest wording ".fledge/scratch/digest-foraging.md"
    foraging_digest_test.go:34: foraging.md verify-and-release paragraph missing digest wording "commissioner"
FAIL
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
FAIL
```

Failing for the expected reason: no digest-write language in the verify-and-release paragraph yet.

After implementation (adding the digest-write instruction to the verify-and-release paragraph of `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md`):

```
$ go test ./internal/bootstrap -run TestForagingDocDescribesDigestWrite
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.001s
```

## AC-2

The "**On the final message, verify and release.**" paragraph of `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` now reads (relevant sentence, captured verbatim):

```
$ grep -o 'Before or alongside relaying.*ends at its final message\.' internal/bootstrap/core/skills/fledge-orchestrate/foraging.md
Before or alongside relaying, write `.fledge/scratch/digest-foraging.md` (overwriting any prior one) with the coverage outcome (which concern docs were written or updated, and the commit `index.md` is stamped to), any open questions the forager flagged, and a pointer to `.fledge/nest/index.md`; this digest is written by whoever is acting as commissioner at that point (orchestrator or incubator), never the forager — its job ends at its final message.
```

This documents writing `digest-foraging.md` (overwritten, per FC-4), attributed to the commissioner (per the feather's Approach, which supersedes PLM-029 FC-2's "forager" attribution for this phase), with the FC-3 content shape: outcome (concern docs written/updated, commit stamped), open questions (key unresolved items from the phase), and pointers (`.fledge/nest/index.md`).

Note: `cmd/fledge/testdata/forager_contract.txtar:35` asserted the exact old wording `force-terminates it if it does not exit promptly`; the digest insertion detached that sentence, so "it" became "the forager" and the fixture was updated to match (per CLAUDE.md: update txtar fixtures alongside embedded `core/` changes).

## AC-3

```
$ go test ./internal/bootstrap/...
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.008s
```

Full suite, gofmt, and vet also clean in the worktree:

```
$ go test ./... 2>&1 | grep -v "^ok" ; gofmt -l . ; go vet ./...
(no output — all packages ok, no fmt/vet findings)
```
