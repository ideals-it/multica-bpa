# BPA production workflow source map

- `server/internal/bpa/workflow.go` parses BPA metadata, fingerprints the
  current plan, and fails closed when approval is absent or stale.
- `server/internal/handler/bpa_workflow.go` starts the production template,
  records an approval request when it begins in `In Review`, preserves an
  unchanged approved scope, and permits only a human member to decide it.
- `server/internal/handler/issue.go` initializes the native Production
  contract when an agent moves an otherwise untemplated agent-owned root to
  `In Review`, preventing a prompt-only approval state, and rejects a manual
  move out of pending `In Review` before approval is recorded.
- `server/internal/handler/comment.go` appends one Lead handoff mention to a
  task-scoped specialist result on the root only when it names no next owner.
- `server/internal/service/task.go` checks the policy before direct assignment,
  mentions, and reruns enqueue an agent task.
- `server/internal/handler/issue_child_done.go` wakes the root assignee only
  after every child in a stage finishes.
- `server/internal/handler/bpa_workflow.go` (`validateBPACompletion`) requires
  the assigned Lead's final plain-language summary before the main task can
  reach `Done`.

The skill deliberately does not use a scheduler, event protocol, or comments
as a dispatch mechanism. Native child stages and the server-side approval gate
are the source of truth.
