# FTHR-030 Evidence

## AC-1

Before/after evidence for the `Bash(fledge update *)` allow-list entry.

### Before

```
$ /tmp/fledge-030 version
fledge 0.5.4
$ cat VERSION
0.5.4

$ grep -c 'fledge update' .claude/settings.local.json
ugrep: warning: .claude/settings.local.json: No such file or directory
(exit 2 — file does not exist in this fresh worktree; .claude/settings.local.json
is gitignored globally (~/.config/git/ignore) and thus never copied into a new
git worktree, only tracked files are. This satisfies the "absent before refresh"
condition equivalently to a 0 count.)

$ /tmp/fledge-030 preen
...
WARN  .claude/settings.local.json: scaffold file is missing — run fledge init --refresh
WARN  .fledge/nest/raw/.gitkeep: scaffold file is missing — run fledge init --refresh
8 warning(s)
(exit 0; no stamp-mismatch warning — only "file missing" warnings, consistent
with the spec's expectation that the stamp itself is already current.)
```

### After

```
$ /tmp/fledge-030 init --refresh
note: refreshed 1 file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.
created .fledge/nest/raw/.gitkeep
created .claude/settings.local.json
updated .fledge/scaffold.json
exists .fledge/broods/.gitkeep
... (all other fledge-owned files: "exists", unchanged)
scaffolded agents: claude

$ grep -c 'fledge update' .claude/settings.local.json
1

$ cat .claude/settings.local.json
{
  "permissions": {
    "allow": [
      ...
      "Bash(fledge broods *)",
      "Bash(fledge version *)",
      "Bash(fledge update *)"
    ]
  }
}

$ /tmp/fledge-030 preen
WARN  .fledge/pluma/feathers/FTHR-029-...: checked criteria missing evidence sections in .../FTHR-029.md: AC-1, AC-2, AC-3, AC-4
WARN  .fledge/pluma/feathers/FTHR-032-...: checked criteria missing evidence sections in .../FTHR-032.md: AC-1, AC-2, AC-3, AC-4
WARN  .fledge/pluma/feathers/FTHR-033-...: checked criteria missing evidence sections in .../FTHR-033.md: AC-1, AC-2, AC-3, AC-4
WARN  .fledge/pluma/feathers/FTHR-034-...: checked criteria missing evidence sections in .../FTHR-034.md: AC-1, AC-2, AC-3, AC-4
WARN  .fledge/pluma/feathers/FTHR-035-...: checked criteria missing evidence sections in .../FTHR-035.md: AC-1, AC-2, AC-3, AC-4
WARN  .fledge/pluma/feathers/FTHR-030-...: status hatching but no brood is held
6 warning(s)
exit=0
```

No `scaffold file is missing` warnings remain for `.claude/settings.local.json` or
`.fledge/nest/raw/.gitkeep` (both were created by the refresh). No stamp-mismatch
warning was printed before or after — the remaining 6 warnings are pre-existing,
unrelated to this feather (missing evidence sections in other feathers' molt files,
and the routine "no brood held" note for this feather's own spec, which is
orchestrator-owned state, not scaffold state).

## AC-2

`.claude/settings.local.json` (gitignored, not committed to git — see AC-3) contains
the `Bash(fledge update *)` entry after regeneration, confirmed above via
`grep -c 'fledge update' .claude/settings.local.json` → `1`, and the full file
listing shows the entry as the last allow-list item.

## AC-3

`.fledge/scaffold.json` diff after `--refresh`:

```
$ git diff .fledge/scaffold.json
diff --git a/.fledge/scaffold.json b/.fledge/scaffold.json
index 7108e76..b02dc75 100644
--- a/.fledge/scaffold.json
+++ b/.fledge/scaffold.json
@@ -34,7 +34,7 @@
     },
     ".claude/settings.local.json": {
       "policy": "generate",
-      "sha256": "957b8dfb49a596cc308630a89a9803eec58a6d6ae6ade325ef9a483a4aa1aa12"
+      "sha256": "a0de69d0313ebce586cec612f6e13fa4022cd2fb93b01b3d18ed4c67d343b71f"
     },
     ".claude/skills/fledge-interrogate": {
       "policy": "symlink",
```

Only the content hash for `.claude/settings.local.json` changed (reflecting the new
`fledge update` entry); `fledgeVersion` remains `0.5.4`, unchanged — the stamp was
already current, as expected. `git status` shows no other fledge-owned file changed
(the two "created" files above — `.claude/settings.local.json` and
`.fledge/nest/raw/.gitkeep` — are both globally gitignored per
`~/.config/git/ignore`, so they never appear as git changes; this matches the
main repo's working tree, where these same two paths are also untracked/ignored).
No `fledge` command (`preen`, `init --refresh`) printed a stamp-mismatch warning,
per the AC-1 before/after captures above.

## AC-4

```
$ /tmp/fledge-030 preen
... (6 warnings, all pre-existing/unrelated — see AC-1 "After")
exit=0

$ go test ./...
ok  	github.com/Harrison-Blair/fledge/cmd/fledge	0.074s
ok  	github.com/Harrison-Blair/fledge/internal/bootstrap	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/check	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/ciconfig	0.003s
ok  	github.com/Harrison-Blair/fledge/internal/cli	4.011s
ok  	github.com/Harrison-Blair/fledge/internal/doctest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/graph	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/hooktest	0.120s
ok  	github.com/Harrison-Blair/fledge/internal/lock	0.005s
ok  	github.com/Harrison-Blair/fledge/internal/nest	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/repo	0.001s
ok  	github.com/Harrison-Blair/fledge/internal/scan	0.007s
ok  	github.com/Harrison-Blair/fledge/internal/spec	0.007s
```

`preen` exits 0 with only pre-existing/unrelated warnings (none about scaffold
drift or stamp mismatch), and the full test suite is green.
