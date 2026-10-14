# Review units: task and pull request sizes

**Date:** 2026-10-13. Repository data is from `dev` at `621a7af`. Every web source was retrieved on
2026-10-13; the access date below is that date unless a source says otherwise.

## Purpose

Fix how much change one Fledge task and one feature pull request may carry, so that one verifier
pass holds a task and one human sitting holds a pull request. The note recommends four things,
each with its derivation: a task ceiling, a pull request target and ceiling, guard rails beyond
lines (files changed and domains touched), and a decision rule that tells an orchestrator, from a
wave plan, whether a set of tasks ships as one feature pull request, a stack, or separate pull
requests. The governing principle is the tracer bullet: a reviewable unit is one end-to-end change
a user could name. Tooling is out of scope; a later planning round takes it up. The decisions
section at the end records what the owner settled.

## Recommendations at a glance

| Limit | Value | Basis (derived below) |
| --- | --- | --- |
| Task ceiling | 300 hand-written source lines | A task must fit one normal sitting alone. 400 reviewed lines per review (SmartBear) / 2.2 reviewed lines per source line (this repository) = 182. Policy relaxed it to 200 (440 reviewed lines, 53 minutes at 500 lines/hour). The owner decision of 2026-10-14 raised it to 300: 300 x 2.2 = 660 reviewed lines, 79 minutes at 500 lines an hour |
| Pull request target | 200 source lines | One normal sitting: the 200-line derivation in the task ceiling row (440 reviewed lines, 53 minutes at 500 lines an hour) |
| Pull request ceiling | 300 source lines | 90-minute exception x 500 lines/hour = 750 reviewed lines; / 2.2 = 341; rounded down to 300 so that pull requests up to a 2.5 ratio still fit |
| Files changed (excluding raw captures and `testdata/`) | Task and pull request target 20, pull request ceiling 30 | Policy choice: Google's 10-file mark for 90% of changes, doubled for this repository's source-plus-test file pairs; Google's style guide calls a 50-file spread "usually too large" |
| Domains touched | Task and pull request target 3, pull request ceiling 5 code domains | This repository: 75th percentile of 2 code domains per task; Microsoft: fewer files, a higher share of useful review comments (a direction; the paper gives no number) |
| Sitting | 30 to 60 minutes; up to 90 when the change warrants it | Owner decision; SmartBear's 60 to 90 minute guidance |

Lines are additions plus deletions. Every limit is a planning limit, applied before implementation
from estimates. The one exception class is mechanical change (see the decision rule).

## What counts as a line

**Hand-written source** for this repository is:

- non-test Go: `*.go` except `*_test.go` and anything under a `testdata/` directory;
- built-in profiles, `internal/lib/profiles/builtin/*.md`, because they ship agent behavior as text;
- `.github/scripts/*.py` except `*_test.py`;
- `.github/workflows/*.yml`.

Everything else is reviewed but not counted: Go and Python tests, `testdata/` fixtures, user
documentation (`README.md`, `AGENTS.md`, `reference/**`), and Herdr raw captures. The reasons:

- **Tests scale with source.** In the 26 verified tasks with at least 50 source lines, the median
  ratio of all changed lines to source lines is 2.16 (quartiles 1.7 / 2.2 / 2.8), and of source
  plus test lines to source lines 2.13. A source budget therefore budgets the test read through a
  stable multiplier, and the limits above apply that multiplier (2.2) instead of counting tests
  twice. Counting tests directly would also penalize the test-first rule in `AGENTS.md`.
- **Documentation is erratic and is prose.** Per-task Markdown runs from 0 to 2,097 lines, and the
  PR #16 program has a 3,912-line documentation commit with no source at all (`0b6fc49`). A
  documentation change is read for accuracy, not for control flow, and a docs-only pull request
  stays bounded by the files guard.
- **Fixtures and captures are data, not authored logic.** `reference/herdr/raw/` holds tool
  output kept by `RESYNC.md`, and `testdata/` holds inputs.

These excluded files still appear in the full diff, and the agent gate and the human read both
cover them. The owner settled the exclusion on 2026-10-13.

### Per-task data under this definition

Dataset: the 81 first-parent merge commits on `dev` whose subject contains `verified task`, from
`git log dev --merges --first-parent --format='%h %s' | grep 'verified task'`, each measured with
`git diff --numstat <h>^1 <h>`. The dataset excludes older merges whose subject says
`(task <id>)` without `verified` (for example `536fe69`) and direct commits to `dev` (for
example `c7b2cc0`, `010b799`). Quartiles use Python's `statistics.quantiles(n=4,
method='inclusive')`.

| Measure | Q1 / median / Q3 | Max |
| --- | --- | --- |
| All lines | 21 / 89 / 166 | 5,854 |
| Hand-written source (this definition) | 0 / 20 / 67 | 2,892 |
| Tests (Go and Python tests, `testdata/`) | 3 / 32 / 89 | 2,793 |
| Markdown not counted as source | 0 / 0 / 4 | 2,097 |
| Files changed, all files | 2 / 4 / 9 | 183 |
| Files changed, guard-eligible (no `reference/herdr/raw/`, no `testdata/`) | 2 / 3 / 9 | 183 |
| Code domains, `cmd`/`internal` pair as one | 1 / 1 / 2 | 18 |
| Code domains, pair counted as two | 1 / 1 / 3 | 22 |
| All areas, including `reference/` areas | 1 / 2 / 3 | 18 |

The brief's earlier figures counted only non-test Go (0 / 19 / 67, max 2,883) and all Markdown as
documentation (Q3 6). The move of built-in profiles into source accounts for the difference.
27 of the 81 tasks change no source line. By source lines: 55 tasks at 50 or under, 19 at 51 to
150, 4 at 151 to 300, none at 301 to 750, and 3 over 750.

## Evidence

**SmartBear, "Best Practices for Code Review" (secondary source for Cohen 2006).**
<https://smartbear.com/learn/code-review/best-practices-for-peer-code-review/>, accessed
2026-10-13. The page attributes its guidance to "a SmartBear study of a Cisco Systems programming
team". It says developers "should review no more than 200 to 400 lines of code (LOC) at a time";
that "a review of 200-400 LOC over 60 to 90 minutes should yield 70-90% defect discovery"; that
performance "starts dropping off after about 60 minutes"; and that "SmartBear research shows a
significant drop in defect density at rates faster than 500 LOC per hour". The page does not cite
Cohen's book by name and gives no study size. This note takes three numbers from it: 400 reviewed
lines per review, 500 lines per hour as a maximum rate, and 60 to 90 minutes per sitting. The book
itself is under "Not verified".

**Sadowski, Söderberg, Church, Sipko, Bacchelli, "Modern Code Review: A Case Study at Google",
ICSE-SEIP 2018.** Full text: <https://sback.it/publications/icse2018seip.pdf>; listing:
<https://research.google/pubs/modern-code-review-a-case-study-at-google/>; both accessed
2026-10-13. The study covers 12 interviews, a 44-person survey, and logs of 9 million reviewed
changes. "Over 35% of the changes under consideration modify only a single file and about 90%
modify fewer than 10 files. Over 10% of changes modify only a single line of code, and the median
number of lines modified is 24." The median reviewer count is 1. Median time to first feedback is
"under an hour for small changes and about 5 hours for very large changes"; the overall median
review latency is under 4 hours. Comments per change grow with lines changed, "reaching a peak of
12.5 comments per change for changes of about 1250 lines". Developers "are strongly encouraged to
make small, incremental changes (with the exception of large deletions and automated
refactoring)". This note takes the 10-file figure for the files guard and the deletion and
refactoring exception for the mechanical-change rule.

**Rigby and Bird, "Convergent Contemporary Software Peer Review Practices", ESEC/FSE 2013.**
Course copy: <https://users.ece.utexas.edu/~miryung/teaching/EE379K-Spring2014/Papers/Paper%204.pdf>,
accessed 2026-10-13. The copy has no title block (its header reads "PAPER 4"); its content matches
the paper's abstract (Android, Chromium OS, Bing, Office, MS SQL, AMD, Lucent, six OSS projects)
and the figures Sadowski et al. cite from it. "Convergent Practice 3: Change sizes are small."
Android and AMD have a median change of 44 lines, Apache 25, Linux 32, Lucent inspections 263;
"Chrome's median change is 78 lines and includes 5 files". Contemporary review "is performed
regularly and quickly just before the code is committed instead of when a larger work product is
complete as in inspection", and "usually involves two reviewers". This note uses it to show that
a 200-line limit sits well above the median of every studied industrial project and below
traditional inspection.

**Bosu, Greiler, Bird, "Characteristics of Useful Code Reviews: An Empirical Study at Microsoft",
MSR 2015.** Listing: <https://www.microsoft.com/en-us/research/?p=168510>, accessed 2026-10-13.
Full text: <https://www.microsoft.com/en-us/research/wp-content/uploads/2016/02/bosu2015useful.pdf>
(HTTP 200, an 11-page PDF of the paper); publication page:
<https://www.microsoft.com/en-us/research/publication/characteristics-of-useful-code-reviews-an-empirical-study-at-microsoft/>
(HTTP 200); both accessed 2026-10-13. From 1.5 million review comments in five Microsoft projects,
the abstract says: "the more files that are in a change, the lower the proportion of comments in
the code review that will be of value to the author of the change." Section VI-B1, "Do larger code
reviews (i.e., with higher number of files) get less useful comments?", says: "The trendline shows
that as number of files in the change increases, the proportion of comments that are useful
drops." Figure 8 (PDF page 9), "Usefulness density vs. Number of files", plots one series for each
of the five projects (Azure, Bing, Exchange, Office, Visual Studio) over 1 to 40 files. In
paraphrase: the series are noisy, most points lie between about 60 and 70 percent useful, the
decline is gradual with no sharp cutoff at any file count, Bing's series starts near 63 percent and
rises over the first files, and the figure draws no trendline. The paper's explanation, in section
VI-B1: with more files "a thorough review takes more time and effort", so "reviewers may opt for
cursory review of large changesets and may miss some changes", which may lead "to false positives
or more questions to the author in an effort to understand the change". The paper gives no
file-count threshold. This note takes a direction from it: fewer files, a higher share of useful
comments. That supports a files guard in principle; the guard's numbers come from this
repository's data and Google's remarks.

**Forsgren, Humble, Kim, *Accelerate* (2018), on batch size.** Publisher excerpt:
<https://itrevolution.com/wp-content/uploads/2022/06/ACC_excerpt.pdf>, Chapter 2, accessed
2026-10-13. "Reducing batch sizes reduces cycle times and variability in flow, accelerates
feedback, reduces risk and overhead, improves efficiency, increases motivation and urgency, and
reduces costs and schedule growth (Reinertsen 2009, Chapter 5)." Because batch size has "no
visible inventory" in software, the authors "settled on deployment frequency as a proxy for batch
size". They add that small batches matter because they let a team "gather user feedback quickly".
DORA's current capability page, <https://dora.dev/capabilities/working-in-small-batches/>
(accessed 2026-10-13), says small batches predict "software delivery performance and
organizational performance", and that "any batch of code that takes longer than a week to
complete and check is too big". This note uses them for the direction (smaller units to `dev`),
not for a line number; neither source gives one.

**Google engineering practices, "Small CLs".**
<https://google.github.io/eng-practices/review/developer/small-cls.html>, accessed 2026-10-13.
The right size is "one self-contained change"; "100 lines is usually a reasonable size for a CL"
and "1000 lines is usually too large"; "A 200-line change in one file might be okay, but spread
across 50 files it would usually be too large." On stacking: write a small CL, send it, and
"immediately start writing another CL" based on it. It also names splitting "vertically" into
"smaller, full-stack, vertical features" and "horizontally" with "shared code or stubs". The
vertical split is the tracer-bullet split; the 50-file remark bounds the files ceiling.

**Stacked changes at Google and Meta (secondary).** Gergely Orosz, "Stacked Diffs (and why you
should know about them)", The Pragmatic Engineer, 2023-10-17,
<https://newsletter.pragmaticengineer.com/p/stacked-diffs>, accessed 2026-10-13: "Stacking refers
to breaking down a pull request (PR) for a feature into several, smaller PRs"; "Google uses a
concept identical to stacking, but calls it 'chaining'"; Meta "was the first company to really
popularize this workflow outside the company" through Phabricator's Differential, and "Meta
engineers live and breathe stacked diffs". First-party for Meta: "Sapling: Source control that's
user-friendly and scalable", Engineering at Meta, 2022-11-15,
<https://engineering.fb.com/2022/11/15/open-source/sapling-source-control-scalable/>, accessed
2026-10-13: "At Meta, working with stacks of commits is a common part of our workflow"; "A full
feature will consist of many of these small, incremental, individually reviewed commits on top of
one another"; and "effectively reviewing stacks requires a code review tool that is tailored to
them", because "many external code review tools are optimized for reviewing the entire pull
request at once".

**Graphite stacked pull requests.** "CLI quick start",
<https://graphite.com/docs/cli-quick-start.md>, accessed 2026-10-13: "A stack is a sequence of
pull requests, each building off of its parent." `gt submit --stack` pushes every branch and opens
one pull request per branch; `gt modify` edits a branch and will "automatically restack all the
branches above it"; `gt sync` rebases open pull requests onto the latest `main`; the stack merges
from the Graphite UI. These are the operations a stack needs that `gh` lacks.

**GitHub stacked pull requests.** "About stacked pull requests",
<https://docs.github.com/en/pull-requests/get-started/about-stacked-prs>, HTTP 200, accessed
2026-10-13. GitHub supports stacks natively; the fetched pages state no release status (no
preview or beta label). A stack is "two or more pull requests in the same repository", where the
bottom one targets the trunk and "Each subsequent pull request targets the branch of the pull
request below it"; each one "shows only the diff for its layer". "When you merge a pull request
at the bottom of the stack, the remaining branches are automatically rebased so the next pull
request targets the default base branch." "Merging stacked pull requests",
<https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/merging-stacked-pull-requests>,
HTTP 200, accessed 2026-10-13: stacks merge "from the bottom (closest to the trunk) up", a
mid-stack merge takes every pull request below it, and "Auto-merge is not supported for stacked
pull requests". "Stacked pull requests CLI commands",
<https://docs.github.com/en/pull-requests/reference/stacked-prs-cli-commands>, HTTP 200, accessed
2026-10-13: the `gh stack` extension (`gh extension install github/gh-stack`) has `init`, `add`,
`link`, `submit`, `rebase`, `sync`, `merge`, and others, so it can link, submit, rebase, and merge
a stack.

**`gh` on this machine.** `gh version 2.101.0 (2026-09-15)`. `gh help` lists no built-in stack
command. `gh extension list` (2026-10-13) shows `gh stack  github/gh-stack  v0.2.1`, so the
`gh-stack` extension `v0.2.1` is installed. The stack operations come from that extension or from
the GitHub website, not from built-in `gh`.

**Tracer bullets, Hunt and Thomas.** Bill Venners, "Tracer Bullets and Prototypes: A Conversation
with Andy Hunt and Dave Thomas, Part VIII", Artima, 2003-04-21, <https://www.artima.com/intv/tracer.html>,
accessed 2026-10-13. Hunt: tracer bullet development builds "a skeleton application, in which one
thin line of execution goes end to end", and "Whatever the skeleton application does, it does end
to end, skeletally thin." Features are then added one use case at a time. Unlike a prototype,
which is "designed to be thrown away" (Thomas), tracer code is kept. The book's own wording (*The
Pragmatic Programmer*, Topic 12) is under "Not verified"; this note relies on the authors'
interview. For this note, a tracer bullet is a slice that runs from the command a user types to
the state it changes and the output it prints, so that a user could name it.

### Not verified

| Source | Why | What was expected from it |
| --- | --- | --- |
| Jason Cohen, *Best Kept Secrets of Peer Code Review* (2006) | <https://smartbear.com/lp/collaborator/best-kept-code-review-secrets> served the book behind a sign-up form and returned HTTP 404 when retried on 2026-10-13; the StickyMinds case-study article, <https://www.stickyminds.com/article/best-kept-secrets-peer-code-review>, returned HTTP 403 | Cisco study size and method; the rate and duration curves behind the 200-400 LOC and 60-90 minute guidance |
| *The Pragmatic Programmer*, 20th anniversary edition, Topic 12 "Tracer Bullets" | The O'Reilly preview, <https://www.oreilly.com/library/view/the-pragmatic-programmer/9780135956977/>, returned HTTP 403; the pragprog.com title page, <https://pragprog.com/titles/tpp20/the-pragmatic-programmer-20th-anniversary-edition/>, returned HTTP 200 but lists "Tracer Bullets" without an extract (its extracts are the preface, DRY, Don't Outrun Your Headlights, Inheritance Tax, and Shared State) | The book's definition and tip text |
| *Accelerate*, Chapter 4 (trunk-based development) | Not in the publisher excerpt, <https://itrevolution.com/wp-content/uploads/2022/06/ACC_excerpt.pdf> (HTTP 200) | Branch-lifetime and branch-count findings that would bound how long a feature branch should live |
| Phabricator Differential documentation | <https://secure.phabricator.com/book/phabricator/article/differential/> returned HTTP 503 | First-party description of dependent revisions |

## Derivations

**Sitting arithmetic.** The human read covers the whole diff. One normal sitting reviews at most
400 lines (SmartBear's upper bound for one review); at SmartBear's 500 lines/hour maximum rate
that is 48 minutes, inside 60. A 90-minute exception sitting holds 90/60 x 500 = 750 lines. This
repository's median ratio of reviewed lines to source lines is 2.2 (quartiles 1.7 / 2.2 / 2.8).

- Pull request target: 400 / 2.2 = 182 source lines, relaxed by policy to **200**. This is a
  relaxation of SmartBear's 400-reviewed-line maximum, not compliance with it: 200 x 2.2 = 440
  reviewed lines, 53 minutes at 500 lines/hour. The 40 extra lines buy a round number and still
  fit the 60-minute sitting.
- Pull request ceiling: 750 / 2.2 = 341 source lines, rounded down to **300**. Check: 300 x 2.2 =
  660 lines, 79 minutes; a pull request at the 2.5 ratio still fits 90 minutes (750 lines), and one
  at the upper-quartile ratio of 2.8 (840 lines, 101 minutes) does not, so a reviewer may ask for a
  split of a test- or docs-heavy pull request at the ceiling.
- Task ceiling: **200**, equal to the pull request target. A task must be able to ship alone as a
  pull request inside one normal sitting, and the verifier reads the same diff the human reads. No
  source measures agent verifiers, so this note bounds the verifier by the human limit. The
  literature agrees: Google calls 100 lines "usually reasonable"; Google's median is 24 lines, the
  industrial medians in Rigby and Bird are 44 to 78. In this repository 75 of 81 tasks are at or
  under 200.

**Files guard.** The guard counts guard-eligible files: every changed file except Herdr raw
captures (`reference/herdr/raw/`) and `testdata/` fixtures, which are data, as in the line rule.
The number 20 is a policy choice, not a conversion of a measured figure. Google reports that about
90% of changes touch fewer than 10 files, counting every changed file; this note takes that 10 as
the scale of a normal change. In this repository a source file usually travels with its test file:
in the 26 verified tasks with at least 50 source lines, guard-eligible files per source file have a
median of 1.75 (quartiles 1.33 / 1.75 / 2.0). Doubling 10 to **20** covers that pairing up to the
upper quartile, and 20 is the task limit and the pull request target. Google's style guide calls a
200-line change spread across 50 files "usually too large"; that remark is the upper bound, and the
pull request ceiling of **30** stays well under it. Microsoft's finding that the share of useful
comments falls as files grow supports a files guard in principle but gives no number; 20 and 30
remain policy choices from this repository's data and Google's remarks.
In the dataset, guard-eligible files have
quartiles 2 / 3 / 9, and 8 of 81 tasks exceed 20 and 5 exceed 30. Counting all files instead
(quartiles 2 / 4 / 9) gives 9 over 20 and 5 over 30; the difference is `fix/docs-07`, which is 20
guard-eligible files plus 4 raw captures.

**Domains guard.** Repository data, not literature, sets this number: the code-domain quartiles per
task are 1 / 1 / 2, and 14 of 81 tasks exceed 3. A task or pull request target of **3** code domains
admits every task at or under the upper quartile with room for one more; the pull request ceiling
of **5** excludes the 6 widest tasks. Google's advice to split by files that need different
reviewers, and Microsoft's finding that fewer files bring a higher share of useful comments (a
direction, not a number), are the reasons a second, structural guard helps:
`fix/code-05` (`55a7ea2`) is 94 source lines but 39 files and 5 domains.

## Domains

A **domain** is a unit of review context, one of:

- a command: `cmd/<name>/` together with `internal/<name>/` (and their nested subcommands);
- a shared library: each `internal/lib/<name>/`;
- the command root: files directly in `cmd/` (the root command and the cross-command tests);
- the module root: files at the repository root (`main.go`, `ste_test.go`, `AGENTS.md`), but not
  root `testdata/` fixtures (see below);
- `.github/`;
- a documentation area: each `reference/<area>/`.

The guard counts **code domains**: domains of changed source and test files. Documentation areas
are listed for the reader but not counted, in line with the line rule.

**Root fixtures are excluded from code domains.** A file under the repository-root `testdata/`
directory, for example `testdata/ste-baseline.json`, adds no domain. Fixtures are data, not
authored logic: the line rule does not count them as source, and the files guard does not count
them either, so the domain guard follows the same population. A change that edits
`testdata/ste-baseline.json` together with `ste_test.go` still touches the module root through the
test file. Under this choice wave 1 counts 7 code domains and wave 2A 12 (each would count one
more if root fixtures belonged to the module root), wave 3 counts 0, and the dataset figures do not
change: quartiles 1 / 1 / 2, maximum 18, and 14 of 81 tasks over 3.

**Recommendation: count a `cmd/<x>` and `internal/<x>` pair as one domain.** `AGENTS.md` Layout
makes `cmd/<x>` thin Cobra wiring that calls into `internal/<x>`, so a command change touches both
by design and a reviewer reads them as one unit. Counting them apart penalizes the required layout:
it raises the upper quartile from 2 to 3 and the maximum from 18 to 22 without adding review
context. Each `internal/lib/<x>` counts separately, because each is a distinct API used by several
commands.

Domain counts for past tasks (code domains, pair as one):

| Task (merge) | Source lines | Files, all / guard-eligible | Code domains | Domains |
| --- | --- | --- | --- | --- |
| `feat/wave15-repoconfig-schema` (`8f6e97a`) | 61 | 3 / 3 | 1 | `lib/repoconfig` |
| `fix/docs-07` (`0819e6c`) | 0 | 24 / 20 | 0 | (docs only: `reference/herdr`) |
| `fix/code-03` (`44b4713`) | 93 | 6 / 6 | 2 | `agent`, `lib/agent` |
| `feat/wave4-codex-no-daemon` (`621a7af`) | 34 | 14 / 14 | 2 | `cmd` root, `agent` |
| `fix/runtime-07` (`6750875`) | 102 | 14 / 14 | 5 | `agent`, `task`, `lib/agent`, `lib/identity`, `lib/task` |
| `fix/code-04` (`c7287fb`) | 92 | 21 / 21 | 5 | `task`, `lib/agent`, `lib/identity`, `lib/selector`, `lib/task` |
| `feat/wave1-dispatcher-config` (`d869229`) | 207 | 23 / 22 | 7 | `agent`, `worktree`, `lib/fledgedir`, `lib/gitstatus`, `lib/profiles`, `lib/repoconfig`, `lib/worktree` |
| `feat/wave2a-supervise-inbox` (`4ef38f8`) | 2,892 | 61 / 60 | 12 | `cmd` root, `agent`, `attention`, `inbox`, `supervise`, `task`, `lib/fledgedir`, `lib/inbox`, `lib/profiles`, `lib/state`, `lib/supervise`, `lib/task` |

## Schemes compared

| Scheme | Review unit size | Narrative for the reviewer | Fit with the tracer bullet | Tooling cost in Fledge |
| --- | --- | --- | --- | --- |
| **Per task**: each verified task opens its own pull request into `dev` | The task: median 20 source lines, 75% at or under 67 here; very small | Fragmented: a reviewer sees a lib change without its caller, or a test-only change without its reason; 81 pull requests for PR #16's tasks | Poor: most tasks are layers or fixes, not a change a user could name | Works today: `task create --delivery-mode pr`, then `task land` pushes, opens, checks, and merges one `gh` pull request. Missing: a way to open the pull request for the human read without merging it in the same command, and a size report |
| **Per feature**: tasks land locally into a feature branch; the feature branch opens one pull request into `dev` | The sum of the feature's tasks; must be held to 200 / 300 by planning | Best: one story, one description, one decision to merge | Best when the feature is one nameable change | Partly today: tasks take `--base-branch feat/<x>` and land `local` into it; a parent task (`--parent`) can stand for the feature, be verified once its subtasks are, and be landed with `--branch feat/<x> --mode pr`. Missing: creating and refreshing the feature branch from `dev` (`worktree create --base` makes a checkout, not an integration branch), a feature-level size report, and the open-without-merge step |
| **Stacked**: one feature becomes a chain of pull requests, each building on the one below | Each pull request within 200 / 300; the feature may be larger | Good when each layer is a working slice; Meta and Google do this at scale | Good when each pull request is itself a thin end-to-end slice; poor when the stack is horizontal layers (lib, then command, then docs) | Hardest. Two shapes exist on today's primitives: (a) each task's `--base-branch` names the task below it, so `task land` merges each pull request into its parent's branch, not `dev`, and the stack then needs bottom-up retargeting (`gh pr edit --base`), which Fledge cannot do because a task's base is fixed at creation; (b) each task targets `dev`, branches from the task below (`worktree create --base`), and runs `--after` it, so it lands only after its parent; its pull request shows the parent's commits until the parent merges. Both need restacking when a lower pull request changes, and a restack moves the head, so `land` refuses (`land_head_mismatch`) until `task verify --commit` runs again. Built-in `gh` has no stack, restack, or chained-merge command; GitHub's native stacks and the `gh stack` extension provide them, and GitHub retargets the next pull request after the bottom one merges |

Shape (b) needs no retargeting and fits Fledge's dependency model; shape (a) matches Graphite's
model but needs a base-edit command and retargeting. GitHub's native stacks are shape (a); the
choice between the shapes waits for the probe in a throwaway repository.

## Decision rule

An orchestrator applies this to each wave plan before any task starts. Inputs per task: the
user-visible change it serves, estimated source lines, files, code domains, and dependencies.

1. Write one sentence per user-visible change the wave delivers ("a user can now …"). Each sentence
   is a candidate feature; never put two sentences in one pull request.
2. Attach every task to the sentence it serves. A task that serves none (refactor, test-only,
   docs-only) rides with the feature that needs it; if no feature needs it in this wave, it is
   its own sentence ("…, with no behavior change").
3. Split any task over 300 source lines, 20 files, or 3 code domains. Split vertically first (a
   thinner end-to-end slice); split horizontally (stubs, a library first) only when no vertical
   cut exists.
4. Mechanical exception: a change that applies one rule uniformly (rename, import move, envelope
   swap, generated code) or that only deletes code may exceed the line and files limits, but not
   the domain limit without the owner's word. Ship it as its own pull request and say
   "mechanical" in its title.
5. Sum each feature's estimates. If the feature is within the target (200 lines, 20 files, 3
   domains), it ships as **one feature pull request**.
6. If the feature exceeds the target but not the ceiling (300 lines, 30 files, 5 domains), it
   ships as one feature pull request and the plan asks for a 90-minute sitting.
7. If the feature exceeds the ceiling, cut it into slices of at most 200 lines, each a thinner
   end-to-end version that a user could try. If the slices depend on each other, they ship as a
   **stack**; if they do not, as **separate pull requests**.
8. Features that share no dependency always ship as separate pull requests, even when small
   enough to combine.
9. Record the scheme, the estimates, and the sentence in the wave plan. When implementation
   exceeds a ceiling, return to step 3 before verification, not after.

## Worked example: PR #16 re-planned

PR #16 promoted `dev` to `main`: 81 verified-task merges and 17 other first-parent commits between
`9e5c57f` and `621a7af`, +22,943 / −8,505 over 387 files. Under this note's definition the net diff
is 10,018 source lines, 13,668 test lines, 7,238 Markdown lines, and 524 other lines. Summed over
the 98 first-parent commits the churn is 38,388 lines and 12,690 source lines; the sum exceeds the
net diff because the task board was added (894 source lines in `3fe3783`) and later removed (934
in `86e1206`).

At the 200-line normal target, the program's 10,018 net source lines need at least 51 sittings
under any scheme; at the 300-line ceiling with 90-minute sittings, at least 34. The mechanical
exception changes both bounds further, because a mechanical pull request may exceed the line
limits. The scheme decides only where those sittings fall and what story each one tells.

### Feature pull requests to `dev`, program level

| Work (commits) | Source lines | Re-planned |
| --- | --- | --- |
| Repository memory (`33c6301`) | 686 | Stack of 4, each slice at most 200 source lines and carrying its own docs: `memory add` end to end with the store's write path, then `list`, then `get`/`remove`, then profile brief injection |
| Simplified task briefs (`0b71a5a`) | 361 | Stack of 2 |
| Simplified spawn placement and Markdown profiles (`f8fae2c`) | 1,216 | Two features, not one, 7 slices in all: placement (about 443 source lines in `internal/agent/spawn` and `cmd/agent/spawn`) as a stack of 3, and Markdown profiles (about 773) as a stack of 4 |
| Task board (`3fe3783`, `8d54ba0`) | 894 | Stack of 5 |
| Task board removal (`86e1206`) | 934 | One pull request under the deletion exception |
| Review fan-out fixes (`28acda6`, `2197cac`, `8b0a289`) | 106 | One pull request if one sentence names the fix, else three |
| Permission bypass default (`010b799`) | 75 | One pull request |
| STE ratchet (`869a9c2`) | 0 (573 test) | One pull request |
| Fix sweep: 71 tasks `fix/code-*`, `fix/runtime-*`, `fix/tests-*`, `fix/docs-*` | 3,732 | `fix/code-30` (1,142) as its own mechanical pull request under step 4, with the owner's word for its 18 domains; its two largest commits (514 and 436) rule out a stack of slices at most 200. The other 2,590 source lines as at least 13 separate pull requests of at most 200 lines, grouped by the user-visible defect and its domain, with `fix/runtime-12` (257) and `fix/runtime-08` (240) each split per step 3; docs-only fixes as one pull request per `reference/` area within 30 guard-eligible files |
| README and usage guides (`0b6fc49`) | 0 (3,912 Markdown) | One docs pull request, 15 files |
| Adopt registers only the caller's agent (`7f7a975`, `c7b2cc0`) | 248 | One pull request at the 90-minute tier |
| Waves 1 to 4 | 4,438 | Below |

### The last six waves in detail

Source / tests / Markdown lines per merge; together 9,835 lines of churn and 4,420 net source
lines.

| Wave (merge) | Source / tests / Markdown | Files, code domains | Re-planned pull requests |
| --- | --- | --- | --- |
| 1 (`d869229`) and 1.5 (`8f6e97a`) | 207 / 357 / 226 and 61 / 49 / 15 | 23 files (22 guard-eligible), 7 domains; 3 files, 1 domain | 3 or 4: **(a)** "Fledge reads the integration branch from `.fledge/config.toml`" (`f5243eb`, plus wave 1.5's schema check, planned together): about 224 source lines, 3 domains (`worktree`, `lib/gitstatus`, `lib/repoconfig`), 90-minute tier. **(b)** "The managed ignore keeps `.fledge/config.toml` trackable" (`be00942`, `db03f8c`, `1d48f81`): 11 source lines, 4 domains (three of them only for updated test expectations, so a task split per step 3 or an owner exception). **(c)** "The orchestrator profile is a strict dispatcher" (`668c4c9`, `657eebf`): 33 source lines, 2 domains. **(d)** The firstmate comparison note: docs only, or with (c). The real merge was one task over the task ceiling (207) and over the domain guard (7) |
| 2A (`4ef38f8`) | 2,892 / 2,793 / 169 | 61 files (60 guard-eligible), 12 domains, one task | Three features. **Inbox** (`dd1b0b9`, `119d876`, `17ce592`, `2b0ecf8`: 1,137 source lines): a stack of 6; slice 1 is "a message to a registered agent is stored, rings a doorbell, and `fledge inbox read` prints it", and later slices add task notices, acknowledgment, and archiving. **Supervision daemon** (`b4dbb22`, `11fbfe4`, `6fd017d` and three fixes: about 1,630 source lines; `11fbfe4` alone is 1,106): a stack of 9 on top of the inbox stack; slice 1 is "`fledge supervise run` wakes the orchestrator when a worker blocks". **Attention** (`9850a2f`, 222): one pull request at the 90-minute tier, after both stacks. About 16 pull requests |
| 2B (`420dde3`) | 1,244 / 1,380 / 121 | 51 files (51 guard-eligible), 7 domains | A stack of 7: **(a)** "`task verify --commit` binds a verification to a commit" (`8711e48`, `90394e3`, `8a7bd18`: about 199); **(b)** "`task create` and `import` take a delivery mode and base branch" (`0ba7903`, `744837f`: 123); **(c)** to **(e)** "`task land` merges locally" as 3 slices of `3288afc`; **(f)** and **(g)** "`task land` delivers through a pull request" as 2 slices of `3288afc` with `6912d08` and the profile edits. Slices (c) to (g) share about 922 source lines, at most 200 each; `3288afc` is 895 source lines, so the split between local and pull request delivery is an estimate |
| 3 (`8ca37c1`) | 0 / 2 / 109 | 4 files (3 guard-eligible), 0 domains | One docs pull request, as merged |
| 4 (`621a7af`) | 34 / 89 / 84 | 14 files (14 guard-eligible), 2 domains | One pull request, as merged |

The six wave merges become about 28 to 29 feature pull requests to `dev`, 22 of them in three
stacks (6 inbox, 9 supervision, 7 `task land`). The same 4,420 source lines need at least 23 normal sittings in either plan; the
difference is that each re-planned pull request is one nameable change that fits one sitting.

### Tasks over the task ceiling

From the 81-task dataset (source lines over 200): `feat/wave2a-supervise-inbox` (`4ef38f8`,
2,892), `feat/wave2b-task-land` (`420dde3`, 1,244), `fix/code-30` (`1f265f9`, 1,142; mechanical,
183 files), `fix/runtime-12` (`f0c7e26`, 257), `fix/runtime-08` (`1427b2a`, 240), and
`feat/wave1-dispatcher-config` (`d869229`, 207). Outside the dataset, the same ceiling would have
split `feat/memory` (686), `simplify/profiles` (1,216), `simplify/tasks` (361), the task board
commit `3fe3783` (894), and the direct commit `7f7a975` (228); the board removal `86e1206` (934)
falls under the deletion exception.

Over the files guard of 20 guard-eligible files: 8 tasks, among them `fix/code-30` (183),
`fix/code-05` (39), and `fix/code-29` (33); 5 of them exceed 30. Counted over all files, 9 tasks
exceed 20: the ninth is the docs-only `fix/docs-07`, 20 guard-eligible files plus 4 raw captures. Over the domain guard of 3: 14
tasks; `fix/tests-09` (`e3b41b5`) is the widest small one, 61 source lines over 11 domains.

## Open decisions

1. **Verifier ceiling.** Whether an agent verifier may hold more than the human sitting.
   Recommendation: no; keep the task ceiling at 200 until verifier misses are measured.
   The owner settled it in the Decisions entry "Recorded 2026-10-14 by the owner": the task
   ceiling is 300.
2. **Where the limits are checked.** Plan time only, a size field on the task, or a check at
   landing. Recommendation: plan time now, then an advisory size report from `task land` (and any
   future diff command); no refusal until the numbers are tested on a few waves.
3. **Total-read warning.** Whether to warn when the whole diff (tests and docs included) passes 750
   lines even when source is within the ceiling. Recommendation: warn, do not block, since the
   line budget counts source only.
4. **Mechanical exception approval.** Who approves it and where it is recorded. Recommendation: the
   owner's word, recorded in the task brief and the pull request title.
5. **Stack shape.** Shape (a), per-task base branches like Graphite, or shape (b), `--after` with
   `dev` as base. Recommendation: (b); it needs no retargeting and no base-edit command.
6. **Human read gate in pull request mode.** `task land --mode pr` opens and merges in one command,
   so the human read cannot happen between them. Recommendation: plan an open step and a merge
   step for the tooling round; every scheme needs it.
7. **Promotion pull requests to `main`.** Whether the dev-to-main promotion is held to these
   limits. Recommendation: no; it promotes pull requests already read in full, and the owner sets
   its cadence by judgment.

## Decisions

Recorded 2026-10-13 by the owner:

- The line budget counts hand-written source only; tests, documentation, fixtures, and raw
  captures are excluded from the count.
- A pull request gets an agent gate, in which an agent reads the full diff, and a full human read.
  The human read bounds the size. Where the gate runs is deferred.
- The deliverable is this note; tooling changes come in a later planning round.
- Both a task ceiling and a pull request target and ceiling are wanted.
- Branch model, planned and not yet implemented (today `.fledge/config.toml` holds only
  `schema_version` and `base_branch = "dev"`, and `internal/lib/repoconfig` accepts only
  `schema_version`, `base_branch`, `delivery_mode`, and `inbox`): `.fledge/config.toml` will name a
  main branch (default `main`) and a dev branch (default `dev`). Features branch off dev and merge
  into dev through reviewed pull requests. Main receives a large release promotion from dev at a
  cadence the owner chooses by judgment, not by configuration. When the dev branch does not exist,
  Fledge offers to create it from main.
- The schemes to compare are per task, per feature, and stacked.
- Sitting length is 30 to 60 minutes, with exceptions up to 90.
- The guards beyond lines are files changed and domains touched. The governing principle is the
  tracer bullet: a thin end-to-end slice a user recognizes as one change.

Recorded 2026-10-13, round two, by the owner:

- The delivery modes are `local`, `pr`, `auto`, and `none`, in one field. `pr` now means an agent
  review, CI checks, and the owner's read; `auto` is the earlier one-command pull request. There
  is no `pr-human` mode.
- The human gap is two steps: `task land` stops at `review`, and `task land --merge` merges on the
  owner's word. Fledge also records a merge that the owner makes with the GitHub button.
- The size limits are constants. An advisory size report shows them and never refuses.
- Stacks are probed in a throwaway repository before any stack tooling is built.

Recorded 2026-10-14 by the owner:

- The task ceiling is 300 hand-written source lines. Files stay at 20, and code domains stay at 3.
  The pull request target of 200 and the pull request ceiling of 300 do not change.

Further decisions are appended as planning rounds settle them.
