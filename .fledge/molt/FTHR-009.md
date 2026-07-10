# FTHR-009 Evidence

## AC-1

Tests observed FAILING against unchanged code before implementation.

### Commands run

```
go test ./internal/bootstrap/ -run "TestStampRoundTrip|TestStampAbsent|TestStampDeterministic|TestExpectedFilesCoversAllPolicies|TestRenderEntryMatchesWritePath"
```

### Output (FAIL — types/functions undefined)

```
# github.com/Harrison-Blair/fledge/internal/bootstrap [github.com/Harrison-Blair/fledge/internal/bootstrap.test]
internal/bootstrap/stamp_test.go:14:11: undefined: Stamp
internal/bootstrap/stamp_test.go:17:21: undefined: StampEntry
internal/bootstrap/stamp_test.go:41:14: undefined: LoadStamp
internal/bootstrap/stamp_test.go:81:14: undefined: LoadStamp
internal/bootstrap/stamp_test.go:93:8: undefined: Stamp
internal/bootstrap/stamp_test.go:96:21: undefined: StampEntry
internal/bootstrap/stamp_test.go:101:13: undefined: marshalStamp
internal/bootstrap/stamp_test.go:105:13: undefined: marshalStamp
internal/bootstrap/stamp_test.go:125:16: undefined: ExpectedFiles
internal/bootstrap/stamp_test.go:131:20: undefined: stampPath
internal/bootstrap/stamp_test.go:131:20: too many errors
FAIL	github.com/Harrison-Blair/fledge/internal/bootstrap [build failed]
FAIL
```

### Commands run (txtar — init.txtar)

```
go test ./cmd/fledge -run "TestScripts/init$"
```

### Output (FAIL — stamp not written)

```
FAIL: testdata/init.txtar:29: no match for `created .fledge/scaffold.json` found in stdout
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.004s
```

### Commands run (txtar — init_agents.txtar)

```
go test ./cmd/fledge -run "TestScripts/init_agents"
```

### Output (FAIL — stamp file does not exist)

```
FAIL: testdata/init_agents.txtar:24: $WORK/pi-repo/.fledge/scaffold.json does not exist
FAIL	github.com/Harrison-Blair/fledge/cmd/fledge	0.006s
```

All tests failed for the expected reason: `Stamp`, `StampEntry`, `LoadStamp`, `marshalStamp`,
`ExpectedFiles`, `stampPath` are not yet defined, and `fledge init` does not yet write
`.fledge/scaffold.json`.

## AC-2

`fledge init` writes `.fledge/scaffold.json` containing the binary version, the
scaffolded agents, and a manifest entry for every file it wrote (excluding itself).

Verified by `go test ./cmd/fledge -run TestScripts/init` — the txtar script asserts:
```
stdout 'created .fledge/scaffold.json'
exists .fledge/scaffold.json
grep '"fledgeVersion"' .fledge/scaffold.json
grep '".fledge/skills/fledge-orchestrate/SKILL.md"' .fledge/scaffold.json
grep '".claude/fledge-adapter.md"' .fledge/scaffold.json
grep '"claude"' .fledge/scaffold.json
```

Example output (from the txtar trace):
```
".fledge/scaffold.json" written with fledgeVersion, agents: ["claude"],
files map containing every scaffolded path with policy+sha256/target/lines.
```

Agents union verified by `TestScripts/init_agents`: after `fledge init --agent claude,pi`
then `fledge init --agent codex`, the stamp agents field contains all three:
`grep '"codex"' .fledge/scaffold.json` ✓
`grep '"claude"' .fledge/scaffold.json` ✓
`grep '"pi"' .fledge/scaffold.json` ✓

## AC-3

Second `fledge init` leaves the stamp byte-identical (idempotence).

Verified by init.txtar second-run block:
```
exec fledge init
stdout 'exists .fledge/scaffold.json'
! stdout 'created'
! stdout 'updated'
```

The `! stdout 'updated'` assertion covers the stamp file specifically — byte-identical
content means `writeIfChanged` skips the write and the stamp goes into skipped.

Command + output (after implementation):
```
go test ./cmd/fledge -run TestScripts/init -v
--- PASS: TestScripts/init (0.01s)
```

## AC-4

`ExpectedFiles` output matches the write path byte-for-byte; `go test ./...` and
`go vet ./...` pass.

Verified by `TestRenderEntryMatchesWritePath` in `internal/bootstrap/stamp_test.go`:
for each content-bearing file in the claude adapter (generate, primitive_map, overwrite,
default), `renderEntry` bytes equal what `writeFileEntry` actually writes to disk.

```
go test ./internal/bootstrap/ -run TestRenderEntryMatchesWritePath -v
--- PASS: TestRenderEntryMatchesWritePath (0.00s)
    --- PASS: TestRenderEntryMatchesWritePath/.claude/agents/fledge-brooder.md (0.00s)
    --- PASS: TestRenderEntryMatchesWritePath/.claude/agents/fledge-forager.md (0.00s)
    --- PASS: TestRenderEntryMatchesWritePath/.claude/agents/fledge-context-scout.md (0.00s)
    --- PASS: TestRenderEntryMatchesWritePath/.claude/agents/fledge-skua.md (0.00s)
    --- PASS: TestRenderEntryMatchesWritePath/.claude/settings.json (0.00s)
    --- PASS: TestRenderEntryMatchesWritePath/.claude/settings.local.json (0.00s)
    --- PASS: TestRenderEntryMatchesWritePath/.claude/team-loop.md (0.00s)
    --- PASS: TestRenderEntryMatchesWritePath/.claude/fledge-adapter.md (0.00s)
```

Full suite:
```
go test ./... && go vet ./...
ok  github.com/Harrison-Blair/fledge/cmd/fledge	0.062s
ok  github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
ok  github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  github.com/Harrison-Blair/fledge/internal/cli	0.002s
ok  github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  github.com/Harrison-Blair/fledge/internal/lock	0.001s
ok  github.com/Harrison-Blair/fledge/internal/nest	0.001s
ok  github.com/Harrison-Blair/fledge/internal/scan	0.008s
ok  github.com/Harrison-Blair/fledge/internal/spec	0.002s
(vet: no output)
```
