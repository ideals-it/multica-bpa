# Single-Ticket BPA Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make one root ticket the default BPA workflow for a single deliverable; create child tickets only for genuine parallel or independently deliverable work.

**Architecture:** Agent work is queued on the existing root issue through explicit mentions. The root stays `in_progress` while agent work is active, moves to `in_review` only for a human decision, and is closed by Team Lead after evidence is accepted. Existing child-ticket behavior remains available but is no longer prescribed by BPA templates.

**Tech Stack:** Go backend built-in skill bundles, Go tests, live Multica agent instructions.

## Global Constraints

- Keep the existing board and statuses unchanged.
- Do not change production approval enforcement; only its task topology changes.
- No child ticket merely to transfer work between Lead, specialist, and Quality.
- Preserve children for independent deliverables or parallel work.

---

### Task 1: Replace Standard template topology

**Files:**
- Modify: `server/internal/service/builtin_skills/multica-bpa-standard-workflow/SKILL.md`
- Modify: `server/internal/service/builtin_skills/multica-bpa-standard-workflow/references/standard-workflow-source-map.md`
- Test: `server/internal/service/builtin_skills_test.go`

**Interfaces:**
- Consumes: existing comment mention dispatch in `server/internal/handler/comment.go`.
- Produces: Lead instructions to queue Builder, Quality, and Lead sequentially on one root issue.

- [x] Write tests asserting the standard skill teaches a single root issue, explicit mentions for action owners, and child creation only for independent work.
- [x] Run the focused test and observe failure against the current staged-child content.
- [x] Replace staged-child instructions with root-comment handoffs and the `Todo → In Progress → Done` / `In Review` status contract.
- [x] Update source-map references to mention dispatch rather than child-stage barriers.
- [x] Re-run the focused test.

### Task 2: Align Production and Investigation templates

**Files:**
- Modify: `server/internal/service/builtin_skills/multica-bpa-production-workflow/SKILL.md`
- Modify: `server/internal/service/builtin_skills/multica-bpa-investigation-workflow/SKILL.md`
- Test: `server/internal/service/builtin_skills_test.go`

**Interfaces:**
- Consumes: `TaskService.CanEnqueueIssue`, which still enforces production approval on the root.
- Produces: one-root sequential preparation/review/approval flow; child tickets only when the deliverable is independently split.

- [x] Add failing assertions that neither template requires a specialist/Quality child solely for a handoff.
- [x] Change the templates to use same-root mentions and status movement.
- [x] Preserve production `In Review` and scope-bound human approval before execution.
- [x] Preserve investigation read-only behavior and its separate-fix-root rule.
- [x] Run focused built-in skill tests.

### Task 3: Synchronize board state, update live contracts, and verify

**Files:**
- Modify: `server/internal/service/task.go`
- Test: `server/internal/service/task_status_transition_test.go`
- Modify: live local `AT Team Lead` instructions through Multica API/DB configuration
- Modify: `docs/bpa/CHANGELOG.md`

**Interfaces:**
- Consumes: root ticket mention dispatch.
- Produces: no child for agent handoff; child only for parallel/independent deliverable.

- [x] Add the default single-ticket and status rule to Team Lead instructions.
- [x] Move `Todo` or `Backlog` to `In Progress` only after an agent task is queued; preserve `In Review`, `Done`, and `Blocked`.
- [x] Verify the current local team configuration contains the rule.
- [x] Run `gofmt` and the relevant Go tests, then the full backend suite.
- [x] Rebuild/restart only the local self-hosted BPA runtime and probe `/health`; do not deploy remote production.
