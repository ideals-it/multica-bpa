package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/bpa"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type StartBPAWorkflowRequest struct {
	Template bpa.Template `json:"template"`
}

type BPAWorkflowResponse struct {
	State    bpa.State      `json:"state"`
	Metadata map[string]any `json:"metadata"`
}

func (h *Handler) GetBPAWorkflow(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	h.writeBPAWorkflow(w, issue)
}

func (h *Handler) StartBPAWorkflow(w http.ResponseWriter, r *http.Request) {
	var req StartBPAWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actorType, _ := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	if actorType != "member" {
		writeError(w, http.StatusForbidden, "only a human member can start a BPA workflow")
		return
	}
	parentID := ""
	if issue.ParentIssueID.Valid {
		parentID = uuidToString(issue.ParentIssueID)
	}
	if err := bpa.ValidateTemplateStart(req.Template, bpa.IssueRef{ParentID: parentID, AssigneeType: issue.AssigneeType.String}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.setBPAWorkflowValues(r, issue, map[string]any{
		"bpa.template":    string(req.Template),
		"bpa.waiting_for": string(bpa.WaitingForLead),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start BPA workflow")
		return
	}
	h.writeBPAWorkflow(w, updated)
}

func (h *Handler) setBPAWorkflowValues(r *http.Request, issue db.Issue, values map[string]any) (db.Issue, error) {
	userID := requestUserID(r)
	workspaceID := uuidToString(issue.WorkspaceID)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	raw, err := json.Marshal(values)
	if err != nil {
		return db.Issue{}, err
	}
	updated, err := h.Queries.SetIssueMetadataValues(r.Context(), db.SetIssueMetadataValuesParams{
		ID: issue.ID, WorkspaceID: issue.WorkspaceID, Values: raw,
	})
	if err != nil {
		return db.Issue{}, err
	}
	h.publish(protocol.EventIssueMetadataChanged, workspaceID, actorType, actorID, map[string]any{
		"issue_id": uuidToString(updated.ID), "metadata": parseIssueMetadata(updated.Metadata),
	})
	h.queueBPAArchivist(r.Context(), updated, "workflow_changed")
	return updated, nil
}

func (h *Handler) writeBPAWorkflow(w http.ResponseWriter, issue db.Issue) {
	metadata := parseIssueMetadata(issue.Metadata)
	state, err := bpa.ParseState(metadata)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid BPA workflow metadata")
		return
	}
	writeJSON(w, http.StatusOK, BPAWorkflowResponse{State: state, Metadata: metadata})
}

func (h *Handler) beginBPAHumanReview(r *http.Request, issue db.Issue) (db.Issue, error) {
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil || state.Template != bpa.TemplateProduction || issue.ParentIssueID.Valid {
		return issue, err
	}
	description := ""
	if issue.Description.Valid {
		description = issue.Description.String
	}
	return h.setBPAWorkflowValues(r, issue, map[string]any{
		"bpa.scope_fingerprint":          bpa.TicketScopeFingerprint(issue.Title, description),
		"bpa.approved_scope_fingerprint": "",
		"bpa.approval_status":            string(bpa.ApprovalPending),
		"bpa.waiting_for":                string(bpa.WaitingForHumanApproval),
	})
}

// bpaRootHasOpenChildren enforces Lead fan-in on a BPA root. The generic
// Multica board remains unchanged; only an enabled BPA template gets this
// workflow guard before the root can be closed.
func (h *Handler) bpaRootHasOpenChildren(ctx context.Context, issue db.Issue) (bool, error) {
	if issue.ParentIssueID.Valid {
		return false, nil
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil || !state.Enabled() {
		return false, err
	}
	children, err := h.Queries.ListChildIssues(ctx, issue.ID)
	if err != nil {
		return false, err
	}
	statuses := make([]string, 0, len(children))
	for _, child := range children {
		statuses = append(statuses, child.Status)
	}
	return bpa.HasOpenChildren(statuses), nil
}

// bpaChildHasCommitEvidence applies the repository checkpoint rule only to a
// child whose root explicitly uses a BPA workflow template.
func (h *Handler) bpaChildHasCommitEvidence(ctx context.Context, issue db.Issue) (bool, error) {
	if !issue.ParentIssueID.Valid {
		return true, nil
	}
	_, enabled, err := h.bpaRoot(ctx, issue)
	if err != nil || !enabled {
		return true, err
	}
	return bpa.HasCommitEvidence(parseIssueMetadata(issue.Metadata)), nil
}

// validateBPACompletion is the one completion gate shared by direct edits,
// batch updates, and GitHub merge completion.
func (h *Handler) validateBPACompletion(ctx context.Context, issue db.Issue) error {
	hasCommitEvidence, err := h.bpaChildHasCommitEvidence(ctx, issue)
	if err != nil {
		return err
	}
	if !hasCommitEvidence {
		return fmt.Errorf("BPA child needs a commit SHA or explicit no repo changes reason before completion")
	}
	hasOpenChildren, err := h.bpaRootHasOpenChildren(ctx, issue)
	if err != nil {
		return err
	}
	if hasOpenChildren {
		return fmt.Errorf("BPA main task cannot be closed while child tasks are still open")
	}
	return nil
}

// queueBPAArchivist schedules one read-only Archivist refresh for a BPA root.
// The task reads live issue state, so an existing pending run already covers
// later events and the native active-task unique index provides deduplication.
func (h *Handler) queueBPAArchivist(ctx context.Context, issue db.Issue, event string) {
	issue, enabled, err := h.bpaRoot(ctx, issue)
	if err != nil || !enabled {
		return
	}
	metadata := parseIssueMetadata(issue.Metadata)
	state, err := bpa.ParseState(metadata)
	if err != nil {
		return
	}
	issue, archivistID, ok := h.resolveBPAArchivist(ctx, issue, metadata)
	if !ok {
		return
	}
	for key, value := range map[string]any{
		"bpa.knowledge_version":    1,
		"bpa.knowledge_updated_at": time.Now().UTC().Format(time.RFC3339),
		"bpa.knowledge_event":      event,
		"bpa.knowledge_status":     issue.Status,
		"bpa.knowledge_template":   string(state.Template),
	} {
		raw, _ := json.Marshal(value)
		updated, setErr := h.Queries.SetIssueMetadataKey(ctx, db.SetIssueMetadataKeyParams{ID: issue.ID, WorkspaceID: issue.WorkspaceID, Key: key, Value: raw})
		if setErr == nil {
			issue = updated
		}
	}
	if h.hasActiveTaskForIssueAndAgent(ctx, issue.ID, archivistID) {
		return
	}
	if _, err := h.TaskService.EnqueueTaskForBPAArchivist(ctx, issue, archivistID); err != nil {
		slog.Debug("BPA Archivist enqueue skipped", "issue_id", uuidToString(issue.ID), "error", err)
	}
}

// resolveBPAArchivist prefers an explicit human-configured agent ID. When it
// is absent, the exact workspace role name makes Archivist autonomous without
// adding settings UI or a new workspace table.
func (h *Handler) resolveBPAArchivist(ctx context.Context, issue db.Issue, metadata map[string]any) (db.Issue, pgtype.UUID, bool) {
	if agentID, ok := metadata["bpa.archivist_agent_id"].(string); ok && strings.TrimSpace(agentID) != "" {
		archivistID, err := util.ParseUUID(agentID)
		return issue, archivistID, err == nil
	}
	agents, err := h.Queries.ListAgents(ctx, issue.WorkspaceID)
	if err != nil {
		return issue, pgtype.UUID{}, false
	}
	for _, agent := range agents {
		if agent.Name != "AT Archivist" {
			continue
		}
		raw, _ := json.Marshal(uuidToString(agent.ID))
		updated, err := h.Queries.SetIssueMetadataKey(ctx, db.SetIssueMetadataKeyParams{
			ID: issue.ID, WorkspaceID: issue.WorkspaceID, Key: "bpa.archivist_agent_id", Value: raw,
		})
		if err != nil {
			return issue, pgtype.UUID{}, false
		}
		return updated, agent.ID, true
	}
	return issue, pgtype.UUID{}, false
}

func isConfiguredBPAArchivist(issue db.Issue, agentID string) bool {
	archivistID, _ := parseIssueMetadata(issue.Metadata)["bpa.archivist_agent_id"].(string)
	return archivistID != "" && archivistID == agentID
}

// queueBPAArchivistAfterTaskCompletion refreshes the root knowledge index even
// when a worker run completes without posting a comment or changing the issue.
// The Archivist's own completion is excluded to prevent a self-refresh loop.
func (h *Handler) queueBPAArchivistAfterTaskCompletion(ctx context.Context, task db.AgentTaskQueue) {
	if !task.IssueID.Valid {
		return
	}
	issue, err := h.Queries.GetIssue(ctx, task.IssueID)
	if err != nil {
		return
	}
	root, enabled, err := h.bpaRoot(ctx, issue)
	if err != nil || !enabled {
		return
	}
	archivistID, _ := parseIssueMetadata(root.Metadata)["bpa.archivist_agent_id"].(string)
	if archivistID != "" && archivistID == uuidToString(task.AgentID) {
		return
	}
	h.queueBPAArchivist(ctx, issue, "task_completed")
}

func isBPAApprovalComment(content string) bool {
	switch strings.ToLower(strings.TrimSpace(content)) {
	case "approve", "approved", "погоджую", "погоджено":
		return true
	default:
		return false
	}
}

func (h *Handler) approveBPAReviewComment(r *http.Request, issue db.Issue, actorType, content string) (db.Issue, error) {
	if actorType != "member" || issue.Status != "in_review" || !isBPAApprovalComment(content) {
		return issue, nil
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil || state.Template != bpa.TemplateProduction || state.ApprovalStatus != bpa.ApprovalPending || state.ScopeFingerprint == "" {
		return issue, err
	}
	description := ""
	if issue.Description.Valid {
		description = issue.Description.String
	}
	if state.ScopeFingerprint != bpa.TicketScopeFingerprint(issue.Title, description) {
		return h.beginBPAHumanReview(r, issue)
	}
	return h.setBPAWorkflowValues(r, issue, map[string]any{
		"bpa.approval_status":            string(bpa.ApprovalApproved),
		"bpa.approved_scope_fingerprint": state.ScopeFingerprint,
		"bpa.waiting_for":                string(bpa.WaitingForLead),
	})
}
