package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestLeadReactionDoesNotBypassProductionReview(t *testing.T) {
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
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil || state.ApprovalStatus != bpa.ApprovalPending {
		t.Fatalf("reaction must remain a normal signal, state = %#v, err=%v", state, err)
	}
	_ = comment
}

func TestBPAReviewOwnerCommentQueuesContinuationWithoutRecordingApproval(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "NaturalLanguageApprovalLead", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92142, "natural language review")
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
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template": string(bpa.TemplateProduction),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testHandler.beginBPAHumanReview(newRequest(http.MethodPost, "/", nil), issue); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/comments", CreateCommentRequest{
		Content: "Так, але спочатку перевір резервну копію.",
	})
	req = withURLParam(req, "id", issueID)
	testHandler.CreateComment(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateComment: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var queued int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*)
		FROM agent_task_queue
		WHERE issue_id = $1 AND agent_id = $2 AND trigger_comment_id IS NOT NULL AND status = 'queued'
	`, issueID, leadID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("owner review comment must queue one continuation, got %d", queued)
	}

	issue, err = testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil || state.ApprovalStatus != bpa.ApprovalPending || state.ReviewCommentID == "" {
		t.Fatalf("comment must not be interpreted by the server, state=%#v err=%v", state, err)
	}
}

func TestLegacyPendingProductionReviewCommentInitializesReviewTriggerAndAllowsContinuation(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "LegacyReviewDiscussionAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92155, "legacy production review discussion")
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	// This is intentionally an older pending review: it predates the
	// review_requested_at marker used to bind the next owner/admin reply to
	// the agent task that receives it. The agent still interprets the full
	// natural-language reply; initializing this marker is not itself approval.
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template":          string(bpa.TemplateProduction),
		"bpa.waiting_for":       string(bpa.WaitingForHumanApproval),
		"bpa.approval_status":   string(bpa.ApprovalPending),
		"bpa.scope_fingerprint": bpa.TicketScopeFingerprint(issue.Title, ""),
	})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/comments", CreateCommentRequest{
		Content: "[@LegacyReviewDiscussionAgent](mention://agent/" + agentID + ") усе перевірив: можна продовжувати саме з погодженим планом.",
	})
	req = withURLParam(req, "id", issueID)
	testHandler.CreateComment(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateComment: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var comment CommentResponse
	if err := json.NewDecoder(w.Body).Decode(&comment); err != nil {
		t.Fatal(err)
	}

	var taskID string
	if err := testPool.QueryRow(ctx, `
		SELECT id::text
		FROM agent_task_queue
		WHERE issue_id = $1 AND agent_id = $2 AND trigger_comment_id = $3::uuid AND status = 'queued'
	`, issueID, agentID, comment.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}

	issue, err = testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil {
		t.Fatal(err)
	}
	if state.ReviewRequestedAt == "" || state.ReviewCommentID != comment.ID {
		t.Fatalf("legacy review must be initialized from the current member comment, state=%#v", state)
	}
	if state.ApprovalStatus != bpa.ApprovalPending || state.WaitingFor != bpa.WaitingForHumanApproval {
		t.Fatalf("initializing a legacy review trigger must not itself approve it, state=%#v", state)
	}

	if _, err := testPool.Exec(ctx, `UPDATE agent_task_queue SET status = 'running', started_at = now() WHERE id = $1`, taskID); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "in_progress"})
	req = withURLParam(req, "id", issueID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", taskID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("legacy review continuation must enter In Progress, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStaleReviewCommentCannotMarkAReplacementProductionReview(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "StaleReviewMarkerAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92153, "original production scope")
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template": string(bpa.TemplateProduction),
	})
	if err != nil {
		t.Fatal(err)
	}
	staleReview, err := testHandler.beginBPAHumanReview(newRequest(http.MethodPost, "/", nil), issue)
	if err != nil {
		t.Fatal(err)
	}
	comment, err := testHandler.Queries.CreateComment(ctx, db.CreateCommentParams{
		IssueID: staleReview.ID, WorkspaceID: staleReview.WorkspaceID,
		AuthorType: "member", AuthorID: parseUUID(testUserID),
		Content: "Погоджую лише початковий scope.", Type: "comment",
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID := createHandlerTestTaskForAgentOnIssue(t, agentID, issueID)
	if _, err := testPool.Exec(ctx, `UPDATE agent_task_queue SET trigger_comment_id = $1 WHERE id = $2`, comment.ID, taskID); err != nil {
		t.Fatal(err)
	}
	task, err := testHandler.Queries.GetAgentTask(ctx, parseUUID(taskID))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := testPool.Exec(ctx, `UPDATE issue SET title = 'replacement production scope', updated_at = clock_timestamp() WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	current, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testHandler.beginBPAHumanReview(newRequest(http.MethodPost, "/", nil), current); err != nil {
		t.Fatal(err)
	}

	testHandler.markBPAReviewTrigger(ctx, staleReview, task, comment.ID)
	current, err = testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	state, err := bpa.ParseState(parseIssueMetadata(current.Metadata))
	if err != nil {
		t.Fatal(err)
	}
	if state.ReviewCommentID != "" {
		t.Fatalf("stale review comment marked replacement review: %s", state.ReviewCommentID)
	}
}

func TestDeletingCurrentReviewCommentAllowsAReplacementTrigger(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "DeletedReviewTriggerAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92154, "replace deleted review trigger")
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template": string(bpa.TemplateProduction),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testHandler.beginBPAHumanReview(newRequest(http.MethodPost, "/", nil), issue); err != nil {
		t.Fatal(err)
	}

	createReviewComment := func(content string) CommentResponse {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/comments", CreateCommentRequest{Content: content})
		req = withURLParam(req, "id", issueID)
		testHandler.CreateComment(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("CreateComment: expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var response CommentResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	first := createReviewComment("Перший коментар на погодження.")
	w := httptest.NewRecorder()
	req := newRequest(http.MethodDelete, "/api/issues/"+issueID+"/comments/"+first.ID, nil)
	req = withURLParam(req, "id", issueID)
	req = withURLParam(req, "commentId", first.ID)
	testHandler.DeleteComment(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DeleteComment: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	second := createReviewComment("Другий актуальний коментар на погодження.")
	issue, err = testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil {
		t.Fatal(err)
	}
	if state.ReviewCommentID != second.ID {
		t.Fatalf("replacement review trigger = %q, want %q", state.ReviewCommentID, second.ID)
	}
}

func TestBPAReviewLaterCommentKeepsApprovalTriggerAndIsDelivered(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "ReviewCommentCoalesceAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92147, "review comment coalescing")
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template": string(bpa.TemplateProduction),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testHandler.beginBPAHumanReview(newRequest(http.MethodPost, "/", nil), issue); err != nil {
		t.Fatal(err)
	}

	createComment := func(content string) string {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/comments", CreateCommentRequest{Content: content})
		req = withURLParam(req, "id", issueID)
		testHandler.CreateComment(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("CreateComment: expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var response CommentResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		return response.ID
	}

	approvalCommentID := createComment("Можна виконувати описаний деплой.")
	laterCommentID := createComment("Перед запуском ще перевір поточну revision.")

	var triggerID string
	var coalescedIDs []string
	if err := testPool.QueryRow(ctx, `
		SELECT trigger_comment_id::text, coalesced_comment_ids::text[]
		FROM agent_task_queue
		WHERE issue_id = $1 AND agent_id = $2 AND status = 'queued'
	`, issueID, agentID).Scan(&triggerID, &coalescedIDs); err != nil {
		t.Fatal(err)
	}
	if triggerID != approvalCommentID {
		t.Fatalf("production review trigger = %s, want original owner comment %s", triggerID, approvalCommentID)
	}
	if len(coalescedIDs) != 1 || coalescedIDs[0] != laterCommentID {
		t.Fatalf("coalesced comments = %v, want [%s]", coalescedIDs, laterCommentID)
	}
}

func TestBacklogIssueCommentDoesNotStartAssignedAgent(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "BacklogCommentAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92145, "backlog must remain parked")
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'backlog', origin_type = 'autopilot' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/comments", CreateCommentRequest{Content: "Це уточнення, не запуск."})
	req = withURLParam(req, "id", issueID)
	testHandler.CreateComment(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateComment: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var queued int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1`, issueID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("backlog comment queued %d task(s), want 0", queued)
	}
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != "backlog" {
		t.Fatalf("backlog issue status = %q, want backlog", issue.Status)
	}
}

func TestOrdinaryBacklogIssueCommentKeepsNativeAssigneeRouting(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "NativeBacklogCommentAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92148, "ordinary backlog keeps native routing")
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'backlog', origin_type = NULL, origin_id = NULL WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/comments", CreateCommentRequest{Content: "Починай роботу над цією задачею."})
	req = withURLParam(req, "id", issueID)
	testHandler.CreateComment(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateComment: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var queued int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2 AND status = 'queued'`, issueID, agentID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("ordinary Backlog comment queued %d task(s), want native routing to queue 1", queued)
	}
}

func TestBPAReviewContinuationCanEnterInProgressAfterAgentInterpretation(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	leadID := createHandlerTestAgent(t, "ReviewContinuationLead", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92143, "review continuation execution")
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
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template": string(bpa.TemplateProduction),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testHandler.beginBPAHumanReview(newRequest(http.MethodPost, "/", nil), issue); err != nil {
		t.Fatal(err)
	}
	comment, err := testHandler.Queries.CreateComment(ctx, db.CreateCommentParams{
		IssueID: issue.ID, WorkspaceID: issue.WorkspaceID, AuthorType: "member", AuthorID: parseUUID(testUserID),
		Content: "Можна виконувати саме описаний деплой після перевірки резервної копії.", Type: "comment",
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID := createHandlerTestTaskForAgentOnIssue(t, leadID, issueID)
	if _, err := testPool.Exec(ctx, `UPDATE agent_task_queue SET trigger_comment_id = $1 WHERE id = $2`, comment.ID, taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET metadata = metadata || jsonb_build_object('bpa.review_comment_id', $2::text) WHERE id = $1`, issueID, uuidToString(comment.ID)); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "in_progress"})
	req = withURLParam(req, "id", issueID)
	req.Header.Set("X-Agent-ID", leadID)
	req.Header.Set("X-Task-ID", taskID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("legacy member headers must not impersonate review continuation, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "in_progress"})
	req = withURLParam(req, "id", issueID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", leadID)
	req.Header.Set("X-Task-ID", taskID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("interpreted review continuation must enter In Progress, got %d: %s", w.Code, w.Body.String())
	}

	issue, err = testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil {
		t.Fatal(err)
	}
	if state.ApprovalStatus != bpa.ApprovalApproved || state.ApprovedScopeFingerprint != state.ScopeFingerprint {
		t.Fatalf("continuation must approve only the current scope, got %#v", state)
	}
}

func TestBPAReviewContinuationRejectsCommentFromPreviousReview(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "StaleReviewContinuationAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92146, "stale production review")
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
	if _, err := testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template":                   string(bpa.TemplateProduction),
		"bpa.waiting_for":                string(bpa.WaitingForHumanApproval),
		"bpa.scope_fingerprint":          bpa.TicketScopeFingerprint(issue.Title, ""),
		"bpa.approval_status":            string(bpa.ApprovalPending),
		"bpa.review_requested_at":        time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),
		"bpa.approved_scope_fingerprint": "",
	}); err != nil {
		t.Fatal(err)
	}
	comment, err := testHandler.Queries.CreateComment(ctx, db.CreateCommentParams{
		IssueID: issue.ID, WorkspaceID: issue.WorkspaceID, AuthorType: "member", AuthorID: parseUUID(testUserID),
		Content: "Погоджую попередній scope.", Type: "comment",
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID := createHandlerTestTaskForAgentOnIssue(t, agentID, issueID)
	if _, err := testPool.Exec(ctx, `UPDATE agent_task_queue SET trigger_comment_id = $1 WHERE id = $2`, comment.ID, taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET metadata = metadata || jsonb_build_object('bpa.review_comment_id', $2::text) WHERE id = $1`, issueID, uuidToString(comment.ID)); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "in_progress"})
	req = withURLParam(req, "id", issueID)
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", taskID)
	req.Header.Set("X-Actor-Source", "task_token")
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale review continuation: expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBPAReviewContinuationRejectsCommentWrittenBeforeReviewWasVisible(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "PreReviewCommentAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92150, "pre-review production comment")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	comment, err := testHandler.Queries.CreateComment(ctx, db.CreateCommentParams{
		IssueID: issue.ID, WorkspaceID: issue.WorkspaceID, AuthorType: "member", AuthorID: parseUUID(testUserID),
		Content: "Цей коментар написаний до появи review.", Type: "comment",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the narrow race: the in-memory review timestamp was sampled,
	// then the comment landed, and only afterward did review state become
	// visible through the persisted issue update timestamp.
	if _, err := testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template":                   string(bpa.TemplateProduction),
		"bpa.waiting_for":                string(bpa.WaitingForHumanApproval),
		"bpa.scope_fingerprint":          bpa.TicketScopeFingerprint(issue.Title, ""),
		"bpa.approval_status":            string(bpa.ApprovalPending),
		"bpa.review_requested_at":        comment.CreatedAt.Time.Add(-time.Second).Format(time.RFC3339Nano),
		"bpa.approved_scope_fingerprint": "",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	taskID := createHandlerTestTaskForAgentOnIssue(t, agentID, issueID)
	if _, err := testPool.Exec(ctx, `UPDATE agent_task_queue SET trigger_comment_id = $1 WHERE id = $2`, comment.ID, taskID); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "in_progress"})
	req = withURLParam(req, "id", issueID)
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", taskID)
	req.Header.Set("X-Actor-Source", "task_token")
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("pre-review comment continuation: expected 409, got %d: %s", w.Code, w.Body.String())
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

func TestApprovedProductionScopeCannotChangeAfterWorkResumes(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "ApprovedScopeChangeAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92144, "approved production scope")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	scope := bpa.TicketScopeFingerprint(issue.Title, "")
	if _, err := testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template":                   string(bpa.TemplateProduction),
		"bpa.waiting_for":                string(bpa.WaitingForLead),
		"bpa.scope_fingerprint":          scope,
		"bpa.approval_status":            string(bpa.ApprovalApproved),
		"bpa.approved_scope_fingerprint": scope,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"description": "expanded production scope"})
	req = withURLParam(req, "id", issueID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("change approved production scope: expected 409, got %d: %s", w.Code, w.Body.String())
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

	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "blocked"})
	req = withURLParam(req, "id", issueID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("pending review to Blocked: expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProductionReviewContinuationCannotChangeScopeWhileResuming(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "ReviewScopeResumeAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92149, "reviewed production scope")
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
		"bpa.template": string(bpa.TemplateProduction),
	})
	if err != nil {
		t.Fatal(err)
	}
	issue, err = testHandler.beginBPAHumanReview(newRequest(http.MethodPost, "/", nil), issue)
	if err != nil {
		t.Fatal(err)
	}
	comment, err := testHandler.Queries.CreateComment(ctx, db.CreateCommentParams{
		IssueID: issue.ID, WorkspaceID: issue.WorkspaceID, AuthorType: "member", AuthorID: parseUUID(testUserID),
		Content: "Погоджую описану задачу без зміни scope.", Type: "comment",
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID := createHandlerTestTaskForAgentOnIssue(t, agentID, issueID)
	if _, err := testPool.Exec(ctx, `UPDATE agent_task_queue SET trigger_comment_id = $1 WHERE id = $2`, comment.ID, taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET metadata = metadata || jsonb_build_object('bpa.review_comment_id', $2::text) WHERE id = $1`, issueID, uuidToString(comment.ID)); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{
		"status":      "in_progress",
		"description": "new production action that was not reviewed",
	})
	req = withURLParam(req, "id", issueID)
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", taskID)
	req.Header.Set("X-Actor-Source", "task_token")
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("resume with changed scope: expected 409, got %d: %s", w.Code, w.Body.String())
	}
	updated, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "in_review" || updated.Description.Valid {
		t.Fatalf("rejected scope change persisted status=%q description=%q", updated.Status, updated.Description.String)
	}
}

func TestOrdinaryInReviewIssueCanChangeScopeWhileLeavingReview(t *testing.T) {
	ctx := context.Background()
	issueID := createMetadataTestIssue(t, "ordinary review scope change")
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_review' WHERE id = $1`, issueID); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{
		"status": "in_progress", "description": "ordinary reviewed scope",
	})
	req = withURLParam(req, "id", issueID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ordinary In Review scope change: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPendingProductionReviewCannotBeBatchAdvancedOrGitHubCompleted(t *testing.T) {
	ctx := context.Background()
	issueID := createMetadataTestIssue(t, "pending production batch guard")
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
	req := newRequest(http.MethodPost, "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{issueID}, "updates": map[string]any{"status": "in_progress"},
	})
	testHandler.BatchUpdateIssues(w, req)
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
		t.Fatalf("pending review batch advanced %d issues, want 0", result.Updated)
	}
	issue, err = testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	testHandler.advanceIssueToDone(ctx, issue, testWorkspaceID)
	issue, err = testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	if issue.Status != "in_review" {
		t.Fatalf("GitHub completion moved pending review to %q", issue.Status)
	}
}

func TestBatchUpdateCannotChangeProductionScopeAcrossApprovalGate(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	t.Run("pending review", func(t *testing.T) {
		issueID := createMetadataTestIssue(t, "pending production batch scope")
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
		req := newRequest(http.MethodPost, "/api/issues/batch-update", map[string]any{
			"issue_ids": []string{issueID}, "updates": map[string]any{"description": "unreviewed production scope"},
		})
		testHandler.BatchUpdateIssues(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("batch update: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		updated, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
		if err != nil {
			t.Fatal(err)
		}
		state, err := bpa.ParseState(parseIssueMetadata(updated.Metadata))
		if err != nil {
			t.Fatal(err)
		}
		wantScope := bpa.TicketScopeFingerprint(updated.Title, "unreviewed production scope")
		if updated.Description.String != "unreviewed production scope" || state.ScopeFingerprint != wantScope ||
			state.ApprovalStatus != bpa.ApprovalPending || state.ReviewCommentID != "" {
			t.Fatalf("batch scope edit did not start a fresh review: issue=%#v state=%#v", updated, state)
		}
	})

	t.Run("approved execution", func(t *testing.T) {
		issueID := createMetadataTestIssue(t, "approved production batch scope")
		issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
		if err != nil {
			t.Fatal(err)
		}
		scope := bpa.TicketScopeFingerprint(issue.Title, "")
		if _, err := testHandler.setBPAWorkflowValues(newRequest(http.MethodPost, "/", nil), issue, map[string]any{
			"bpa.template":                   string(bpa.TemplateProduction),
			"bpa.waiting_for":                string(bpa.WaitingForLead),
			"bpa.scope_fingerprint":          scope,
			"bpa.approved_scope_fingerprint": scope,
			"bpa.approval_status":            string(bpa.ApprovalApproved),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issueID); err != nil {
			t.Fatal(err)
		}

		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/issues/batch-update", map[string]any{
			"issue_ids": []string{issueID}, "updates": map[string]any{"title": "expanded production scope"},
		})
		testHandler.BatchUpdateIssues(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("batch update: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		updated, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
		if err != nil {
			t.Fatal(err)
		}
		if updated.Title != issue.Title {
			t.Fatalf("batch changed approved production scope to %q", updated.Title)
		}
	})
}

func TestUpdateIssueCompareAndSwapRejectsStaleSnapshot(t *testing.T) {
	ctx := context.Background()
	issueID := createMetadataTestIssue(t, "optimistic issue update")
	issue, err := testHandler.Queries.GetIssue(ctx, parseUUID(issueID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET title = 'concurrent title', updated_at = clock_timestamp() WHERE id = $1`, issue.ID); err != nil {
		t.Fatal(err)
	}
	_, err = testHandler.Queries.UpdateIssue(ctx, db.UpdateIssueParams{
		ID: issue.ID, Description: pgtype.Text{String: "stale update", Valid: true},
		AssigneeType: issue.AssigneeType, AssigneeID: issue.AssigneeID, StartDate: issue.StartDate,
		DueDate: issue.DueDate, ParentIssueID: issue.ParentIssueID, ProjectID: issue.ProjectID,
		Stage: issue.Stage, ExpectedUpdatedAt: issue.UpdatedAt,
	})
	if err == nil {
		t.Fatal("stale UpdateIssue snapshot unexpectedly overwrote concurrent scope")
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

func TestProductionReviewStartsWithPendingScope(t *testing.T) {
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
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil || state.ApprovalStatus != bpa.ApprovalPending || state.ApprovedScopeFingerprint != "" {
		t.Fatalf("review state = %#v, err = %v", state, err)
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

func TestBPARootCompletionDoesNotRequireLeadSummary(t *testing.T) {
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

	if err := testHandler.validateBPACompletion(ctx, issue); err != nil {
		t.Fatalf("completion without a prescribed final summary: %v", err)
	}
}

func TestBPAChildCanCloseWithoutServerCommitMetadata(t *testing.T) {
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
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBatchUpdateCanCloseBPAChildWithoutServerCommitMetadata(t *testing.T) {
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
	if result.Updated != 1 {
		t.Fatalf("batch should close BPA child without server commit metadata, updated=%d", result.Updated)
	}
}

func TestGitHubMergeCanCloseBPAChildWithoutServerCommitMetadata(t *testing.T) {
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
	if updated.Status != "done" {
		t.Fatalf("GitHub merge should close BPA child without server commit metadata, got %q", updated.Status)
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
