# Dogfooding and Delegation
- All implementation, review, research, and testing subagents for this repository must be spawned and managed through the inherited Fledge session.
- Delegated work must use `fledge agent spawn --new-tab` so the coordinator keeps its own pane.
- Treat the inherited `HERDR_*` environment variables as the current-session identity. Never hard-code a session name, unset those variables, or substitute another session.
- Do not use native or platform subagent mechanisms when the current Fledge session is available.
- Do not run `fledge start`, directly create a Herdr server/session, or bootstrap nested orchestration for repository work.
- If the inherited session is missing or Fledge cannot reach it, stop and ask the user. Do not silently implement locally or create a replacement session.
- Tests may create isolated fake or real sessions only within their scoped lifecycle and must tear them down. Never reuse a test session to orchestrate development.
- Use the inherited `TMPDIR` for disposable artifacts; do not create ad hoc temporary directories.

# Project Overview
This is a `go` CLI tool aimed to make agentic engineering more efficient

This project seeks to:
- Add determinism to AI coding

# Single User
- I am the only user of this project, and the sole developer
- I am a visual learner, create/show graphics where relevant to explaining implimentation or ideas to me

# Slim Entrypoint
- Keep `main.go` slim, move logic into functions and other files.

# Authoring CLI Commands
- Always add a `--json` flag when applicable for machine parsable output
- Use the `--flag | -f` convention when adding flags

# Reference Docs
- `docs/reference/` — condensed refactoring.guru references (code smells, refactoring techniques, Go design patterns) for planning, refactoring, and review work

<!-- <fledge-managed-orchestrator> -->
## Fledge Orchestrator (managed)

Use only the inherited Fledge session for subagent orchestration.
Spawn subagents with `fledge agent spawn --new-tab`.
Delegate every task with `fledge agent message send <name> <task>` and let the command return after its bounded delivery handshake.
Never use direct prompts, waits, repeated status/read calls, or background polling to detect task completion.
After sending a task, continue other useful work or return control; Fledge injects replies into your pane as they arrive.
When you receive a delegated task, complete it and respond with `fledge agent message reply <message-id> <result>`; the reply both acknowledges the task and returns its result.
Use `fledge agent message ack <message-id>` only for informational messages that require no result.
After a delegated task is complete and its result has been received, run `fledge agent stop <name>` to tear down that completed agent and its dedicated tab.
Send another message only when there is genuinely new task information.
Never use harness-native subagents or start nested Fledge or Herdr sessions.
If Fledge is unavailable, stop and ask the user.
<!-- </fledge-managed-orchestrator> -->
