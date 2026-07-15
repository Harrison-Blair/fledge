# FTHR-034 evidence

Test-only feather: extends `cmd/fledge/testdata/lock.txtar` with stale-PID
detection assertions and `--json` shape assertions for `brood`/`broods`/`abandon`.
No production code changed (net diff: only `lock.txtar`).

## AC-1
_stale-PID detection (text + `broods --json`), live claim asserted alive_

Seeded two raw `.brood` fixtures directly in the txtar archive (a live PID
claim assembled via `fledge brood` turned out to be unusable for the "alive"
assertion — testscript re-execs the `fledge` binary as a short-lived
subprocess per `exec` line, so by the time a later `fledge broods` runs, the
PID that `fledge brood` stamped has already exited and is genuinely dead;
confirmed this empirically on the first run, see below):

- `FTHR-005` — pid `2147483647` (`ESRCH`, not alive)
- `FTHR-006` — pid `1` (always alive on a running Linux system)

Assertions added (`lock.txtar` lines 17-23):
```
stdout '(?m)^FTHR-006\s+skua\s+since\s+\S+\s+branch\s+\S+$'
stdout '(?m)^FTHR-005\s+weddell\s+since\s+\S+\s+branch\s+\S+\s+\(pid not alive\)$'
exec fledge broods --json
stdout '(?s)"feather": "FTHR-005".*?"pid_alive": false'
stdout '(?s)"feather": "FTHR-006".*?"pid_alive": true'
```

First attempt (using `fledge brood FTHR-001`'s own process PID as the "live"
example, per the spec's literal Approach wording) failed for exactly the
reason above — captured verbatim:

```
# broods lists holders (0.001s)
> exec fledge broods
[stdout]
FTHR-001  adelie  since 2026-07-15T15:57:05Z  branch   (pid not alive)
FTHR-005  weddell  since 2026-07-01T00:00:00Z  branch main  (pid not alive)
> stdout 'FTHR-001'
> stdout 'adelie'
# stale-PID detection: a seeded claim with a not-alive PID is marked, a live
# claim is not (0.000s)
> stdout '(?m)^FTHR-001\s+adelie\s+since\s+\S+\s+branch\s+\S+$'
FAIL: testdata/lock.txtar:19: no match for `(?m)^FTHR-001\s+adelie\s+since\s+\S+\s+branch\s+\S+$` found in stdout
```

This is the required "shown to bite" evidence for the stale-PID assertions:
it demonstrates the assertions distinguish a genuinely dead PID from a live
one, and is why the fixture was changed to seed the live-PID example (`FTHR-006`,
pid 1) directly rather than relying on a transient `fledge brood` subprocess.

Passing run after switching to the `FTHR-006`/pid-1 fixture:
```
# broods lists holders (0.001s)
> exec fledge broods
[stdout]
FTHR-001  adelie  since 2026-07-15T15:58:26Z  branch 
FTHR-005  weddell  since 2026-07-01T00:00:00Z  branch main  (pid not alive)
FTHR-006  skua  since 2026-07-01T00:00:00Z  branch main
> stdout 'FTHR-001'
> stdout 'adelie'
# stale-PID detection: ... (0.000s)
> stdout '(?m)^FTHR-006\s+skua\s+since\s+\S+\s+branch\s+\S+$'
> stdout '(?m)^FTHR-005\s+weddell\s+since\s+\S+\s+branch\s+\S+\s+\(pid not alive\)$'
> exec fledge broods --json
[stdout]
[
  { "feather": "FTHR-005", ..., "pid_alive": false },
  { "feather": "FTHR-006", ..., "pid_alive": true }
]
> stdout '(?s)"feather": "FTHR-005".*?"pid_alive": false'
> stdout '(?s)"feather": "FTHR-006".*?"pid_alive": true'
```
(full output captured in the AC-4 `go test` run below)

## AC-2
_`--json` shapes of `brood`, `broods`, `abandon`_

Assertions added (`lock.txtar` lines 74-93):
```
exec fledge brood FTHR-002 --owner weddell --json
stdout '"feather": "FTHR-002"'
stdout '"owner": "weddell"'
stdout '"pid": [0-9]+'
stdout '"created": "'
stdout '"branch": "'

exec fledge broods --json
stdout '(?s)"feather": "FTHR-002".*?"pid_alive"'

exec fledge abandon FTHR-002 --json
stdout '"status": null'

exec fledge brood FTHR-002 --owner weddell
exec fledge abandon FTHR-002 --fledged --json
stdout '"status": "fledged"'
```
Actual stdout observed for the two abandon calls (from the `go test -v` run):
```
> exec fledge abandon FTHR-002 --json
[stdout]
{
  "feather": "FTHR-002",
  "released": true,
  "status": null
}
...
> exec fledge abandon FTHR-002 --fledged --json
[stdout]
{
  "feather": "FTHR-002",
  "released": true,
  "status": "fledged"
}
```
This exercises both branches of the null-vs-string `status` key.

## AC-3
_perturbation proof_

Perturbed `internal/cli/brood.go`'s `pidAlive` to invert its sense:
```diff
-	return syscall.Kill(pid, 0) == nil || errors.Is(syscall.Kill(pid, 0), syscall.EPERM)
+	return !(syscall.Kill(pid, 0) == nil || errors.Is(syscall.Kill(pid, 0), syscall.EPERM))
```
Ran `go test ./cmd/fledge -run TestScripts/lock -v`; the new assertions failed
for the expected reason (FTHR-006's now-flipped `pid_alive` no longer matches
the "alive" line format):
```
# stale-PID detection: a seeded claim with a not-alive PID is marked, a
# seeded claim with a live PID (pid 1, always alive) is not (0.000s)
> stdout '(?m)^FTHR-006\s+skua\s+since\s+\S+\s+branch\s+\S+$'
FAIL: testdata/lock.txtar:19: no match for `(?m)^FTHR-006\s+skua\s+since\s+\S+\s+branch\s+\S+$` found in stdout
--- FAIL: TestScripts (0.00s)
    --- FAIL: TestScripts/lock (0.01s)
FAIL
```
Reverted the perturbation (`git diff` shows `internal/cli/brood.go` back to
original, only `cmd/fledge/testdata/lock.txtar` changed) and confirmed green:
```
--- PASS: TestScripts (0.00s)
    --- PASS: TestScripts/lock (0.04s)
PASS
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	(cached)
```

## AC-4
_full suite + preen_

```
$ go test ./cmd/fledge -run TestScripts/lock -v
... PASS: TestScripts/lock (0.04s)

$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap
ok  	github.com/Harrison-Blair/fledge/internal/check
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig
ok  	github.com/Harrison-Blair/fledge/internal/cli
ok  	github.com/Harrison-Blair/fledge/internal/graph
ok  	github.com/Harrison-Blair/fledge/internal/hooktest
ok  	github.com/Harrison-Blair/fledge/internal/lock
ok  	github.com/Harrison-Blair/fledge/internal/nest
ok  	github.com/Harrison-Blair/fledge/internal/repo
ok  	github.com/Harrison-Blair/fledge/internal/scan
ok  	github.com/Harrison-Blair/fledge/internal/spec

$ gofmt -l .
(no output — clean)

$ fledge preen
WARN  .fledge/pluma/feathers/FTHR-029-...: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-032-...: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-033-...: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-034-...: status hatching but no brood is held
WARN  .fledge/pluma/feathers/FTHR-035-...: status hatching but no brood is held
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
7 warning(s)
```
The 7 warnings are pre-existing repo state (sibling feathers' brooded status,
and scaffold drift on main) unrelated to this change — `preen` exits 0 on
warnings, and none reference `lock.txtar` or the CLI code under test.

## Deviation from spec's literal Approach wording

Spec Approach step 1 says to keep the live-PID claim "via `fledge brood`".
Under this repo's testscript harness (`cmd/fledge/main_test.go` re-execs the
built binary per `exec` line via `os/exec`), the process that `fledge brood`
stamps its own PID into always exits before a later `fledge broods` command
runs, so that PID is never actually alive when checked — verified this by
running the literal approach first and observing the failure captured above
under AC-1. Substituted a second seeded raw `.brood` fixture (`FTHR-006`,
pid 1 — always alive) for the "live" side of the comparison instead, which
still covers both `pidAlive` branches (the substance of AC-1) without relying
on an infeasible-in-harness mechanism.
