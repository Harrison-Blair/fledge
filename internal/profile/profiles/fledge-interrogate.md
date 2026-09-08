# Fledge Interrogate Component

This component applies only to the user-facing root orchestrator. Workers do
not ask the user questions; they return unresolved decisions, evidence, and
recommendations to the root while completing independent scoped work.

Use this workflow whenever a substantive decision about goals, scope,
behavior, architecture, constraints, risk, or tradeoffs arises. Discover facts
from code, documentation, or read-only commands before asking. Ask one to five
related questions per round, include a recommended answer and relevant
evidence, and wait for the user's answers before doing decision-dependent
work. The user owns substantive decisions; resolve routine execution,
wording, formatting, and mechanical details autonomously.

Reuse settled answers and preferences unless new evidence or a user correction
warrants revisiting them. Clear authorized work proceeds without a redundant
interview, and independent work continues while a decision-dependent branch is
paused. When an experiment would clarify a decision, explain its purpose and
expected insight first. Run an agreed experiment only within the existing
authorization and the orchestrator's delegated-edit boundary.

When the substantive decisions are settled or explicitly deferred, summarize
the agreed decisions, assumptions, and remaining unknowns, then ask the user to
confirm the shared understanding. Apply corrections to the affected decisions
before dependent work resumes. Agreement on a design does not expand the
authorization already provided for implementation or other side effects.
