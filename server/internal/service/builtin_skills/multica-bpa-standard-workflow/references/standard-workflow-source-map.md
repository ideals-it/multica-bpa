# BPA standard workflow source map

Evidence for the single-agent, single-ticket workflow. Recheck these references
after an upstream merge before changing the skill.

| Contract | Source |
| --- | --- |
| An explicit agent mention routes one task on the same issue | `server/internal/handler/comment.go` (`computeCommentAgentTriggers`, `enqueueCommentAgentTriggers`) |
| A normal human comment without an agent mention resumes the issue assignee when it is runnable | `server/internal/handler/comment.go` (`routeAssigneeFallback`) |
| A queued comment is merged into an unclaimed task; an active task gets one completion-time follow-up instead of being dropped or duplicated | `server/internal/handler/comment.go` (`mergeCommentIntoPendingTask`, `reconcileCommentsOnCompletion`) |
| Autopilot-created Backlog tickets stay parked; ordinary Backlog comments retain native routing | `server/internal/service/autopilot.go` (`dispatchCreateIssue`); `server/internal/handler/comment.go` (`triggerTasksForComment`) |
| Assignment briefs set `Done` for ordinary completed work and reserve `In Review` for a human production decision | `server/internal/daemon/execenv/runtime_config_sections.go` (`writeWorkflowAssignment`) |
| BPA events write non-blocking knowledge markers and do not create Archivist runtime work | `server/internal/handler/bpa_workflow.go` (`queueBPAArchivist`) |
| Desktop resolves task-local artifact links only through the daemon-scoped endpoint | `server/internal/handler/daemon.go` (`ResolveTaskLocalArtifact`) |

The workflow uses native issue comments, task runs, and statuses. Child issues
remain optional for independently useful deliverables.
