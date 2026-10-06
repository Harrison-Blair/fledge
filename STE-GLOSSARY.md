# STE glossary

One Fledge term for each thing agent instructions name. The "Do not use" column
lists synonyms the instruction text still rotates for the same meaning. Each row
is grounded in a Fledge command or in `AGENTS.md`.

| Term | Meaning | Do not use |
| --- | --- | --- |
| agent | A harness process in a Herdr pane that Fledge tracks under a record id. | |
| assign | Give a created task to an agent with `fledge task assign`. | |
| brief | The Markdown text that gives a task or a role its scope. | |
| cancel | End a task without a completion, with `fledge task cancel`. | |
| check | Run the repository check suite, or test behavior yourself read-only. | confirm, validate |
| checkout | The Git working directory an agent works in. | |
| complete | Report a finished task with `fledge task complete`. | |
| memory | One durable project fact, stored with `fledge memory add`. | |
| message | Send text to another agent with `fledge agent message`. | ping |
| orchestrator | The agent that delegates every file edit and requires independent verification of it. | |
| pane | The Herdr terminal pane that one agent runs in, labeled with the agent name. | |
| profile | A named role brief, built in or from `.fledge/profiles`. | |
| send | Type raw input or keys into an agent with `fledge agent send`. | |
| spawn | Start a new agent with `fledge agent spawn`. | launch |
| stop | End a live agent with `fledge agent stop`. | retire |
| tab | The Herdr tab that holds panes, labeled at spawn time. | |
| task | One unit of tracked work with a brief, created by `fledge task create`. | work unit |
| verify | Record a passed independent check with `fledge task verify`. | |
| worker | An agent that another agent spawned. | sub-agent, subagent |
| worktree | The managed checkout that `fledge worktree create` makes and `--worktree` names. | |
