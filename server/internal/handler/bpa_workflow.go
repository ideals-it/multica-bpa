package handler

import (
	"context"
	"encoding/json"
	"fmt"
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
	values := map[string]any{
		"bpa.template":    string(req.Template),
		"bpa.waiting_for": string(bpa.WaitingForLead),
	}
	if req.Template == bpa.TemplateProduction && issue.Status == "in_review" {
		reviewRequestedAt, timestampErr := h.bpaReviewTimestamp(r.Context())
		if timestampErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to prepare BPA review")
			return
		}
		description := ""
		if issue.Description.Valid {
			description = issue.Description.String
		}
		values["bpa.scope_fingerprint"] = bpa.TicketScopeFingerprint(issue.Title, description)
		values["bpa.approved_scope_fingerprint"] = ""
		values["bpa.approval_status"] = string(bpa.ApprovalPending)
		values["bpa.review_requested_at"] = reviewRequestedAt.Format(time.RFC3339Nano)
		values["bpa.review_comment_id"] = ""
		values["bpa.waiting_for"] = string(bpa.WaitingForHumanApproval)
	}
	updated, err := h.setBPAWorkflowValues(r, issue, values)
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
	reviewRequestedAt, err := h.bpaReviewTimestamp(r.Context())
	if err != nil {
		return db.Issue{}, err
	}
	return h.setBPAWorkflowValues(r, issue, map[string]any{
		"bpa.scope_fingerprint":          bpa.TicketScopeFingerprint(issue.Title, description),
		"bpa.approved_scope_fingerprint": "",
		"bpa.approval_status":            string(bpa.ApprovalPending),
		"bpa.review_requested_at":        reviewRequestedAt.Format(time.RFC3339Nano),
		"bpa.review_comment_id":          "",
		"bpa.waiting_for":                string(bpa.WaitingForHumanApproval),
	})
}

func (h *Handler) bpaReviewTimestamp(ctx context.Context) (time.Time, error) {
	var timestamp time.Time
	if err := h.DB.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&timestamp); err != nil {
		return time.Time{}, err
	}
	return timestamp.UTC(), nil
}

// isBPAProductionReviewContinuation allows the assigned root agent to resume
// a pending review only from a live task triggered by an owner/admin comment.
// The server deliberately does not interpret the comment text: the agent reads
// the full conversation and decides whether the human response is a clear
// approval. Requiring the task/comment chain prevents a manual card move or an
// unrelated agent task from bypassing the production gate.
func (h *Handler) isBPAProductionReviewContinuation(r *http.Request, issue db.Issue, actorType, actorID string) bool {
	if r.Header.Get("X-Actor-Source") != "task_token" || actorType != "agent" || !h.bpaActorIsCoordinator(r.Context(), issue, actorID) {
		return false
	}
	agentID, err := util.ParseUUID(actorID)
	if err != nil {
		return false
	}
	taskID, err := util.ParseUUID(r.Header.Get("X-Task-ID"))
	if err != nil {
		return false
	}
	task, err := h.Queries.GetAgentTask(r.Context(), taskID)
	if err != nil || task.Status != "running" || task.AgentID != agentID || !task.IssueID.Valid || task.IssueID != issue.ID || !task.TriggerCommentID.Valid {
		return false
	}
	comment, err := h.Queries.GetCommentInWorkspace(r.Context(), db.GetCommentInWorkspaceParams{
		ID: task.TriggerCommentID, WorkspaceID: issue.WorkspaceID,
	})
	if err != nil || comment.AuthorType != "member" || !comment.AuthorID.Valid {
		return false
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil || state.ReviewRequestedAt == "" {
		return false
	}
	reviewRequestedAt, err := time.Parse(time.RFC3339Nano, state.ReviewRequestedAt)
	if err != nil {
		return false
	}
	if comment.CreatedAt.Time.Before(reviewRequestedAt) {
		return false
	}
	if state.ReviewCommentID == "" || state.ReviewCommentID != uuidToString(comment.ID) {
		return false
	}
	member, err := h.Queries.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{
		UserID: comment.AuthorID, WorkspaceID: issue.WorkspaceID,
	})
	return err == nil && roleAllowed(member.Role, "owner", "admin")
}

// bpaRootHasOpenChildren prevents closing a BPA root before its explicit child
// tasks are complete. The generic Multica board remains unchanged; this guard
// only applies when a workflow template is enabled.
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

// validateBPACompletion is the one completion gate shared by direct edits,
// batch updates, and GitHub merge completion. A root agent owns its final
// result comment, so completion never depends on a rigid Lead-only template.
func (h *Handler) validateBPACompletion(ctx context.Context, issue db.Issue) error {
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil {
		return err
	}
	if !issue.ParentIssueID.Valid && issue.Status == "in_review" && state.Template == bpa.TemplateProduction &&
		state.ApprovalStatus == bpa.ApprovalPending {
		return fmt.Errorf("pending production review cannot be completed before the assigned agent interprets an owner or admin comment")
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

func isPendingBPAProductionReviewExit(issue db.Issue, targetStatus string) bool {
	if issue.ParentIssueID.Valid || issue.Status != "in_review" || targetStatus == "in_review" {
		return false
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	return err == nil && state.Template == bpa.TemplateProduction && state.ApprovalStatus == bpa.ApprovalPending
}

// queueBPAArchivist records server-owned knowledge markers for a BPA root.
// The detailed Archivist summary is deliberately on-demand: automatically
// dispatching an AI run made archival contend for the shared local directory
// and added no delivery value. This function therefore never creates a task.
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
	issue, _, ok := h.resolveBPAArchivist(ctx, issue, metadata)
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
