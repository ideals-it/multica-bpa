package handler

import (
	"context"
	"net/http"
	"testing"
)

func TestCompleteTaskDoesNotQueueRootAssigneeAfterSpecialist(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	leadID := createHandlerTestAgent(t, "RootCompletionLead", []byte("[]"))
	builderID := createHandlerTestAgent(t, "RootCompletionBuilder", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92130, "root-specialist-completion")
	builderTaskID := insertRunningIssueTask(t, builderID, issueID)

	w := completeTaskViaHandler(t, builderTaskID, "specialist evidence is ready")
	if w.Code != http.StatusOK {
		t.Fatalf("CompleteTask: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := pendingTaskCountForAgentIssue(t, issueID, leadID); got != 0 {
		t.Fatalf("queued root-assignee tasks = %d, want 0", got)
	}

	var systemComments int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM comment WHERE issue_id = $1 AND author_type = 'system'`, issueID,
	).Scan(&systemComments); err != nil {
		t.Fatalf("count system comments: %v", err)
	}
	if systemComments != 0 {
		t.Fatalf("automatic root handoff must not create a system comment, got %d", systemComments)
	}
}

func TestCompleteTaskDoesNotQueueRootAssigneeWhileOtherSpecialistIsActive(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	leadID := createHandlerTestAgent(t, "RootCompletionParallelLead", []byte("[]"))
	firstBuilderID := createHandlerTestAgent(t, "RootCompletionFirstBuilder", []byte("[]"))
	secondBuilderID := createHandlerTestAgent(t, "RootCompletionSecondBuilder", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92131, "root-specialist-parallel")
	firstTaskID := insertRunningIssueTask(t, firstBuilderID, issueID)
	insertRunningIssueTask(t, secondBuilderID, issueID)

	w := completeTaskViaHandler(t, firstTaskID, "first specialist evidence is ready")
	if w.Code != http.StatusOK {
		t.Fatalf("CompleteTask: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := pendingTaskCountForAgentIssue(t, issueID, leadID); got != 0 {
		t.Fatalf("queued Lead tasks while a specialist remains active = %d, want 0", got)
	}
}

func TestCompleteTaskDoesNotQueueRootAssigneeAfterAnotherAgentFinishes(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	leadID := createHandlerTestAgent(t, "RootCompletionPendingLead", []byte("[]"))
	builderID := createHandlerTestAgent(t, "RootCompletionPendingBuilder", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, leadID, 92134, "root-specialist-handoff-during-lead")
	leadTaskID := insertRunningIssueTask(t, leadID, issueID)
	builderTaskID := insertRunningIssueTask(t, builderID, issueID)

	if w := completeTaskViaHandler(t, builderTaskID, "specialist evidence arrived while root agent was running"); w.Code != http.StatusOK {
		t.Fatalf("complete Builder task: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := pendingTaskCountForAgentIssue(t, issueID, leadID); got != 0 {
		t.Fatalf("queued root-assignee tasks while original task is active = %d, want 0", got)
	}

	if w := completeTaskViaHandler(t, leadTaskID, "root agent finished its earlier step"); w.Code != http.StatusOK {
		t.Fatalf("complete original root task: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := pendingTaskCountForAgentIssue(t, issueID, leadID); got != 0 {
		t.Fatalf("queued root-assignee follow-up after another agent completion = %d, want 0", got)
	}
}

func TestCompleteTaskDoesNotQueueRootAssigneeForOwnOrChildRun(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	leadID := createHandlerTestAgent(t, "RootCompletionSameLead", []byte("[]"))
	builderID := createHandlerTestAgent(t, "RootCompletionChildBuilder", []byte("[]"))
	rootIssueID := insertAgentAssignedIssue(t, leadID, 92132, "root-specialist-own")
	leadTaskID := insertRunningIssueTask(t, leadID, rootIssueID)

	w := completeTaskViaHandler(t, leadTaskID, "Lead completed its own run")
	if w.Code != http.StatusOK {
		t.Fatalf("complete Lead task: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := pendingTaskCountForAgentIssue(t, rootIssueID, leadID); got != 0 {
		t.Fatalf("queued Lead tasks after its own completion = %d, want 0", got)
	}

	var childIssueID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, priority, creator_id, creator_type, number, position, assignee_type, assignee_id, parent_issue_id)
		VALUES ($1, 'root-specialist-child', 'todo', 'medium', $2, 'member', 92133, 0, 'agent', $3, $4)
		RETURNING id
	`, testWorkspaceID, testUserID, builderID, rootIssueID).Scan(&childIssueID); err != nil {
		t.Fatalf("create child issue: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, childIssueID) })
	childTaskID := insertRunningIssueTask(t, builderID, childIssueID)

	w = completeTaskViaHandler(t, childTaskID, "child specialist completed")
	if w.Code != http.StatusOK {
		t.Fatalf("complete child task: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := pendingTaskCountForAgentIssue(t, childIssueID, builderID); got != 0 {
		t.Fatalf("queued child assignee tasks = %d, want 0", got)
	}
}
