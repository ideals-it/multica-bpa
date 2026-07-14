# Simple Task Board Agent Model

**Date:** 2026-07-14

**Status:** Accepted

**Base:** Multica `upstream/main` at `8614b198`

## Summary

Multica will be used primarily as a task board for work performed by autonomous AI agents, not as a multi-agent orchestration engine. A ticket normally belongs to one agent for its entire lifecycle. That agent investigates, implements, verifies, commits, and reports the result in the same ticket.

The active team is intentionally small:

- **Codex** — general-purpose engineering agent with full task lifecycle ownership.
- **EventCatalog** — documentation and EventCatalog specialist, combining the current Documentation Writer quality with EventCatalog change triage.
- **n8n Prod** — n8n production specialist for the N8N Prod BPA Team project.
- **Archivist** — read-only knowledge agent, invoked explicitly when detailed cross-project context is needed.

This model keeps Multica's existing board, statuses, issue assignment, runtime, and UI. It removes workflow complexity created by Team Lead routing, child tickets, staged Builder/Quality handoffs, and GitHub-only agents.

## Goals

- Make working from a Multica ticket feel like working in a normal Codex App task.
- Keep one owner and one conversation for a normal task.
- Make board status reflect the real state of work.
- Require human approval once per production task scope, not once per command.
- Keep comments concise, readable, and useful to a non-specialist.
- Preserve specialist agents where specialization materially improves results.
- Keep the fork small and easy to update from upstream Multica.
- Ensure scheduled automations create visible work without starting it automatically.

## Non-goals

- No UI, board, or predefined status changes.
- No Team Lead → specialist → Quality orchestration.
- No mandatory child tickets for implementation or review stages.
- No Standard, Investigation, or Production workflow templates.
- No server-side comment formatting engine.
- No agent-per-command production approvals.
- No AI execution for routine Archivist metadata maintenance.

## Agent Model

### Codex

**Codex** is the default agent for engineering and operational tickets. It owns the full lifecycle:

1. Understand the ticket and inspect the relevant source of truth.
2. Investigate the problem and choose an implementation approach.
3. Implement the change.
4. Review its own diff and test the result proportionally to risk.
5. Resolve ordinary local blockers, including installing project dependencies and using already-authorized local credentials without exposing secrets.
6. Create a task-scoped Git commit for every repository-changing ticket.
7. Push when publishing the result is a natural part of the ticket; do not push merely because a commit exists.
8. Request production approval when required.
9. Report the final result in the same ticket.

The agent does not create review or Git child tickets for work it can complete itself.

### EventCatalog

**EventCatalog** replaces the separate Documentation Writer and EventCatalog Triage agents. It retains the Documentation Writer's strong writing behavior and gains EventCatalog change-significance triage.

Responsibilities:

- Write and maintain EventCatalog documentation and event/service catalog entries.
- Analyze service changes and determine whether documentation must change.
- Work only through the EventCatalog project unless a ticket explicitly provides another scope.
- Run the EventCatalog lint/build after every documentation change and fix failures in the same ticket.
- Commit every repository change.
- Prepare Cloud Run deployment and request approval through `In Review` before deploying.

The separate daily build-check agent/autopilot is unnecessary because verification is part of every documentation ticket.

### n8n Prod

**n8n Prod** owns n8n work in the N8N Prod BPA Team project. It investigates, implements, validates, commits, and reports independently.

It may perform read-only production inspection without approval. Any ticket whose approved scope includes changing or deploying production must move to `In Review` and wait for user approval before the production action.

### Archivist

**Archivist** is read-only and does not participate in the execution path of ordinary tickets. It is invoked explicitly when the user or another agent needs a detailed answer across projects, agents, tasks, commits, or implementation history.

Routine archive metadata is maintained by deterministic server-side bookkeeping without starting an Archivist AI run, occupying a local runtime, changing ticket status, or waking another agent.

## Agent Migration

| Current role | Target state |
|---|---|
| AT Documentation Writer | Rename and evolve into **EventCatalog**; retain writing skills and add significance-triage capability |
| AT EventCatalog Triage | Archive after its triage capability is attached to **EventCatalog** |
| AT n8n Prod | Keep and simplify instructions to the single-agent lifecycle |
| AT Archivist | Keep as explicitly invoked read-only agent |
| AT Team Lead | Archive after active work is safely migrated |
| AT Builder | Archive after active work is safely migrated |
| AT Quality | Archive after active work is safely migrated |
| AT GitHub Ops | Archive; Git responsibility moves to the ticket-owning agent |
| AT Multica Doctor | Archive as a dedicated role; its autopilot assigns actionable tickets to **Codex** |

Archiving preserves ticket and comment history. Existing active tickets are migrated deliberately rather than bulk-reassigned without inspection.

## Ticket Lifecycle

The existing statuses keep their current UI representation and meaning:

- `Backlog` — recorded but not authorized to start.
- `Todo` — ready for the assigned agent to pick up.
- `In Progress` — the assigned agent is actively working.
- `Blocked` — progress requires an unavailable dependency, access, information, or external state change.
- `In Review` — the ticket is waiting for the user's task-level production approval.
- `Done` — the requested outcome is complete and verified to the appropriate level.

### Activation

An issue created in `Backlog` may already be assigned to an agent, but assignment alone must not start an agent run. The user activates it by moving it to `Todo` or `In Progress`, or by explicitly running it.

All autopilot-created tickets follow the same rule: assigned owner, `Backlog` status, no automatic execution.

### Single-ticket default

All analysis, implementation, verification, approval, and final reporting stay in the main ticket. A child ticket is created only when the parent genuinely contains two or more independently deliverable outcomes with different ownership or lifecycle. Implementation and review phases alone are not reasons to split a ticket.

### Status ownership

The assigned agent updates the ticket as work advances. Status must describe reality, not a planned workflow stage. A completed comment must not leave an active ticket in `In Progress`, and an `In Review` ticket must contain a real pending approval request.

## Production Approval

Production approval is a ticket-level gate implemented through the existing `In Review` status.

1. The assigned agent completes investigation, implementation, local validation, and deployment preparation.
2. The agent moves the same ticket to `In Review` and posts a concise, non-technical explanation of:
   - what will change;
   - expected effect;
   - material risk and rollback;
   - the exact approval requested.
3. The comment contains a real clickable mention of the user.
4. The user may approve with clear natural language or an allowed approval reaction such as 👍 or 👌.
5. The same assigned agent resumes the same ticket, performs the approved production action, verifies it, and finishes the ticket.

Approval applies to the described ticket scope, not to individual shell or cloud commands. A material scope or risk change invalidates the approval and requires a new request. Ambiguous comments or reactions from agents do not count as human approval.

## Autonomy and Blockers

Agents should solve normal execution problems themselves: install declared dependencies, use the repository's supported tooling, inspect logs and configuration, and retrieve already-authorized credentials through approved local mechanisms.

An agent moves a ticket to `Blocked` only when meaningful progress cannot continue without user input, new authorization, missing external data, unavailable infrastructure, or another external state change. The blocker comment must state what is missing, what has already been attempted, and the single next action required. It must mention the user with a clickable mention when user action is required.

Runtime unavailability is retried by the platform according to the autopilot retry policy; it should not immediately become a permanent ticket blocker.

## Comment Contract

Comments use natural, concise Markdown similar to high-quality Codex App updates. They are written for a human who needs the result and decision context, not raw execution telemetry.

### Intermediate updates

During active work, the agent posts a brief natural update every 3–5 minutes when there is meaningful ongoing work and no newer result. The update has no rigid template: it explains the current stage, what has materially changed since the previous update, and whether user action is needed.

Markdown formatting remains available and encouraged where it improves scanning: short paragraphs, **bold emphasis**, inline `code`, code blocks, lists, and links. Formatting must not turn a status update into a verbose report.

### Final result

The final comment leads with the outcome and is self-contained. It includes the important verification, material limitations or risks, and any remaining user action. It does not require fixed headings when plain prose is clearer.

Do not include raw logs, tool-call transcripts, large JSON payloads, repetitive progress narration, or implementation details that do not help the user make a decision. Technical identifiers and file links are included only when useful.

### Mentions

When the user must approve, answer, provide access, or perform an action, the agent uses Multica's real clickable mention representation. Writing the user's name as plain text is not sufficient. Agent mentions are used only when another agent genuinely owns a requested action; ordinary work should not need cross-agent routing.

## Autopilots

| Autopilot | Schedule and behavior | Owner | Ticket behavior |
|---|---|---|---|
| EventCatalog Daily Significance Diff | Daily scan of relevant repositories; create a ticket only for a material undocumented change; deduplicate open findings | **EventCatalog** | EventCatalog project, assigned, `Backlog`, no auto-start |
| EventCatalog Weekly Deploy | Weekly check; create a ticket only when verified undeployed catalog changes exist | **EventCatalog** | EventCatalog project, assigned, `Backlog`; after activation, build and prepare, then `In Review` for Cloud Run deploy approval |
| n8n Stable Version Check | Daily comparison of the latest stable release with the actual production version; create only when a newer stable version is available; deduplicate | **n8n Prod** | N8N Prod BPA Team project, assigned, `Backlog`; production upgrade requires `In Review` approval |
| Multica Doctor Daily Audit | Daily health audit; stay silent when healthy; create one deduplicated actionable issue for a real problem | **Codex** | Common Issues project, assigned, `Backlog`, no auto-start |

The following existing autopilots are disabled:

- EventCatalog Daily Build Check — build becomes part of every EventCatalog change.
- Comment Router — there is no Team Lead routing layer.

Each schedule period may produce at most one successful run. If the assigned local runtime is offline or unavailable at schedule time, the platform retries without creating duplicate tickets and without marking the period successful until one run completes.

## Minimal Fork Boundary

The implementation starts from current `upstream/main`, not from the existing behavior-heavy fork. Only behavior that cannot be expressed reliably through agent instructions or existing Multica configuration is carried as code:

1. **Generic production approval guard** — ticket-scope approval in `In Review`, including natural-language approval and supported user reactions, resumption of the same agent, and invalidation after material scope changes.
2. **Autopilot admission retry** — retry a scheduled `run_only` occurrence skipped before task creation because its local runtime was unavailable, while enforcing one successful run per schedule period. Upstream's existing retry for already-created failed tasks remains unchanged.
3. **Safe local artifact links in Desktop** — clickable links that open a file or containing directory only when the server authorizes the task and Desktop confirms the canonical path remains inside its work directory.
4. **Backlog-only `create_issue` autopilots** — create the assigned issue in `Backlog` and do not enqueue its assignee until the user activates the ticket.
5. **Single-agent completion status** — retain upstream's status-management protocol but replace its unconditional final `In Review` transition with `Done`; reserve `In Review` for a real pending production approval.

The fork does not port old orchestration handlers, Lead wakeups, child handoff rules, role-specific closure/commit gates, Archivist execution hooks, formatting middleware, workflow templates, or UI/status modifications.

Local development authentication uses upstream's existing `MULTICA_DEV_VERIFICATION_CODE`; no custom auth patch is required.

## Reused Upstream Behavior

The fork must reuse, not reimplement:

- Existing board, issue statuses, assignment triggers, and the rule that an assigned `Backlog` issue does not start.
- Existing assignment protocol for reading the issue, recent comments, metadata, entering `In Progress`, reporting blockers, and posting a final comment.
- Existing Markdown comment formatting and clickable mention representation.
- Existing task retry for tasks that were already created and later failed with `runtime_offline`, `runtime_recovery`, timeout, or Codex inactivity.
- Existing comment/reaction storage, task deduplication, runtime authentication, Codex CLI integration, and `MULTICA_DEV_VERIFICATION_CODE` support.

## Upstream Update Strategy

- Keep custom behavior in a small number of isolated packages/files with generic names and tests.
- Do not modify upstream UI or board primitives.
- Avoid embedding BPA agent names, project IDs, or workflow-specific rules in server code.
- Store live agent, skill, project, and autopilot configuration outside the upstream code delta.
- Regularly fetch upstream and rebase or merge it into the minimal branch after running the full verification suite.
- Document each downstream patch and its upstream equivalent/removal condition.
- Keep the current patch inventory in [downstream-patches.md](../../downstream-patches.md).

## Migration Sequence

1. Snapshot current agents, skills, projects, autopilots, active tickets, and configuration for rollback.
2. Build and test the three minimal code behaviors on the clean upstream branch.
3. Create/configure **Codex** and simplify the retained specialist instructions.
4. Merge Documentation Writer and EventCatalog Triage capabilities into **EventCatalog**.
5. Reconfigure autopilots and verify that previewed issues are assigned in `Backlog` without triggering a run.
6. Validate one real, low-risk ticket per active agent.
7. Validate one approval-gated production dry run without performing an unapproved production mutation.
8. Migrate active tickets deliberately.
9. Archive replaced agents and disable obsolete autopilots only after the new model passes live validation.
10. Keep the previous service/config snapshot available for rollback until acceptance is complete.

## Verification and Acceptance Criteria

### Automated verification

- Go tests cover approval creation, text approval, emoji reaction approval, wrong-user rejection, stale/scope-changed approval rejection, and same-agent resumption.
- Go tests cover offline-runtime autopilot retry, deduplication, and one successful run per schedule period.
- Desktop tests cover allowed local artifact links, traversal rejection, paths outside the task directory, missing files, and open-containing-directory behavior.
- Existing server, TypeScript, desktop, and build checks remain green.

### Live acceptance

- A manually created ticket assigned to **Codex** completes in one ticket without child orchestration.
- An autopilot-created ticket appears assigned in `Backlog` and produces no agent run until user activation.
- Ticket status follows actual work throughout execution.
- A long task produces useful, non-repetitive updates every 3–5 minutes.
- A production task pauses once in `In Review`, accepts a supported user approval, resumes the same agent, and does not loop back for the unchanged scope.
- User-action requests contain a working clickable mention.
- Repository-changing tasks create a scoped commit; push occurs only when appropriate to the ticket.
- EventCatalog documentation work builds successfully within the same ticket.
- **Archivist** can answer a detailed cross-project question without changing project state or occupying the normal task execution path.
- No old Team Lead, Builder, Quality, or GitHub Ops handoff is required for successful completion.

## References

- [Multica](https://github.com/multica-ai/multica)
- [Unofficial Codex prompt snapshot](https://github.com/asgeirtj/system_prompts_leaks/blob/main/OpenAI/Codex/codex-full.md) — used only as inspiration for concise progress and completion communication.
- [Unofficial GPT prompt snapshot](https://github.com/asgeirtj/system_prompts_leaks/blob/main/OpenAI/Codex/gpt-5.6.md) — used only as inspiration for outcome-first communication.
