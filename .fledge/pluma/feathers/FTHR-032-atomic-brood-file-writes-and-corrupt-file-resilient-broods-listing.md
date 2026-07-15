---
id: FTHR-032
title: Atomic brood-file writes and corrupt-file-resilient broods listing
plumage: PLM-016
status: hatching
priority: P1
depends_on: []
authored: 2026-07-15T15:13:39Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-032: Atomic brood-file writes and corrupt-file-resilient broods listing

## Description
Two coupled fixes in the lock package: (1) write claim (`.brood`) files atomically so a
crash mid-write can't leave a partial file, and (2) make the listing skip and surface an
individual unparseable claim file instead of aborting the whole listing. Today `Acquire`
streams JSON straight into the `O_EXCL`-created live path (no temp+rename), and `List`
returns `nil, err` on the first `Get` that fails to unmarshal — so one truncated file hides
every healthy claim.

Satisfies PLM-016 FC-2 (resilient listing) and FC-3 (atomic write).

## Affected Modules
- `internal/lock/lock.go` — `Acquire` (lines 32–53, non-atomic write) and `List`
  (lines 78–99, aborts on any corrupt file). See `.fledge/nest/modules.md → internal/lock`.
- `internal/cli/brood.go` — `runLocks` (the `broods` command) consumes `List`; adjust it to
  report skipped/corrupt files (e.g. a stderr warning naming them) if `List` surfaces them
  as a separate return rather than logging directly.

## Approach
1. **Atomic + exclusive Acquire.** Keep the "fail loudly if already held" semantics while
   making the content write atomic. Write the full JSON record to a temp file in the broods
   dir, then place it via an atomic link/rename that preserves exclusivity: prefer
   `os.Link(tmp, finalPath)` — it fails with `EEXIST` when the claim is already held (map to
   the existing `*HeldError`, reading the current holder as `Acquire` does today) and, on
   success, makes the fully-written file appear in one atomic step; then remove the temp.
   This mirrors the repo's atomic-write idiom (`spec.WriteFileAtomic`, frontmatter.go:224 —
   temp + `os.Rename`) while keeping the O_EXCL-equivalent guard. (`os.Link` within one dir
   on a single filesystem is atomic on Unix, which fledge targets.)
2. **Resilient List.** In `List`, on a `Get` error for an individual entry, skip that file
   and continue rather than returning the error, and surface which files were skipped —
   e.g. return the healthy records plus a slice of problem filenames, or accept an
   `io.Writer` for warnings. `runLocks` then prints a warning naming any skipped file so the
   corruption is visible, not silent. Do not let one bad file suppress the healthy claims.
   Keep the sorted-by-ID output for the healthy set.

Constraints: preserve `Acquire`'s `*HeldError` contract and `List`'s sorted output. Don't
change the `Record` schema or the `.brood` filename convention. The `Get` error message
("corrupt brood file for …") stays; it's now caught by `List` instead of propagated.

## Tests
Written test-first, failing before, passing after:
- `TestListSkipsCorruptBroodFile` (`internal/lock`) — pins FC-2: write two valid `.brood`
  files and one containing non-JSON garbage (and separately, a zero-length file), call
  `List`, assert it returns the two healthy records and reports the bad file(s) as skipped;
  fails today (List returns an error, zero records).
- `TestAcquireWritesAtomically` (`internal/lock`) — pins FC-3: assert a successful `Acquire`
  leaves exactly one complete, parseable `.brood` file and no leftover temp file, and that
  a second `Acquire` for the same task still returns `*HeldError`; assert (via the link
  approach) the final file is never observed in a zero-length/partial state. Structure it so
  it fails against the current streaming write where the seam differs.

## Acceptance Criteria
- [x] AC-1: The tests above were observed failing before implementation and pass after.
- [x] AC-2: `List` returns all healthy claims and surfaces (does not swallow) an individual corrupt/partial `.brood`, instead of aborting; `broods` prints a warning naming skipped files (FC-2).
- [x] AC-3: `Acquire` writes the claim atomically (temp + link/rename) and still returns `*HeldError` when the claim is already held; no partial file is observable in place of a valid one (FC-3).
- [x] AC-4: `fledge preen` passes and `go test ./...` is green.
