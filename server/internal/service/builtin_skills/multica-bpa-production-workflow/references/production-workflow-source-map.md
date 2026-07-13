# BPA production workflow source map

- `server/internal/bpa/workflow.go` parses BPA metadata, fingerprints the
  current plan, and fails closed when approval is absent or stale.
- `server/internal/handler/bpa_workflow.go` starts the production template,
  records an approval request, and permits only a human member to decide it.
- `server/internal/service/task.go` checks the policy before direct assignment,
  mentions, and reruns enqueue an agent task.
- `server/internal/handler/issue_child_done.go` wakes the root assignee only
  after every child in a stage finishes.

The skill deliberately does not use a scheduler, event protocol, or comments
as a dispatch mechanism. Native child stages and the server-side approval gate
are the source of truth.
