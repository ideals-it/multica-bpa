package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/bpa"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type StartBPAWorkflowRequest struct {
	Template bpa.Template `json:"template"`
}

type ApprovalRequest struct {
	Plan             string `json:"plan"`
	Summary          string `json:"summary"`
	ProductionAction bool   `json:"production_action"`
}

type ApprovalDecisionRequest struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
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
	if _, ok := requireUserID(w, r); !ok {
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

func (h *Handler) RequestBPAApproval(w http.ResponseWriter, r *http.Request) {
	var req ApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !req.ProductionAction || strings.TrimSpace(req.Plan) == "" || strings.TrimSpace(req.Summary) == "" {
		writeError(w, http.StatusBadRequest, "production action, plan, and short summary are required")
		return
	}
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if _, ok := requireUserID(w, r); !ok {
		return
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil || state.Template != bpa.TemplateProduction {
		writeError(w, http.StatusBadRequest, "production BPA workflow is required")
		return
	}
	updated, err := h.setBPAWorkflowValues(r, issue, map[string]any{
		"bpa.production_action":    true,
		"bpa.plan_fingerprint":     bpa.PlanFingerprint(req.Plan),
		"bpa.approval_summary":     strings.TrimSpace(req.Summary),
		"bpa.approval_status":      string(bpa.ApprovalPending),
		"bpa.approval_fingerprint": "",
		"bpa.waiting_for":          string(bpa.WaitingForHumanApproval),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to request BPA approval")
		return
	}
	h.writeBPAWorkflow(w, updated)
}

func (h *Handler) DecideBPAApproval(w http.ResponseWriter, r *http.Request) {
	var req ApprovalDecisionRequest
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
	if actorType, _ := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID)); actorType != "member" {
		writeError(w, http.StatusForbidden, "only a human member can decide BPA approval")
		return
	}
	if req.Decision != string(bpa.ApprovalApproved) && req.Decision != string(bpa.ApprovalRejected) {
		writeError(w, http.StatusBadRequest, "decision must be approved or rejected")
		return
	}
	state, err := bpa.ParseState(parseIssueMetadata(issue.Metadata))
	if err != nil || state.Template != bpa.TemplateProduction || !state.ProductionAction || state.PlanFingerprint == "" {
		writeError(w, http.StatusBadRequest, "current production approval request is required")
		return
	}
	updated, err := h.setBPAWorkflowValues(r, issue, map[string]any{
		"bpa.approval_status":      req.Decision,
		"bpa.approval_fingerprint": state.PlanFingerprint,
		"bpa.waiting_for":          string(bpa.WaitingForLead),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save BPA approval decision")
		return
	}
	h.writeBPAWorkflow(w, updated)
}

func (h *Handler) setBPAWorkflowValues(r *http.Request, issue db.Issue, values map[string]any) (db.Issue, error) {
	updated := issue
	userID := requestUserID(r)
	workspaceID := uuidToString(issue.WorkspaceID)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			return db.Issue{}, err
		}
		updated, err = h.Queries.SetIssueMetadataKey(r.Context(), db.SetIssueMetadataKeyParams{
			ID: issue.ID, WorkspaceID: issue.WorkspaceID, Key: key, Value: raw,
		})
		if err != nil {
			return db.Issue{}, err
		}
		h.publish(protocol.EventIssueMetadataChanged, workspaceID, actorType, actorID, map[string]any{
			"issue_id": uuidToString(updated.ID), "metadata": parseIssueMetadata(updated.Metadata),
		})
	}
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
