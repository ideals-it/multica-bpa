# BPA continuation and local artifacts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure a single-ticket specialist completion reliably returns control to the assigned Lead and let Desktop open only task-owned local artifacts from comments.

**Architecture:** A completion hook inspects the completed task and queues the root issue assignee only for a genuine specialist-to-owner handoff. Local artifact links carry a source task ID and a relative path; the server resolves and authorizes the task root, while Electron verifies final real-path containment before calling shell APIs.

**Tech Stack:** Go/Chi/sqlc/PostgreSQL; Electron main/preload IPC; React Markdown; Vitest and Go tests.

## Global Constraints

- Do not change board columns, predefined statuses, or layout.
- Do not write a system comment for an automatic Lead handoff.
- Do not permit `file://` or arbitrary renderer-controlled filesystem paths.
- Keep existing HTTP/HTTPS, mention and slash links unchanged.
- Run Go tests after Go changes and `pnpm typecheck` after TypeScript changes.

---

### Task 1: Root specialist completion handoff

**Files:**
- Modify: `server/internal/handler/daemon.go`
- Modify: `server/internal/handler/bpa_workflow.go`
- Create: `server/internal/handler/root_specialist_completion_handoff_test.go`

**Interfaces:**
- Produces `queueRootAssigneeAfterSpecialistCompletion(ctx, task)`.
- Consumes `TaskService.EnqueueTaskForIssue` and its existing pending-task deduplication.

- [ ] Write a handler test that completes a specialist task on an active root issue owned by a different agent and asserts exactly one queued owner task.
- [ ] Add cases proving no queue for a root-owner completion, a child issue, a terminal root, or a still-active specialist sibling.
- [ ] Run the focused Go test and confirm it fails before the hook exists.
- [ ] Call the hook after successful daemon task completion; load the issue, apply all eligibility guards, and enqueue through `EnqueueTaskForIssue` without creating a comment.
- [ ] Run the focused test and `go test ./...` with the test database URL.

### Task 2: Authorized local-artifact resolution API

**Files:**
- Modify: `server/internal/handler/daemon.go` or the existing task handler file containing task routes
- Modify: `server/internal/handler/routes.go` or the existing task-route registration file
- Create: `server/internal/handler/task_local_artifact_test.go`

**Interfaces:**
- Produces authenticated task artifact resolution returning `{ work_dir, relative_path }` only for a workspace-owned task.
- Accepts task UUID and an `artifact_path` that is relative to the task `work_dir`.

- [ ] Write tests for same-workspace resolution and rejection of absolute paths, `..` traversal, task without `work_dir`, and a cross-workspace task.
- [ ] Run the focused test and confirm the route is absent or validation fails.
- [ ] Add the authenticated handler using existing task/workspace loaders; normalize with `filepath.Clean`, reject unsafe input, and return no filesystem existence detail.
- [ ] Register the route and run the focused test plus `go test ./...`.

### Task 3: Desktop local-artifact IPC and Markdown behavior

**Files:**
- Create: `apps/desktop/src/main/local-artifact.ts`
- Modify: `apps/desktop/src/main/index.ts`
- Modify: `apps/desktop/src/preload/index.ts`
- Modify: `apps/desktop/src/preload/index.d.ts`
- Modify: `packages/views/editor/readonly-content.tsx`
- Create: `apps/desktop/src/main/local-artifact.test.ts`

**Interfaces:**
- Produces `desktopAPI.openLocalArtifact(taskId, action, relativePath)`.
- Accepts only parsed `local-artifact://task/<uuid>/<open|reveal>/<relative-path>` links.

- [ ] Write filesystem tests using a temporary `work_dir` to prove normal files resolve and `..` or an escaping symlink are rejected.
- [ ] Run the test and confirm the resolver is missing.
- [ ] Implement main-process resolution: request the server with the active local profile token, canonicalize root and target, enforce containment, and call `shell.openPath` or `shell.showItemInFolder` only after validation.
- [ ] Expose a typed preload method and register its IPC handler in main startup.
- [ ] Extend the Markdown schema only for `local-artifact`; render the link as a Desktop action and as inert text outside Desktop. Do not pass it to `openExternal`.
- [ ] Run the focused desktop tests, `pnpm typecheck`, and relevant views tests.

### Task 4: Agent-facing contract and regression verification

**Files:**
- Modify: relevant built-in or BPA agent skill instructions that describe artifact reporting
- Modify: `docs/bpa/CHANGELOG.md`

- [ ] Document the exact Markdown form agents use: full absolute path as label, task-bound `local-artifact` target, and a plain path fallback when no source task exists.
- [ ] Add concise changelog entries for the completion handoff and Desktop artifact behavior.
- [ ] Run formatter, full Go tests, TypeScript typecheck, and focused local runtime smoke checks.
- [ ] Commit only the implementation, tests, and documentation; leave existing `.gitignore` and `.codebase-memory/` untouched.
