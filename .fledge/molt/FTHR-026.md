# FTHR-026 evidence

Implemented by fledge-brooder-chinstrap, test-first, in worktree
`.fledge/burrows/FTHR-026` on branch `feather/FTHR-026-update-check-prompt`.

## AC-1

Tests written first in `internal/cli/update_test.go` against the not-yet-existing
`update` command. Pre-implementation run (no `internal/cli/update.go` yet):

```
$ go test ./internal/cli/ -run TestUpdate -v
# github.com/Harrison-Blair/fledge/internal/cli [github.com/Harrison-Blair/fledge/internal/cli.test]
internal/cli/update_test.go:28:10: undefined: updateAPIBaseURL
internal/cli/update_test.go:29:2: undefined: updateAPIBaseURL
internal/cli/update_test.go:30:21: undefined: updateAPIBaseURL
internal/cli/update_test.go:38:10: undefined: runUpdateWith
internal/cli/update_test.go:56:10: undefined: runUpdateWith
internal/cli/update_test.go:81:10: undefined: runUpdateWith
internal/cli/update_test.go:96:10: undefined: runUpdateWith
internal/cli/update_test.go:111:10: undefined: runUpdateWith
internal/cli/update_test.go:131:10: undefined: runUpdateWith
internal/cli/update_test.go:135:10: undefined: updateJSON
internal/cli/update_test.go:135:10: too many errors
FAIL	github.com/Harrison-Blair/fledge/internal/cli [build failed]
```

Fails for the expected reason: the `update` command and its seams
(`updateAPIBaseURL`, `runUpdateWith`, `updateJSON`) do not exist yet.

Post-implementation (`internal/cli/update.go` authored):

```
$ go test ./internal/cli/ -run TestUpdate -v
=== RUN   TestUpdate_AlreadyUpToDate
--- PASS: TestUpdate_AlreadyUpToDate (0.00s)
=== RUN   TestUpdate_NewerAvailable_PromptsAndShowsNotes
--- PASS: TestUpdate_NewerAvailable_PromptsAndShowsNotes (0.00s)
=== RUN   TestUpdate_ConfirmYes
--- PASS: TestUpdate_ConfirmYes (0.00s)
=== RUN   TestUpdate_ConfirmDefaultDeny
--- PASS: TestUpdate_ConfirmDefaultDeny (0.00s)
=== RUN   TestUpdate_YesFlagSkipsPrompt
--- PASS: TestUpdate_YesFlagSkipsPrompt (0.00s)
=== RUN   TestUpdate_JSONFlagIsDryRun
--- PASS: TestUpdate_JSONFlagIsDryRun (0.00s)
PASS
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.007s
```

Full suite unaffected:

```
$ go vet ./... && go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.188s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.015s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.004s
ok  	github.com/Harrison-Blair/fledge/internal/cli	0.009s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.002s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.004s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.004s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.004s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.017s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.005s
```

## AC-2

`TestUpdate_AlreadyUpToDate` (`internal/cli/update_test.go`): server reports
`tag_name: "v"+binaryVersion`; asserts exit `ExitOK`, output contains
`"already up to date"`, and does not contain the `[y/N]` prompt marker — see
the PASS line above. Confirms the up-to-date short-circuit path in
`runUpdateWith` (`internal/cli/update.go`) never reaches the prompt.

## AC-3

`TestUpdate_NewerAvailable_PromptsAndShowsNotes` (`internal/cli/update_test.go`):
server reports `tag_name: "v99.99.99"`, `body: "shiny new release notes"`;
asserts the output contains the current version (`binaryVersion`), the latest
version (`99.99.99`), the release notes text, and the `[y/N]` prompt marker —
see the PASS line above.

## AC-4

Two tests pin the confirm boundary in `internal/cli/update_test.go`:

- `TestUpdate_ConfirmYes`: fake stdin `"y\n"` → output contains
  `"update mechanics not yet implemented"` (the placeholder confirm-path
  action runs).
- `TestUpdate_ConfirmDefaultDeny`: fake stdin `"\n"` (bare Enter) → output
  does NOT contain `"update mechanics not yet implemented"` (default-deny:
  only an explicit `y`/`yes` proceeds, via the shared `promptYesNo` helper
  already used by `fledge init`).

Both PASS above. This feather never touches the installed binary at all (no
download/swap code exists yet — that's FTHR-027), so the "no changes to the
binary" half of PLM-014 AC-3 holds trivially by construction, not by a
runtime check.

## AC-5

`TestUpdate_JSONFlagIsDryRun` (`internal/cli/update_test.go`) runs `--json`
twice against the same `runUpdateWith`: once with the server reporting the
current version (up to date) and once reporting `v99.99.99` (update
available). Both runs decode the stdout as JSON into `updateJSON{Current,
Latest, UpToDate, Notes}` and assert the field values match the server
response; both assert the output never contains the `[y/N]` prompt marker.
See the PASS line above — confirms `--json` is a pure dry-run in both the
up-to-date and newer-available cases.
