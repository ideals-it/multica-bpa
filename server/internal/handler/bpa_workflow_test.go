package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestDecideBPAApprovalRejectsAgentActor(t *testing.T) {
	issueID := createMetadataTestIssue(t, "approval actor")
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/"+issueID+"/bpa/approval-decision", ApprovalDecisionRequest{Decision: "approved"})
	req = withURLParam(req, "id", issueID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", "00000000-0000-0000-0000-000000000001")
	testHandler.DecideBPAApproval(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
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

func TestQueueBPAArchivistDeduplicatesPendingRuns(t *testing.T) {
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
		"bpa.template":           "production",
		"bpa.archivist_agent_id": agentID,
		"bpa.scope_fingerprint":  "sha256:pending-production-scope",
		"bpa.approval_status":    "pending",
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
	if count != 1 {
		t.Fatalf("pending Archivist runs = %d, want 1", count)
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

func TestQueueBPAArchivistAfterNonArchivistTaskCompletion(t *testing.T) {
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
	if count != 1 {
		t.Fatalf("Archivist runs after worker completion = %d, want 1", count)
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

func TestQueueBPAArchivistDiscoversNamedWorkspaceAgent(t *testing.T) {
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
	if count != 1 {
		t.Fatalf("automatically discovered Archivist runs = %d, want 1", count)
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
