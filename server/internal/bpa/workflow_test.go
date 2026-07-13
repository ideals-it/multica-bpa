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

func TestCanDispatchAllowsMatchingHumanApproval(t *testing.T) {
	state := State{
		Template:            TemplateProduction,
		ProductionAction:    true,
		PlanFingerprint:     "plan-a",
		ApprovalFingerprint: "plan-a",
		ApprovalStatus:      ApprovalApproved,
	}
	if decision := CanDispatch(state); !decision.Allowed || decision.Err != nil {
		t.Fatalf("expected allowed dispatch, got %#v", decision)
	}
}
