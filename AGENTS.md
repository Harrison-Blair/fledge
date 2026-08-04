# Delegation Model
Prefer delegating implimentation and review to sub-agents. Before delegating, ask if:
    - The seperate tasks do not have overlapping surface-area
    - The tasks can be done by both agents without communicating with one another
    - The tasks do not rely one one another in any way

If delegating, you are to act as purely an orchestrator. Do not complete tasks yourself, instead spawn seperate agents to complete tasks and review work.

Create new agents for different tasks, and for different review sessions.

# Agent Coordination
- Communicate with spawned agents through Fledge messages. Treat an agent's message reply as its completion signal.
- Do not use `herdr agent wait` or `herdr agent read` to poll for completion or collect results.
- After receiving an agent's final reply, stop it with `fledge agent stop <name>` before reporting its result to the user.
- If an agent fails or is no longer needed, stop it before finishing the task.
- Never run `fledge start` or `fledge stop`; session lifecycle remains under direct user control.

# Completion is Non-Breaking
Ensure the full test suite and build are running without error before determining a task is done.

# Write testable code
- Separate logic from side effects. Keep business logic in pure functions; inject IO, network, clock, and randomness at the edges.
- Ship tests with the code. Any new module or nontrivial function gets tests in the same change, not as a follow-up.
- If something is hard to test, treat it as a design smell. Refactor for testability rather than reaching for heavy mocks.
- Keep units deterministic: explicit inputs and outputs, no hidden global state, no reliance on wall-clock time or environment unless injected.
