package service

import (
	"context"
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

	if err := service.CanEnqueueIssue(context.Background(), issue, pgtype.UUID{}); !errors.Is(err, bpa.ErrHumanApprovalRequired) {
		t.Fatalf("expected approval error, got %v", err)
	}
}

func TestCanEnqueueIssueAllowsApprovedProductionTicketScope(t *testing.T) {
	service := &TaskService{}
	issue := db.Issue{Metadata: []byte(`{
		"bpa.template":"production",
		"bpa.scope_fingerprint":"scope-a",
		"bpa.approved_scope_fingerprint":"scope-a",
		"bpa.approval_status":"approved"
	}`), ID: pgtype.UUID{Valid: true}}

	if err := service.CanEnqueueIssue(context.Background(), issue, pgtype.UUID{}); err != nil {
		t.Fatalf("expected allowed dispatch, got %v", err)
	}
}

func TestCanEnqueueIssueAllowsPreparationAndStandardWork(t *testing.T) {
	service := &TaskService{}
	for _, metadata := range [][]byte{
		nil,
		[]byte(`{"bpa.template":"standard"}`),
	} {
		if err := service.CanEnqueueIssue(context.Background(), db.Issue{Metadata: metadata}, pgtype.UUID{}); err != nil {
			t.Fatalf("expected allowed dispatch for %s, got %v", metadata, err)
		}
	}
}

func TestCanEnqueueIssueRejectsProductionChildBeforeScopedApproval(t *testing.T) {
	service := &TaskService{}
	issue := db.Issue{
		ParentIssueID: pgtype.UUID{Valid: true},
		Metadata:      []byte(`{"bpa.template":"production"}`),
	}
	if err := service.CanEnqueueIssue(context.Background(), issue, pgtype.UUID{}); !errors.Is(err, bpa.ErrHumanApprovalRequired) {
		t.Fatalf("production child must wait for ticket-scope approval, got %v", err)
	}
}

func TestCanEnqueueIssueFailsClosedForMalformedBPAState(t *testing.T) {
	service := &TaskService{}
	issue := db.Issue{Metadata: []byte(`{"bpa.production_action":"true"}`)}

	if err := service.CanEnqueueIssue(context.Background(), issue, pgtype.UUID{}); !errors.Is(err, bpa.ErrHumanApprovalRequired) {
		t.Fatalf("expected fail-closed approval error, got %v", err)
	}
}

func TestCanEnqueueIssueRejectsUnpreparedProductionRootInReview(t *testing.T) {
	service := &TaskService{}
	issue := db.Issue{Status: "in_review", Metadata: []byte(`{
		"bpa.template":"production",
		"bpa.waiting_for":"lead"
	}`)}

	if err := service.CanEnqueueIssue(context.Background(), issue, pgtype.UUID{}); !errors.Is(err, bpa.ErrHumanApprovalRequired) {
		t.Fatalf("production root in review must not use the preparation exception, got %v", err)
	}
}

func TestCanEnqueueIssueAllowsOnlyRootLeadDuringProductionPreparation(t *testing.T) {
	leadID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	otherID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	service := &TaskService{}
	issue := db.Issue{
		AssigneeType: pgtype.Text{String: "agent", Valid: true},
		AssigneeID:   leadID,
		Metadata:     []byte(`{"bpa.template":"production","bpa.waiting_for":"lead"}`),
	}
	if err := service.CanEnqueueIssue(context.Background(), issue, leadID); err != nil {
		t.Fatalf("root Lead must be allowed to prepare production plan: %v", err)
	}
	if err := service.CanEnqueueIssue(context.Background(), issue, otherID); !errors.Is(err, bpa.ErrHumanApprovalRequired) {
		t.Fatalf("specialist must not bypass approval during preparation, got %v", err)
	}
}
