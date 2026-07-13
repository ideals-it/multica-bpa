package service

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/bpa"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestCanEnqueueIssueRejectsUnapprovedProductionAction(t *testing.T) {
	service := &TaskService{}
	issue := db.Issue{Metadata: []byte(`{
		"bpa.template":"production",
		"bpa.production_action":true,
		"bpa.plan_fingerprint":"plan-a"
	}`)}

	if err := service.CanEnqueueIssue(issue); !errors.Is(err, bpa.ErrHumanApprovalRequired) {
		t.Fatalf("expected approval error, got %v", err)
	}
}

func TestCanEnqueueIssueAllowsApprovedProductionAction(t *testing.T) {
	service := &TaskService{}
	issue := db.Issue{Metadata: []byte(`{
		"bpa.template":"production",
		"bpa.production_action":true,
		"bpa.plan_fingerprint":"plan-a",
		"bpa.approval_fingerprint":"plan-a",
		"bpa.approval_status":"approved"
	}`), ID: pgtype.UUID{Valid: true}}

	if err := service.CanEnqueueIssue(issue); err != nil {
		t.Fatalf("expected allowed dispatch, got %v", err)
	}
}

func TestCanEnqueueIssueAllowsPreparationAndStandardWork(t *testing.T) {
	service := &TaskService{}
	for _, metadata := range [][]byte{
		nil,
		[]byte(`{"bpa.template":"standard"}`),
		[]byte(`{"bpa.template":"production","bpa.production_action":false}`),
	} {
		if err := service.CanEnqueueIssue(db.Issue{Metadata: metadata}); err != nil {
			t.Fatalf("expected allowed dispatch for %s, got %v", metadata, err)
		}
	}
}

func TestCanEnqueueIssueFailsClosedForMalformedBPAState(t *testing.T) {
	service := &TaskService{}
	issue := db.Issue{Metadata: []byte(`{"bpa.production_action":"true"}`)}

	if err := service.CanEnqueueIssue(issue); !errors.Is(err, bpa.ErrHumanApprovalRequired) {
		t.Fatalf("expected fail-closed approval error, got %v", err)
	}
}
