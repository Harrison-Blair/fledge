# Claude Code — fledge adapter

fledge's agent-neutral workflow lives at `.fledge/skills/fledge-orchestrate/` (Claude Code discovers it via the `skills` pointer in `.claude/settings.json`). This file maps fledge's 6 orchestration primitives to Claude Code mechanisms.

## Derived tier

**Tier {{.Tier}}** — provided: {{range $i, $p := .Provided}}{{if $i}}, {{end}}`{{$p}}`{{end}}.{{if .NotProvided}} Not provided: {{range $i, $p := .NotProvided}}{{if $i}}, {{end}}`{{$p}}`{{end}}.{{end}}

## Primitive map

| Primitive | Capability | Claude mechanism | Provided | Required for |
|---|---|---|---|---|
{{- range .Rows}}
| `{{.Name}}` | {{.Desc}} | {{if .Provided}}{{.Mechanism}}{{else}}—{{end}} | {{if .Provided}}yes{{else}}no{{end}} | {{.Tier}} |
{{- end}}
{{if .PipingFile}}
## Harness piping

For Tier C team-loop runtime behavior (tmux display, `/resume` recovery, permission inheritance, team task list), see `{{.PipingFile}}`.{{end}}

## Notes

- Spec mutation goes through `run-fledge` (`Bash(fledge …)`); never hand-edit spec frontmatter the CLI can write.
- See `.fledge/skills/fledge-orchestrate/SKILL.md` for routing and ground rules, and `implementation.md` for the phase that matches this tier.
