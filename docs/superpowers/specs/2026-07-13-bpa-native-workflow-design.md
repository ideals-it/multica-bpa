# BPA Native Workflow Design

**Status:** Approved for planning

## Goal

Create a minimal, upstream-friendly Multica fork for a reliable AI team. The
standard task path is **Team Lead → specialist agents → Team Lead → Quality →
Team Lead**, with a human gate before any production-impacting action.

## Non-goals

- Do not introduce a second workflow engine, event bus, or external scheduler.
- Do not use JSON messages or mentions in comments as workflow transport.
- Do not replace Multica's issue, run, assignment, or stage primitives.
- Do not change production infrastructure, IAM, secrets, or deployments.
- Do not migrate or alter the currently running Multica instance during the
  first implementation phase.

## Existing Multica behavior we retain

Multica already persists parent and child issues, ordered sibling `stage`
barriers, agent assignments, run history, status transitions, system comments,
and an idempotent parent-assignee wake when a stage closes. The server leaves
next-stage advancement to the parent assignee. This is the native foundation
for the BPA workflow.

## Product model

### Workflow templates

The fork exposes three small templates:

| Template | Use | Stages |
| --- | --- | --- |
| `standard` | ordinary code, documentation, and investigation work | specialist delivery, quality review |
| `production` | work that may lead to a deploy or production mutation | specialist delivery, quality review, human approval |
| `investigation` | read-only incident or research work | specialist delivery, optional quality review |

Templates are task-starting constraints, not a general graph language. Each
template describes allowed stage roles, the required parent owner, and the
terminal condition. It does not create new issue statuses or replace native
stage handling.

### Roles

| Role | Responsibility |
| --- | --- |
| Team Lead | Owns the main issue, creates child issues, synthesizes results, starts Quality, and closes the main issue. |
| Builder | Delivers repository or service changes in one child issue. |
| n8n Prod | Handles n8n-specific work in one child issue; it never deploys without a human gate. |
| Documentation Writer | Produces a named documentation artifact in one child issue. |
| Quality | Independently verifies a delivery and either accepts it or returns the named delivery issue for changes. |
| Human owner | Approves or rejects production-impacting plans. |

Every child has exactly one assignee and one durable result. A review is its
own child issue, not a reassignment of the delivery issue.

### Status semantics

The fork keeps Multica's existing statuses. `in_review` is the only waiting
state used for Quality and human approval. A namespaced BPA metadata record
distinguishes the reason:

| Status | BPA metadata | Meaning |
| --- | --- | --- |
| `in_review` | `bpa.waiting_for=quality` | Quality owns the next action. |
| `in_review` | `bpa.waiting_for=human_approval` | A human decision is required before production work may continue. |
| `blocked` | `bpa.blocker_owner` and `bpa.blocker_action` | The issue cannot continue until the named owner takes the named action. |

`done` on a child closes a native stage barrier. Multica wakes the Team Lead
through its existing child-done handler; BPA code never posts a routing
comment or directly reassigns the parent to simulate that wake.

## Native flow

```text
Main issue (Team Lead)
  Stage 1: one or more specialist child issues
    ↓ native stage barrier closes
  Team Lead: concise synthesis on main issue
  Stage 2: one Quality child issue per accepted delivery scope
    ↓ native stage barrier closes
  Team Lead: close main issue or reopen the named delivery issue
  Production only: human approval before the gated action is dispatched
```

The Team Lead must leave the main issue in one explicit state after every
wake: create the next children, set a named blocker, request human approval,
or close it. An active main issue with no active child, no current run, and no
waiting marker is a detectable workflow error.

## Human approval

Production-impacting work includes deploys, production configuration changes,
production data writes, IAM changes, and secret changes. The fork must enforce
the following rules in server-side dispatch code:

1. An agent may prepare a production plan and request approval.
2. Only a human member can approve or reject the current plan.
3. Approval is bound to a deterministic plan fingerprint; changing the plan
   invalidates an old approval.
4. The agent runtime is not dispatched for the gated action until a matching
   human approval exists.
5. Quality acceptance never substitutes for human production approval.

The initial UI presents the human a short Ukrainian summary: requested action,
affected environment, expected effect, rollback, and residual risk. It must
not expose raw agent transcript as the decision surface.

## Handoffs and comments

Comments are human-readable evidence, not workflow commands. The UI and role
instructions use one concise Ukrainian handoff format:

```text
Результат: <what is ready>

Перевірка: <how it was checked>

Ризик: <remaining risk or "немає відомого">

Наступне: <the named owner and action>
```

A system-generated stage notice contains only the completed stage, a link to
the relevant children, and the current owner. It never embeds agent JSON,
runtime data, or an instruction to mention another agent.

## Implementation boundaries

New BPA-owned code lives under these paths whenever possible:

```text
server/internal/bpa/
packages/core/bpa-workflow/
packages/views/bpa-workflow/
docs/bpa/
```

Upstream files may change only to register a narrow server hook, expose a
typed API field, or mount a shared UI view. Upstream issue state, child-stage
logic, and agent dispatcher behavior remain authoritative.

## Upstream update policy

The repository keeps two durable branches:

```text
main       exact fork tracking branch; no BPA product commits
bpa/main   BPA product branch; only this branch is deployed or used for pilots
```

`upstream` points to `https://github.com/multica-ai/multica.git`. The update
routine is:

1. Fetch `upstream/main`.
2. Fast-forward local `main` only to the upstream commit.
3. Create an `upstream-sync/<date>` branch from `bpa/main`.
4. Merge `main`, resolve only documented hook-point conflicts, and run the
   focused BPA tests plus Multica's relevant server and TypeScript checks.
5. Review the diff before merging the sync branch into `bpa/main`.

No BPA code is committed to `main`. No local deployment occurs as part of a
sync.

## Initial pilot and acceptance criteria

The first pilot is local and read-only. It creates a `standard` main issue
with Team Lead, one Documentation Writer child in stage 1, and one Quality
child in stage 2. It succeeds only when:

1. Team Lead owns and comments on the main issue before child execution.
2. The specialist completes one named artifact.
3. Native stage completion wakes Team Lead without a mention or JSON event.
4. Team Lead creates the Quality child.
5. Quality accepts or returns the specific delivery child.
6. Team Lead records the final concise summary and closes the main issue.
7. The board makes the current owner and any waiting reason visible.

Production templates and gates are tested locally with a simulated gated
action only. They never perform a deploy or mutate a production system during
the pilot.

## Test strategy

- Go handler and service tests prove template validation, stage progression
  decisions, approval invalidation, and dispatch denial without a matching
  human approval.
- TypeScript tests prove API schemas and metadata interpretation.
- Shared view tests prove concise handoff and waiting-state presentation.
- One end-to-end local pilot verifies the Lead → agents → Lead path.
- Upstream sync is validated by focused BPA tests plus the repository checks
  required by each touched layer.
