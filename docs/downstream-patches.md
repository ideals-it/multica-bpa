# BPA downstream patches

This fork keeps the upstream Multica UI, board and standard issue statuses unchanged. The patches below exist only where workspace configuration cannot express the BPA task-board behavior.

| Patch | Relevant implementation | Why configuration is insufficient | Remove when upstream provides |
| --- | --- | --- | --- |
| Ticket-scoped production approval | `server/internal/bpa/workflow.go`, `server/internal/handler/issue.go`, `server/internal/handler/comment.go` | The server must bind a human approval to the unchanged ticket scope and enqueue exactly one continuation. | Native approval that supports scope binding, natural-language member approval and reactions. |
| Single-agent completion | `server/internal/daemon/execenv/runtime_config_sections.go` | The generated assignment brief must finish ordinary work in `Done` and reserve `In Review` for a real approval wait. | Upstream task brief has the same status semantics. |
| Autopilot Backlog creation | `server/internal/service/autopilot.go` | A create-issue autopilot must create an assigned Backlog ticket without silently starting the agent. | Upstream create-issue mode supports Backlog without enqueueing. |
| Runtime-admission retry | `server/internal/service/autopilot.go`, `server/internal/scheduler/jobs_autopilot.go`, `server/migrations/167_autopilot_runtime_retry.up.sql` | Retry state and deduplication must persist while a scheduled runtime is offline. | Upstream supports bounded retry for scheduled `run_only` autopilots. |
| Local artifact links | `server/internal/handler/daemon.go`, `apps/desktop/src/main/local-artifact.ts`, `packages/views/editor/readonly-content.tsx` | The Desktop process must authorize and contain a local task path before opening or revealing it. | Upstream provides an equivalent task-scoped local-artifact protocol. |

Ordinary failed-task retry, Markdown rendering, clickable mentions, assignment entry and blocker reporting remain upstream-owned. BPA-specific agent behavior is configured live on the retained agents rather than implemented as new board or UI behavior.
