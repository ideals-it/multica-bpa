package handler

import (
	"strings"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var bpaFinalSummaryHeadings = []string{
	"**Що було не так:**",
	"**Що змінили:**",
	"**Що перевірили:**",
	"**Результат:**",
	"**Ризик / наступне:**",
}

func hasBPAFinalRootSummary(root db.Issue, comments []db.Comment) bool {
	if root.AssigneeType.String != "agent" || !root.AssigneeID.Valid {
		return false
	}
	for _, comment := range comments {
		if comment.AuthorType != "agent" || comment.AuthorID != root.AssigneeID {
			continue
		}
		if allBPAFinalSummaryHeadingsPresent(comment.Content) {
			return true
		}
	}
	return false
}

func allBPAFinalSummaryHeadingsPresent(content string) bool {
	for _, heading := range bpaFinalSummaryHeadings {
		if !strings.Contains(content, heading) {
			return false
		}
	}
	return true
}
