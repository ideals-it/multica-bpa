package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
