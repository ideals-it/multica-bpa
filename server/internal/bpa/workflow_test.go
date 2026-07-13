package bpa

import (
	"errors"
	"testing"
)

func TestValidateTemplateStartRequiresAgentLeadOnRootIssue(t *testing.T) {
	err := ValidateTemplateStart(TemplateStandard, IssueRef{AssigneeType: "member"})
	if !errors.Is(err, ErrLeadMustOwnMainIssue) {
		t.Fatalf("expected ErrLeadMustOwnMainIssue, got %v", err)
	}
}

func TestValidateTemplateStartRejectsChildIssue(t *testing.T) {
	err := ValidateTemplateStart(TemplateProduction, IssueRef{ParentID: "parent", AssigneeType: "agent"})
	if !errors.Is(err, ErrWorkflowMustStartOnMainIssue) {
		t.Fatalf("expected ErrWorkflowMustStartOnMainIssue, got %v", err)
	}
}

func TestParseStateTreatsMissingMetadataAsDisabled(t *testing.T) {
	state, err := ParseState(nil)
	if err != nil {
		t.Fatalf("ParseState() error = %v", err)
	}
	if state.Template != "" || state.Enabled() {
		t.Fatalf("expected disabled state, got %#v", state)
	}
}

func TestParseStateRejectsMalformedProductionAction(t *testing.T) {
	_, err := ParseState(map[string]any{"bpa.production_action": "true"})
	if !errors.Is(err, ErrMalformedState) {
		t.Fatalf("expected ErrMalformedState, got %v", err)
	}
}

func TestPlanFingerprintIsDeterministic(t *testing.T) {
	if got, want := PlanFingerprint("deploy revision A"), PlanFingerprint("deploy revision A"); got != want {
		t.Fatalf("same plan fingerprints differ: %q != %q", got, want)
	}
	if PlanFingerprint("deploy revision A") == PlanFingerprint("deploy revision B") {
		t.Fatal("different plans share a fingerprint")
	}
}

func TestCanDispatchRejectsProductionActionWithoutMatchingHumanApproval(t *testing.T) {
	state := State{
		Template:            TemplateProduction,
		ProductionAction:    true,
		PlanFingerprint:     "new-plan",
		ApprovalFingerprint: "old-plan",
		ApprovalStatus:      ApprovalApproved,
	}
	if decision := CanDispatch(state); decision.Allowed || !errors.Is(decision.Err, ErrHumanApprovalRequired) {
		t.Fatalf("expected approval denial, got %#v", decision)
	}
}

func TestCanDispatchAllowsMatchingTicketScopeApproval(t *testing.T) {
	state := State{
		Template:                 TemplateProduction,
		ScopeFingerprint:         "scope-a",
		ApprovedScopeFingerprint: "scope-a",
		ApprovalStatus:           ApprovalApproved,
	}
	if decision := CanDispatch(state); !decision.Allowed || decision.Err != nil {
		t.Fatalf("expected allowed dispatch, got %#v", decision)
	}
}

func TestCanDispatchRejectsLegacyPlanApprovalWithoutTicketScope(t *testing.T) {
	state := State{
		Template:            TemplateProduction,
		ProductionAction:    true,
		PlanFingerprint:     "plan-a",
		ApprovalFingerprint: "plan-a",
		ApprovalStatus:      ApprovalApproved,
	}
	if decision := CanDispatch(state); decision.Allowed || !errors.Is(decision.Err, ErrHumanApprovalRequired) {
		t.Fatalf("legacy plan approval must not dispatch production work, got %#v", decision)
	}
}

func TestTicketScopeFingerprintChangesWhenTicketScopeChanges(t *testing.T) {
	first := TicketScopeFingerprint("Deploy service", "Deploy revision A")
	changed := TicketScopeFingerprint("Deploy service", "Deploy revision B")
	if first == changed {
		t.Fatal("ticket scope fingerprint did not change")
	}
}

func TestCanDispatchRejectsProductionTicketWithoutApprovedScope(t *testing.T) {
	state := State{
		Template:                 TemplateProduction,
		ScopeFingerprint:         "scope-a",
		ApprovedScopeFingerprint: "scope-b",
		ApprovalStatus:           ApprovalApproved,
	}
	if decision := CanDispatch(state); decision.Allowed || !errors.Is(decision.Err, ErrHumanApprovalRequired) {
		t.Fatalf("expected scope approval denial, got %#v", decision)
	}
}

func TestHasOpenChildrenRecognizesOnlyTerminalChildStatuses(t *testing.T) {
	if HasOpenChildren([]string{"done", "cancelled"}) {
		t.Fatal("terminal children must not keep a BPA root open")
	}
	if !HasOpenChildren([]string{"done", "in_review"}) {
		t.Fatal("an in-review child must keep a BPA root open")
	}
}

func TestHasCommitEvidenceAcceptsCommitOrExplicitNoChange(t *testing.T) {
	if HasCommitEvidence(map[string]any{}) {
		t.Fatal("empty metadata must not satisfy the commit gate")
	}
	if !HasCommitEvidence(map[string]any{"bpa.commit_sha": "abc123"}) {
		t.Fatal("commit SHA must satisfy the commit gate")
	}
	if !HasCommitEvidence(map[string]any{"bpa.no_repo_changes": "read-only investigation"}) {
		t.Fatal("explicit no-change reason must satisfy the commit gate")
	}
}

func TestCanWriteArchivistMetadataAllowsOnlyConfiguredArchivistArchiveKeys(t *testing.T) {
	if !CanWriteArchivistMetadata("bpa.archive_summary", "agent-1", "agent-1") {
		t.Fatal("configured Archivist must be able to write archive summary")
	}
	if CanWriteArchivistMetadata("bpa.archive_summary", "agent-2", "agent-1") {
		t.Fatal("another agent must not write Archivist metadata")
	}
	if CanWriteArchivistMetadata("bpa.knowledge_status", "agent-1", "agent-1") {
		t.Fatal("Archivist must not write server-owned knowledge metadata")
	}
	if CanWriteArchivistMetadata("title", "agent-1", "agent-1") {
		t.Fatal("Archivist guard must not authorize unrelated keys")
	}
}
