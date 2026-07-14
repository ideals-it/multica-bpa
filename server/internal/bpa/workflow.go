// Package bpa contains the fork-specific policy for a small set of native
// Multica workflow templates. It does not schedule or dispatch work itself.
package bpa

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	ErrLeadMustOwnMainIssue         = errors.New("an agent or squad lead must own the main issue")
	ErrWorkflowMustStartOnMainIssue = errors.New("a workflow can only start on a main issue")
	ErrMalformedState               = errors.New("malformed BPA workflow state")
	ErrHumanApprovalRequired        = errors.New("human approval is required for the current production plan")
)

type Template string

const (
	TemplateStandard      Template = "standard"
	TemplateProduction    Template = "production"
	TemplateInvestigation Template = "investigation"
)

type WaitingFor string

const (
	WaitingForLead          WaitingFor = "lead"
	WaitingForQuality       WaitingFor = "quality"
	WaitingForHumanApproval WaitingFor = "human_approval"
)

type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
)

type IssueRef struct {
	ParentID     string
	AssigneeType string
}

type State struct {
	Template                 Template
	WaitingFor               WaitingFor
	ProductionAction         bool
	PlanFingerprint          string
	ApprovalFingerprint      string
	ApprovalStatus           ApprovalStatus
	ApprovalSummary          string
	BlockerOwner             string
	BlockerAction            string
	ScopeFingerprint         string
	ApprovedScopeFingerprint string
	ReviewRequestedAt        string
	ReviewCommentID          string
}

func (s State) Enabled() bool {
	return s.Template != ""
}

type DispatchDecision struct {
	Allowed bool
	Reason  string
	Err     error
}

func ValidateTemplateStart(template Template, issue IssueRef) error {
	if !isKnownTemplate(template) {
		return fmt.Errorf("%w: unknown template %q", ErrMalformedState, template)
	}
	if issue.ParentID != "" {
		return ErrWorkflowMustStartOnMainIssue
	}
	if issue.AssigneeType != "agent" && issue.AssigneeType != "squad" {
		return ErrLeadMustOwnMainIssue
	}
	return nil
}

// PlanFingerprint binds approval to the exact plan text. Trimming avoids
// invalidating approval for accidental surrounding whitespace only.
func PlanFingerprint(plan string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(plan)))
	return fmt.Sprintf("sha256:%x", sum)
}

// TicketScopeFingerprint binds approval to what the ticket asks to execute,
// rather than to individual production commands.
func TicketScopeFingerprint(title, description string) string {
	return PlanFingerprint(title + "\n" + description)
}

// ParseState reads BPA's namespaced metadata keys only. Missing BPA metadata
// is a disabled workflow, so non-BPA issues keep their existing behavior.
func ParseState(metadata map[string]any) (State, error) {
	var state State
	if metadata == nil {
		return state, nil
	}

	var err error
	if state.Template, err = templateValue(metadata, "bpa.template"); err != nil {
		return State{}, err
	}
	if state.WaitingFor, err = waitingForValue(metadata, "bpa.waiting_for"); err != nil {
		return State{}, err
	}
	if state.ProductionAction, err = boolValue(metadata, "bpa.production_action"); err != nil {
		return State{}, err
	}
	if state.PlanFingerprint, err = stringValue(metadata, "bpa.plan_fingerprint"); err != nil {
		return State{}, err
	}
	if state.ApprovalFingerprint, err = stringValue(metadata, "bpa.approval_fingerprint"); err != nil {
		return State{}, err
	}
	if state.ApprovalStatus, err = approvalStatusValue(metadata, "bpa.approval_status"); err != nil {
		return State{}, err
	}
	if state.ApprovalSummary, err = stringValue(metadata, "bpa.approval_summary"); err != nil {
		return State{}, err
	}
	if state.BlockerOwner, err = stringValue(metadata, "bpa.blocker_owner"); err != nil {
		return State{}, err
	}
	if state.BlockerAction, err = stringValue(metadata, "bpa.blocker_action"); err != nil {
		return State{}, err
	}
	if state.ScopeFingerprint, err = stringValue(metadata, "bpa.scope_fingerprint"); err != nil {
		return State{}, err
	}
	if state.ApprovedScopeFingerprint, err = stringValue(metadata, "bpa.approved_scope_fingerprint"); err != nil {
		return State{}, err
	}
	if state.ReviewRequestedAt, err = stringValue(metadata, "bpa.review_requested_at"); err != nil {
		return State{}, err
	}
	if state.ReviewCommentID, err = stringValue(metadata, "bpa.review_comment_id"); err != nil {
		return State{}, err
	}
	return state, nil
}

// CanDispatch allows production execution only after native review records a
// human approval for the current ticket scope.
func CanDispatch(state State) DispatchDecision {
	if state.Template != TemplateProduction {
		return DispatchDecision{Allowed: true}
	}
	if state.ScopeFingerprint == "" {
		return DispatchDecision{Reason: "потрібне актуальне погодження людини для ticket scope", Err: ErrHumanApprovalRequired}
	}
	if state.ApprovalStatus == ApprovalApproved && state.ApprovedScopeFingerprint == state.ScopeFingerprint {
		return DispatchDecision{Allowed: true}
	}
	return DispatchDecision{Reason: "потрібне актуальне погодження людини для ticket scope", Err: ErrHumanApprovalRequired}
}

// HasOpenChildren reports whether a root still has a child that needs an
// explicit Lead decision. Only done and cancelled children are terminal.
func HasOpenChildren(statuses []string) bool {
	for _, status := range statuses {
		if status != "done" && status != "cancelled" {
			return true
		}
	}
	return false
}

// HasCommitEvidence accepts either a local commit made by the delivery agent
// or an explicit statement that its completed logical unit changed no repo.
func HasCommitEvidence(metadata map[string]any) bool {
	if sha, ok := metadata["bpa.commit_sha"].(string); ok && validCommitSHA(strings.TrimSpace(sha)) {
		return true
	}
	if reason, ok := metadata["bpa.no_repo_changes"].(string); ok && utf8.RuneCountInString(strings.TrimSpace(reason)) >= 12 {
		return true
	}
	return false
}

func validCommitSHA(sha string) bool {
	if len(sha) < 7 || len(sha) > 64 {
		return false
	}
	for _, char := range sha {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') && !(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

// CanWriteArchivistMetadata grants one configured agent a deliberately tiny
// write surface. Server-owned knowledge keys are never agent-writable.
func CanWriteArchivistMetadata(key, actorAgentID, archivistAgentID string) bool {
	if actorAgentID == "" || actorAgentID != archivistAgentID {
		return false
	}
	return key == "bpa.archive_summary" || key == "bpa.archive_updated_at"
}

func isKnownTemplate(template Template) bool {
	return template == TemplateStandard || template == TemplateProduction || template == TemplateInvestigation
}

func templateValue(metadata map[string]any, key string) (Template, error) {
	value, err := stringValue(metadata, key)
	if err != nil || value == "" {
		return Template(value), err
	}
	template := Template(value)
	if !isKnownTemplate(template) {
		return "", fmt.Errorf("%w: %s", ErrMalformedState, key)
	}
	return template, nil
}

func waitingForValue(metadata map[string]any, key string) (WaitingFor, error) {
	value, err := stringValue(metadata, key)
	if err != nil || value == "" {
		return WaitingFor(value), err
	}
	waitingFor := WaitingFor(value)
	if waitingFor != WaitingForLead && waitingFor != WaitingForQuality && waitingFor != WaitingForHumanApproval {
		return "", fmt.Errorf("%w: %s", ErrMalformedState, key)
	}
	return waitingFor, nil
}

func approvalStatusValue(metadata map[string]any, key string) (ApprovalStatus, error) {
	value, err := stringValue(metadata, key)
	if err != nil || value == "" {
		return ApprovalStatus(value), err
	}
	status := ApprovalStatus(value)
	if status != ApprovalPending && status != ApprovalApproved && status != ApprovalRejected {
		return "", fmt.Errorf("%w: %s", ErrMalformedState, key)
	}
	return status, nil
}

func stringValue(metadata map[string]any, key string) (string, error) {
	value, ok := metadata[key]
	if !ok || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s must be a string", ErrMalformedState, key)
	}
	return text, nil
}

func boolValue(metadata map[string]any, key string) (bool, error) {
	value, ok := metadata[key]
	if !ok || value == nil {
		return false, nil
	}
	flag, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%w: %s must be a boolean", ErrMalformedState, key)
	}
	return flag, nil
}
