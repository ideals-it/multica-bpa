# BPA active-handoff status guard

## Goal

Keep a BPA main ticket truthful while work is delegated on that same ticket.
An agent must not mark the ticket `Blocked` merely because a delegated run is
temporarily `waiting_local_directory`.

## Decision

At the existing server-side issue update boundary, reject an agent-originated
transition to `Blocked` when another agent task for the same issue is active:
`queued`, `dispatched`, `waiting_local_directory`, or `running`.

The caller's own active task does not count. This keeps a genuine agent block
available when it has not delegated active work and needs an external answer.

## Scope

- No UI, board column, or status definition changes.
- Applies only to agent task-token updates, not human updates.
- Uses the existing task queue as the source of truth.
- Returns a clear conflict response rather than silently rewriting status.
- Adds handler regression coverage for both rejected and allowed transitions.

## Current BPA-182 repair

After the guard is live, reconcile BPA-182 from its stale `Blocked` state to
`In Progress` through the normal local API because its Builder task is active.
