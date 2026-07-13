DROP INDEX IF EXISTS idx_autopilot_run_runtime_retry_due;

-- A deferred retry cannot be represented after restoring the pre-retry status
-- constraint, so retain its audit trail as a terminal skipped run.
UPDATE autopilot_run
SET status = 'skipped',
    completed_at = COALESCE(completed_at, now()),
    failure_reason = COALESCE(failure_reason, runtime_retry_reason, 'runtime retry migration rollback')
WHERE status = 'pending';

ALTER TABLE autopilot_run DROP CONSTRAINT IF EXISTS autopilot_run_status_check;
ALTER TABLE autopilot_run ADD CONSTRAINT autopilot_run_status_check
    CHECK (status IN ('issue_created', 'running', 'completed', 'failed', 'skipped'));

ALTER TABLE autopilot_run
    DROP COLUMN runtime_retry_reason,
    DROP COLUMN runtime_retry_after,
    DROP COLUMN runtime_retry_attempt;

ALTER TABLE autopilot
    DROP COLUMN retry_on_runtime_unavailable;
