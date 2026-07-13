# BPA standard workflow source map

Evidence for the single-ticket Lead → specialist → Quality → Lead template.
Recheck these references after an upstream merge before changing the skill.

| Contract | Source |
| --- | --- |
| An explicit `mention://agent/...` in an issue comment resolves to a runnable agent task on that same issue | `server/internal/handler/comment.go` (`computeCommentAgentTriggers`, `enqueueSingleCommentTrigger`) |
| A task-scoped specialist result on an agent-owned root with no explicit next-owner mention gets one compact handoff mention to the assigned Lead, even before a BPA template starts | `server/internal/handler/comment.go` (`ensureBPAWorkerHandoffMention`) |
| BPA events write lightweight server-owned knowledge markers only; automatic archival never creates an Archivist runtime task, occupies a local directory, or wakes Lead | `server/internal/handler/bpa_workflow.go` (`queueBPAArchivist`, `queueRootAssigneeAfterSpecialistCompletion`) |
| Different agents may have tasks on one issue while one agent remains serialized with itself | `server/pkg/db/queries/agent.sql` (`ClaimAgentTask`) |
| Direct assignment, mention, and rerun share BPA production gating | `server/internal/service/task.go` (`CanEnqueueIssue`) |
| Queued work moves an inactive native ticket to `In Progress` while preserving review, done, and blocked states | `server/internal/service/task.go` (`markIssueInProgressAfterQueue`) |
| Native issue updates preserve the existing board statuses | `server/internal/handler/issue.go` (`UpdateIssue`) |
| A BPA main ticket cannot reach `Done` before its assigned Lead has posted the required plain-language final summary | `server/internal/handler/bpa_workflow.go` (`validateBPACompletion`) |
| A completed non-owner specialist task on a root issue queues the assigned Lead after no other task remains active; if the Lead was already running, its completion queues one follow-up when a specialist produced newer evidence during that run | `server/internal/handler/bpa_workflow.go` (`queueRootAssigneeAfterSpecialistCompletion`), called by `server/internal/handler/daemon.go` (`CompleteTask`) |
| `MULTICA_TASK_ID` is available for an agent to bind an artifact link to its own task | `server/internal/daemon/daemon.go` (`agentEnv["MULTICA_TASK_ID"]`) |
| Desktop artifact links are authorized per task and canonicalized before an OS open/reveal action | `server/internal/handler/daemon.go` (`ResolveTaskLocalArtifact`), `apps/desktop/src/main/local-artifact.ts` (`resolveLocalArtifactPath`) |

The template deliberately uses native issue comments, task runs, and statuses.
It adds no child stage, scheduler, event protocol, or custom board status for a
single deliverable. Child issues remain a native option for independently
deliverable work.
