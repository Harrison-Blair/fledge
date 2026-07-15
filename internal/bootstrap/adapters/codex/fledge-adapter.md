# Codex CLI — fledge adapter

fledge's agent-neutral workflow lives at `.fledge/skills/fledge-orchestrate/`. This file maps fledge's 6 orchestration primitives to OpenAI Codex CLI mechanisms.

## Derived tier

**Tier {{.Tier}}** — provided: {{range $i, $p := .Provided}}{{if $i}}, {{end}}`{{$p}}`{{end}}.{{if .NotProvided}} Not provided: {{range $i, $p := .NotProvided}}{{if $i}}, {{end}}`{{$p}}`{{end}}.{{end}}

## Primitive map

| Primitive | Capability | Codex mechanism | Provided | Required for |
|---|---|---|---|---|
{{- range .Rows}}
| `{{.Name}}` | {{.Desc}} | {{if .Provided}}{{.Mechanism}}{{else}}—{{end}} | {{if .Provided}}yes{{else}}no{{end}} | {{.Tier}} |
{{- end}}
{{if .PipingFile}}
## Harness piping

For team-loop runtime behavior, see `{{.PipingFile}}`.{{end}}

## Notes

- Spec mutation goes through `run-fledge` (Codex shell running `fledge …`); never hand-edit spec frontmatter the CLI can write.
- `confirm-gate` (review): present the material under review — for a spec-body draft, a summary plus the on-disk file path to open (and a diff on each revision), never the pasted body — then ask the user in chat for a structured "Accept" / "Make changes" choice; loop on "Make changes" until "Accept".
- See `.fledge/skills/fledge-orchestrate/SKILL.md` for routing and ground rules. With this Tier A profile, planning foraging and implementation both run solo (in-session) per `planning.md` step 2 and `implementation.md` §2.
- Codex auto-loads `AGENTS.md` at the repo root; `fledge init` adds a one-line pointer there (additively — it never overwrites your existing `AGENTS.md`).
