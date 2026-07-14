# BPA production workflow source map

- `server/internal/bpa/workflow.go` parses ticket metadata and fingerprints the
  current production scope.
- `server/internal/handler/bpa_workflow.go` begins a pending review when a
  production template enters `In Review`.
- `server/internal/service/task.go` fails closed before it queues production
  execution without a valid approved scope.
- `server/internal/handler/comment.go` delivers the full text of human comments
  to the assigned agent through native comment routing; queued work is merged
  and active work receives a bounded follow-up.
- `server/internal/handler/reaction.go` persists reactions as visible signals;
  a reaction does not itself authorize production work.
- `server/internal/daemon/execenv/runtime_config_sections.go`
  (`writeWorkflowComment`) instructs the agent to interpret the full human
  reply, explain a clear interpretation, and fail closed on ambiguity.

The agent, not a server keyword list, understands the human reply. The server
continues to protect the pending scope from accidental dispatch.
