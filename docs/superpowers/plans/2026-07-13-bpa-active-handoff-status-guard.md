# BPA Active-Handoff Status Guard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent an agent from marking a BPA main ticket `Blocked` while another agent task on that ticket is still active.

**Architecture:** `Handler.UpdateIssue` already resolves the authenticated actor before persisting the status. A small handler helper will inspect existing active tasks via `ListActiveTasksByIssue`, ignore the caller itself, and reject only agent-originated `Blocked` transitions while another task is queued, dispatched, waiting for the local directory, or running.

**Tech Stack:** Go, existing sqlc task query, HTTP handler tests, local Multica API.

## Global Constraints

- Do not change the UI, board columns, status definitions, or task lifecycle.
- Do not block a member from setting `Blocked`.
- Keep a real agent `Blocked` transition valid when no different active task exists.
- Treat `waiting_local_directory` as active work, not a failed handoff.
- Repair BPA-182 only through the normal local API after the server test passes.

---

### Task 1: Guard an agent `Blocked` transition

**Files:**

- Modify: `server/internal/handler/issue.go`
- Modify: `server/internal/handler/issue_test.go` or the existing focused BPA handler test file

**Interfaces:**

- Consumes: `Queries.ListActiveTasksByIssue(ctx, issueID)` and actor type/id
- Produces: `validateAgentBlockedTransition(ctx, issue, actorType, actorID) error`

- [ ] **Step 1: Write the failing handler regression test**

Create a BPA root assigned to Lead and insert a separate active Builder task
with status `waiting_local_directory`. Send an agent-authenticated `PUT` with
`{"status":"blocked"}` as Lead. Assert HTTP `409`, unchanged `in_progress`,
and an error containing `delegated task is still active`.

Add two control cases: the same Lead task alone may set `Blocked`, and a
member may set `Blocked` while Builder is active.

- [ ] **Step 2: Run it red**

Run:

```bash
cd server && DATABASE_URL="$DATABASE_URL" go test ./internal/handler -run TestAgentBlockedTransition -count=1
```

Expected: the active-handoff case returns `200` before the guard exists.

- [ ] **Step 3: Add the minimal helper and call it before UpdateIssue**

```go
func (h *Handler) validateAgentBlockedTransition(
    ctx context.Context,
    issue db.Issue,
    actorType, actorID string,
) error {
    if actorType != "agent" {
        return nil
    }
    tasks, err := h.Queries.ListActiveTasksByIssue(ctx, issue.ID)
    if err != nil {
        return fmt.Errorf("load active delegated tasks: %w", err)
    }
    for _, task := range tasks {
        if uuidToString(task.AgentID) != actorID {
            return errors.New("cannot mark ticket blocked while a delegated task is still active")
        }
    }
    return nil
}
```

Invoke it only when `req.Status` is `blocked` and differs from the current
status, after actor resolution and before `Queries.UpdateIssue`.

- [ ] **Step 4: Run regression tests green**

Run:

```bash
cd server && DATABASE_URL="$DATABASE_URL" go test ./internal/handler -run TestAgentBlockedTransition -count=1
```

Expected: all three cases pass.

- [ ] **Step 5: Format and commit**

Run:

```bash
gofmt -w server/internal/handler/issue.go server/internal/handler/<focused-test>.go
git diff --check
git add server/internal/handler/issue.go server/internal/handler/<focused-test>.go
git commit -m "fix(bpa): preserve active handoff status"
```

### Task 2: Verify local behavior and repair BPA-182

**Files:**

- Modify: `docs/bpa/CHANGELOG.md`

- [ ] **Step 1: Rebuild the local backend and check health**

```bash
make selfhost-build
curl -fsS http://127.0.0.1:8080/health
```

Expected: `{"status":"ok"}`.

- [ ] **Step 2: Reconcile BPA-182 through the local API**

Use the authenticated local API to set BPA-182 to `in_progress` only while its
Builder task is active. Do not create tickets, change production, or cancel
the Builder task.

- [ ] **Step 3: Verify live state and document it**

Verify BPA-182 is `in_progress`, Builder remains active or has completed, and
the root no longer carries the stale local-directory `blocked_reason`. Append
the behavior to `docs/bpa/CHANGELOG.md`, then commit:

```bash
git add docs/bpa/CHANGELOG.md
git commit -m "docs(bpa): record active handoff guard"
```
