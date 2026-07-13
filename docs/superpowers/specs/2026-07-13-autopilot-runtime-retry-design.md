# Autopilot Runtime Retry Design

## Goal

Allow an explicitly configured Autopilot plan to recover one scheduled
`run_only` execution when its local runtime was offline, without changing the
board, issue statuses, or the behaviour of other Autopilots.

## Scope

- The policy is opt-in per Autopilot plan. Existing plans keep their current
  terminal-skip behaviour until enabled deliberately.
- It applies only to `run_only` schedule occurrences skipped because the
  assigned local runtime was unavailable.
- A recovered occurrence may run once, and only when that same Autopilot has
  not already completed a successful run on that calendar day in the trigger's
  configured timezone.
- The limit is per Autopilot, not per workspace, runtime, agent, or Multica
  installation. Plans do not block one another.
- `create_issue` is unchanged: it already creates a durable queued task while
  the runtime is offline.

## Non-goals

- No new board column, issue status, approval UI, or comment is added.
- No retry occurs for invalid configuration, missing/archived agents,
  permissions, agent errors, or failed task execution.
- No historical runs are backfilled on rollout.
- This does not retry an arbitrary failed agent task; it is a recovery of a
  known offline-runtime skip.

## Data model

Add a plan-level boolean `retry_on_runtime_unavailable`, defaulting to false.
Add retry bookkeeping to `autopilot_run`:

- `runtime_retry_attempt` — number of deferred dispatch attempts;
- `runtime_retry_after` — earliest retry time;
- `runtime_retry_reason` — the original offline-runtime admission reason.

The existing run remains the durable identity for `(trigger_id, planned_at)`.
It is not recreated and no second issue/task is created for the occurrence.

## Dispatch and recovery

1. A scheduled `run_only` occurrence reaches admission control.
2. If its plan has the opt-in policy and the only blocking reason is a local
   runtime being unavailable, create/update one deferred run rather than a
   terminal `skipped` run.
3. Keep the original `planned_at`; it identifies the missed occurrence and
   prevents duplicate dispatch.
4. When the runtime next reports online, recover due deferred runs assigned to
   that runtime. A periodic scheduler scan is the durable fallback if the
   online event was missed or the server restarted.
5. Before retrying, check whether the plan already has a successful run on the
   calendar day of the original occurrence in the trigger timezone. If it
   does, finish the deferred occurrence without dispatching it.
6. Otherwise retry the same run through the regular dispatch path. On success,
   normal run/task lifecycle updates its final status.

## Retry policy

Use bounded backoff: 1 minute, 5 minutes, 15 minutes, 1 hour, then hourly.
Stop after six attempts or 12 hours from the original planned time, whichever
comes first. The terminal result states plainly that the local runtime was not
available in time. These named limits are constants with comments explaining
their operator-facing purpose.

The recovery query must be idempotent and lock/claim each due run so a runtime
online event and the scheduler fallback cannot dispatch it twice.

## API and compatibility

Expose the plan opt-in field through the existing Autopilot create/update/read
contracts. It defaults false in the database and is absent from no existing
client payload requirement. No UI changes are made; clients that support the
field may opt in, while existing clients remain unchanged.

## Testing

Regression coverage must prove:

1. An offline `run_only` plan without opt-in remains terminal `skipped`.
2. An opted-in offline plan creates exactly one deferred run and no task.
3. Reconnection recovers the same occurrence exactly once.
4. Scheduler fallback recovers a due occurrence when no online event arrives.
5. A successful run on the same plan/day suppresses the late catch-up.
6. Different plans do not suppress each other.
7. `create_issue`, permanent admission failures, and ordinary failed tasks do
   not enter this retry path.
8. Retry-budget expiry creates one understandable terminal outcome with no
   duplicate task or issue.

## Verification

Run focused service/handler tests, full `go test ./...`, and a local live
scenario: schedule an opted-in `run_only` plan while its runtime is offline,
bring the runtime online, and verify one execution of the original occurrence
and no duplicate execution. Production is out of scope.
