# BPA standard workflow source map

Evidence for the native Lead → specialist → Quality → Lead template. Recheck
these references after an upstream merge before changing the skill.

| Contract | Source |
| --- | --- |
| `backlog` parks an agent-assigned issue; transition from backlog to an active status starts the ordinary run path | `server/internal/service/issue_trigger.go:89-115` |
| Issue creation and update consult `WillEnqueueRun` before dispatch | `server/internal/handler/issue.go:2657-2666`, `server/internal/handler/issue.go:3162-3171` |
| A terminal child completion posts a parent system comment and dispatches the parent assignee | `server/internal/handler/issue_child_done.go:62-66`, `:334`, `:536-586` |
| The parent is woken only when the lowest unfinished child stage is terminal | `server/internal/handler/issue_child_done.go:370-430` |
| Child stages are persisted by the issue-stage migration | `server/migrations/123_issue_stage.up.sql` |
| The issue CLI supports create/update with a stage | `server/cmd/multica/cmd_issue.go` (`--stage` flag) |

The template intentionally relies only on these existing contracts. It adds no
runtime message transport, workflow engine, new issue status, or scheduler.
