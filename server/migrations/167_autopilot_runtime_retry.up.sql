-- Persist an autopilot's opt-in runtime-unavailable retry policy and the
-- deferred scheduled occurrence state. Deferred runs remain pending until the
-- retry worker can safely dispatch them.
ALTER TABLE autopilot
    ADD COLUMN retry_on_runtime_unavailable BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE autopilot_run
    ADD COLUMN runtime_retry_attempt INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN runtime_retry_after TIMESTAMPTZ,
    ADD COLUMN runtime_retry_reason TEXT;

-- Migration 079 intentionally removed pending while admission failures were
-- terminal skips. Runtime retries reintroduce pending as a non-terminal state.
ALTER TABLE autopilot_run DROP CONSTRAINT IF EXISTS autopilot_run_status_check;
ALTER TABLE autopilot_run ADD CONSTRAINT autopilot_run_status_check
    CHECK (status IN ('pending', 'issue_created', 'running', 'completed', 'failed', 'skipped'));

CREATE INDEX idx_autopilot_run_runtime_retry_due
    ON autopilot_run (runtime_retry_after)
    WHERE runtime_retry_after IS NOT NULL
      AND status = 'pending';
