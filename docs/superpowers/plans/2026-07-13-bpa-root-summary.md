# BPA Root Summary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Require a concise Team Lead final summary in every completed BPA root ticket.

**Architecture:** A pure Go helper validates five visible Ukrainian headings. The existing shared `Handler.validateBPACompletion` completion gate loads root comments and refuses `Done` unless the assigned root Team Lead authored a qualifying final summary. BPA templates and live Lead instructions make the same summary normal behavior; the server never invents or copies prose.

**Tech Stack:** Go, sqlc, existing handler tests, built-in BPA skills, local Multica agent configuration.

## Global Constraints

- Keep the existing UI, board columns, predefined statuses, and ticket topology unchanged.
- Apply only to BPA-enabled root issues moving to `Done`.
- Require a final summary only from the assigned root Team Lead.
- Preserve non-BPA completion behavior and every non-`Done` transition.
- Do not use direct SQL writes for the BPA-176 repair; use the normal local API.

---

### Task 1: Validate a qualifying Team Lead root summary

**Files:**

- Create: `server/internal/handler/bpa_root_summary.go`
- Create: `server/internal/handler/bpa_root_summary_test.go`

**Interfaces:**

- Consumes: `db.Issue` and ordered `[]db.Comment`.
- Produces: `hasBPAFinalRootSummary(root db.Issue, comments []db.Comment) bool`.

- [ ] **Step 1: Write the failing unit table test**

```go
valid := "**Що було не так:** причина\n\n" +
    "**Що змінили:** зміна\n\n" +
    "**Що перевірили:** тест\n\n" +
    "**Результат:** готово\n\n" +
    "**Ризик / наступне:** немає відомого"

// Test the assigned agent with valid, missing-heading, other-agent, and
// member-author cases. Only the first case must return true.
```

- [ ] **Step 2: Run it red**

Run: `cd server && go test ./internal/handler -run TestHasBPAFinalRootSummary -count=1`

Expected: FAIL because the helper does not exist.

- [ ] **Step 3: Implement the pure helper**

```go
var bpaFinalSummaryHeadings = []string{
    "**Що було не так:**", "**Що змінили:**", "**Що перевірили:**",
    "**Результат:**", "**Ризик / наступне:**",
}

func hasBPAFinalRootSummary(root db.Issue, comments []db.Comment) bool {
    if root.AssigneeType.String != "agent" || !root.AssigneeID.Valid {
        return false
    }
    for _, comment := range comments {
        if comment.AuthorType != "agent" || comment.AuthorID != root.AssigneeID {
            continue
        }
        if allBPAFinalSummaryHeadingsPresent(comment.Content) {
            return true
        }
    }
    return false
}
```

`allBPAFinalSummaryHeadingsPresent` uses `strings.Contains` for every exact
heading; this is a visible agent contract, not Markdown parsing.

- [ ] **Step 4: Run the unit test green and commit**

Run: `cd server && go test ./internal/handler -run TestHasBPAFinalRootSummary -count=1`

Expected: PASS.

```bash
git add server/internal/handler/bpa_root_summary.go server/internal/handler/bpa_root_summary_test.go
git commit -m "feat(bpa): validate lead root summaries"
```

### Task 2: Reuse the shared BPA completion gate

**Files:**

- Modify: `server/internal/handler/bpa_workflow.go:160-176`
- Modify: `server/internal/handler/bpa_workflow_test.go`
- Modify: `server/pkg/db/queries/comment.sql`
- Regenerate: `server/pkg/db/generated/comment.sql.go`

**Interfaces:**

- Consumes: `Handler.validateBPACompletion`, called by `UpdateIssue`,
  `BatchUpdateIssues`, and GitHub merge completion.
- Produces: `validateBPAFinalRootSummary(ctx, issue) error` after current
  commit-evidence and open-child checks.

- [ ] **Step 1: Add failing HTTP regression tests**

Add `TestBPARootCannotCloseWithoutLeadFinalSummary` and
`TestBPARootCanCloseWithLeadFinalSummary` to
`server/internal/handler/bpa_workflow_test.go`. Reuse
`createMetadataTestIssue` and `setBPAWorkflowValues`. The first sends
`PUT {"status":"done"}` and expects `409` plus `final root summary`; the
second inserts a qualifying Team Lead comment through `Queries.CreateComment`
and expects `200`. Add an equivalent `BatchUpdateIssues` test to prevent a
bypass.

- [ ] **Step 2: Run the new tests red**

Run: `cd server && go test ./internal/handler -run 'TestBPA.*FinalSummary' -count=1`

Expected: the no-summary transition incorrectly succeeds.

- [ ] **Step 3: Add an ordered sqlc comment query**

Append this to `server/pkg/db/queries/comment.sql`:

```sql
-- name: ListAllCommentsForIssue :many
SELECT *
FROM comment
WHERE issue_id = $1
ORDER BY created_at ASC, id ASC;
```

Run: `make sqlc`

Expected: generated `Queries.ListAllCommentsForIssue(ctx, issueID)`.

- [ ] **Step 4: Add the root-only completion guard**

```go
func (h *Handler) validateBPAFinalRootSummary(ctx context.Context, issue db.Issue) error {
    if issue.ParentIssueID.Valid {
        return nil
    }
    state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
    if err != nil || !state.Enabled() {
        return err
    }
    comments, err := h.Queries.ListAllCommentsForIssue(ctx, issue.ID)
    if err != nil {
        return fmt.Errorf("load BPA root comments: %w", err)
    }
    if !hasBPAFinalRootSummary(issue, comments) {
        return fmt.Errorf("BPA main task needs a final root summary from Team Lead before completion")
    }
    return nil
}
```

Call it as the final statement of `validateBPACompletion` after the existing
open-child guard.

- [ ] **Step 5: Run focused tests and commit**

Run: `cd server && go test ./internal/handler -run 'TestBPA.*(Close|Completion|FinalSummary)' -count=1`

Expected: PASS, including existing commit-evidence and open-child checks.

```bash
git add server/internal/handler/bpa_workflow.go server/internal/handler/bpa_workflow_test.go server/pkg/db/queries/comment.sql server/pkg/db/generated/comment.sql.go
git commit -m "fix(bpa): require lead summary before root completion"
```

### Task 3: Teach Lead the root-summary contract and repair BPA-176

**Files:**

- Modify: `server/internal/service/builtin_skills/multica-bpa-standard-workflow/SKILL.md`
- Modify: `server/internal/service/builtin_skills/multica-bpa-production-workflow/SKILL.md`
- Modify: `server/internal/service/builtin_skills/multica-bpa-investigation-workflow/SKILL.md`
- Modify: `server/internal/service/builtin_skills_test.go`
- Modify: live local `AT Team Lead` instructions through Multica configuration
- Modify: `docs/bpa/CHANGELOG.md`

**Interfaces:**

- Consumes: the five headings accepted by Task 1.
- Produces: root comments that pass the completion guard and remain plain
  Ukrainian summaries, not technical transcripts.

- [ ] **Step 1: Extend built-in skill assertions red**

Require every BPA workflow skill to contain each exact heading:

```go
for _, heading := range []string{
    "**Що було не так:**", "**Що змінили:**", "**Що перевірили:**",
    "**Результат:**", "**Ризик / наступне:**",
} {
    if !strings.Contains(body, heading) {
        t.Errorf("BPA workflow skill missing final root heading %q", heading)
    }
}
```

Run: `cd server && go test ./internal/service -run 'TestBPA.*Workflow' -count=1`

Expected: FAIL until templates include the contract.

- [ ] **Step 2: Update all templates and the live Lead prompt**

Add this compact section to every BPA template and the live **AT Team Lead**
instructions:

```text
After each material specialist or Quality handoff, post one short plain-language
progress update on the root before mentioning the next owner. Before Done,
post one final root summary with **Що було не так:**, **Що змінили:**,
**Що перевірили:**, **Результат:**, **Ризик / наступне:**. Do not paste logs,
commands, child comments, or a task transcript.
```

- [ ] **Step 3: Repair the current case through normal APIs**

Use the authenticated local Multica API to add a Lead-authored BPA-176 root
summary: Google API temporary failures exhausted retry and became 500; `fd6a512`
returns 503 with `Retry-After`; 14/14 tests passed; Cloud Run
`signature-ai-service-00017-hxw` is Ready at 100% traffic. Reconcile BPA-177,
BPA-179, and BPA-180 to `Done`, unused BPA-178 to `Cancelled`, then close
BPA-176 through the normal status API. Do not create a ticket or invoke a
deployment while doing this bookkeeping.

- [ ] **Step 4: Verify skills, live ticket, changelog, and commit**

Run:

```bash
cd server && go test ./internal/service -run 'TestBPA.*Workflow' -count=1
go test ./internal/handler -run 'TestBPA.*' -count=1
```

Verify BPA-176 root has the five headings and `Done`; verify children are in
their factual terminal statuses. Append the behavior to `docs/bpa/CHANGELOG.md`.

```bash
git add server/internal/service/builtin_skills server/internal/service/builtin_skills_test.go docs/bpa/CHANGELOG.md
git commit -m "docs(bpa): require plain-language root summaries"
```

### Task 4: Full verification and local rollout

**Files:**

- Verify only: all files from Tasks 1-3.

**Interfaces:**

- Consumes: the completed server gate and Lead contract.
- Produces: local self-hosted Multica enforcing the contract.

- [ ] **Step 1: Format and run the complete backend suite**

```bash
gofmt -w server/internal/handler/bpa_root_summary.go server/internal/handler/bpa_root_summary_test.go server/internal/handler/bpa_workflow.go server/internal/handler/bpa_workflow_test.go
git diff --check
cd server && go test ./...
```

Expected: every command passes.

- [ ] **Step 2: Rebuild only local Multica and prove health**

```bash
make selfhost-build
curl -fsS http://127.0.0.1:8080/health
```

Expected: response contains `"status":"ok"`. Do not deploy the Multica fork
remotely.

- [ ] **Step 3: Commit only generated residue if any**

```bash
git status --short
git add server/pkg/db/generated/comment.sql.go
git commit -m "chore(bpa): regenerate summary guard queries"
```

Run this only if sqlc left the generated file unstaged. Preserve unrelated
`.gitignore` and `.codebase-memory/` changes.
