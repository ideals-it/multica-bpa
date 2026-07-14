# Simple Task Board Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn upstream Multica into a simple AI task board with one-agent ticket ownership, task-level production approval, recoverable scheduled checks, safe local artifact links, and a four-agent live configuration.

**Architecture:** Keep upstream board, issue lifecycle, runtime, comment formatting, mentions, and UI unchanged. Add five isolated downstream behaviors at existing server/Desktop boundaries, then migrate live configuration from orchestration roles to **Codex**, **EventCatalog**, **n8n Prod**, and read-only **Archivist**. Every code task is independently testable and committed before live configuration changes.

**Tech Stack:** Go, PostgreSQL/sqlc, Chi, Electron, TypeScript, React Markdown, Vitest, pnpm/Turborepo, Multica CLI.

## Global Constraints

- Base all implementation on `upstream/main` commit `8614b198` in branch `codex/simple-task-board`.
- Do not change Multica UI, board columns, predefined statuses, or status labels.
- Do not port Team Lead, Builder, Quality, GitHub Ops, child-handoff, workflow-template, or server-side formatting behavior.
- A normal ticket has one assigned agent and one conversation for its full lifecycle.
- Production approval is bound to the current ticket title and description and applies once to that scope.
- Only a human member may approve; supported reactions are 👍 and 👌.
- All `create_issue` autopilots create an assigned `Backlog` ticket and do not enqueue an agent task.
- Reuse upstream task auto-retry for already-created failed tasks; the new retry path covers only a scheduled `run_only` occurrence skipped before task creation because its runtime is unavailable.
- Reuse upstream comment formatting, clickable mentions, assignment entry steps, and blocker reporting; change only its unconditional final `In Review` instruction.
- Long-running agent comments, final-result style, user mentions, commit policy, and specialist boundaries remain live instructions rather than server formatting rules.
- Never expose secret values. Do not mutate Doppler, Keychain, Secret Manager, IAM, or production without explicit approval.
- Do not push or deploy until the user explicitly authorizes those actions.

---

### Task 1: Generic ticket-scope approval policy

**Files:**
- Create: `server/internal/approval/policy.go`
- Create: `server/internal/approval/policy_test.go`

**Interfaces:**
- Produces `approval.ScopeFingerprint(title, description string) string`.
- Produces `approval.Parse(metadata map[string]any) (State, error)`.
- Produces `approval.IsApprovalText(content string) bool`.
- Produces `approval.IsApprovalEmoji(emoji string) bool`.
- Metadata namespace: `production_approval.status`, `production_approval.scope`, `production_approval.approved_scope`, `production_approval.request_comment_id`.

- [ ] **Step 1: Write the failing policy tests**

```go
func TestScopeFingerprintIgnoresOuterWhitespace(t *testing.T) {
	if ScopeFingerprint(" Deploy ", " image v2\n") != ScopeFingerprint("Deploy", "image v2") {
		t.Fatal("equivalent ticket scope produced different fingerprints")
	}
}

func TestApprovalText(t *testing.T) {
	for _, text := range []string{"Погоджую", "погоджено", "так", "ок", "approve", "approved", "go ahead", "роби", "запускай"} {
		if !IsApprovalText(text) { t.Errorf("expected approval: %q", text) }
	}
	for _, text := range []string{"не погоджую", "не роби", "поясни ризик", "поки ні"} {
		if IsApprovalText(text) { t.Errorf("unexpected approval: %q", text) }
	}
}

func TestApprovalEmoji(t *testing.T) {
	if !IsApprovalEmoji("👍") || !IsApprovalEmoji("👌") || IsApprovalEmoji("❤️") {
		t.Fatal("approval emoji set is incorrect")
	}
}
```

- [ ] **Step 2: Run the focused test and confirm the package is absent**

Run: `cd server && go test ./internal/approval -count=1`

Expected: FAIL because `server/internal/approval` does not exist.

- [ ] **Step 3: Implement the pure policy**

```go
type Status string

const (
	Pending  Status = "pending"
	Approved Status = "approved"
)

type State struct {
	Status           Status
	Scope            string
	ApprovedScope    string
	RequestCommentID string
}

func ScopeFingerprint(title, description string) string {
	normalized := strings.TrimSpace(title) + "\n" + strings.TrimSpace(description)
	sum := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (s State) AllowsCurrentScope(scope string) bool {
	return s.Status == Approved && scope != "" && s.ApprovedScope == scope
}
```

Normalize text with lower-casing, punctuation trimming, and removal of a leading Multica mention. Check explicit negative phrases before the positive allow-list.

- [ ] **Step 4: Format and verify the policy**

Run: `gofmt -w server/internal/approval && cd server && go test ./internal/approval -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the policy unit**

```bash
git add server/internal/approval
git commit -m "feat: add ticket-scope approval policy"
```

### Task 2: Production approval lifecycle in existing handlers

**Files:**
- Create: `server/internal/handler/production_approval.go`
- Create: `server/internal/handler/production_approval_test.go`
- Modify: `server/pkg/db/queries/issue.sql`
- Modify: `server/pkg/db/generated/issue.sql.go`
- Modify: `server/internal/handler/issue.go`
- Modify: `server/internal/handler/comment.go`
- Modify: `server/internal/handler/reaction.go`

**Interfaces:**
- Consumes `approval.State` and `approval.ScopeFingerprint` from Task 1.
- Produces sqlc query `SetIssueMetadataValues(ctx, SetIssueMetadataValuesParams) (Issue, error)` using atomic JSONB merge.
- Produces `beginProductionReview`, `approveProductionReviewComment`, and `approveProductionReviewReaction` handler helpers.
- Resumes work through existing `TaskService.EnqueueTaskForIssue` on the same issue and assignee.

- [ ] **Step 1: Add failing integration tests**

Create table-driven handler tests with these complete request/result cases:

| Case | Actor and request | Required result |
|---|---|---|
| Start review | assigned agent PATCHes `status=in_review` | HTTP 200; pending metadata; scope equals fingerprint of title+description; no continuation task |
| Text approval | member POSTs `Погоджую` | HTTP 201; approved metadata; exactly one queued task for the issue assignee |
| Emoji approval | member reacts 👍 to the stored request comment | HTTP 201; approved metadata; exactly one queued task for the issue assignee |
| Request binding | assigned agent posts the first comment after entering review | comment ID is stored as `production_approval.request_comment_id` |
| Wrong actor | agent posts approval or reacts 👍 | ordinary comment/reaction only; pending metadata; zero continuation tasks |
| Wrong comment | member reacts 👍 to a different comment | ordinary reaction only; pending metadata; zero continuation tasks |
| Changed scope | title or description changes after approval | approved scope cleared; pending metadata; zero continuation tasks |
| Manual bypass | member PATCHes pending ticket away from `in_review` | HTTP 409; status remains `in_review` |
| No loop | assigned agent keeps unchanged approved ticket in `in_review` | approval remains valid; no new pending request |

Use existing `newRequest`, `testHandler`, `insertAgentAssignedIssue`, and direct SQL count helpers from `server/internal/handler/*_test.go`; cleanup every inserted comment, task, issue, and agent with `t.Cleanup`.

- [ ] **Step 2: Run the focused tests and verify failure**

Run: `cd server && go test ./internal/handler -run 'Test(AgentMoveToInReview|MemberApproval|AgentReaction|ReactionOnUnrelated|TicketScopeChange|PendingApproval|ApprovedUnchanged)' -count=1`

Expected: FAIL because production approval metadata and resume hooks do not exist.

- [ ] **Step 3: Add atomic metadata merge and regenerate sqlc**

```sql
-- name: SetIssueMetadataValues :one
UPDATE issue
SET metadata = metadata || sqlc.arg('values')::jsonb,
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND workspace_id = sqlc.arg('workspace_id')
RETURNING *;
```

Run: `make sqlc`

Expected: generated `SetIssueMetadataValuesParams` contains `ID`, `WorkspaceID`, and `Values`.

- [ ] **Step 4: Implement pending-review creation**

```go
func (h *Handler) beginProductionReview(r *http.Request, issue db.Issue, requestCommentID string) (db.Issue, error) {
	description := ""
	if issue.Description.Valid { description = issue.Description.String }
	scope := approval.ScopeFingerprint(issue.Title, description)
	return h.setProductionApprovalValues(r, issue, map[string]any{
		"production_approval.status":             string(approval.Pending),
		"production_approval.scope":              scope,
		"production_approval.approved_scope":     "",
		"production_approval.request_comment_id": requestCommentID,
	})
}
```

In `UpdateIssue`, initialize or refresh pending review only when an agent moves its assigned root issue to `in_review`, or changes title/description while a pending review exists. Reject leaving `in_review` while pending. A member dragging a card must not silently approve it. The first top-level comment written by the assigned agent while the request is pending becomes `request_comment_id`; this supports the normal sequence “move to In Review, then explain the approval request.”

- [ ] **Step 5: Implement comment and reaction approval**

```go
func (h *Handler) completeProductionApproval(ctx context.Context, issue db.Issue, state approval.State) error {
	description := ""
	if issue.Description.Valid { description = issue.Description.String }
	current := approval.ScopeFingerprint(issue.Title, description)
	if current != state.Scope { return nil }
	updated, err := h.setProductionApprovalValuesContext(ctx, issue, map[string]any{
		"production_approval.status":         string(approval.Approved),
		"production_approval.approved_scope": current,
	})
	if err != nil { return err }
	_, err = h.TaskService.EnqueueTaskForIssue(ctx, updated)
	return err
}
```

`CreateComment` accepts approval only from `authorType == "member"` while the issue is pending in `in_review`. When approval is accepted, skip the ordinary comment-trigger enqueue path and create exactly one explicit continuation. `AddReaction` additionally requires the reacted-to comment ID to equal `production_approval.request_comment_id`, the comment author to be the assigned agent, and emoji 👍 or 👌. Existing mention/comment triggers remain unchanged for non-approval comments.

- [ ] **Step 6: Verify handler behavior**

Run: `gofmt -w server/internal/handler/production_approval.go server/internal/handler/production_approval_test.go server/internal/handler/issue.go server/internal/handler/comment.go server/internal/handler/reaction.go && cd server && go test ./internal/handler -run 'Test(AgentMoveToInReview|MemberApproval|AgentReaction|ReactionOnUnrelated|TicketScopeChange|PendingApproval|ApprovedUnchanged)' -count=1`

Expected: PASS with exactly one queued continuation for each valid approval.

- [ ] **Step 7: Commit the handler lifecycle**

```bash
git add server/internal/handler/production_approval.go \
  server/internal/handler/production_approval_test.go \
  server/internal/handler/issue.go server/internal/handler/comment.go \
  server/internal/handler/reaction.go server/pkg/db/queries/issue.sql \
  server/pkg/db/generated/issue.sql.go
git commit -m "feat: gate production tickets on human approval"
```

### Task 3: Single-agent completion status in the built-in assignment protocol

**Files:**
- Modify: `server/internal/daemon/execenv/runtime_config_sections.go`
- Modify: `server/internal/daemon/execenv/runtime_config_test.go`

**Interfaces:**
- Reuses upstream `writeWorkflowAssignment` and all existing Comment Formatting/Agent Identity sections.
- Changes only the final status instruction: `done` for a completed outcome; `in_review` only when a production action is waiting for human approval.

- [ ] **Step 1: Add focused protocol assertions**

Extend the existing assignment-protocol tests with these exact assertions:

| Context | Required generated instruction |
|---|---|
| Ordinary assignment | contains `multica issue status <id> done`; does not instruct unconditional final `in_review` |
| Production work not yet approved | instructs the agent to post the approval request, set `in_review`, and stop before the production action |
| Approval continuation | instructs the agent to verify current approval metadata, perform only the approved scope, then set `done` |
| Blocked task | retains existing `blocked` instruction and explanatory comment requirement |
| Agent Identity forbids status mutation | retains the existing identity-boundary exception |

- [ ] **Step 2: Run the focused tests and prove the upstream conflict**

Run: `cd server && go test ./internal/daemon/execenv -run 'Test(AssignmentTriggeredProtocol|SimpleTaskCompletionStatus)' -count=1`

Expected: FAIL because upstream step 8 always instructs `in_review`.

- [ ] **Step 3: Replace only step 8 of `writeWorkflowAssignment`**

Generate this behavior without changing the other assignment steps:

```go
fmt.Fprintf(b, "8. When the requested outcome is fully complete, run `multica issue status %s done` unless your Agent Identity forbids issue status changes. Use `in_review` only when a production action is still pending human approval: post the concise approval request with a real user mention, set `in_review`, and stop before the production action. On an approval-triggered continuation, verify that `production_approval.approved_scope` matches the current ticket scope, perform only that approved scope, then finish in `done`.\n", ctx.IssueID)
```

Do not duplicate upstream Comment Formatting, mention syntax, metadata-reading steps, `In Progress`, or blocker handling in agent-specific instructions.

- [ ] **Step 4: Verify and commit the protocol change**

Run: `gofmt -w server/internal/daemon/execenv/runtime_config_sections.go server/internal/daemon/execenv/runtime_config_test.go && cd server && go test ./internal/daemon/execenv -run 'Test(AssignmentTriggeredProtocol|SimpleTaskCompletionStatus)' -count=1`

Expected: PASS.

```bash
git add server/internal/daemon/execenv/runtime_config_sections.go server/internal/daemon/execenv/runtime_config_test.go
git commit -m "feat: finish single-agent tasks in done"
```

### Task 4: Backlog-only issue creation by autopilots

**Files:**
- Modify: `server/internal/service/autopilot.go`
- Modify: `server/internal/service/autopilot_test.go`
- Modify: `server/cmd/server/autopilot_dispatch_for_plan_test.go`

**Interfaces:**
- Changes only `execution_mode=create_issue` dispatch.
- Produces assigned issue with `status=backlog`, `origin_type=autopilot`, linked `autopilot_run.issue_id`, and no `agent_task_queue` row.

- [ ] **Step 1: Write failing service regressions**

```go
func TestCreateIssueAutopilotCreatesAssignedBacklogWithoutTask(t *testing.T) {
	// Dispatch once; assert issue.status == "backlog", assignee preserved,
	// run.issue_id is set, and COUNT(agent_task_queue WHERE issue_id=$1) == 0.
}

func TestCreateIssueAutopilotPublishesIssueCreatedWithoutStartingAgent(t *testing.T) {
	// Assert one issue:created event and no task:queued event.
}
```

- [ ] **Step 2: Run the tests and prove upstream behavior is wrong**

Run: `cd server && go test ./internal/service ./cmd/server -run 'TestCreateIssueAutopilotCreatesAssignedBacklog|TestCreateIssueAutopilotPublishesIssueCreated' -count=1`

Expected: FAIL because upstream creates `todo` and enqueues the assignee.

- [ ] **Step 3: Make the minimal dispatch change**

```go
newPosition, err := issueposition.NextTopPosition(ctx, tx, ap.WorkspaceID, "backlog")
// ...
Status: "backlog",
```

Delete only the final `EnqueueTaskForSquadLeader` / `EnqueueTaskForIssue` block from `dispatchCreateIssue`. Keep leader resolution for creator identity, invocation validation at autopilot save-time, subscriber fan-out, issue event publication, run linkage, and deduplication.

- [ ] **Step 4: Verify focused and existing autopilot tests**

Run: `gofmt -w server/internal/service/autopilot.go server/internal/service/autopilot_test.go server/cmd/server/autopilot_dispatch_for_plan_test.go && cd server && go test ./internal/service ./cmd/server -run 'Autopilot|CreateIssue' -count=1`

Expected: PASS; no existing test expects an automatic task from `create_issue`.

- [ ] **Step 5: Commit the backlog behavior**

```bash
git add server/internal/service/autopilot.go server/internal/service/autopilot_test.go server/cmd/server/autopilot_dispatch_for_plan_test.go
git commit -m "feat: keep autopilot issues in backlog"
```

### Task 5: Persist and dispatch deferred scheduled runtime retries

**Files:**
- Create: `server/migrations/179_autopilot_runtime_retry.up.sql`
- Create: `server/migrations/179_autopilot_runtime_retry.down.sql`
- Modify: `server/pkg/db/queries/autopilot.sql`
- Modify: `server/pkg/db/generated/models.go`
- Modify: `server/pkg/db/generated/autopilot.sql.go`
- Modify: `server/internal/handler/autopilot.go`
- Modify: `server/internal/handler/autopilot_list_test.go`
- Modify: `server/internal/service/autopilot.go`
- Modify: `server/internal/service/autopilot_test.go`

**Interfaces:**
- Produces `Autopilot.RetryOnRuntimeUnavailable bool` with JSON field `retry_on_runtime_unavailable`.
- Produces deferred run fields `runtime_retry_attempt`, `runtime_retry_after`, and `runtime_retry_reason`.
- Produces `RecoverRuntimeDeferredRuns(ctx, runtimeID)` and `RecoverDueRuntimeDeferredRuns(ctx)`.

- [ ] **Step 1: Write failing API and service tests**

Cover these exact assertions in handler/service tests:

| Case | Required result |
|---|---|
| API field omitted | create/get returns `retry_on_runtime_unavailable=false` |
| API field updated | PATCH true persists and list/get returns true |
| Opted-in offline scheduled `run_only` | one pending run with original `planned_at`, retry timestamp/reason, and no task |
| Two concurrent recovery calls | one task and one preserved occurrence row |
| Same autopilot already succeeded in the schedule period | deferred row becomes skipped and creates no task |
| Different autopilot succeeded | does not suppress the deferred row |
| `create_issue`, invalid config, or failed task | never enters deferred runtime retry |

- [ ] **Step 2: Run focused tests and verify failure**

Run: `cd server && go test ./internal/handler ./internal/service -run 'Test(AutopilotRetry|DispatchAutopilotForPlanDefers|RecoverRuntimeDeferred|RuntimeRetry)' -count=1`

Expected: FAIL because the schema and service methods are absent.

- [ ] **Step 3: Add migration 179**

```sql
ALTER TABLE autopilot
  ADD COLUMN retry_on_runtime_unavailable BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE autopilot_run
  ADD COLUMN runtime_retry_attempt INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN runtime_retry_after TIMESTAMPTZ,
  ADD COLUMN runtime_retry_reason TEXT;

CREATE INDEX idx_autopilot_run_runtime_retry_due
  ON autopilot_run (runtime_retry_after)
  WHERE runtime_retry_after IS NOT NULL AND status = 'pending';
```

The down migration drops the index, the three `autopilot_run` columns, then `autopilot.retry_on_runtime_unavailable`.

- [ ] **Step 4: Add sqlc queries and regenerate**

Add queries that atomically defer, claim with `FOR UPDATE SKIP LOCKED`, reschedule, exhaust, and dispatch a deferred row while preserving `(trigger_id, planned_at)`. Add a query for a successful run inside the trigger-local schedule period. Run: `make sqlc`.

Expected: generated models expose all four new fields.

- [ ] **Step 5: Implement bounded recovery**

```go
const runtimeRetryMaxAttempts = 6
const runtimeRetryDeadline = 12 * time.Hour

var runtimeRetryBackoff = []time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	time.Hour,
}
```

Only scheduled `run_only` dispatch opted into retry may defer. `create_issue`, invalid configuration, permission failures, and already-started failed tasks never enter this path. Reuse the original occurrence row and create at most one downstream task.

- [ ] **Step 6: Expose the opt-in API field and verify**

Run: `gofmt -w server/internal/handler/autopilot.go server/internal/handler/autopilot_list_test.go server/internal/service/autopilot.go server/internal/service/autopilot_test.go && cd server && go test ./internal/handler ./internal/service -run 'Test(AutopilotRetry|DispatchAutopilotForPlanDefers|RecoverRuntimeDeferred|RuntimeRetry)' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit persistence and recovery**

```bash
git add server/migrations/179_autopilot_runtime_retry.* \
  server/pkg/db/queries/autopilot.sql server/pkg/db/generated \
  server/internal/handler/autopilot.go server/internal/handler/autopilot_list_test.go \
  server/internal/service/autopilot.go server/internal/service/autopilot_test.go
git commit -m "feat: defer autopilots while runtimes are offline"
```

### Task 6: Wake deferred retries when a runtime returns

**Files:**
- Modify: `server/pkg/protocol/events.go`
- Modify: `server/internal/handler/daemon.go`
- Modify: `server/cmd/server/autopilot_listeners.go`
- Modify: `server/cmd/server/main.go`
- Modify: `server/internal/scheduler/jobs_autopilot.go`
- Test: `server/internal/handler/heartbeat_test.go`
- Test: `server/cmd/server/autopilot_listeners_test.go`
- Test: `server/cmd/server/autopilot_schedule_job_test.go`

**Interfaces:**
- Produces `protocol.EventRuntimeOnline` only for offline → online transition.
- Both event listener and periodic fallback call Task 4's shared recovery service.

- [ ] **Step 1: Write failing transition tests**

Cover these exact assertions:

| Case | Required result |
|---|---|
| First heartbeat after offline state | one `runtime:online` event |
| Repeated online heartbeat | no second transition event |
| Runtime-online listener | one recovered task for that runtime's due occurrence |
| Missed transition/server restart | periodic job recovers the same due occurrence |
| Retry deadline/attempt limit reached | run becomes failed once and never creates a task |

- [ ] **Step 2: Run the focused tests and verify failure**

Run: `cd server && go test ./internal/handler ./cmd/server -run 'Test(OfflineToOnlineHeartbeat|RuntimeOnlineRecovers|RuntimeRetryScheduler|RuntimeRetryExhaustion)' -count=1`

Expected: FAIL because the event and registrations do not exist.

- [ ] **Step 3: Publish and consume the runtime transition**

Add `EventRuntimeOnline = "runtime:online"`. Emit only after persisted state changes from offline to online, not on every heartbeat. Register the listener in `registerAutopilotListeners` and invoke `RecoverRuntimeDeferredRuns` in a bounded goroutine.

- [ ] **Step 4: Add restart-safe periodic fallback**

Register a low-frequency scheduler job that calls `RecoverDueRuntimeDeferredRuns`. An already-claimed or no-longer-due row is a no-op, so the heartbeat and scheduler paths may race safely.

- [ ] **Step 5: Format, verify, and commit**

Run: `gofmt -w server/pkg/protocol/events.go server/internal/handler/daemon.go server/cmd/server/autopilot_listeners.go server/cmd/server/main.go server/internal/scheduler/jobs_autopilot.go server/internal/handler/heartbeat_test.go server/cmd/server/autopilot_listeners_test.go server/cmd/server/autopilot_schedule_job_test.go && cd server && go test ./internal/handler ./cmd/server -run 'Test(OfflineToOnlineHeartbeat|RuntimeOnlineRecovers|RuntimeRetryScheduler|RuntimeRetryExhaustion)' -count=1`

Expected: PASS.

```bash
git add server/pkg/protocol/events.go server/internal/handler/daemon.go \
  server/cmd/server/autopilot_listeners.go server/cmd/server/main.go \
  server/internal/scheduler/jobs_autopilot.go server/internal/handler/heartbeat_test.go \
  server/cmd/server/autopilot_listeners_test.go server/cmd/server/autopilot_schedule_job_test.go
git commit -m "feat: resume autopilots when runtimes return"
```

### Task 7: Task-authorized local artifact resolution

**Files:**
- Modify: `server/internal/handler/daemon.go`
- Modify: `server/cmd/server/router.go`
- Create: `server/internal/handler/task_local_artifact_test.go`

**Interfaces:**
- Produces `GET /api/daemon/tasks/{taskId}/local-artifact?path=<relative>`.
- Returns `{ "work_dir": string, "relative_path": string }` only for the authenticated daemon's workspace-owned task.

- [ ] **Step 1: Write failing route tests**

Use a table-driven route test with these inputs and expected statuses:

| Input | Expected |
|---|---|
| owned task, `audit/result.json` | 200 with stored work root and normalized relative path |
| `/tmp/secret` | 400 |
| `../secret` | 400 |
| task from another workspace | 404 |
| owned task with empty `work_dir` | 409 |

Create the task through existing handler test fixtures and authenticate every request with `newDaemonTokenRequest`.

- [ ] **Step 2: Run the focused test and verify the route is absent**

Run: `cd server && go test ./internal/handler -run TestResolveTaskLocalArtifact -count=1`

Expected: FAIL with 404 or missing handler.

- [ ] **Step 3: Implement server-side lexical validation**

```go
clean := filepath.Clean(requested)
if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
	writeError(w, http.StatusBadRequest, "artifact path must stay inside the task work directory")
	return
}
```

Load the task through daemon-token workspace authorization, require non-empty `work_dir`, and return no file-existence information. Register the route beside existing daemon task routes.

- [ ] **Step 4: Verify and commit the API**

Run: `gofmt -w server/internal/handler/daemon.go server/internal/handler/task_local_artifact_test.go server/cmd/server/router.go && cd server && go test ./internal/handler -run TestResolveTaskLocalArtifact -count=1`

Expected: PASS.

```bash
git add server/internal/handler/daemon.go server/internal/handler/task_local_artifact_test.go server/cmd/server/router.go
git commit -m "feat: resolve task-owned local artifacts"
```

### Task 8: Safe Desktop opening and Markdown links

**Files:**
- Create: `apps/desktop/src/main/local-artifact.ts`
- Create: `apps/desktop/src/main/local-artifact.test.ts`
- Modify: `apps/desktop/src/main/index.ts`
- Modify: `apps/desktop/src/preload/index.ts`
- Modify: `apps/desktop/src/preload/index.d.ts`
- Modify: `packages/views/editor/readonly-content.tsx`

**Interfaces:**
- Produces `desktopAPI.openLocalArtifact(taskId, action, artifactPath)`.
- Accepts only `local-artifact://task/<uuid>/<open|reveal>/<relative-path>`.
- Uses Task 6 to obtain the authorized root; Electron enforces real-path containment before calling `shell.openPath` or `shell.showItemInFolder`.

- [ ] **Step 1: Write failing filesystem resolver tests**

Build a temporary root with `mkdtemp`, an inside file, an outside file, and an inside symlink to the outside file. Assert:

| Input | Required result |
|---|---|
| inside file | canonical inside file path |
| `../outside` | rejected before shell call |
| escaping symlink | rejected after `realpath` |
| missing file with inside existing parent and `reveal` | validated parent may be revealed |
| missing file with escaping parent | rejected |

- [ ] **Step 2: Run the focused test and verify failure**

Run: `pnpm --filter @multica/desktop test -- local-artifact.test.ts`

Expected: FAIL because the module is absent.

- [ ] **Step 3: Implement canonical path containment**

```ts
export async function resolveLocalArtifactPath(root: string, relativePath: string): Promise<string> {
  const realRoot = await realpath(root);
  const candidate = resolve(realRoot, relativePath);
  const realCandidate = await realpath(candidate);
  if (realCandidate !== realRoot && !realCandidate.startsWith(`${realRoot}${sep}`)) {
    throw new Error("Artifact path escapes the task directory");
  }
  return realCandidate;
}
```

For `reveal`, validate an existing ancestor when the final file is absent. Never accept a renderer-supplied absolute root.

- [ ] **Step 4: Wire IPC and typed preload API**

Register `local-artifact:open` in Desktop main, request the authorized root from the active local Multica profile, then call Electron shell APIs only after validation. Expose the typed method from preload.

- [ ] **Step 5: Render the custom Markdown protocol safely**

Extend only the read-only Markdown schema with `local-artifact`. Parse the exact URI shape, decode the relative path, and call the Desktop preload API. Outside Desktop render inert text. Never pass this protocol to `openExternal`.

- [ ] **Step 6: Verify and commit Desktop behavior**

Run: `pnpm --filter @multica/desktop test -- local-artifact.test.ts && pnpm typecheck && pnpm test`

Expected: all commands PASS.

```bash
git add apps/desktop/src/main/local-artifact.ts apps/desktop/src/main/local-artifact.test.ts \
  apps/desktop/src/main/index.ts apps/desktop/src/preload/index.ts \
  apps/desktop/src/preload/index.d.ts packages/views/editor/readonly-content.tsx
git commit -m "feat: open task-owned artifacts from Desktop"
```

### Task 9: Full code verification and downstream patch documentation

**Files:**
- Create: `docs/downstream-patches.md`
- Modify: `docs/superpowers/specs/2026-07-14-simple-task-board-design.md`

**Interfaces:**
- Documents each downstream patch, its tests, upstream-touch points, and removal condition.

- [ ] **Step 1: Document the five downstream patches**

Create a table with rows `production approval`, `single-agent completion status`, `autopilot backlog`, `runtime admission retry`, and `local artifact links`; include exact files, why configuration alone cannot implement it, and the condition under which the patch can be deleted after an upstream equivalent lands. Explicitly record that ordinary failed-task retry, Markdown formatting, mentions, and assignment entry/blocker steps are upstream-owned and unchanged.

- [ ] **Step 2: Run the full repository gate**

Run: `make check`

Expected: Go formatting/tests, TypeScript formatting/typecheck/tests, and builds PASS.

- [ ] **Step 3: Run local service smoke checks**

Run: `make dev`, then probe the local health endpoint shown by the dev command and preview one assigned `Backlog` issue trigger.

Expected: health is OK; preview reports no agent run for assigned `Backlog`.

- [ ] **Step 4: Commit verification documentation**

```bash
git add docs/downstream-patches.md docs/superpowers/specs/2026-07-14-simple-task-board-design.md
git commit -m "docs: document minimal Multica patches"
```

### Task 10: Shared agent working contract

**Files:**
- Live workspace configuration: skill `simple-task-agent-contract`
- Live workspace configuration: **Codex** agent instructions
- Live workspace configuration: **EventCatalog** agent instructions
- Live workspace configuration: **n8n Prod** agent instructions
- Live workspace configuration: **Archivist** agent instructions

**Interfaces:**
- Assigns one shared contract to **Codex**, **EventCatalog**, and **n8n Prod** without duplicating it in each role description.
- Keeps role-specific expertise and production boundaries in each agent's own instructions.
- Gives **Archivist** a separate read-only evidence contract rather than the execution contract.
- Uses the unofficial OpenAI prompt snapshots only as design inspiration; do not copy large passages or OpenAI-internal tool/runtime rules.

- [ ] **Step 1: Audit upstream-generated instructions before creating the shared skill**

Generate one current assignment brief and record which behaviors are already supplied by Multica. Do not duplicate these in the workspace skill:

- Reading the issue, recent comments, and issue metadata.
- Entering `In Progress`, reporting a blocker, and posting the final comment.
- Markdown comment formatting and Multica mention syntax.
- Agent Identity boundaries, available CLI commands, runtime authentication, and task context.
- Existing retry behavior for already-created failed tasks.

Expected: the shared skill contains only principles that add behavioral quality or clarify ownership.

- [ ] **Step 2: Create `simple-task-agent-contract` with the curated principles**

Use this semantic contract, written concisely rather than copied verbatim from any source prompt:

```markdown
# Simple Task Agent Contract

## Ownership

- Own the assigned ticket end-to-end within your role: understand, investigate, implement, verify, commit repository changes, and report the result in the same ticket.
- Do not stop at analysis or a partial fix when safe in-scope work can still complete the requested outcome.
- Do not create implementation, review, or Git child tickets merely to represent phases of one task. Split only genuinely independent deliverables.
- Match the ticket's intent: a question or review does not authorize unrelated mutation; an implementation request includes normal safe implementation steps.

## Evidence And Judgment

- Inspect the relevant source of truth before concluding: current code and tests for code behavior; API, logs, deployed configuration, or persisted state for live behavior.
- Separate verified facts from assumptions. State important assumptions and limitations explicitly.
- Fix the root cause. Do not hide failures behind broad retries, ignored warnings, or vague success claims.
- Use concrete evidence when challenging an assumption or recommending a different approach.

## Autonomy And Scope

- Resolve ordinary local blockers yourself, including declared dependency installation, supported project tooling, and already-authorized credential loading through approved mechanisms.
- Make reasonable reversible assumptions that stay within the ticket scope. Ask only when a missing choice materially changes the result, new authority is required, or an external state change is unavoidable.
- Never expand into production, IAM, secret mutation, destructive Git, or another materially different action without the required authorization.
- If blocked, exhaust safe in-scope alternatives, then explain the exact blocker and the single action needed from the user.

## Engineering Quality

- Preserve user and unrelated changes in a dirty worktree. Never revert work you did not create.
- Review your own change before completion with a finding-first mindset: look for bugs, behavioral regressions, security or operational risks, and missing tests.
- Verify proportionally to risk using the repository's formatter, typecheck, tests, build, and relevant local smoke path. State clearly what was not verified.
- Create one coherent task-scoped commit for every repository-changing ticket. Push only when publishing is a natural part of the requested result.

## Production

- Read-only production inspection is allowed when needed to establish facts.
- Before a production change or deployment, finish preparation and validation, post a concise approval request with a real user mention, move the same ticket to `In Review`, and stop.
- Approval applies to the described ticket scope, not to individual commands. A material scope or risk change requires a new approval.
- After approval, perform only the approved production scope, verify it, summarize the result, and move the ticket to `Done`.

## Communication

- Keep the user informed during active long work with one short natural update every 3-5 minutes. Explain the current stage or material progress, not command-by-command activity.
- Use Markdown deliberately: short paragraphs, bold emphasis, inline code, code blocks, lists, and links only when they improve scanning.
- Lead the final comment with the outcome. Keep it self-contained and include only useful verification, material limitations or risks, and remaining user action.
- Prefer plain language over jargon. Do not expose raw logs, tool transcripts, large JSON payloads, secrets, or repetitive technical narration.
- Use a real clickable user mention whenever approval, access, an answer, or another user action is required. Plain-text names are not sufficient.
```

- [ ] **Step 3: Keep role expertise as small overlays**

Apply the shared skill to **Codex**, **EventCatalog**, and **n8n Prod**, then keep only these role-specific additions in agent instructions:

| Agent | Role overlay |
|---|---|
| **Codex** | General engineering and operations across the full ticket lifecycle; may work in any explicitly attached project/resource; default owner for Common Issues |
| **EventCatalog** | Event/service documentation and significance triage in the EventCatalog project; preserve strong technical writing; lint/build after every documentation change; Cloud Run deploy requires approval |
| **n8n Prod** | n8n and its GCE production environment in N8N Prod BPA Team; inspect actual runtime/version/configuration; prepare upgrades and rollback; production mutation requires approval |

Do not repeat the shared ownership, Git, review, communication, formatting, or approval rules inside these overlays.

- [ ] **Step 4: Give Archivist a separate read-only evidence contract**

Use this concise overlay without assigning the execution skill:

```markdown
You are the read-only cross-project knowledge agent. Build answers from Multica tickets, comments, task history, agents, projects, resources, and repository commit history. Separate verified facts from inference and cite concrete ticket IDs, commits, files, or run records. Answer briefly by default and expand deeply when asked. Never create or update tickets, change status, start agents, edit repositories, commit, push, deploy, mutate secrets, or perform production actions. If evidence is incomplete, identify the exact missing source instead of inventing a conclusion.
```

- [ ] **Step 5: Explicitly exclude incompatible prompt-snapshot rules**

Do not transfer:

- OpenAI-internal channels, tools, sandbox paths, product UI, model identity, memory implementation, or system-message precedence.
- Browser, image, ads, frontend-aesthetic, writing-block, or unrelated product policies.
- Prompt-specific update intervals of 30-60 seconds; retain the agreed 3-5 minute ticket cadence.
- A separate auto-review agent; the ticket owner performs self-review.
- Rigid final-answer headings or a mandatory report schema.
- Assumptions that every user message authorizes code changes; respect the ticket's actual intent and scope.

- [ ] **Step 6: Validate the contract on generated task briefs**

Generate one task brief for each execution agent and one for **Archivist**. Assert:

- Each execution brief contains the shared contract exactly once.
- Role overlays contain specialist boundaries without old Team Lead, Builder, Quality, or GitHub Ops handoffs.
- **Archivist** contains the read-only contract and not the execution contract.
- Multica's generated comment formatting and mention instructions remain present exactly once.
- No OpenAI-internal tool names, channels, model claims, system paths, or copied long prompt passages appear.

- [ ] **Step 7: Record provenance without vendoring prompt snapshots**

Add these references to the live skill description or migration record, stating that they were used only as inspiration:

- `https://github.com/asgeirtj/system_prompts_leaks/blob/main/OpenAI/Codex/gpt-5.6.md`
- `https://github.com/asgeirtj/system_prompts_leaks/blob/main/OpenAI/Codex/codex-auto-review.md`
- `https://github.com/asgeirtj/system_prompts_leaks/blob/main/OpenAI/Codex/personality_pragmatic.md`
- `https://github.com/asgeirtj/system_prompts_leaks/blob/main/OpenAI/Codex/personality_friendly.md`

Do not vendor or mirror the source prompt files into the Multica repository.

### Task 11: Live agent and autopilot migration

**Files:**
- Create outside Git: `/tmp/multica-simple-board-backup-2026-07-14/agents.json`
- Create outside Git: `/tmp/multica-simple-board-backup-2026-07-14/autopilots.json`
- Create outside Git: `/tmp/multica-simple-board-backup-2026-07-14/projects.json`
- Create outside Git: `/tmp/multica-simple-board-backup-2026-07-14/active-issues.json`

**Interfaces:**
- Live agents: **Codex**, **EventCatalog**, **n8n Prod**, **Archivist**.
- Live autopilots create assigned `Backlog` tickets; scheduled checker runs use the retry opt-in.
- No archive action occurs until one low-risk ticket per retained agent passes.

- [ ] **Step 1: Snapshot live state without secrets**

```bash
mkdir -p /tmp/multica-simple-board-backup-2026-07-14
multica agent list --output json > /tmp/multica-simple-board-backup-2026-07-14/agents.json
multica autopilot list --output json > /tmp/multica-simple-board-backup-2026-07-14/autopilots.json
multica project list --output json > /tmp/multica-simple-board-backup-2026-07-14/projects.json
multica issue list --output json > /tmp/multica-simple-board-backup-2026-07-14/active-issues.json
```

Expected: four valid JSON files; no custom environment or MCP secret values requested.

- [ ] **Step 2: Create/configure Codex from the current local Codex runtime**

Read the active runtime ID from `multica runtime list --output json`. Create **Codex** with `max_concurrent_tasks=1`, the shared single-agent lifecycle instructions, existing approved MCP configuration copied through a mode that does not print secrets, and the current Codex model/runtime settings. Do not rename or archive AT Team Lead yet.

- [ ] **Step 3: Merge EventCatalog roles**

Rename AT Documentation Writer to **EventCatalog**, keep its writing skills, add `eventcatalog-significance-triage`, remove `eventcatalog-build-check`, set `max_concurrent_tasks=1`, and replace old handoff/Lead instructions with the single-agent lifecycle plus build-after-every-change rule.

- [ ] **Step 4: Simplify n8n Prod and Archivist**

Set **n8n Prod** to `max_concurrent_tasks=1` and remove handoff/Quality/GitHub Ops instructions while retaining n8n/GCE expertise and production approval behavior. Keep **Archivist** read-only, on-demand, and free of automatic execution instructions.

- [ ] **Step 5: Reconfigure autopilots**

- EventCatalog Daily Significance Diff: `run_only`, **EventCatalog**, active, daily, retry enabled; it creates a deduplicated EventCatalog `Backlog` ticket only for material changes.
- EventCatalog Weekly Deploy: `run_only`, **EventCatalog**, active, weekly, retry enabled; it creates a `Backlog` ticket only when undeployed changes exist.
- n8n Stable Version Check: `run_only`, **n8n Prod**, active, daily, retry enabled; it creates a deduplicated N8N Prod BPA Team `Backlog` ticket only for a newer stable version.
- Multica Doctor Daily Audit: `run_only`, **Codex**, active, daily, retry enabled; it stays silent when healthy and creates one deduplicated Common Issues `Backlog` ticket for a real problem.
- Pause EventCatalog Daily Build Check and Comment Router.

Use the new API field `retry_on_runtime_unavailable=true` after updating each checker. Never include credentials in CLI arguments.

- [ ] **Step 6: Verify no autopilot ticket auto-starts**

Trigger one safe checker manually. If it creates a ticket, verify `status=backlog`, correct project/assignee, and zero tasks for that issue. Then move it to `Todo` and verify exactly one task starts.

- [ ] **Step 7: Run one low-risk live ticket per retained execution agent**

Create one non-production validation ticket for **Codex**, **EventCatalog**, and **n8n Prod**. Confirm one-ticket ownership, correct status movement, concise intermediate comments, scoped commit when repository files change, and no child orchestration.

- [ ] **Step 8: Validate production approval without unapproved production mutation**

Use a harmless deployment-preparation ticket. Confirm the assigned agent moves it to `In Review`, a member comment or 👍/👌 on the approval request queues exactly one continuation, and unchanged scope does not request approval again. Stop before any real production mutation unless separately approved.

- [ ] **Step 9: Archive replaced agents only after acceptance**

Archive AT EventCatalog Triage, AT Team Lead, AT Builder, AT Quality, AT GitHub Ops, and AT Multica Doctor after confirming no active issue remains assigned to them. Preserve all history. If any acceptance check fails, keep old agents unarchived and restore autopilots from the snapshot.

### Task 12: Publish and Desktop rollout after explicit approval

**Files:** None beyond committed implementation.

**Interfaces:**
- Publishes `codex/simple-task-board` only after explicit user approval.
- Production/server/Desktop rollout is a separate approval boundary.

- [ ] **Step 1: Inspect final repository state**

Run: `git status --short && git log --oneline upstream/main..HEAD && git diff --stat upstream/main...HEAD`

Expected: clean worktree and only the documented minimal patches plus specs/plans.

- [ ] **Step 2: Request independent code review**

Use `superpowers:requesting-code-review` against `upstream/main...HEAD`. Fix only validated findings consistent with this specification, rerun `make check`, and create separate fix commits.

- [ ] **Step 3: Ask for explicit push/deploy approval**

Do not push, merge, deploy the server, or ship the Desktop build until the user approves the reviewed diff and rollout target.

- [ ] **Step 4: Publish and verify after approval**

Push the branch, integrate by the repository's chosen PR/merge path, deploy the approved target, connect Desktop to the deployed/local server as intended, and repeat the production-approval and backlog-autopilot smoke tests against the actual running version.

## Completion audit — 2026-07-14

This section records the verified state after the original implementation work was integrated into `bpa/main`. The unchecked boxes above remain the original implementation sequence; they are not a current to-do list.

### Implemented in `bpa/main`

- Ticket-scoped production approval, including same-ticket continuation and approval reactions, is implemented through `server/internal/bpa/workflow.go`, `server/internal/handler/issue.go`, and `server/internal/handler/comment.go`.
- A completed normal task is instructed to finish in `Done`; `In Review` is reserved for pending production approval in `server/internal/daemon/execenv/runtime_config_sections.go`.
- Autopilot-created issues remain assigned in `Backlog` without an automatic task in `server/internal/service/autopilot.go`.
- Scheduled runtime retry is implemented and registered through `server/internal/service/autopilot.go`, `server/internal/scheduler/jobs_autopilot.go`, and migration `server/migrations/167_autopilot_runtime_retry.up.sql`.
- Safe Desktop local-artifact links are implemented through `server/internal/handler/daemon.go`, `apps/desktop/src/main/local-artifact.ts`, and `packages/views/editor/readonly-content.tsx`.

### Live configuration completed

- Retained agents are `AT Codex`, `AT EventCatalog`, `AT n8n Prod`, and read-only `AT Archivist`; former orchestration roles are archived.
- The retained agents have the agreed one-ticket ownership, natural-language production approval, progress-comment, mention, and non-template final-result instructions.
- EventCatalog significance triage and the base n8n skill are assigned to their respective retained agents.
- EventCatalog Daily Build Check and the legacy Comment Router are paused.
- A live non-production smoke ticket confirmed `Backlog` does not start work and a manual move to `In Progress` creates one `AT Codex` run that reaches `Done`.

### Remaining acceptance/documentation items

- Low-risk live smoke tickets passed for `AT Codex`, `AT EventCatalog`, and `AT n8n Prod`: each stayed idle in `Backlog`, started once after manual `In Progress`, left one concise comment and reached `Done`.
- Runtime-retry opt-in is enabled for the active scheduled checkers, including `n8n Stable Version Check`.
- `make check` passed against the final merged state on 2026-07-14.
- The companion design is preserved at `docs/superpowers/specs/2026-07-14-simple-task-board-design.md`; downstream ownership is documented in `docs/downstream-patches.md`.
- A separate shared `simple-task-agent-contract` skill was not created. The approved contract is stored directly in the live agent instructions so there is one operational source for the current four-agent setup. This is an intentional simplification relative to Task 10's original implementation shape.
