package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func localArtifactRequest(t *testing.T, taskID, artifactPath string) *http.Request {
	t.Helper()
	req := newDaemonTokenRequest(http.MethodGet, "/api/daemon/tasks/"+taskID+"/local-artifact", nil, testWorkspaceID, "legit-daemon")
	q := req.URL.Query()
	q.Set("artifact_path", artifactPath)
	req.URL.RawQuery = q.Encode()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("taskId", taskID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestResolveTaskLocalArtifact(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	agentID := createHandlerTestAgent(t, "LocalArtifactAgent", []byte("[]"))
	issueID := insertAgentAssignedIssue(t, agentID, 92140, "local-artifact")
	taskID := insertRunningIssueTask(t, agentID, issueID)
	const workDir = "/Users/tester/projects/service"
	if _, err := testPool.Exec(context.Background(), `UPDATE agent_task_queue SET work_dir = $1 WHERE id = $2`, workDir, taskID); err != nil {
		t.Fatalf("set task work dir: %v", err)
	}

	t.Run("returns task root and safe relative path", func(t *testing.T) {
		w := httptest.NewRecorder()
		testHandler.ResolveTaskLocalArtifact(w, localArtifactRequest(t, taskID, "audit-results/report.json"))
		if w.Code != http.StatusOK {
			t.Fatalf("ResolveTaskLocalArtifact: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var got TaskLocalArtifactResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.WorkDir != workDir || got.RelativePath != "audit-results/report.json" {
			t.Fatalf("response = %#v", got)
		}
	})

	for _, artifactPath := range []string{"", "/tmp/outside.json", "../outside.json", "audit-results/../../outside.json"} {
		t.Run("rejects unsafe path "+artifactPath, func(t *testing.T) {
			w := httptest.NewRecorder()
			testHandler.ResolveTaskLocalArtifact(w, localArtifactRequest(t, taskID, artifactPath))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("unsafe path %q: expected 400, got %d: %s", artifactPath, w.Code, w.Body.String())
			}
		})
	}
}
