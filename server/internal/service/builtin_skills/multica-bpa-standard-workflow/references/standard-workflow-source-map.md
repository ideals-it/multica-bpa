# BPA standard workflow source map

Evidence for the single-ticket Lead → specialist → Quality → Lead template.
Recheck these references after an upstream merge before changing the skill.

| Contract | Source |
| --- | --- |
| An explicit `mention://agent/...` in an issue comment resolves to a runnable agent task on that same issue | `server/internal/handler/comment.go` (`computeCommentAgentTriggers`, `enqueueSingleCommentTrigger`) |
| Different agents may have tasks on one issue while one agent remains serialized with itself | `server/pkg/db/queries/agent.sql` (`ClaimAgentTask`) |
| Direct assignment, mention, and rerun share BPA production gating | `server/internal/service/task.go` (`CanEnqueueIssue`) |
| Queued work moves an inactive native ticket to `In Progress` while preserving review, done, and blocked states | `server/internal/service/task.go` (`markIssueInProgressAfterQueue`) |
| Native issue updates preserve the existing board statuses | `server/internal/handler/issue.go` (`UpdateIssue`) |

The template deliberately uses native issue comments, task runs, and statuses.
It adds no child stage, scheduler, event protocol, or custom board status for a
single deliverable. Child issues remain a native option for independently
deliverable work.
