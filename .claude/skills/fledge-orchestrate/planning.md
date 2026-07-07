# Planning phase

Turns a feature request into approved requirements and implementable tasks, grounded in fresh repository context. Four steps, in order. The user is interrogated throughout — this phase runs in the main session and must stay interactive; only context gathering is delegated.

## 1. Freshness gate

- If `.fledge/context/index.md` does not exist → go to step 2.
- Otherwise compare the `commit` in its frontmatter to `git rev-parse HEAD`:
  - Equal → context is fresh; skip to step 3.
  - Different → summarize the staleness (`git log --oneline <commit>..HEAD`: how many commits, which areas changed) and ask the user (AskUserQuestion): regenerate context, or proceed with existing context. Respect their choice.

## 2. Gather context (when needed)

Spawn the `fledge-context-gatherer` agent (it self-orchestrates `fledge-context-scout` subagents; see `.claude/agents/fledge-context-gatherer.md`). Wait for completion, then verify `.fledge/context/index.md` exists and its `commit` matches HEAD before continuing. Relay the gatherer's coverage notes to the user.

## 3. Requirements interrogation

1. Read `.fledge/context/index.md`; load the concern docs whose `Read this when:` lines match the feature request (typically `modules.md`, `architecture.md`, `domain.md`).
2. If the request contains multiple features, split it into separate requirements, one per concern — each REQ must stand alone with its own user stories and criteria. Present the proposed breakdown (titles + one-line scopes) and run a confirmation gate on it (Accept / Make changes — see the SKILL.md ground rules) before authoring anything.
3. Author requirements **one at a time**. For the current requirement only, run the interrogate protocol from `.claude/skills/interrogate/SKILL.md`: one question at a time, recommended answer first, facts looked up rather than asked, every decision put to the user. Walk the branches: scope and motivation, user stories, functional criteria, acceptance criteria, out-of-scope, priority.
4. When that requirement's tree is resolved, create the file with `fledge new req --title "<title>" --priority <P0-P3>` (it allocates the next ID, names the file, and fills the frontmatter), then write the interrogation's outcome into the body sections. Show the user the full file and run a confirmation gate (Accept / Make changes). On "Make changes", revise and re-present in full until accepted; on "Accept", run `fledge status REQ-### approved`.
5. Only then move to the next requirement in the breakdown and repeat from 3. Do not proceed to tasks until every requirement in the breakdown is approved (or the user defers some explicitly).

## 4. Task interrogation

Run this step once per approved requirement, completing one requirement's tasks before starting the next.

1. For the current requirement, continue interrogating — still one question at a time — over the decomposition: task boundaries, ordering and blocking dependencies, priorities, which modules each task touches (cite the context docs; load more of them as needed), how each task's behavior will be tested (framework, test location, what each test pins down), and whether any task needs human oversight during implementation (`oversight: during` — the user participates while the task is built, with the orchestrator relaying decision checkpoints between the implementor and the user, since the strict topology gives the implementor no direct user channel; `oversight: merge` — the user signs off on the reviewed diff before it merges; omitted — fully autonomous).
2. Structure the decomposition around **tracer bullets**: the first task(s) build a thin, working end-to-end slice through every layer the feature touches — minimal but real and verifiable, proving the architecture — and later tasks widen that slice (more cases, robustness, polish). Prefer this over layer-by-layer tasks that only integrate at the end; each completed task should leave the system demonstrably working. Make the tracer slice the root of the `depends_on` graph.
3. Decompose for **parallel implementation**: wherever the tracer-bullet ordering allows, shape tasks so independent subagents can implement them concurrently — disjoint files/modules per task, explicit interfaces (types, function signatures, file contracts) defined at the boundaries so parallel work composes, and `depends_on` reserved for true ordering constraints rather than shared-file conflicts. When two candidate tasks would touch the same files, either merge them or move the shared surface into an earlier task both depend on.
4. Every task must be test-driven and its design testable. Each TASK file's Tests section names the tests that prove its behavior, and its acceptance criteria require the test-first cycle: tests written first, observed FAILING against the unchanged code for the expected reason, then the implementation corrected until they pass. Reject task boundaries whose behavior can't be pinned down by a test, and shape the Approach so the code exposes the seams those tests need.
5. Propose the decomposition as an outline (task titles + dependency graph, flagging which tasks can run in parallel), refine it through the interrogation, and close with a confirmation gate (Accept / Make changes) on the final shape.
6. Author the tasks **one at a time**, in dependency order: create each with `fledge new task --title "<title>" --req REQ-### [--depends-on TASK-a,TASK-b] [--priority <P>] [--oversight merge|during]` (it allocates the ID, links the requirement, and sets the initial ready/blocked hint from the dependency statuses — that hint is authoring-time only; the implementation phase recomputes dispatch readiness from `depends_on` completion and never writes `blocked`→`ready` back). Then write the interrogation's outcome into the body sections (Description, Affected Modules, Approach, Tests, Acceptance Criteria — criteria are authored as unchecked `- [ ] AC-N: …` boxes and only ever checked via `fledge criteria` during implementation or requirement closeout), show the user the full file, and run a confirmation gate (Accept / Make changes); loop on "Make changes" before writing the next task. To adjust an existing task's frontmatter, use `fledge set` (priority, oversight, depends_on, title) — never hand-edit fields the CLI can write.
7. After the last task, run `fledge check` and fix every finding before closing. Close by listing the created files, the dependency waves (`fledge graph`), and the ready-to-start tasks (`fledge ready`). Offer to start the implementation phase (`implementation.md`) on the ready tasks.
