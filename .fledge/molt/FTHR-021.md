# FTHR-021 evidence

Implemented solo by the orchestrator (fledge-orchestrator), test-first, in worktree
`.../scratchpad/FTHR-021` on branch `feather/FTHR-021-migrate-specs-and-bump`.

## AC-1

Pre-implementation (bumped VERSION to 0.4.0, binaryVersion still 0.3.4):

```
$ go test ./internal/cli -run TestBinaryVersionMatchesVersionFile
--- FAIL: TestBinaryVersionMatchesVersionFile (0.00s)
    version_test.go:18: binaryVersion = "0.3.4", VERSION file = "0.4.0" — bump internal/cli/version.go
FAIL
```

Post-fix (binaryVersion → "0.4.0"):

```
$ go test ./internal/cli -run TestBinaryVersionMatchesVersionFile
ok  	github.com/Harrison-Blair/fledge/internal/cli
```

## AC-2

```
$ cat VERSION            → 0.4.0
$ ./fledge-local version → fledge 0.4.0
internal/cli/version.go:10  var binaryVersion = "0.4.0"
```

## AC-3

`git mv pluma .fledge/pluma` (rename-detected, no content change). After:

```
$ ls -d pluma                     → No such file or directory
$ ls .fledge/pluma                → feathers  plumage
$ ls .fledge/pluma/plumage | wc -l → 11
$ ls .fledge/pluma/feathers | wc -l → 21   (16 pre-existing + FTHR-017..021)
```

## AC-4

`./fledge-local init --refresh` regenerated `.fledge/scaffold.json` and resynced the two
skill-mirror files still carrying old paths (`templates/plumage.md`, `worker-protocols.md`);
`.fledge/pluma/{plumage,feathers}/.gitkeep` present; no unexpected prunes, no
"kept (user-edited)". `./fledge-local preen` reports the scaffold healthy (`spec clean`).
(In the worktree, preen additionally warns "FTHR-021 status hatching but no brood is held"
— an artifact of the claim lock living in the main working tree, not the worktree; verified
clean on main post-merge with the lock present.)

## AC-5

```
$ ./fledge-local unfledged --feathers | grep FTHR-021
  FTHR-021  hatching  P2  Migrate this repo's specs ... (plumage PLM-011)
$ ./fledge-local vee --json | grep -c FTHR-0   → 58 (all feathers resolved at new location)
```

## AC-6

The 0.4.0 bump broke one version assertion — `cmd/fledge/testdata/stamp_warning.txtar`
hard-coded `binary is 0.3.4` in its expected stderr. Updated it to `0.4.0` (a direct
consequence of the version bump; path-only FTHR-020 correctly did not touch version strings).
After that fix:

```
$ go build ./...   → ok
$ go test ./...    → all packages ok (cmd/fledge, internal/*), 0 failures
```
