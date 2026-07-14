# BPA Natural-language Approval Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace BPA's phrase-only production approval with owner/admin comment continuations interpreted by the assigned agent.

**Architecture:** Keep the existing board and statuses. The handler treats an owner/admin comment on a pending BPA production review as a safe continuation trigger, never as a server-side semantic approval. The resumed assigned agent reads the full ticket and comment, fails closed on ambiguity, and records its interpretation before production work.

**Tech Stack:** Go, PostgreSQL/sqlc, existing task queue, runtime instruction generator, Go tests.

## Global Constraints

- Do not change UI, board columns, issue statuses, or add an approval dialog.
- Preserve **Archivist** as a read-only, non-blocking background lifecycle.
- Preserve human mention links for real clarifications; never create acknowledgement loops.
- Production work must fail closed when the human response is ambiguous.

---

### Task 1: Remove phrase-only BPA approval semantics

**Files:**
- Modify: `server/internal/handler/bpa_workflow.go`
- Modify: `server/internal/handler/bpa_workflow_test.go`
- Modify: `server/internal/handler/comment.go`
- Modify: `server/internal/handler/reaction.go`

**Interfaces:**
- Replace `isBPAApprovalComment` and `approveBPAReviewComment` with `shouldResumeBPAReviewConversation(issue db.Issue, actorType string, member db.Member) bool`.
- Keep reactions as ordinary reactions; they do not independently authorize production work.

- [x] **Step 1: Write failing handler tests**

Add tests proving an owner/admin comment such as `"Так, але спочатку перевір резервну копію"` queues exactly one continuation for the assigned root agent, while an ordinary member and an agent comment do not. Add a test proving `"поясни ризик"` queues a conversation continuation but does not change `bpa.approval_status`.

- [x] **Step 2: Run the focused tests and verify failure**

Run: `cd server && go test ./internal/handler -run 'TestBPAReview(Comment|Conversation)' -count=1`

Expected: FAIL because the current implementation checks `isBPAApprovalComment` against a fixed phrase list.

- [x] **Step 3: Implement minimal routing**

Delete the phrase whitelist and do not mutate approval metadata in `CreateComment`. After the comment is persisted, resolve the member role in the ticket workspace. For a pending production root review written by owner/admin, enqueue the root assignee once using the comment ID as trigger context. Keep `queueBPAArchivist` after the comment path so Archivist remains independent.

- [x] **Step 4: Keep reactions non-semantic**

Remove `approveBPAReviewReaction` from `AddReaction`; retain normal reaction persistence and broadcast. A reaction may remain a visible signal to the agent, but it is not a server-side production authorization.

- [x] **Step 5: Verify and commit**

Run: `cd server && go test ./internal/handler -run 'TestBPAReview(Comment|Conversation)|TestAddReaction' -count=1`

Commit: `git commit -am "feat(bpa): resume production review conversations"`

### Task 2: Make agent interpretation explicit and safe

**Files:**
- Modify: `server/internal/daemon/execenv/runtime_config_sections.go`
- Modify: `server/internal/daemon/execenv/runtime_config_test.go`
- Modify: `server/internal/handler/bpa_workflow.go`
- Modify: `server/internal/handler/bpa_workflow_test.go`

- [x] **Step 1: Write failing instruction and guard tests**

Assert that an approval continuation instructs the assignee to read the complete human comment, state its interpretation in a concise ticket comment, perform production work only for a clear approval, and request clarification with a real member mention otherwise. Assert a changed title/description resets a pending review and prevents use of an older scope fingerprint.

- [x] **Step 2: Implement the instruction contract**

Add the following behavioural text to the assignment brief: `Treat the human reply as natural language, not a keyword. If it clearly approves the described scope, state that interpretation briefly and perform only that scope. If it is conditional, a question, a refusal, or ambiguous, do not perform a production action; reply with the remaining question and mention the human owner.`

- [x] **Step 3: Preserve scope protection**

Use the existing BPA scope fingerprint when a production review begins. On a material title/description update, clear pending approval state; after production execution has started, reject a material scope update and require a follow-up ticket.

- [x] **Step 4: Verify and commit**

Run: `cd server && go test ./internal/handler ./internal/daemon/execenv -count=1`

Commit: `git commit -am "fix(bpa): make production replies context aware"`

### Task 3: Integrate the simple-task patch without losing Archivist

**Files:**
- Modify only the conflicted server files identified during rebase: `server/internal/handler/comment.go`, `server/internal/handler/issue.go`, `server/internal/handler/reaction.go`, `server/internal/handler/bpa_workflow.go`, `server/internal/service/autopilot.go`, and their tests.
- Preserve: `server/internal/handler/bpa_archivist.go` and its non-blocking queue hooks.

- [x] **Step 1: Rebase selected simple-task commits onto `bpa/main`**

Use a fresh integration branch from `bpa/main`. Resolve conflicts by preserving Archivist hooks and replacing old Lead/Builder/Quality routing only where the simple-task contract explicitly requires it.

- [x] **Step 2: Run regression checks**

Run: `make migrate-up && cd server && go test ./internal/handler ./internal/service ./internal/daemon/execenv ./cmd/server -count=1`

- [ ] **Step 3: Final review and draft PR**

Run one consolidated independent review for the integrated diff, apply confirmed findings, and rerun the complete verification suite. Push only the rebased integration branch and create a draft PR into `bpa/main`; do not deploy or restart the real local Multica before merge.
