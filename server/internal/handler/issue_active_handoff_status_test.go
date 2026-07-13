package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentBlockedTransition(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	newAgentRequest := func(agentID, issueID string) *http.Request {
		req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "blocked"})
		req = withURLParam(req, "id", issueID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		return req
	}

	prepareIssue := func(t *testing.T, number int) (string, string, string) {
		t.Helper()
		leadID := createHandlerTestAgent(t, "ActiveHandoffLead", []byte("[]"))
		builderID := createHandlerTestAgent(t, "ActiveHandoffBuilder", []byte("[]"))
		issueID := insertAgentAssignedIssue(t, leadID, number, "active-handoff-status")
		if _, err := testPool.Exec(context.Background(), `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issueID); err != nil {
			t.Fatalf("set issue in progress: %v", err)
		}
		return issueID, leadID, builderID
	}

	t.Run("agent cannot block while another agent waits for local directory", func(t *testing.T) {
		issueID, leadID, builderID := prepareIssue(t, 92120)
		insertRunningIssueTask(t, leadID, issueID)
		builderTaskID := insertRunningIssueTask(t, builderID, issueID)
		if _, err := testPool.Exec(context.Background(), `UPDATE agent_task_queue SET status = 'waiting_local_directory' WHERE id = $1`, builderTaskID); err != nil {
			t.Fatalf("set builder task waiting: %v", err)
		}

		w := httptest.NewRecorder()
		testHandler.UpdateIssue(w, newAgentRequest(leadID, issueID))
		if w.Code != http.StatusConflict {
			t.Fatalf("agent block with active handoff: expected 409, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "delegated task is still active") {
			t.Fatalf("agent block error = %s", w.Body.String())
		}
		issue, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(issueID))
		if err != nil {
			t.Fatal(err)
		}
		if issue.Status != "in_progress" {
			t.Fatalf("issue status = %q, want in_progress", issue.Status)
		}
	})

	t.Run("agent can block when only its own task is active", func(t *testing.T) {
		issueID, leadID, _ := prepareIssue(t, 92121)
		insertRunningIssueTask(t, leadID, issueID)

		w := httptest.NewRecorder()
		testHandler.UpdateIssue(w, newAgentRequest(leadID, issueID))
		if w.Code != http.StatusOK {
			t.Fatalf("agent block without handoff: expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("member can block while an agent task is active", func(t *testing.T) {
		issueID, leadID, builderID := prepareIssue(t, 92122)
		insertRunningIssueTask(t, leadID, issueID)
		insertRunningIssueTask(t, builderID, issueID)

		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "blocked"})
		req = withURLParam(req, "id", issueID)
		testHandler.UpdateIssue(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("member block with active handoff: expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}
