package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/bpa"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestStartBPAWorkflowRejectsMainIssueWithoutAgentLead(t *testing.T) {
	issueID := createMetadataTestIssue(t, "member-owned BPA root")
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/"+issueID+"/bpa/template", StartBPAWorkflowRequest{Template: "production"})
	req = withURLParam(req, "id", issueID)
	testHandler.StartBPAWorkflow(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBPAApprovalCommentRecognizesClearDeploymentDirective(t *testing.T) {
	content := "[@AT Team Lead](mention://agent/7188d30e-c3eb-4f5b-8acf-cb2377069bf7) Деплой"
	if !isBPAApprovalComment(content) {
		t.Fatalf("clear deployment directive %q must approve the pending Production scope", content)
	}
}

func TestBPAApprovalCommentRejectsDeploymentQuestionOrNegation(t *testing.T) {
	for _, content := range []string{"Що з деплоєм?", "Деплой не роби"} {
		if isBPAApprovalComment(content) {
			t.Fatalf("non-approval comment %q must not approve the pending Production scope", content)
		}
	}
}

func TestBPAApprovalEmojiAcceptsThumbsUpAndOKOnly(t *testing.T) {
	for _, emoji := range []string{"👍", "👌"} {
		if !isBPAApprovalEmoji(emoji) {
			t.Fatalf("approval emoji %q was rejected", emoji)
		}
	}
	for _, emoji := range []string{"❤️", "✅", "👎"} {
		if isBPAApprovalEmoji(emoji) {
			t.Fatalf("non-approval emoji %q was accepted", emoji)
		}
	}
}

func TestLeadApprovalReactionApprovesPendingProductionScope(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "ReactionApprovalLead", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92141, "reaction approval")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	req := newRequest(http.MethodPost, "/", nil)
	issue, err = testHandler.setBPAWorkflowValues(req, issue, map[string]any{"bpa.template": string(bpa.TemplateProduction)})
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.beginBPAHumanReview(req, issue)
	if err != nil {
		t.Fatal(err)
	}

	comment := db.Comment{IssueID: issue.ID, AuthorType: "agent", AuthorID: parseUUID(leadID)}
	updated, approved, err := testHandler.approveBPAReviewReaction(req, comment, "member", "👍")
	if err != nil || !approved {
		t.Fatalf("thumbs-up approval = approved:%t err:%v", approved, err)
	}
	state, err := bpa.ParseState(parseIssueMetadata(updated.Metadata))
	if err != nil || state.ApprovalStatus != bpa.ApprovalApproved || state.WaitingFor != bpa.WaitingForLead {
		t.Fatalf("reaction approval state = %#v, err=%v", state, err)
	}
}

func TestBPAWorkerCommentGetsLeadHandoffWhenNoOwnerMentioned(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "HandoffLead", []byte("[]"))
	workerID := createHandlerTestAgent(t, "HandoffWorker", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92139, "worker handoff mention")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{"bpa.template": "standard"})
	if err != nil {
		t.Fatal(err)
	}

	content := testHandler.ensureBPAWorkerHandoffMention(ctx, issue, parseUUID(workerID), pgtype.UUID{Valid: true}, "готово")
	want := "mention://agent/" + leadID
	if !strings.Contains(content, want) {
		t.Fatalf("worker result must hand off to Lead, got %q", content)
	}
}

func TestAgentOwnedRootWorkerCommentGetsLeadHandoffWithoutTemplate(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "UntemplatedHandoffLead", []byte("[]"))
	workerID := createHandlerTestAgent(t, "UntemplatedHandoffWorker", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92140, "untemplated worker handoff mention")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}

	content := testHandler.ensureBPAWorkerHandoffMention(ctx, issue, parseUUID(workerID), pgtype.UUID{Valid: true}, "готово")
	want := "mention://agent/" + leadID
	if !strings.Contains(content, want) {
		t.Fatalf("worker result on an agent-owned root must hand off to Lead, got %q", content)
	}
}

func TestAgentMovingRootToReviewStartsPendingProductionApproval(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "ReviewAutoProductionLead", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92135, "review auto-production approval")
	taskID := createHandlerTestTaskForAgent(t, leadID)

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "in_review"})
	req = withURLParam(req, "id", issueID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", leadID)
	req.Header.Set("X-Task-ID", taskID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("agent transition to In Review: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil {
		t.Fatal(err)
	}
	if state.Template != bpa.TemplateProduction || state.ApprovalStatus != bpa.ApprovalPending || state.WaitingFor != bpa.WaitingForHumanApproval || state.ScopeFingerprint == "" {
		t.Fatalf("review workflow state = %#v, want pending production approval", state)
	}
}

func TestStartProductionWorkflowOnInReviewStartsPendingApproval(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "ReviewStartedProductionLead", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92136, "start production workflow in review")
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/bpa/template", StartBPAWorkflowRequest{Template: bpa.TemplateProduction})
	req = withURLParam(req, "id", issueID)
	testHandler.StartBPAWorkflow(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("start production workflow: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil {
		t.Fatal(err)
	}
	if state.ApprovalStatus != bpa.ApprovalPending || state.WaitingFor != bpa.WaitingForHumanApproval || state.ScopeFingerprint == "" {
		t.Fatalf("started review workflow state = %#v, want pending approval", state)
	}
}

func TestAgentReviewTransitionKeepsApprovedUnchangedProductionScope(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "ReviewApprovedScopeLead", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92137, "keep approved production scope")
	taskID := createHandlerTestTaskForAgent(t, leadID)

	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	scope := bpa.TicketScopeFingerprint(issue.Title, "")
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template":                   string(bpa.TemplateProduction),
		"bpa.waiting_for":                string(bpa.WaitingForLead),
		"bpa.scope_fingerprint":          scope,
		"bpa.approval_status":            string(bpa.ApprovalApproved),
		"bpa.approved_scope_fingerprint": scope,
	})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "in_review"})
	req = withURLParam(req, "id", issueID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", leadID)
	req.Header.Set("X-Task-ID", taskID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("agent transition to In Review: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	issue, err = testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil {
		t.Fatal(err)
	}
	if state.ApprovalStatus != bpa.ApprovalApproved || state.ApprovedScopeFingerprint != scope || state.WaitingFor == bpa.WaitingForHumanApproval {
		t.Fatalf("approved unchanged scope must remain approved, got %#v", state)
	}
}

func TestPendingProductionReviewCannotLeaveReviewBeforeApproval(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "PendingReviewLead", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92138, "pending production review")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template":          string(bpa.TemplateProduction),
		"bpa.waiting_for":       string(bpa.WaitingForHumanApproval),
		"bpa.scope_fingerprint": bpa.TicketScopeFingerprint(issue.Title, ""),
		"bpa.approval_status":   string(bpa.ApprovalPending),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "in_progress"})
	req = withURLParam(req, "id", issueID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("leave pending review: expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBPARootResolvesThroughNestedChildren(t *testing.T) {
	ctx := context.Background()
	rootID := createMetadataTestIssue(t, "nested BPA root")
	root, err := testHandler.Queries.GetIssue(ctx, parseUUID(rootID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testHandler.setBPAWorkflowValues(newRequest("POST", "/", nil), root, map[string]any{"bpa.template": "standard"}); err != nil {
		t.Fatal(err)
	}

	createChild := func(title, parentID string) IssueResponse {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
			"title":           title,
			"parent_issue_id": parentID,
		})
		testHandler.CreateIssue(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create child: expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var child IssueResponse
		if err := json.NewDecoder(w.Body).Decode(&child); err != nil {
			t.Fatal(err)
		}
		return child
	}
	child := createChild("nested BPA child", rootID)
	grandchild := createChild("nested BPA grandchild", child.ID)
	grandchildIssue, err := testHandler.Queries.GetIssue(ctx, parseUUID(grandchild.ID))
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, enabled, err := testHandler.bpaRoot(ctx, grandchildIssue)
	if err != nil || !enabled || resolvedRoot.ID != root.ID {
		t.Fatalf("nested BPA root resolution = root=%s enabled=%t err=%v", uuidToString(resolvedRoot.ID), enabled, err)
	}
}

func TestApproveCommentApprovesCurrentTicketScope(t *testing.T) {
	issueID := createMetadataTestIssue(t, "scope approval")
	ctx := context.Background()
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	req := newRequest("POST", "/api/issues/"+issueID+"/comments", nil)
	issue, err = testHandler.setBPAWorkflowValues(req, issue, map[string]any{"bpa.template": "production"})
	if err != nil {
		t.Fatal(err)
	}
	issue.Status = "in_review"
	issue, err = testHandler.Queries.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{ID: issue.ID, WorkspaceID: issue.WorkspaceID, Status: "in_review"})
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.beginBPAHumanReview(req, issue)
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.approveBPAReviewComment(req, issue, "member", "Погоджую")
	if err != nil {
		t.Fatal(err)
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil || state.ApprovalStatus != bpa.ApprovalApproved || state.ApprovedScopeFingerprint != state.ScopeFingerprint {
		t.Fatalf("approval state = %#v, err = %v", state, err)
	}
}

func TestBPARootCannotCloseWhileChildIsOpen(t *testing.T) {
	ctx := context.Background()
	parentID := createMetadataTestIssue(t, "BPA root cannot close with open child")
	parent, err := testHandler.Queries.GetIssue(ctx, parseUUID(parentID))
	if err != nil {
		t.Fatal(err)
	}
	req := newRequest("POST", "/api/issues/"+parentID+"/bpa/template", nil)
	if _, err := testHandler.setBPAWorkflowValues(req, parent, map[string]any{"bpa.template": "standard"}); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	createChild := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":           "open BPA child " + time.Now().Format(time.RFC3339Nano),
		"status":          "in_progress",
		"parent_issue_id": parentID,
	})
	testHandler.CreateIssue(w, createChild)
	if w.Code != http.StatusCreated {
		t.Fatalf("create child: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var child IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&child); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, child.ID)
	})

	w = httptest.NewRecorder()
	update := newRequest("PUT", "/api/issues/"+parentID, map[string]any{"status": "done"})
	update = withURLParam(update, "id", parentID)
	testHandler.UpdateIssue(w, update)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBPARootCompletionRequiresLeadFinalSummary(t *testing.T) {
	ctx := context.Background()
	issueID := createMetadataTestIssue(t, "BPA root needs final summary")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}

	var leadID string
	if err := testPool.QueryRow(ctx, `SELECT id FROM agent WHERE workspace_id = $1 AND name = 'Handler Test Agent'`, testWorkspaceID).Scan(&leadID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET assignee_type = 'agent', assignee_id = $2 WHERE id = $1`, issue.ID, leadID); err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.Queries.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.setBPAWorkflowValues(newRequest("POST", "/", nil), issue, map[string]any{"bpa.template": "standard"})
	if err != nil {
		t.Fatal(err)
	}

	if err := testHandler.validateBPACompletion(ctx, issue); err == nil || !strings.Contains(err.Error(), "final root summary") {
		t.Fatalf("completion without final summary error = %v, want final summary conflict", err)
	}

	_, err = testHandler.Queries.CreateComment(ctx, db.CreateCommentParams{
		IssueID:     issue.ID,
		WorkspaceID: issue.WorkspaceID,
		AuthorType:  "agent",
		AuthorID:    parseUUID(leadID),
		Content: "**Що було не так:** transient Google API error became 500\n\n" +
			"**Що змінили:** return 503 after retries\n\n" +
			"**Що перевірили:** 14 tests passed\n\n" +
			"**Результат:** deploy is ready\n\n" +
			"**Ризик / наступне:** немає відомого",
		Type: "comment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := testHandler.validateBPACompletion(ctx, issue); err != nil {
		t.Fatalf("completion with final summary: %v", err)
	}
}

func TestBPAChildCannotCloseWithoutCommitEvidence(t *testing.T) {
	ctx := context.Background()
	parentID := createMetadataTestIssue(t, "BPA child requires commit evidence")
	parent, err := testHandler.Queries.GetIssue(ctx, parseUUID(parentID))
	if err != nil {
		t.Fatal(err)
	}
	req := newRequest("POST", "/api/issues/"+parentID+"/bpa/template", nil)
	if _, err := testHandler.setBPAWorkflowValues(req, parent, map[string]any{"bpa.template": "standard"}); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	createChild := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":           "BPA child without evidence " + time.Now().Format(time.RFC3339Nano),
		"status":          "in_progress",
		"parent_issue_id": parentID,
	})
	testHandler.CreateIssue(w, createChild)
	if w.Code != http.StatusCreated {
		t.Fatalf("create child: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var child IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&child); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, child.ID) })

	w = httptest.NewRecorder()
	update := newRequest("PUT", "/api/issues/"+child.ID, map[string]any{"status": "done"})
	update = withURLParam(update, "id", child.ID)
	testHandler.UpdateIssue(w, update)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBatchUpdateCannotCloseBPAChildWithoutCommitEvidence(t *testing.T) {
	ctx := context.Background()
	parentID := createMetadataTestIssue(t, "BPA batch child requires commit evidence")
	parent, err := testHandler.Queries.GetIssue(ctx, parseUUID(parentID))
	if err != nil {
		t.Fatal(err)
	}
	req := newRequest("POST", "/api/issues/"+parentID+"/bpa/template", nil)
	if _, err := testHandler.setBPAWorkflowValues(req, parent, map[string]any{"bpa.template": "standard"}); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	createChild := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":           "BPA batch child without evidence " + time.Now().Format(time.RFC3339Nano),
		"status":          "in_progress",
		"parent_issue_id": parentID,
	})
	testHandler.CreateIssue(w, createChild)
	if w.Code != http.StatusCreated {
		t.Fatalf("create child: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var child IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&child); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, child.ID) })

	w = httptest.NewRecorder()
	batch := newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{child.ID},
		"updates":   map[string]any{"status": "done"},
	})
	testHandler.BatchUpdateIssues(w, batch)
	if w.Code != http.StatusOK {
		t.Fatalf("batch update: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Updated int `json:"updated"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Updated != 0 {
		t.Fatalf("batch must not close BPA child without evidence, updated=%d", result.Updated)
	}
}

func TestGitHubMergeCannotCloseBPAChildWithoutCommitEvidence(t *testing.T) {
	ctx := context.Background()
	parentID := createMetadataTestIssue(t, "BPA GitHub child requires commit evidence")
	parent, err := testHandler.Queries.GetIssue(ctx, parseUUID(parentID))
	if err != nil {
		t.Fatal(err)
	}
	req := newRequest("POST", "/api/issues/"+parentID+"/bpa/template", nil)
	if _, err := testHandler.setBPAWorkflowValues(req, parent, map[string]any{"bpa.template": "standard"}); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	createChild := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":           "BPA GitHub child without evidence " + time.Now().Format(time.RFC3339Nano),
		"status":          "in_progress",
		"parent_issue_id": parentID,
	})
	testHandler.CreateIssue(w, createChild)
	if w.Code != http.StatusCreated {
		t.Fatalf("create child: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var child IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&child); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, child.ID) })

	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(child.ID))
	if err != nil {
		t.Fatal(err)
	}
	testHandler.advanceIssueToDone(ctx, issue, testWorkspaceID)
	updated, err := testHandler.Queries.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status == "done" {
		t.Fatal("GitHub merge must not close BPA child without evidence")
	}
}

func TestConfiguredArchivistCannotCreateIssueComment(t *testing.T) {
	ctx := context.Background()
	issueID := createMetadataTestIssue(t, "BPA Archivist cannot comment")
	var archivistID string
	if err := testPool.QueryRow(ctx, `SELECT id FROM agent WHERE workspace_id = $1 AND name = 'Handler Test Agent'`, testWorkspaceID).Scan(&archivistID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET metadata = $2::jsonb WHERE id = $1`, issueID, fmt.Sprintf(`{"bpa.template":"standard","bpa.archivist_agent_id":%q}`, archivistID)); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/"+issueID+"/comments", CreateCommentRequest{Content: "архівний прогрес"})
	req = withURLParam(req, "id", issueID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", archivistID)
	testHandler.CreateComment(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("configured Archivist comment: expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestQueueBPAArchivistRecordsKnowledgeWithoutCreatingRun(t *testing.T) {
	ctx := context.Background()
	issueID := createMetadataTestIssue(t, "BPA Archivist queue dedupe")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	var agentID string
	if err := testPool.QueryRow(ctx, `SELECT id FROM agent WHERE workspace_id = $1 AND name = 'Handler Test Agent'`, testWorkspaceID).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	req := newRequest("POST", "/api/issues/"+issueID, nil)
	issue, err = testHandler.setBPAWorkflowValues(req, issue, map[string]any{
		"bpa.template":           "standard",
		"bpa.archivist_agent_id": agentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM agent_task_queue WHERE issue_id = $1`, issue.ID) })

	testHandler.queueBPAArchivist(ctx, issue, "root_changed")
	testHandler.queueBPAArchivist(ctx, issue, "root_changed")

	var count int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2 AND status IN ('queued','dispatched','running','waiting_local_directory')`, issue.ID, agentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("background Archivist must not create a runtime task, got %d run(s)", count)
	}
	updated, err := testHandler.Queries.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	metadata := parseIssueMetadata(updated.Metadata)
	if metadata["bpa.knowledge_version"] != float64(1) || metadata["bpa.knowledge_event"] != "root_changed" {
		t.Fatalf("knowledge metadata = %#v", metadata)
	}
}

func TestQueueBPAArchivistWaitsUntilDeliveryIsIdle(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "ArchivistIdleLead", []byte("[]"))
	workerID := createHandlerTestAgent(t, "ArchivistIdleWorker", []byte("[]"))
	archivistID := createHandlerTestAgent(t, "ArchivistIdleArchivist", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92142, "archivist waits for delivery")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template":           string(bpa.TemplateStandard),
		"bpa.archivist_agent_id": archivistID,
	})
	if err != nil {
		t.Fatal(err)
	}
	insertRunningIssueTask(t, workerID, issueID)

	testHandler.queueBPAArchivist(ctx, issue, "worker_active")
	var count int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2 AND status IN ('queued','dispatched','running','waiting_local_directory')`, issue.ID, archivistID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("Archivist must wait for active delivery, got %d active archive task(s)", count)
	}
}

func TestQueueBPAArchivistWaitsForProductionApproval(t *testing.T) {
	ctx := context.Background()
	issueID := createMetadataTestIssue(t, "BPA Archivist approval gate")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	var agentID string
	if err := testPool.QueryRow(ctx, `SELECT id FROM agent WHERE workspace_id = $1 AND name = 'Handler Test Agent'`, testWorkspaceID).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.setBPAWorkflowValues(newRequest("POST", "/api/issues/"+issueID, nil), issue, map[string]any{
		"bpa.template":           "production",
		"bpa.archivist_agent_id": agentID,
		"bpa.waiting_for":        "human_approval",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM agent_task_queue WHERE issue_id = $1`, issue.ID) })

	testHandler.queueBPAArchivist(ctx, issue, "root_changed")
	var count int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2`, issue.ID, agentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("Archivist must wait for Production approval, got %d runs", count)
	}
}

func TestQueueBPAArchivistAfterTaskCompletionRecordsKnowledgeWithoutRun(t *testing.T) {
	ctx := context.Background()
	issueID := createMetadataTestIssue(t, "BPA Archivist task completion")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	var archivistID string
	if err := testPool.QueryRow(ctx, `SELECT id FROM agent WHERE workspace_id = $1 AND name = 'Handler Test Agent'`, testWorkspaceID).Scan(&archivistID); err != nil {
		t.Fatal(err)
	}
	workerID := createHandlerTestAgent(t, "BPA Archivist completion worker", nil)
	if _, err := testPool.Exec(ctx, `UPDATE issue SET metadata = $2::jsonb WHERE id = $1`, issueID, fmt.Sprintf(`{"bpa.template":"standard","bpa.archivist_agent_id":%q}`, archivistID)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM agent_task_queue WHERE issue_id = $1`, issue.ID) })

	testHandler.queueBPAArchivistAfterTaskCompletion(ctx, db.AgentTaskQueue{IssueID: issue.ID, AgentID: parseUUID(workerID)})
	var count int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2 AND status IN ('queued','dispatched','running','waiting_local_directory')`, issue.ID, archivistID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("task completion must not create an Archivist runtime task, got %d", count)
	}

	if _, err := testPool.Exec(ctx, `DELETE FROM agent_task_queue WHERE issue_id = $1`, issue.ID); err != nil {
		t.Fatal(err)
	}
	testHandler.queueBPAArchivistAfterTaskCompletion(ctx, db.AgentTaskQueue{IssueID: issue.ID, AgentID: parseUUID(archivistID)})
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1`, issue.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("Archivist completion requeued itself: %d runs", count)
	}
}

func TestArchivistCompletionDoesNotWakeRootLead(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "ArchivistWakeLead", []byte("[]"))
	archivistID := createHandlerTestAgent(t, "ArchivistWakeArchivist", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92143, "archivist completion must not wake lead")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template":           string(bpa.TemplateStandard),
		"bpa.archivist_agent_id": archivistID,
	})
	if err != nil {
		t.Fatal(err)
	}

	taskID := insertRunningIssueTask(t, archivistID, issueID)
	if w := completeTaskViaHandler(t, taskID, "archive metadata updated"); w.Code != http.StatusOK {
		t.Fatalf("complete Archivist task: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var count int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2 AND status IN ('queued','dispatched','running','waiting_local_directory')`, issue.ID, leadID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("Archivist completion must not wake root Lead, got %d task(s)", count)
	}
}

func TestQueueBPAArchivistDiscoversNamedWorkspaceAgentWithoutRun(t *testing.T) {
	ctx := context.Background()
	archivistID := createHandlerTestAgent(t, "AT Archivist", nil)
	issueID := createMetadataTestIssue(t, "BPA Archivist automatic discovery")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET metadata = '{"bpa.template":"standard"}'::jsonb WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.Queries.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM agent_task_queue WHERE issue_id = $1`, issue.ID) })

	testHandler.queueBPAArchivist(ctx, issue, "workflow_changed")
	updated, err := testHandler.Queries.GetIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := parseIssueMetadata(updated.Metadata)["bpa.archivist_agent_id"]; got != archivistID {
		t.Fatalf("discovered Archivist ID = %v, want %s", got, archivistID)
	}
	var count int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2 AND status = 'queued'`, issue.ID, archivistID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("automatically discovered Archivist must not run, got %d", count)
	}
}

func TestBPAArchivistCompletionDoesNotCreateIssueComment(t *testing.T) {
	ctx := context.Background()
	issueID := createMetadataTestIssue(t, "BPA Archivist silent completion")
	var archivistID string
	if err := testPool.QueryRow(ctx, `SELECT id FROM agent WHERE workspace_id = $1 AND name = 'Handler Test Agent'`, testWorkspaceID).Scan(&archivistID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET metadata = $2::jsonb WHERE id = $1`, issueID, fmt.Sprintf(`{"bpa.template":"standard","bpa.archivist_agent_id":%q}`, archivistID)); err != nil {
		t.Fatal(err)
	}
	taskID := createHandlerTestTaskForAgentOnIssue(t, archivistID, issueID)
	result, _ := json.Marshal(TaskCompleteRequest{Output: "technical completion output that must stay in run history"})
	if _, err := testHandler.TaskService.CompleteTask(ctx, parseUUID(taskID), result, "", ""); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM comment WHERE issue_id = $1 AND author_id = $2`, issueID, archivistID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("Archivist completion comments = %d, want 0", count)
	}
}
