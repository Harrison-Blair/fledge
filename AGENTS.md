# Delegation Model
Prefer delegating implimentation and review to sub-agents. Before delegating, ask yourself if:
    - The seperate tasks do not have overlapping surface-area
    - The tasks can be done by both agents without communicating with one another
    - The tasks do not rely one one another in any way

If delegating, you are to act as purely an orchestrator. Do not complete tasks yourself, instead spawn seperate agents to complete tasks and review work.

Create new agents for different tasks, and for different review sessions.

# Agent Coordination
- Communicate with spawned agents through Fledge messages. A worker signals completion by transitioning its task (`task complete`/`task fail`); that transition wakes you with the worker's summary in the body. Do not expect — or instruct — a separate completion message; a duplicate message carrying the same summary is redundant. Treat a plain `message` wake as a question or a progress note, not a completion signal.
- Give a worker its first assignment with `fledge agent spawn --task <text>`. Add `--can-delegate` only when that worker may create child tasks, and `--parent-task <id>` when delegating from an assignment you already hold.
- Track work with `fledge agent list` and `fledge agent task assign/progress/blocked/needs-decision/resume/complete/fail/cancel/list/show`. Every verb accepts `--file` for text shell quoting cannot carry.
- Task commands append durable events and return; the session dispatcher wakes the right participant. Progress is recorded without waking anyone. Ordinary messages always wake their recipient.
- Do not use `herdr agent wait` or `herdr agent read` to poll for completion or collect results.
- Never author or run `sleep`, shell `wait`, polling loops, or repeated status commands to await worker updates or task completion. After delegating, yield control; Fledge will wake you when an update requires attention.
- After receiving an agent's final reply, and once its task is terminal, stop it with `fledge agent stop <name>` before reporting its result to the user.
- If an agent fails or is no longer needed, stop it before finishing the task.
- Never run `fledge start` or `fledge stop`; session lifecycle remains under direct user control.

# Completion is Non-Breaking
Ensure the full test suite and build are running without error before determining a task is done.

Ensure `./scripts/build.sh` is running without error

# Write testable code
- Separate logic from side effects. Keep business logic in pure functions; inject IO, network, clock, and randomness at the edges.
- Ship tests with the code. Any new module or nontrivial function gets tests in the same change, not as a follow-up.
- If something is hard to test, treat it as a design smell. Refactor for testability rather than reaching for heavy mocks.
- Keep units deterministic: explicit inputs and outputs, no hidden global state, no reliance on wall-clock time or environment unless injected.
