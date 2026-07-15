# pi — fledge adapter

fledge's agent-neutral workflow lives at `.fledge/skills/fledge-orchestrate/` (pi discovers it via the `skills` pointer in `.pi/settings.json`). This file maps fledge's 6 orchestration primitives to pi mechanisms.

## Derived tier

**Tier {{.Tier}}** — provided: {{range $i, $p := .Provided}}{{if $i}}, {{end}}`{{$p}}`{{end}}.{{if .NotProvided}} Not provided: {{range $i, $p := .NotProvided}}{{if $i}}, {{end}}`{{$p}}`{{end}}.{{end}}

## Primitive map

| Primitive | Capability | pi mechanism | Provided | Required for |
|---|---|---|---|---|
{{- range .Rows}}
| `{{.Name}}` | {{.Desc}} | {{if .Provided}}{{.Mechanism}}{{else}}—{{end}} | {{if .Provided}}yes{{else}}no{{end}} | {{.Tier}} |
{{- end}}
{{if .PipingFile}}
## Harness piping

For team-loop runtime behavior, see `{{.PipingFile}}`.{{end}}

## Notes

- Spec mutation goes through `run-fledge` (the pi `bash` tool running `fledge …`); never hand-edit spec frontmatter the CLI can write.
- `confirm-gate` (review): present the material under review — for a spec-body draft, a summary plus the on-disk file path to open (and a diff on each revision), never the pasted body — then ask the user in chat for a structured "Accept" / "Make changes" choice (or use a `fledge_gate` extension tool if the M4 extension is installed). Loop on "Make changes" until "Accept".
- See `.fledge/skills/fledge-orchestrate/SKILL.md` for routing and ground rules. With this Tier A profile, planning foraging and implementation both run solo (in-session) per `planning.md` step 2 and `implementation.md` §2.
