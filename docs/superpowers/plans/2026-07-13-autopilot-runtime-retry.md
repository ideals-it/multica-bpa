# Autopilot Runtime Retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recover one opted-in scheduled `run_only` Autopilot occurrence after a local runtime returns online, without affecting any other Autopilot plan.

**Architecture:** Persist an opt-in policy on each Autopilot and deferred-retry state on the existing occurrence row. `AutopilotService` owns admission classification, claiming, day-limit checks, and dispatch; a runtime-online listener and a DB-backed periodic job both call that service, so they share one idempotent recovery path.

**Tech Stack:** Go, PostgreSQL migrations, sqlc, existing event bus and scheduler, Go integration tests.

## Global Constraints

- No board, status, comment, or approval UI changes.
- Retry is opt-in per Autopilot and defaults to false.
- Only scheduled `run_only` occurrences skipped solely because a local runtime is unavailable can defer.
- Limit successful work to one run per Autopilot per calendar day in the trigger timezone.
- Do not retry `create_issue`, invalid configuration, access failures, or failed tasks.
- Preserve the original `(trigger_id, planned_at)` identity and never create duplicate issues/tasks.
- No production deployment or push.

---

### Task 1: Persist the opt-in policy and deferred occurrence state

**Files:**
- Create: `server/migrations/137_autopilot_runtime_retry.up.sql`
- Create: `server/migrations/137_autopilot_runtime_retry.down.sql`
- Modify: `server/pkg/db/queries/autopilot.sql`
- Modify: `server/pkg/db/generated/models.go`
- Modify: `server/pkg/db/generated/autopilot.sql.go`
- Modify: `server/internal/handler/autopilot.go`
- Test: `server/internal/handler/autopilot_list_test.go`

**Interfaces:**
- Produces `Autopilot.RetryOnRuntimeUnavailable bool`.
- Produces `AutopilotRun.RuntimeRetryAttempt int32`, `RuntimeRetryAfter pgtype.Timestamptz`, and `RuntimeRetryReason pgtype.Text`.
- Produces create/update/read JSON field `retry_on_runtime_unavailable`.

- [ ] **Step 1: Write failing handler contract tests**

```go
func TestAutopilotRetryOnRuntimeUnavailableDefaultsFalse(t *testing.T) {
    // POST without the field returns false; GET returns false too.
}

func TestAutopilotRetryOnRuntimeUnavailableCanBeUpdated(t *testing.T) {
    // PATCH {"retry_on_runtime_unavailable": true} persists and returns true.
}
```

- [ ] **Step 2: Run the focused tests and verify they fail**

Run: `cd server && go test ./internal/handler -run 'TestAutopilotRetryOnRuntimeUnavailable' -count=1`

Expected: FAIL because the request/response field and generated DB parameter do not exist.

- [ ] **Step 3: Add the migration and SQL queries**

```sql
ALTER TABLE autopilot
  ADD COLUMN retry_on_runtime_unavailable BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE autopilot_run
  ADD COLUMN runtime_retry_attempt INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN runtime_retry_after TIMESTAMPTZ,
  ADD COLUMN runtime_retry_reason TEXT;

CREATE INDEX idx_autopilot_run_runtime_retry_due
  ON autopilot_run (runtime_retry_after)
  WHERE runtime_retry_after IS NOT NULL
    AND status = 'pending';
```

Extend `CreateAutopilot`, `UpdateAutopilot`, and all read models with the boolean. Add dedicated queries, rather than overloading terminal-run updates, for: deferring a run, claiming a due run with `FOR UPDATE SKIP LOCKED`, releasing a claim after a still-offline attempt, and selecting whether the same Autopilot completed a run during a supplied UTC day window.

- [ ] **Step 4: Regenerate sqlc and implement the JSON contract**

Run: `make sqlc`

Add `RetryOnRuntimeUnavailable bool` to `AutopilotResponse`, add a pointer boolean to create/update request payloads, and map it to the generated query parameters. Do not touch frontend files.

- [ ] **Step 5: Run focused tests and formatting**

Run: `gofmt -w server/internal/handler/autopilot.go server/internal/handler/autopilot_list_test.go && cd server && go test ./internal/handler -run 'TestAutopilotRetryOnRuntimeUnavailable' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/migrations/137_autopilot_runtime_retry.up.sql \
  server/migrations/137_autopilot_runtime_retry.down.sql \
  server/pkg/db/queries/autopilot.sql server/pkg/db/generated \
  server/internal/handler/autopilot.go server/internal/handler/autopilot_list_test.go
git commit -m "feat: persist autopilot runtime retry policy"
```

### Task 2: Implement idempotent deferred dispatch in `AutopilotService`

**Files:**
- Modify: `server/internal/service/autopilot.go`
- Modify: `server/pkg/db/queries/autopilot.sql`
- Modify: `server/pkg/db/generated/autopilot.sql.go`
- Test: `server/internal/service/autopilot_test.go`
- Test: `server/cmd/server/autopilot_dispatch_for_plan_test.go`

**Interfaces:**
- Produces `RecoverRuntimeDeferredRuns(ctx context.Context, runtimeID pgtype.UUID) error`.
- Produces `RecoverDueRuntimeDeferredRuns(ctx context.Context) error`.
- `DispatchAutopilotForPlan` leaves an eligible offline occurrence in durable deferred state; all other skips remain terminal.

- [ ] **Step 1: Write failing service tests**

```go
func TestDispatchAutopilotForPlanDefersOptedInOfflineRunOnly(t *testing.T) {
    // one pending deferred run, original planned_at, no agent_task_queue row
}

func TestRecoverRuntimeDeferredRunsDispatchesOccurrenceOnce(t *testing.T) {
    // two concurrent recovery calls yield one task and one run identity
}

func TestRecoverRuntimeDeferredRunsSkipsWhenPlanAlreadySucceededThatDay(t *testing.T) {
    // same autopilot only: no late task after a completed run in trigger timezone
}

func TestRuntimeRetryDoesNotCrossAutopilotBoundaries(t *testing.T) {
    // successful plan A must not suppress deferred plan B
}
```

- [ ] **Step 2: Run focused tests and verify they fail**

Run: `cd server && go test ./internal/service ./cmd/server -run 'Test(DispatchAutopilotForPlanDefers|RecoverRuntimeDeferred|RuntimeRetry)' -count=1`

Expected: FAIL because deferred-run queries and recovery methods are absent.

- [ ] **Step 3: Add named retry policy constants and admission classification**

```go
const (
    runtimeRetryMaxAttempts = 6
    runtimeRetryDeadline    = 12 * time.Hour
)

var runtimeRetryBackoff = []time.Duration{
    time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour,
}
```

Refactor the runtime-unavailable branch of `shouldSkipDispatch` into a typed/explicit classification consumed only by scheduled `run_only` dispatch. When `RetryOnRuntimeUnavailable` is false, retain the existing `skipped` update byte-for-byte in behaviour. When true, create one `pending` run with `runtime_retry_after`, retry reason, and no task.

- [ ] **Step 4: Implement recovery with DB claiming and timezone day-limit**

For each due row, load its plan and trigger, atomically claim it, re-check runtime readiness, and either:

```go
// still offline: increment attempt and set next retry_after;
// exhausted: mark failed with "local runtime was unavailable in time";
// already succeeded during plannedAt's trigger-local day: mark skipped;
// ready and not suppressed: reuse the existing row and enqueue exactly one task.
```

Do not call the current partial-run recovery path for a deferred row: it clears `planned_at`, which would violate occurrence identity. Add an explicit `dispatchDeferredRun` path that transitions the claimed row to `running` only after `CreateAutopilotTask` succeeds.

- [ ] **Step 5: Regenerate sqlc, format, and run focused tests**

Run: `make sqlc && gofmt -w server/internal/service/autopilot.go server/internal/service/autopilot_test.go server/cmd/server/autopilot_dispatch_for_plan_test.go && cd server && go test ./internal/service ./cmd/server -run 'Test(DispatchAutopilotForPlanDefers|RecoverRuntimeDeferred|RuntimeRetry)' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/internal/service/autopilot.go server/internal/service/autopilot_test.go \
  server/cmd/server/autopilot_dispatch_for_plan_test.go \
  server/pkg/db/queries/autopilot.sql server/pkg/db/generated
git commit -m "feat: retry opted-in autopilots after runtime recovery"
```

### Task 3: Trigger recovery from runtime availability and scheduler fallback

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
- Produces `EventRuntimeOnline` containing the runtime ID only when it changes from offline to online.
- Produces a periodic scheduler job that calls `RecoverDueRuntimeDeferredRuns` as a fallback.

- [ ] **Step 1: Write failing trigger tests**

```go
func TestOfflineToOnlineHeartbeatPublishesRuntimeOnlineOnce(t *testing.T) {
    // offline -> online emits; repeated online heartbeat does not emit
}

func TestRuntimeOnlineRecoversDeferredAutopilot(t *testing.T) {
    // listener invokes the shared recovery method and produces one task
}

func TestRuntimeRetrySchedulerRecoversDueRunWithoutHeartbeatEvent(t *testing.T) {
    // due deferred row is recovered by the registered DB-backed job
}
```

- [ ] **Step 2: Run focused tests and verify they fail**

Run: `cd server && go test ./internal/handler ./cmd/server -run 'Test(OfflineToOnlineHeartbeat|RuntimeOnlineRecovers|RuntimeRetryScheduler)' -count=1`

Expected: FAIL because no runtime-online event or retry scheduler job exists.

- [ ] **Step 3: Publish a transition-only runtime event**

Add a protocol event and emit it only after a successful offline-to-online persistence transition. Do not emit on ordinary heartbeats: they are frequent and must not repeatedly scan deferred work. Put the exact transition detection next to `recordHeartbeat`/the synchronous `MarkAgentRuntimeOnline` fallback so HTTP and WebSocket heartbeat paths behave identically.

- [ ] **Step 4: Register the listener and fallback job**

Subscribe in `registerAutopilotListeners` and invoke `RecoverRuntimeDeferredRuns` in a bounded background goroutine. Add a low-frequency DB-backed job in `main.go`; it invokes `RecoverDueRuntimeDeferredRuns`, so recovery still happens after server restarts or a missed event. Both paths must tolerate an already-claimed/no-longer-due row as a no-op.

- [ ] **Step 5: Format and run focused tests**

Run: `gofmt -w server/pkg/protocol/events.go server/internal/handler/daemon.go server/cmd/server/autopilot_listeners.go server/cmd/server/main.go server/internal/scheduler/jobs_autopilot.go server/internal/handler/heartbeat_test.go server/cmd/server/autopilot_listeners_test.go server/cmd/server/autopilot_schedule_job_test.go && cd server && go test ./internal/handler ./cmd/server -run 'Test(OfflineToOnlineHeartbeat|RuntimeOnlineRecovers|RuntimeRetryScheduler)' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/pkg/protocol/events.go server/internal/handler/daemon.go \
  server/cmd/server/autopilot_listeners.go server/cmd/server/main.go \
  server/internal/scheduler/jobs_autopilot.go \
  server/internal/handler/heartbeat_test.go server/cmd/server/autopilot_listeners_test.go \
  server/cmd/server/autopilot_schedule_job_test.go
git commit -m "feat: recover autopilots when runtimes return online"
```

### Task 4: Regression suite, changelog, and local live verification

**Files:**
- Modify: `server/cmd/server/autopilot_listeners_test.go`
- Modify: `server/cmd/server/autopilot_schedule_job_test.go`
- Modify: `docs/bpa/CHANGELOG.md`

**Interfaces:**
- Confirms retry exhaustion, unchanged `create_issue`, and ordinary failed tasks remain outside recovery.

- [ ] **Step 1: Add negative-path regression tests**

```go
func TestRuntimeRetryExhaustionFailsOnceWithoutTask(t *testing.T) {}
func TestCreateIssueOfflineDispatchDoesNotEnterRuntimeRetry(t *testing.T) {}
func TestFailedAutopilotTaskDoesNotEnterRuntimeRetry(t *testing.T) {}
func TestOfflineRunOnlyWithoutOptInRemainsSkipped(t *testing.T) {}
```

- [ ] **Step 2: Run the full backend suite**

Run: `cd server && go test ./...`

Expected: PASS.

- [ ] **Step 3: Record the user-visible change**

Add one concise entry to `docs/bpa/CHANGELOG.md`: retry is opt-in per Autopilot, recovers one same-day missed local-runtime run, and does not alter board/UI behaviour.

- [ ] **Step 4: Run final formatting and full verification**

Run: `gofmt -w $(git diff --name-only -- '*.go') && cd server && go test ./... && git diff --check`

Expected: all commands exit 0.

- [ ] **Step 5: Execute a local live scenario**

1. Start the local fork backend and local runtime with the existing BPA profile.
2. Create a `run_only` scheduled Autopilot with `retry_on_runtime_unavailable: true`.
3. Stop/deregister only its local runtime before its scheduled occurrence.
4. Verify exactly one deferred `autopilot_run`, no task, and no new board item.
5. Restore the runtime and wait for its online event.
6. Verify one task/run for the original `planned_at`, then trigger recovery again and confirm no duplicate task.

- [ ] **Step 6: Commit**

```bash
git add server/cmd/server/autopilot_listeners_test.go \
  server/cmd/server/autopilot_schedule_job_test.go docs/bpa/CHANGELOG.md
git commit -m "test: cover autopilot runtime retry recovery"
```
