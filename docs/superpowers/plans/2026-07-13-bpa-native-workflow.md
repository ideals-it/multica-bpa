# BPA Native Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a small native BPA workflow layer to Multica that preserves the existing stage barrier and implements Lead → specialists → Lead → Quality → Lead with an enforced human gate for production work.

**Architecture:** Keep Multica issues, child stages, task dispatch, runs, and the existing `notifyParentOfChildDone` handler authoritative. Add a pure BPA workflow package for metadata interpretation and dispatch policy, a small server handler for template and approval mutations, and shared UI components that render BPA state from the issue metadata already present in the standard issue response. Production gating happens in the shared task-enqueue service, so assignment, stage wake, a mention, comment trigger, autopilot, and manual rerun all fail closed until a matching member approval exists.

**Tech Stack:** Go 1.26, Chi, PostgreSQL JSONB issue metadata, sqlc, TypeScript, React, TanStack Query, Vitest, Go `testing`.

## Global Constraints

- Do not add a workflow engine, background scheduler, routing comments, JSON-comment event protocol, or a new issue status.
- Retain `server/internal/handler/issue_child_done.go` as the only stage-barrier and parent-wakeup authority.
- Keep BPA-owned code in `server/internal/bpa/`, `packages/core/bpa-workflow/`, `packages/views/bpa-workflow/`, and `docs/bpa/`.
- Make only narrow registrations in existing upstream files: route registration, typed API client exports, dispatch gate, and issue-detail mount.
- Use existing `in_review` plus `bpa.waiting_for` metadata for Quality and human waits; use `blocked` plus named blocker metadata for an actionable blocker.
- Production-impacting actions are deploys, production configuration/data changes, IAM changes, and secrets changes. Before an agent may be dispatched for one, Lead must mark the issue `bpa.production_action=true` through the approval-request endpoint; a matching human approval is then required by the shared enqueue service.
- This fork prevents an unapproved production action from receiving a new Multica run. It does not turn a broadly privileged runtime into a security boundary: the BPA role skill must still require agents to request approval before invoking an external production tool, and production credentials must remain separately scoped.
- Treat comments as evidence only. Use the exact four-section Ukrainian handoff format from the approved design.
- Do not deploy, modify a live Multica workspace, alter IAM, or alter secrets during implementation or the local pilot.
- Add every fork-specific change to `docs/bpa/CHANGELOG.md` in the same commit as the change.
- `main` remains an upstream tracking branch; all BPA work is committed on `bpa/main` or a short-lived branch from it.

---

## File Map

| Path | Responsibility |
| --- | --- |
| `server/internal/bpa/workflow.go` | Pure workflow-template, metadata, approval-fingerprint, and dispatch-gate decisions. |
| `server/internal/bpa/workflow_test.go` | Table-driven tests for all pure BPA policy decisions. |
| `server/internal/handler/bpa_workflow.go` | Authenticated template/approval HTTP handlers that use the existing single-key metadata writes and publish the existing metadata event. |
| `server/internal/handler/bpa_workflow_test.go` | HTTP tests proving Lead ownership, member-only approval, plan invalidation, and persistence. |
| `server/internal/service/task.go` | One shared BPA enqueue guard used by assignee, squad, mention, comment, autopilot, retries, deferred promotion, and manual-rerun task creation. |
| `server/cmd/server/router.go` | Narrow registration of BPA workflow routes beneath an issue. |
| `packages/core/bpa-workflow/types.ts` | Shared BPA client types and metadata parser. |
| `packages/core/bpa-workflow/types.test.ts` | Parser tests for missing, malformed, and complete metadata. |
| `packages/core/api/client.ts` | Typed calls for BPA template, approval request, approval decision, and workflow state retrieval. |
| `packages/core/types/api.ts` | Request and response interfaces exported to shared views. |
| `packages/views/bpa-workflow/workflow-state-notice.tsx` | Read-only concise state notice for current owner/wait reason. |
| `packages/views/bpa-workflow/workflow-state-notice.test.tsx` | Render tests for Quality, human approval, blocked, and idle-error notices. |
| `packages/views/issues/components/issue-detail.tsx` | Mount the shared BPA notice in the existing issue detail layout. |
| `packages/views/issues/components/issue-detail.test.tsx` | Assert the BPA notice appears for a BPA-enabled issue without changing non-BPA issues. |
| `server/internal/service/builtin_skills/bpa-task-workflow/` | Built-in skill defining the role contract and concise handoff text for Lead, specialists, and Quality. |
| `docs/bpa/CHANGELOG.md` | Append-only record of fork-specific product changes. |
| `docs/bpa/upstream-sync.md` | Repeatable, non-deployment upstream sync and verification procedure. |

## Task 1: Create the BPA policy package and fork-maintenance record

**Files:**
- Create: `server/internal/bpa/workflow.go`
- Create: `server/internal/bpa/workflow_test.go`
- Create: `docs/bpa/CHANGELOG.md`
- Create: `docs/bpa/upstream-sync.md`

**Interfaces:**
- Produces `Template`, `WaitingFor`, `State`, `ParseState`, `ValidateTemplateStart`, `PlanFingerprint`, `CanDispatch`, and `HandoffTemplate` from `server/internal/bpa`.
- Consumes `map[string]any` metadata decoded from the existing issue JSONB column.

- [ ] **Step 1: Write failing pure-policy tests**

```go
func TestValidateTemplateStartRequiresAgentLeadOnRootIssue(t *testing.T) {
    err := ValidateTemplateStart(TemplateStandard, IssueRef{ParentID: "", AssigneeType: "member"})
    if !errors.Is(err, ErrLeadMustOwnMainIssue) {
        t.Fatalf("expected ErrLeadMustOwnMainIssue, got %v", err)
    }
}

func TestCanDispatchRejectsProductionActionWithoutMatchingHumanApproval(t *testing.T) {
    state := State{Template: TemplateProduction, PlanFingerprint: "new-plan", ApprovalFingerprint: "old-plan", ApprovalStatus: ApprovalApproved}
    if decision := CanDispatch(state, true); decision.Allowed {
        t.Fatal("expected production dispatch to be denied")
    }
}
```

- [ ] **Step 2: Run the test to verify it fails for the missing package**

Run: `cd server && go test ./internal/bpa -run 'TestValidateTemplateStart|TestCanDispatch'`

Expected: FAIL because `server/internal/bpa` and its exported symbols do not exist.

- [ ] **Step 3: Implement the minimal pure policy**

```go
package bpa

type Template string
const (
    TemplateStandard Template = "standard"
    TemplateProduction Template = "production"
    TemplateInvestigation Template = "investigation"
)

type DispatchDecision struct { Allowed bool; Reason string }

func CanDispatch(state State, productionAction bool) DispatchDecision {
    if !productionAction || state.Template != TemplateProduction {
        return DispatchDecision{Allowed: true}
    }
    if state.ApprovalStatus == ApprovalApproved && state.ApprovalFingerprint == state.PlanFingerprint {
        return DispatchDecision{Allowed: true}
    }
    return DispatchDecision{Reason: "human approval is required for the current production plan"}
}
```

`ParseState` must accept absent BPA keys as a disabled state, reject invalid BPA scalar types as malformed, and read only these namespaced keys: `bpa.template`, `bpa.waiting_for`, `bpa.production_action`, `bpa.plan_fingerprint`, `bpa.approval_fingerprint`, `bpa.approval_status`, `bpa.approval_summary`, `bpa.blocker_owner`, and `bpa.blocker_action`.

- [ ] **Step 4: Add upstream update and changelog documents**

Create `docs/bpa/CHANGELOG.md` with the first dated entry describing this fork's native workflow scope and explicit non-goals. Create `docs/bpa/upstream-sync.md` with these exact operational commands:

```bash
git fetch upstream main
git switch main
git merge --ff-only upstream/main
git switch bpa/main
git switch -c upstream-sync/$(date +%F)
git merge main
cd server && go test ./internal/bpa ./internal/handler
cd .. && pnpm typecheck && pnpm test
```

The document must state that this procedure does not deploy or mutate a live workspace.

- [ ] **Step 5: Run the focused tests and format Go code**

Run: `gofmt -w server/internal/bpa/workflow.go server/internal/bpa/workflow_test.go && cd server && go test ./internal/bpa`

Expected: PASS.

- [ ] **Step 6: Commit the policy foundation**

```bash
git add server/internal/bpa/workflow.go server/internal/bpa/workflow_test.go docs/bpa/CHANGELOG.md docs/bpa/upstream-sync.md
git commit -m "feat(bpa): add native workflow policy"
```

## Task 2: Add server-side template and human-approval endpoints

**Files:**
- Create: `server/internal/handler/bpa_workflow.go`
- Create: `server/internal/handler/bpa_workflow_test.go`
- Modify: `server/cmd/server/router.go`
- Modify: `docs/bpa/CHANGELOG.md`

**Interfaces:**
- Consumes `bpa.ValidateTemplateStart`, `bpa.PlanFingerprint`, and `bpa.ParseState`.
- Produces `POST /api/issues/{id}/bpa/template`, `POST /api/issues/{id}/bpa/approval-request`, `POST /api/issues/{id}/bpa/approval-decision`, and `GET /api/issues/{id}/bpa/workflow`.
- All writes use existing single-key `SetIssueMetadataKey` query semantics, never a JSONB blob overwrite.

- [ ] **Step 1: Write failing handler tests**

```go
func TestStartBPAWorkflowRejectsMainIssueWithoutAgentLead(t *testing.T) {
    issueID := createMetadataTestIssue(t, "member-owned root")
    w := httptest.NewRecorder()
    req := newRequest("POST", "/api/issues/"+issueID+"/bpa/template", StartBPAWorkflowRequest{Template: bpa.TemplateStandard})
    req = withURLParam(req, "id", issueID)
    testHandler.StartBPAWorkflow(w, req)
    if w.Code != http.StatusBadRequest { t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String()) }
}

func TestApproveBPAWorkflowRejectsAgentActor(t *testing.T) {
    issueID := createProductionWorkflowIssue(t, "approval actor")
    w := httptest.NewRecorder()
    req := newRequestAsAgent(testAgentID, "POST", "/api/issues/"+issueID+"/bpa/approval-decision", ApprovalDecisionRequest{Decision: "approved"})
    req = withURLParam(req, "id", issueID)
    testHandler.DecideBPAApproval(w, req)
    if w.Code != http.StatusForbidden { t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String()) }
}

func TestApprovalBecomesInvalidWhenPlanFingerprintChanges(t *testing.T) {
    issueID := createProductionWorkflowIssue(t, "approval invalidation")
    requestBPAApproval(t, issueID, "deploy revision A")
    decideBPAApprovalAsMember(t, issueID, "approved")
    requestBPAApproval(t, issueID, "deploy revision B")
    state := getBPAWorkflow(t, issueID)
    if state.ApprovalStatus != "pending" || state.ApprovalFingerprint == state.PlanFingerprint { t.Fatalf("stale approval remained valid: %#v", state) }
}
```

The tests must use the existing `newRequest`, `withURLParams`, `testHandler`, and database cleanup helpers used by `issue_metadata_test.go` and `issue_child_done_test.go`.

- [ ] **Step 2: Run the handler tests to verify they fail**

Run: `cd server && go test ./internal/handler -run 'TestStartBPAWorkflow|TestApproveBPAWorkflow|TestApprovalBecomesInvalid'`

Expected: FAIL because the handler methods and routes do not exist.

- [ ] **Step 3: Implement template start and approval endpoints**

Implement request structures:

```go
type StartBPAWorkflowRequest struct { Template bpa.Template `json:"template"` }
type ApprovalRequest struct { Plan string `json:"plan"`; Summary string `json:"summary"`; ProductionAction bool `json:"production_action"` }
type ApprovalDecisionRequest struct { Decision string `json:"decision"`; Note string `json:"note"` }
```

`StartBPAWorkflow` loads the issue, rejects child issues, validates that the root assignee is an agent or squad Lead, and writes `bpa.template` plus the default `bpa.waiting_for=lead`.

`RequestBPAApproval` is valid only for a `production` template and only when `production_action=true`. It computes `bpa.PlanFingerprint(plan)`, first writes `bpa.production_action=true`, then writes the new fingerprint, `bpa.approval_summary`, and `bpa.waiting_for=human_approval`. A changed fingerprint cannot reuse an older approval because the enqueue guard requires exact fingerprint equality; clear the older approval status and fingerprint after writing the new fingerprint. `Summary` is stored only as `bpa.approval_summary` and must be the short Ukrainian human-facing form: action, environment, impact, rollback, risk. This endpoint must not post an automatic comment.

`DecideBPAApproval` calls `resolveActor` and rejects every non-`member` actor with HTTP 403. It accepts only `approved` or `rejected`, writes the decision plus the current fingerprint, and never changes the issue status itself.

Every metadata write publishes `protocol.EventIssueMetadataChanged` with the whole parsed metadata map, matching `issue_metadata.go`.

- [ ] **Step 4: Register the routes under the existing issue router**

Add the four routes directly beside the existing `/metadata` routes in `server/cmd/server/router.go`:

```go
r.Get("/bpa/workflow", h.GetBPAWorkflow)
r.Post("/bpa/template", h.StartBPAWorkflow)
r.Post("/bpa/approval-request", h.RequestBPAApproval)
r.Post("/bpa/approval-decision", h.DecideBPAApproval)
```

- [ ] **Step 5: Format and verify the server behavior**

Run: `gofmt -w server/internal/handler/bpa_workflow.go server/internal/handler/bpa_workflow_test.go && cd server && go test ./internal/handler -run 'TestStartBPAWorkflow|TestApproveBPAWorkflow|TestApprovalBecomesInvalid|TestIssueMetadata'`

Expected: PASS.

- [ ] **Step 6: Append the endpoint change to the changelog and commit**

```bash
git add server/internal/handler/bpa_workflow.go server/internal/handler/bpa_workflow_test.go server/cmd/server/router.go docs/bpa/CHANGELOG.md
git commit -m "feat(bpa): add template and approval endpoints"
```

## Task 3: Enforce the production gate in every task-enqueue path

**Files:**
- Modify: `server/internal/service/task.go`
- Create: `server/internal/service/bpa_task_gate_test.go`
- Modify: `docs/bpa/CHANGELOG.md`

**Interfaces:**
- Consumes `bpa.ParseState` and `bpa.CanDispatch`.
- Produces `TaskService.CanEnqueueIssue(issue db.Issue) error`, called before all non-deferred task creation, before `RerunIssue` cancels an existing task, before retry creation, and immediately before a deferred task becomes queued.
- Preserves `IssueService.WillEnqueueRun` as the authority for ordinary enqueue eligibility; it must not know BPA policy.

- [ ] **Step 1: Write the failing dispatch-boundary tests**

```go
func TestEnqueueTaskForIssueRejectsProductionActionWithoutHumanApproval(t *testing.T) {
    issue := createTaskGateIssue(t, map[string]any{"bpa.template": "production", "bpa.production_action": true, "bpa.plan_fingerprint": "plan-a"})
    _, err := testTaskService.EnqueueTaskForIssue(context.Background(), issue)
    if !errors.Is(err, bpa.ErrHumanApprovalRequired) { t.Fatalf("want approval error, got %v", err) }
    assertNoTaskForIssue(t, issue.ID)
}

func TestEnqueueTaskForMentionRejectsProductionActionWithoutHumanApproval(t *testing.T) {
    issue := createTaskGateIssue(t, map[string]any{"bpa.template": "production", "bpa.production_action": true, "bpa.plan_fingerprint": "plan-a"})
    _, err := testTaskService.EnqueueTaskForMention(context.Background(), issue, testAgentID, pgtype.UUID{})
    if !errors.Is(err, bpa.ErrHumanApprovalRequired) { t.Fatalf("want approval error, got %v", err) }
    assertNoTaskForIssue(t, issue.ID)
}

func TestRerunIssueRejectsBeforeCancellingExistingTask(t *testing.T) {
    issue, queued := createQueuedProductionActionTask(t, "plan-a")
    _, err := testTaskService.RerunIssue(context.Background(), issue.ID, pgtype.UUID{}, pgtype.UUID{})
    if !errors.Is(err, bpa.ErrHumanApprovalRequired) { t.Fatalf("want approval error, got %v", err) }
    assertTaskStillQueued(t, queued.ID)
}

func TestAutoRetryDoesNotCreateChildForUnapprovedProductionAction(t *testing.T) {
    parent := createFailedProductionActionTask(t, "plan-a")
    child, err := testTaskService.MaybeRetryFailedTask(context.Background(), parent)
    if err != nil || child != nil { t.Fatalf("want no retry child, child=%#v err=%v", child, err) }
}

func TestDeferredActionDoesNotPromoteUntilMatchingApprovalExists(t *testing.T) {
    issue, deferred := createDueDeferredProductionAction(t, "plan-a")
    if err := testTaskService.PromoteDueDeferredTasksForRuntime(context.Background(), deferred.RuntimeID); err != nil { t.Fatal(err) }
    assertTaskStillDeferred(t, deferred.ID)
    approveTaskGateIssue(t, issue.ID, "plan-a")
    if err := testTaskService.PromoteDueDeferredTasksForRuntime(context.Background(), deferred.RuntimeID); err != nil { t.Fatal(err) }
    assertTaskQueued(t, deferred.ID)
}
```

- [ ] **Step 2: Run the tests and verify the first test fails**

Run: `cd server && go test ./internal/service -run 'TestEnqueueTask.*Approval|TestRerunIssueRejectsBeforeCancelling|TestAutoRetryDoesNotCreateChild|TestDeferredActionDoesNotPromote|TestEnqueueTaskForIssueDoesNotGateStandard'`

Expected: FAIL because the task service currently creates every requested task.

- [ ] **Step 3: Add one fail-closed service guard before all task creation**

Add `TaskService.CanEnqueueIssue(issue db.Issue) error` in `server/internal/service/task.go`. It parses `issue.Metadata`, calls `bpa.CanDispatch(state, state.ProductionAction)`, and returns the stable error `bpa.ErrHumanApprovalRequired` when approval is absent, rejected, stale, or malformed. Call it as the first action in `enqueueIssueTaskWithCommentPlan` and `enqueueMentionTaskWithCommentPlan`, before loading the agent or creating a task. Call it in `RerunIssue` immediately after loading the issue and before `CancelAgentTasksByIssueAndAgent`, so a blocked retry never destroys an existing task. Before each `CreateRetryTask`, load the parent issue and skip retry when the guard denies it. Before promoting each due deferred task, load its issue and leave it deferred when the guard denies it; promote only the allowed subset.

Do not gate a production template until `bpa.production_action=true` exists. That preserves Lead-led investigation and preparation runs while making the explicit action run impossible without a fresh matching member approval. Do not alter `WillEnqueueRun`, `stageBarrierClosed`, `notifyParentOfChildDone`, task queue schema, or generic comment routing.

- [ ] **Step 4: Verify focused behavior and existing enqueue coverage**

Run: `gofmt -w server/internal/service/task.go server/internal/service/bpa_task_gate_test.go && cd server && go test ./internal/service -run 'TestEnqueueTask|TestRerunIssue|TestAutoRetry|TestDeferredAction' && go test ./internal/service -run TestWillEnqueueRun`

Expected: PASS.

- [ ] **Step 5: Append the gate change and commit**

```bash
git add server/internal/service/task.go server/internal/service/bpa_task_gate_test.go docs/bpa/CHANGELOG.md
git commit -m "feat(bpa): gate production task runs on human approval"
```

## Task 4: Expose typed BPA state to the shared client and issue detail

**Files:**
- Create: `packages/core/bpa-workflow/types.ts`
- Create: `packages/core/bpa-workflow/types.test.ts`
- Modify: `packages/core/types/api.ts`
- Modify: `packages/core/api/client.ts`
- Modify: `packages/core/index.ts`
- Modify: `docs/bpa/CHANGELOG.md`

**Interfaces:**
- Produces `parseBpaWorkflowState(metadata: IssueMetadata): BpaWorkflowState` and API methods `getBpaWorkflow`, `startBpaWorkflow`, `requestBpaApproval`, and `decideBpaApproval`.
- Consumes only the standard `Issue.metadata` field and the Task 2 endpoints.

- [ ] **Step 1: Write failing parser tests**

```ts
it("returns disabled for ordinary issue metadata", () => {
  expect(parseBpaWorkflowState({})).toEqual({ enabled: false });
});

it("reports a human approval wait from BPA metadata", () => {
  expect(parseBpaWorkflowState({ "bpa.template": "production", "bpa.waiting_for": "human_approval" }))
    .toMatchObject({ enabled: true, waitingFor: "human_approval" });
});
```

- [ ] **Step 2: Run the parser test and verify it fails**

Run: `pnpm exec vitest run packages/core/bpa-workflow/types.test.ts`

Expected: FAIL because the BPA workflow module does not exist.

- [ ] **Step 3: Implement parser and API client methods**

Use strict string-literal unions:

```ts
export type BpaTemplate = "standard" | "production" | "investigation";
export type BpaWaitingFor = "lead" | "quality" | "human_approval" | null;
export type BpaWorkflowState = { enabled: boolean; template?: BpaTemplate; waitingFor?: BpaWaitingFor; productionAction?: boolean; approvalSummary?: string; blockerOwner?: string; blockerAction?: string; approvalStatus?: "pending" | "approved" | "rejected" };
```

All API responses must pass through the existing `parseWithFallback` schema mechanism before reaching the view layer.

- [ ] **Step 4: Run type and focused unit checks**

Run: `pnpm exec vitest run packages/core/bpa-workflow/types.test.ts && pnpm typecheck --filter=@multica/core`

Expected: PASS.

- [ ] **Step 5: Append changelog entry and commit**

```bash
git add packages/core/bpa-workflow/types.ts packages/core/bpa-workflow/types.test.ts packages/core/types/api.ts packages/core/api/client.ts packages/core/index.ts docs/bpa/CHANGELOG.md
git commit -m "feat(bpa): expose workflow state to clients"
```

## Task 5: Add concise BPA waiting-state UI without a new board or status

**Files:**
- Create: `packages/views/bpa-workflow/workflow-state-notice.tsx`
- Create: `packages/views/bpa-workflow/workflow-state-notice.test.tsx`
- Create: `packages/views/bpa-workflow/index.ts`
- Modify: `packages/views/issues/components/issue-detail.tsx`
- Modify: `packages/views/issues/components/issue-detail.test.tsx`
- Modify: `docs/bpa/CHANGELOG.md`

**Interfaces:**
- Consumes `Issue` and `parseBpaWorkflowState`.
- Produces `<BpaWorkflowStateNotice issue={issue} />`, which returns `null` for non-BPA issues, plus a member-only compact approval control when `waitingFor` is `human_approval`.

- [ ] **Step 1: Write failing shared-view tests**

```tsx
it("shows Quality as the next owner", () => {
  render(<BpaWorkflowStateNotice issue={issueWith({ "bpa.template": "standard", "bpa.waiting_for": "quality" })} />);
  expect(screen.getByText("Наступне: Quality перевіряє результат.")).toBeInTheDocument();
});

it("shows a concise human production decision", () => {
  render(<BpaWorkflowStateNotice issue={issueWith({ "bpa.template": "production", "bpa.waiting_for": "human_approval", "bpa.approval_summary": "Дія: deploy. Середовище: production. Вплив: коротка пауза. Відкат: попередня версія. Ризик: низький." })} />);
  expect(screen.getByText("Потрібне ваше погодження перед production-дією.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Погодити" })).toBeInTheDocument();
});
```

- [ ] **Step 2: Run the view tests to verify they fail**

Run: `pnpm exec vitest run packages/views/bpa-workflow/workflow-state-notice.test.tsx`

Expected: FAIL because the shared BPA view does not exist.

- [ ] **Step 3: Implement the notice and mount it in issue detail**

Render no more than four short Ukrainian paragraphs: `Статус`, `Змінено`, `Ризик`, `Наступне`. For an approval wait, render the plain `bpa.approval_summary` under `Змінено` and show only `Погодити` and `Відхилити` buttons to a signed-in member; the handler response must invalidate the issue query and remove both buttons after either decision. The component must never render comments, raw metadata keys, JSON, an agent transcript, the plan fingerprint, or technical task-run details.

Mount the component in `IssueDetail` under the issue header and above the activity timeline. Keep platform routing and server state out of the view; use `Issue.metadata` already supplied by the detail query.

- [ ] **Step 4: Verify shared view, issue detail, lint, and typecheck**

Run: `pnpm exec vitest run packages/views/bpa-workflow/workflow-state-notice.test.tsx packages/views/issues/components/issue-detail.test.tsx && pnpm lint --filter=@multica/views && pnpm typecheck --filter=@multica/views`

Expected: PASS.

- [ ] **Step 5: Append changelog entry and commit**

```bash
git add packages/views/bpa-workflow packages/views/issues/components/issue-detail.tsx packages/views/issues/components/issue-detail.test.tsx docs/bpa/CHANGELOG.md
git commit -m "feat(bpa): show workflow ownership in issue detail"
```

## Task 6: Encode the five-role contract and run an isolated local pilot

**Files:**
- Create: `server/internal/service/builtin_skills/bpa-task-workflow/SKILL.md`
- Create: `server/internal/service/builtin_skills/bpa-task-workflow/references/bpa-workflow-source-map.md`
- Modify: `server/internal/service/builtin_skills_test.go`
- Create: `docs/bpa/local-pilot.md`
- Modify: `docs/bpa/CHANGELOG.md`

**Interfaces:**
- Consumes native `issue create --parent --stage`, the Task 2 BPA API routes, and the Task 5 state notice.
- Produces a built-in agent skill for Team Lead, Builder, n8n Prod, Documentation Writer, and Quality.

- [ ] **Step 1: Write a failing built-in-skill registration test**

```go
func TestBuiltinSkillsIncludeBPATaskWorkflow(t *testing.T) {
    skill, ok := builtinSkills["bpa-task-workflow"]
    if !ok { t.Fatal("missing bpa-task-workflow") }
    if !strings.Contains(skill.Content, "Результат:") { t.Fatal("missing concise handoff contract") }
}
```

- [ ] **Step 2: Run the registration test to verify it fails**

Run: `cd server && go test ./internal/service -run TestBuiltinSkillsIncludeBPATaskWorkflow`

Expected: FAIL because the BPA skill is not registered.

- [ ] **Step 3: Implement the role contract**

The skill must state exactly:

- Team Lead owns every enabled root issue and is the only role that creates later stages or closes the root.
- Builder, n8n Prod, and Documentation Writer own one child issue and one durable result each.
- Quality owns a separate child in the quality stage and returns a named delivery issue for changes instead of reassigning the review issue.
- A production request pauses at `bpa.waiting_for=human_approval`; no agent retries or bypasses it.
- Every terminal handoff uses the four paragraphs from the design document.

The embed loader discovers the directory automatically; do not add a registry. Add `TestBuiltinSkillsIncludeBPATaskWorkflow` to `server/internal/service/builtin_skills_test.go`, using `findSkill(t, "multica-bpa-task-workflow")`, and assert the loaded content contains `Результат:` and `bpa.waiting_for=human_approval`. Update the skill source map with links to `issue_child_done.go`, `bpa_workflow.go`, and `task.go`.

- [ ] **Step 4: Create the read-only local pilot runbook**

`docs/bpa/local-pilot.md` must describe a local `standard` workflow with one Team Lead root, one Documentation Writer stage-1 child, and one Quality stage-2 child. It must include explicit expected observations after each native wake and a cleanup section that only deletes pilot-local data. It must state that no production service, deployment, IAM binding, secret, or external data write is allowed.

- [ ] **Step 5: Run focused tests and repository checks**

Run: `cd server && go test ./internal/bpa ./internal/handler ./internal/service && cd .. && pnpm exec vitest run packages/core/bpa-workflow/types.test.ts packages/views/bpa-workflow/workflow-state-notice.test.tsx && pnpm typecheck`

Expected: PASS.

- [ ] **Step 6: Execute the local pilot only after checks pass**

Run the runbook against an isolated local database/worktree. Record the observed root owner, child stage, system stage notice, Quality decision, final Lead closeout, and cleanup result in `docs/bpa/CHANGELOG.md`. Stop and investigate if the parent does not wake after the stage barrier; do not add a fallback controller.

- [ ] **Step 7: Commit role contract and pilot evidence**

```bash
git add server/internal/service/builtin_skills/bpa-task-workflow server/internal/service/builtin_skills_test.go docs/bpa/local-pilot.md docs/bpa/CHANGELOG.md
git commit -m "feat(bpa): add lead-centered team workflow"
```

## Plan Self-Review

- Spec coverage: Tasks 1–2 implement templates, metadata state, member-only approval, and upstream hygiene; Task 3 enforces the production dispatch gate; Tasks 4–5 make state visible without a new board; Task 6 adds role contracts and the local pilot.
- No second engine: every task preserves Multica's existing stage barrier and `WillEnqueueRun` predicate. No task creates a scheduler, event bus, routing comment, or custom status.
- Type consistency: server `bpa.State` maps to client `BpaWorkflowState`; both use `standard | production | investigation` and `lead | quality | human_approval` literals.
- Safety: every task is local-only and the final pilot explicitly prohibits production-impacting actions.
