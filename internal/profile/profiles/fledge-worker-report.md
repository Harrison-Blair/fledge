# Fledge Report Protocol

This protocol has two audiences. As a worker, use it for your final report.
As a manager, require this envelope from every worker you dispatch and
correlate each incoming callback; forwarding or receiving a worker callback
does not replace your own report to the user.

## Final callback

A worker's final action is exactly one Fledge message to the callback target
from the brief, with the complete report atomically quoted as one argument
and without `--wait`:

```sh
fledge agent message <callback-target> '<complete report>'
```

Copy the task ID, dispatch ID, role, attempt, and agent name verbatim from
the initial brief. Perform no inline completion after the callback. A prompt
acknowledgement is not completion; only the correlated report is.

## Envelope

```text
FLEDGE REPORT | task=<task-id> | dispatch=<dispatch-id> | role=<role> | attempt=<number> | agent=<agent-name> | outcome=<pass|reject|blocked|failed>
Claim: <what was done or found>
Evidence: <commands, output, and file:line references>
Reasoning: <how the evidence supports the conclusion, with assumptions and tradeoffs>
Verdict: <required for reviewers; otherwise n/a>
Forks: <decisions for the user, or none>
Omissions: <what was not done>
```

`pass` means the goal was met and the evidence supports it. `reject` is a
reviewer's rejecting verdict. `blocked` means required scope, authority, or
input is missing. `failed` means the attempt did not achieve the goal. A
report is not confidential; never place secrets in it. A stale, duplicate,
malformed, or coordinate-mismatched callback changes no task state and is
handled as a transport problem.

## Delivery failure

If the callback fails or delivery is unconfirmed, do not claim the report was
delivered and do not retry automatically, because a retry can deliver a
duplicate. The agent remains available for manual recovery.
