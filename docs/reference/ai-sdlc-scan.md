 AI SDLC Scan — June 15 to July 17, 2026
 
*Monthly developer-tooling scan for a Go-orchestrated multi-agent coding stack (Herdr multiplexer + Pi harness + Claude Code). Technical, terse. Every item dated. "Relevance" = impact on the Herdr/Pi/Claude-Code stack.*
 
## TL;DR
- **Two frontier model drops reshaped coding-agent economics in-window: Claude Sonnet 5 (June 30) became the Claude Code default for Free/Pro with a 1M context and adaptive-thinking-by-default, and OpenAI's GPT-5.6 family — Sol/Terra/Luna — went GA July 9 across Codex and the API. Both ship new tokenizers/pricing tiers that change per-task cost, not just per-token rates.**
- **Harnesses converged hard on multi-agent orchestration and runaway-cost guardrails**: Claude Code added per-session subagent/web-search caps (200) and background-session `/fork`; Codex shipped configurable rollout token budgets and thread-level multi-agent delegation (but encrypted subagent instructions on Sol/Terra, removing local audit access); OpenCode disabled nested subagents by default.
- **Orchestration tooling matured around the exact pattern this stack uses**: Herdr shipped v0.7.1→v0.7.4 (socket-API multiplexing, Pi detection), Databricks open-sourced the Omnigent meta-harness with cross-vendor review (June 13, just pre-window), and Google forced Gemini CLI consumer users onto the Go-based Antigravity CLI (June 18).
## Key Findings
1. **Model layer is the biggest lever.** Sonnet 5 and GPT-5.6 both landed in-window and both change cost math via tokenizer/tier changes. For a subscription-economics-sensitive stack, the tokenizer shift matters more than the headline per-token price.
2. **"Runaway delegation" is now a first-class problem.** Every major harness shipped budget/cap features in-window — a direct signal that multi-agent fan-out is common enough in production to need governors. This is exactly the failure mode a Go orchestrator must handle.
3. **Herdr is actively maintained and fits the stack natively.** It ships a stable socket/JSON API (the same surface a Go orchestrator would drive), detects Pi and Claude Code, and iterated four point releases in-window.
4. **Codex's encrypted subagent delegation (July 14) is a governance red flag** for anyone needing auditability of what a lead agent told its subagents — relevant if the orchestrator logs delegation.
5. **The cross-vendor review pattern (one model writes, a different one reviews) is now the consensus "power move"** and is directly implementable in a Go orchestrator over Pi + Claude Code + Codex.
## Details
 
### 1. Coding-agent harnesses & CLIs
 
**Claude Code — continuous point releases v2.1.181 → v2.1.212 (June 16 – July 17, 2026)** *(source: Claude Code changelog / anthropics/claude-code CHANGELOG)*
- **June 17 (v2.1.181):** `/config key=value` to set any setting from the prompt (interactive, `-p`, Remote Control). *Relevance: scriptable config for orchestrated headless runs.*
- **June 19 (v2.1.183):** auto-mode safety — destructive git commands (`git reset --hard`, `checkout -- .`, `clean -fd`, `stash drop`) blocked in auto mode. *Relevance: safer unattended agents under orchestration.*
- **June 22 (v2.1.186):** `claude mcp login/logout <name>` to auth MCP servers from CLI without the interactive menu. *Relevance: non-interactive MCP setup for spawned panes.*
- **June 30 (v2.1.197):** Claude Sonnet 5 becomes the default model; 1M-token context. *Relevance: default execution-layer model change — re-baseline token budgets.*
- **July 1 (v2.1.198):** Chrome integration out of beta; smarter agent notifications.
- **July 3 (v2.1.200–201):** auto-mode interactions now **pause by default** unless configured otherwise. *Relevance: default autonomy posture changed — check orchestrator assumptions.*
- **July 9 (v2.1.206):** auto-allow `git push` to configured remote; `/doctor` improvements.
- **July 15 (v2.1.211):** `--forward-subagent-text` flag + `CLAUDE_CODE_FORWARD_SUBAGENT_TEXT` env to include subagent text/thinking in stream-json output. *Relevance: lets a Go orchestrator capture subagent reasoning from the stream-json interface.*
- **July 17 (v2.1.212):** `/fork` now copies the conversation into a **new background session** (own row in `claude agents`); in-session subagent renamed `/subtask`. Added session-wide **WebSearch cap (default 200, `CLAUDE_CODE_MAX_WEB_SEARCHES_PER_SESSION`)** and **per-session subagent spawn cap (default 200, `CLAUDE_CODE_MAX_SUBAGENTS_PER_SESSION`)** to stop runaway loops; `/clear` resets. MCP calls > 2 min auto-background (`CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS`). *Relevance: the subagent cap and background `/fork` map directly onto Go-orchestrated fan-out and lifecycle tracking.*
- Agent Teams (experimental, `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`) continued in-window iteration — v2.1.199 changed idle-teammate row behavior in the agent panel. *Relevance: built-in alternative to external orchestration; note it uses ~3–4x tokens of a single session.*
**OpenAI Codex — Rust CLI v0.138 (June 8) → v0.144.5 (July 16, 2026)** *(source: openai/codex releases; developers.openai.com/codex/changelog)*
- **June 16 & 25:** Codex Remote GA — start/continue work on a connected Mac/Windows host from the ChatGPT mobile app; authenticated one-to-one QR pairing.
- In-window features: **configurable rollout token budgets** (track usage across agent threads, remind on remaining budget, abort turns when exhausted); **app-server multi-agent delegation** configurable as disabled / explicit-request-only / proactive at thread and turn level; indexed web-search mode. *Relevance: Codex's own budget/delegation knobs are useful if the orchestrator drives Codex as a reviewer.*
- **July 9:** GPT-5.6 available in Codex; **Codex desktop app merged into the ChatGPT desktop app**; inline diff editing and PR review moved inside Codex. Sam Altman: "codex is the core of our new work product."
- **July 14 (CLI 0.144.4):** GPT-5.6-Sol and -Terra default to **MultiAgentV2 with encrypted subagent delegation** — parent-agent instructions to subagents are encrypted before local storage; developers lose local audit access to what was delegated. Luna stays on MultiAgentV1. *Relevance: significant — breaks local auditability of delegation in a Go orchestrator using Sol/Terra.*
- **July 16 (v0.144.5):** improved dangerous-command detection (more `rm` forms), clearer rejection reasons.
**Gemini CLI → Antigravity CLI (transition effective June 18, 2026)** *(source: Google Developers Blog; Gemini Code Assist release notes)*
- **June 18:** Gemini CLI and Gemini Code Assist IDE extensions **stopped serving requests** for Google AI Pro, AI Ultra, and free Code Assist individual tiers. Successor is **Antigravity CLI, built in Go**, sharing the Antigravity 2.0 harness; preserves Agent Skills, Hooks, Subagents, Extensions (now "plugins"). Antigravity CLI is **not open source**; Gemini CLI repo stays Apache-2.0 for enterprise (Standard/Enterprise license, paid API keys).
- Gemini CLI itself continued shipping (stable **v0.50.0, July 8**; nightly builds through July 16). *Relevance: if the stack ever shells out to Gemini CLI on a consumer tier, it broke June 18 — swap to Antigravity CLI or enterprise/API key. Antigravity CLI being Go-based and closed-source is worth watching but not a fit substitute for the open Herdr/Pi core.*
**OpenCode — v1.17.12 (June 30) → v1.18.3 (July 16, 2026)** *(source: opencode.ai/changelog; anomalyco/opencode releases)*
- **June 30 (v1.17.12):** adaptive thinking for Claude Sonnet 5; **yolo mode** (auto-approve permissions); better default small-model selection.
- **July 14 (v1.18.0):** Desktop v2 migration.
- **July 15 (v1.18.2):** **stopped subagents launching nested subagents by default**, with configurable `subagent_depth`. *Relevance: same runaway-fan-out governor pattern as Claude Code; useful precedent for orchestrator depth limits.*
- **July 16 (v1.18.3):** command-palette session search; startup/scroll fixes.
**Pi (earendil-works/pi) — v0.80.x in-window** *(source: pi.dev/news; earendil-works/pi CHANGELOG)*
- In-window: **Claude Sonnet 5 support** (inherited Anthropic/Bedrock catalogs, adaptive thinking); **GPT-5.6 model metadata** (`gpt-5.6`, `-sol`, `-terra`, `-luna`) plus verified `openai-codex` support for Sol/Terra/Luna; **Kimi K3** support with deferred/dynamic tool loading; **new `max` thinking level** above `xhigh` (`--thinking max`, natively on GPT-5.6 and adaptive Claude); cache-friendly dynamic tool loading preserving prompt-cache prefixes.
- **July 14 (v0.80.7):** breaking change — removed `openai-responses` `compat.sendSessionIdHeader`; session affinity now via `compat.sessionAffinityFormat`.
- Governance note: Pi is owned by Earendil Inc. (Armin Ronacher), open-core under RFC 0015 (MIT core, Fair Source layers); repo passed 70,000 GitHub stars. Pi deliberately ships **no built-in sub-agents, plan mode, or permission system** — it runs with the launching user's permissions unless sandboxed. *Relevance: central to the stack — Pi's minimalism is why the Go orchestrator supplies orchestration/permissions; the RPC/SDK modes and `-p`/JSON modes are the integration surface.*
### 2. Orchestration & multi-agent tooling
 
**Herdr (ogulcancelik/herdr) — v0.7.0 → v0.7.4 in-window** *(source: herdr GitHub releases; herdr.dev)*
- **June 24 (v0.7.1):** `[update].version_check`/`manifest_check` toggles; `HERDR_AGENT=<agent>` Linux hint for agents behind wrappers/VMs/Bubblewrap; configurable pane borders/gaps; Pi detected correctly when launched via npm wrappers on Windows.
- v0.7.2–v0.7.4 (in-window): socket-API additions (`session.snapshot`, `herdr api schema`, `layout.updated` events, `pane.move`), shell completions, local plugin v1 system, popup panes, Devin auto-detection, remote-attach hardening; protocol bumped to v14 (pane.move) with later v15 work. Amp/Codex/Claude Code detection updated for new active-turn UI variants.
- ~12.2k GitHub stars by July 6; reached HN front page (item 48714802). Single Rust binary, AGPL-3.0 (commercial license available), Windows is preview/beta (no `--remote`). *Relevance: core to the stack. The Unix-socket JSON API (protocol v15) is the exact programmatic surface a Go orchestrator drives to spawn panes, read state, and wait on agents; Pi/Claude/Codex are all first-class detected agents. Known caveat: on protocol-version bumps, old clients must restart the server to reattach — build a CI/CD runbook for this.*
**Databricks Omnigent — open-sourced June 13, 2026 (2 days pre-window; included as directly-relevant context)** *(source: Databricks blog; omnigent-ai/omnigent)*
- Apache-2.0 "meta-harness" over Claude Code, Codex, Cursor, OpenCode, Hermes, Pi, and custom YAML agents; reuses existing CLI credentials; local web UI at `localhost:6767`. Authors: Matei Zaharia, Kasey Uhlenhuth, Corey Zumar. Bundled example **"Polly"** is a multi-agent coding orchestrator that writes no code itself — plans, delegates to coding sub-agents (Claude Code/Codex/Pi) in **parallel git worktrees**, then routes each diff to a **reviewer from a different vendor than the one that wrote it**. Three-level policy hierarchy (server → agent → session). *Relevance: the closest existing analog to the stack's intended design; Polly's cross-vendor-review-in-worktrees is a reference architecture. Alpha; Windows runs "degraded" (no native terminal wrappers or sandboxing).*
**Other orchestrators active in-window:** golutra (Tauri desktop, Vue3+Rust, Claude Code/Codex/OpenClaw), Nimbalyst (formerly Crystal), agent-teams-ai (777genius, kanban over 9 harnesses). *Relevance: mostly GUI/desktop — none replace a terminal-native Go orchestrator, but they validate the pattern and the cross-vendor-review consensus.*
 
**Claude Code Agent Teams & Codex delegation** (see Category 1) are the in-harness alternatives to an external orchestrator: Agent Teams gives peer-to-peer teammate messaging + shared task list (experimental); Codex gives configurable thread-level delegation. *Relevance: decide build-vs-adopt — a Go orchestrator over Herdr gives cross-vendor control these single-vendor features cannot.*
 
### 3. Practices & engineering write-ups (in-window)
 
- **Claude Code `/goal` command** (requires v2.1.139+) *(source: code.claude.com/docs/goal)* — sets a completion condition; a small fast model evaluates after each turn and starts another turn until the condition holds or is cleared. Pairs with auto mode (auto mode approves tools within a turn; `/goal` starts the next turn). Evaluation billed on the small fast model, "typically negligible." *Relevance: a native autonomous-loop primitive the orchestrator can invoke per-worktree; encode verifiable end-states (tests green, clean tree) plus a turn cap.*
- **InterWorks — "Building Governed AI Code Review Agents Across Your Engineering Org" (June 26, 2026, Derrick Austin)** — real client (large K-12 education company "all in on Claude Code"): three-role SDLC — Dev (increasingly an agent from a scoped ticket) writes, AI-Reviewer takes first look, Human-Reviewer makes judgment calls; agent instructions "get the same scrutiny as a change to the code." *Relevance: a governance template for the writer/reviewer split. (Self-labeled AI-assisted; no hard metrics.)*
- **Git-worktree parallel-agent practice** matured across practitioner posts in-window (PHP Architect, July 2026, Scott Keck-Warren; jsmanifest/Medium, June 2026): each agent gets an isolated working directory sharing one `.git`; PR-per-agent until ~4–5 concurrent, then orchestrated sequential merge; diff-before-merge discipline; DB migrations/lockfiles need special handling. *Relevance: worktree isolation is the coordination substrate under any Go orchestrator; the "bottleneck shifts from generation to review" is the operating reality to design for.*
- **OpenAI — "The Shift to Agentic AI: Evidence from Codex" (Drew Johnston & David Holtz, June 25, 2026; arXiv 2606.26959; figures as of June 11, 2026), and companion "How agents are transforming work."** Verbatim: "By June 2026, users at the 99th percentile regularly generated more than 60 hours of Codex agent turns per day, distributed across multiple, parallel agents." And: "as of June 11, 2026, Codex accounts for 99.8% of output tokens these workers generate across Codex and ChatGPT. Among organizational users, the corresponding share is 63.3%, while among individual users it is 16.5%." *Relevance: quantifies how far parallel-agent usage has scaled inside OpenAI itself — plan orchestrator throughput and spend accordingly.*
### 4. Model releases & pricing/limit changes
 
**Claude Sonnet 5 — June 30, 2026** *(source: Anthropic; platform.claude.com docs; Simon Willison)*
- Model ID `claude-sonnet-5`; new default for Free/Pro in Claude Code (Max/Team/Enterprise stay on Opus 4.8). 1M context (default and max), 128K output, **adaptive thinking on by default**.
- **New tokenizer.** Anthropic's docs state "the new tokenizer produces approximately 30% more tokens for the same text." Simon Willison's June 30 measurement is language-specific: "the new token is roughly 1.4x times more expensive for English, 1.33x for Spanish, 1.28x for Python code and effectively the same cost for Simplified Mandarin." Intro pricing $2/$10 per M input/output through Aug 31, 2026, then $3/$15. Breaking API changes: `temperature`/`top_p`/`top_k` non-default → 400; `thinking:{type:"enabled",budget_tokens}` → 400 (use `thinking:{type:"adaptive"}` / effort). Anthropic reports Terminal-bench 2.1 ≈ 80.5% (vs 67% for Sonnet 4.6); note independent aggregation (codersera.com) reports Opus 4.8 still leads Sonnet 5 on Terminal-Bench 2.1, SWE-bench Pro (69.2% vs 63.2%), and OSWorld.
- *Relevance: this is the everyday execution model for the stack. Re-count prompts with the token-counting endpoint; for English/Python the effective cost is ~1.28–1.4x per text at equal per-token rates. Remove deprecated sampling/thinking params from any Pi/Claude-Code invocation.*
**GPT-5.6 (Sol / Terra / Luna) — GA July 9, 2026** *(source: OpenAI; TestingCatalog)*
- Three tiers: Sol (flagship), Terra (balanced), Luna (cost-efficient). API list pricing Sol $5/$30, Terra $2.50/$15, Luna $1/$6 per M. **`ultra` mode** coordinates multiple agents across parallel workstreams — per OpenAI it "uses four agents by default, trading higher token use for stronger results." Codex CLI **0.144.0 minimum** to see the models. ChatGPT Work and Codex **share one usage pool**. Preview was government-gated June 26–July 8 (cyber-capability review).
- **Benchmarks were published at GA** (contrary to some early coverage): per OpenAI's July 9 post, "On the Artificial Analysis Coding Agent Index, GPT-5.6 Sol with max reasoning sets a new state of the art at 80, 2.8 points above Fable 5"; ultra mode lifts Terminal-Bench 2.1 from 88.8% to 91.9% and BrowseComp to 92.2%.
- *Relevance: Terra is the interesting cost/quality point for orchestrated review or bulk work; Luna for cheap fan-out. Sol/Terra trigger encrypted delegation in Codex (see July 14). Bump Codex CLI ≥0.144 in the stack.*
**Claude Fable 5 export-control episode** *(source: Anthropic "Redeploying Fable 5"; CNBC; Al Jazeera coverage)*
- Suspended globally June 12 under a US Commerce export-control directive (jailbreak report); **restored globally July 1** across Claude.ai, Claude Platform, Claude Code, Claude Cowork after controls lifted June 30. Pricing $10/$50 per M; included up to 50% of weekly limits through July 7, then usage credits. *Relevance: Fable 5 is a premium planning/review model; if wired into an agent on a subscription, set up credits or pin a fallback after July 7. Illustrates supply-risk of depending on a single top-tier model.*
**Antigravity / Google pricing (context, announced I/O May 19; consumer cutover June 18)** — new AI Ultra tier at $100/mo (5x Pro limits); top Ultra dropped $250→$200. *Relevance: none direct to the stack unless adopting Antigravity.*
 
## Watch list (announced / unshipped / unstable)
- **Codex MultiAgentV2 encrypted delegation** (shipped July 14 but flagged "under development"; audit-access issue open as of July 16) — watch for a local-audit/decrypt option before relying on Sol/Terra subagent orchestration.
- **Claude Code Agent Teams** — still experimental behind `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` with known session-resumption/shutdown limitations; role-based per-teammate model selection still not supported.
- **Antigravity CLI** — Go-based, closed-source, free public preview; usage limits and open-sourcing unresolved (community complaints).
- **Herdr pre-1.0** — protocol-version bumps force server restart to reattach; Windows lacks `--remote`. Watch for 1.0 / protocol stabilization before hard CI dependencies.
- **Sonnet 5 intro pricing expires Aug 31, 2026** → $3/$15; combined with the tokenizer inflation (~1.28–1.4x tokens for English/Python), re-model spend for September.
- **Pi open-core (RFC 0015)** — paid Fair Source layers / hosted "Lefos" platform announced; watch licensing of any layer the orchestrator depends on.
## One-page timeline (key in-window releases)
- **Jun 17** — Claude Code v2.1.181 (`/config key=value`)
- **Jun 18** — Gemini CLI stops serving consumer tiers → Antigravity CLI (Go) successor
- **Jun 24** — Herdr v0.7.1 (agent-behind-wrapper hint, Pi/Windows detection)
- **Jun 25** — OpenAI Codex agentic-usage paper (arXiv 2606.26959)
- **Jun 26** — InterWorks governed AI-code-review write-up
- **Jun 30** — Claude Sonnet 5 GA; Claude Code default (v2.1.197); OpenCode v1.17.12 (yolo mode)
- **Jul 1** — Claude Fable 5 restored globally; Claude Code Chrome GA (v2.1.198)
- **Jul 3** — Claude Code auto-mode pauses by default (v2.1.200–201)
- **Jul 8** — Gemini CLI stable v0.50.0
- **Jul 9** — GPT-5.6 (Sol/Terra/Luna) GA; Codex↔ChatGPT desktop merger; GPT-5.6 in Codex
- **Jul 14** — Codex CLI 0.144.4 (Sol/Terra encrypted MultiAgentV2); OpenCode v1.18.0 (Desktop v2); Pi v0.80.7
- **Jul 15** — Claude Code v2.1.211 (`--forward-subagent-text`); OpenCode v1.18.2 (no nested subagents by default)
- **Jul 16** — Codex v0.144.5; OpenCode v1.18.3; Herdr v0.7.4 (master)
- **Jul 17** — Claude Code v2.1.212 (background `/fork`, subagent/web-search caps)
## Version-bump table
| Tool | Old → New (in-window) | Headline change |
|---|---|---|
| Claude Code | v2.1.179 → v2.1.212 | Sonnet 5 default; subagent/web-search caps (200); background `/fork`; `--forward-subagent-text` |
| Codex CLI | v0.138 → v0.144.5 | Rollout token budgets; multi-agent delegation config; Sol/Terra encrypted delegation; GPT-5.6 |
| Gemini CLI | (consumer) → Antigravity CLI | Consumer tiers cut off June 18; Go-based closed-source successor |
| OpenCode | v1.17.x → v1.18.3 | Yolo mode; Sonnet 5 adaptive thinking; nested subagents off by default; Desktop v2 |
| Pi | v0.80.x | Sonnet 5 + GPT-5.6 + Kimi K3 support; `max` thinking level; cache-friendly dynamic tools |
| Herdr | v0.7.0 → v0.7.4 | Socket-API expansion (`pane.move`, `session.snapshot`); plugin v1; Pi/Devin detection |
| Models | — | Claude Sonnet 5 (Jun 30); GPT-5.6 Sol/Terra/Luna (Jul 9); Fable 5 restored (Jul 1) |
 
## Recommendations
1. **Immediately: bump and re-baseline.** Pin Codex CLI ≥ 0.144.0 (required for GPT-5.6) and update Claude Code past v2.1.212 to get subagent caps and background `/fork`. Re-count representative prompts against `claude-sonnet-5` with the token-counting endpoint before trusting old budgets — for English/Python the effective per-text cost rises ~1.28–1.4x at equal per-token rates. **Benchmark that flips this:** if per-task cost rises >15% vs Sonnet 4.6 on your workloads, route bulk/fan-out work to Terra or Luna instead.
2. **Adopt the runaway-cost governors now.** Set `CLAUDE_CODE_MAX_SUBAGENTS_PER_SESSION` and `CLAUDE_CODE_MAX_WEB_SEARCHES_PER_SESSION` explicitly in your orchestrator's launch env; mirror OpenCode's `subagent_depth` concept as an orchestrator-level depth cap. Use Codex rollout token budgets when driving Codex as a reviewer.
3. **Build the cross-vendor review loop, using Omnigent's Polly as the reference design** (not necessarily the tool): Pi/Claude-Code writes in an isolated worktree; a *different-vendor* model (Codex/Terra or Fable 5) reviews the diff; the Go orchestrator gates the merge. Diff-before-merge, one migration at a time.
4. **Drive Herdr via its socket API, not screen-scraping.** Use `session.snapshot` + `layout.updated` events for state; treat Claude Code's blocked-state as screen-manifest-inferred (less reliable) and prefer `--forward-subagent-text` stream-json for authoritative subagent state. Write a protocol-bump restart runbook for CI.
5. **Avoid Sol/Terra for any delegation you must audit** until Codex restores local visibility into encrypted subagent instructions; use Luna (MultiAgentV1) or non-Codex reviewers where traceability matters. **Threshold:** revisit if/when Codex ships a local decrypt/audit path.
6. **Set a September spend review** for the Sonnet 5 intro-pricing expiry (Aug 31 → $3/$15) and the Fable 5 credits cutover (post-July 7).
## Caveats
- **Changelog aggregation:** version/date specifics for Claude Code, Codex, OpenCode, Pi, and Herdr are drawn from official changelogs/release pages and GitHub releases; some point-release notes were cross-read from third-party changelog mirrors (Releasebot, Havoptic, Gradually) and should be treated as corroboration, not primary.
- **Omnigent (June 13) is 2 days before the hard cutoff** — included only because it is a direct architectural analog to the stack; treat as context, not an in-window event.
- **Benchmark numbers** (Terminal-bench, SWE-bench, Artificial Analysis Coding Index) are vendor-reported or secondhand via press; independent aggregators disagree on some (e.g., Opus 4.8 vs Sonnet 5 on Terminal-Bench 2.1). Not independently verified here; treat as directional.
- Speculative/announced items are confined to the Watch list and flagged as such.
## References (sources accessed)
- Claude Code changelog — https://code.claude.com/docs/en/changelog ; CHANGELOG — https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md
- Claude Code Agent Teams — https://code.claude.com/docs/en/agent-teams ; `/goal` — https://code.claude.com/docs/en/goal
- Claude Sonnet 5 (docs) — https://platform.claude.com/docs/en/about-claude/models/whats-new-sonnet-5 ; Anthropic Sonnet page — https://www.anthropic.com/claude/sonnet
- Anthropic "Redeploying Claude Fable 5" — https://www.anthropic.com/news/redeploying-fable-5 ; CNBC — https://www.cnbc.com/2026/06/30/anthropic-says-trump-admin-has-lifted-export-controls-on-claude-fable-5-and-mythos-5.html ; Al Jazeera — https://www.aljazeera.com/economy/2026/7/1/us-lifts-restrictions-on-powerful-ai-models-fable-mythos-anthropic-says
- OpenAI Codex changelog — https://developers.openai.com/codex/changelog ; releases — https://github.com/openai/codex/releases
- GPT-5.6 launch — https://openai.com/index/gpt-5-6/ ; OpenAI release notes — https://openai.com/products/release-notes/ ; Wikipedia GPT-5.6 — https://en.wikipedia.org/wiki/GPT-5.6
- Codex encrypted delegation coverage — https://www.techtimes.com/articles/320784/20260716/openai-codex-encrypts-agent-instructions-stripping-developers-audit-access.htm
- Gemini→Antigravity CLI — https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/ ; Gemini CLI changelog — https://geminicli.com/docs/changelogs/latest/ ; Gemini Code Assist notes — https://developers.google.com/gemini-code-assist/resources/release-notes
- OpenCode changelog — https://opencode.ai/changelog ; releases — https://github.com/anomalyco/opencode/releases
- Pi — https://pi.dev/ ; Pi news — https://pi.dev/news ; CHANGELOG — https://github.com/earendil-works/pi/blob/main/packages/coding-agent/CHANGELOG.md ; repo — https://github.com/earendil-works/pi
- Herdr — https://herdr.dev/ ; repo — https://github.com/ogulcancelik/herdr ; releases — https://github.com/ogulcancelik/herdr/releases
- Omnigent — https://github.com/omnigent-ai/omnigent
- OpenAI Codex agentic-usage paper — https://openai.com/index/how-agents-are-transforming-work/
- InterWorks governed AI review — https://interworks.com/blog/2026/06/26/building-governed-ai-code-review-agents-across-your-engineering-org
- Simon Willison, Sonnet 5 tokenizer — https://simonwillison.net (June 30, 2026 post)

