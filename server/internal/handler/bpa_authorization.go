package handler

import (
	"context"
	"fmt"

	"github.com/multica-ai/multica/server/internal/bpa"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// bpaRoot returns the workflow root for an issue and whether that root has an
// enabled BPA template. BPA authority always follows the root: children hold
// delivery details only and must not become independent routing authorities.
func (h *Handler) bpaRoot(ctx context.Context, issue db.Issue) (db.Issue, bool, error) {
	root := issue
	seen := map[string]struct{}{uuidToString(issue.ID): {}}
	for root.ParentIssueID.Valid {
		parentID := uuidToString(root.ParentIssueID)
		if _, duplicate := seen[parentID]; duplicate {
			return db.Issue{}, false, fmt.Errorf("BPA issue hierarchy contains a cycle")
		}
		seen[parentID] = struct{}{}
		parent, err := h.Queries.GetIssue(ctx, root.ParentIssueID)
		if err != nil {
			return db.Issue{}, false, fmt.Errorf("load BPA main issue: %w", err)
		}
		root = parent
	}
	state, err := bpa.ParseState(parseIssueMetadata(root.Metadata))
	if err != nil {
		return db.Issue{}, false, err
	}
	return root, state.Enabled(), nil
}

func (h *Handler) bpaActorIsCoordinator(ctx context.Context, root db.Issue, actorID string) bool {
	actorUUID, err := util.ParseUUID(actorID)
	if err != nil {
		return false
	}
	switch root.AssigneeType.String {
	case "agent":
		return root.AssigneeID.Valid && root.AssigneeID == actorUUID
	case "squad":
		if !root.AssigneeID.Valid {
			return false
		}
		squad, err := h.Queries.GetSquad(ctx, root.AssigneeID)
		return err == nil && squad.LeaderID == actorUUID
	default:
		return false
	}
}

// requireBPACoordination rejects agent-side routing mutations unless they come
// from the configured Lead. Human members retain normal board control.
func (h *Handler) requireBPACoordination(ctx context.Context, issue db.Issue, actorType, actorID string) error {
	if actorType != "agent" {
		return nil
	}
	root, enabled, err := h.bpaRoot(ctx, issue)
	if err != nil || !enabled {
		return err
	}
	if isConfiguredBPAArchivist(root, actorID) {
		return fmt.Errorf("the configured BPA Archivist is read-only")
	}
	if !h.bpaActorIsCoordinator(ctx, root, actorID) {
		return fmt.Errorf("only the BPA Team Lead can route or close this workflow task")
	}
	return nil
}

// requireBPAIssueWrite prevents the Archivist from mutating any BPA issue.
// Other specialists may still update their own delivery issue unless the
// caller explicitly requests a coordination operation.
func (h *Handler) requireBPAIssueWrite(ctx context.Context, issue db.Issue, actorType, actorID string, coordination bool) error {
	if actorType != "agent" {
		return nil
	}
	root, enabled, err := h.bpaRoot(ctx, issue)
	if err != nil || !enabled {
		return err
	}
	if isConfiguredBPAArchivist(root, actorID) {
		return fmt.Errorf("the configured BPA Archivist is read-only")
	}
	if coordination && !h.bpaActorIsCoordinator(ctx, root, actorID) {
		return fmt.Errorf("only the BPA Team Lead can route or close this workflow task")
	}
	return nil
}

func (h *Handler) isConfiguredBPAArchivist(ctx context.Context, issue db.Issue, agentID string) bool {
	root, enabled, err := h.bpaRoot(ctx, issue)
	return err == nil && enabled && isConfiguredBPAArchivist(root, agentID)
}

// isBPAArchivistAgent is intentionally workspace-wide: Archivist is a
// read-only role, not merely a role with restricted access to one root issue.
func (h *Handler) isBPAArchivistAgent(ctx context.Context, agentID string) bool {
	id, err := util.ParseUUID(agentID)
	if err != nil {
		return false
	}
	agent, err := h.Queries.GetAgent(ctx, id)
	return err == nil && agent.Name == "AT Archivist"
}
