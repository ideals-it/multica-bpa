package handler

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestHasBPAFinalRootSummary(t *testing.T) {
	leadID := parseUUID("11111111-1111-4111-8111-111111111111")
	root := db.Issue{
		AssigneeType: pgtype.Text{String: "agent", Valid: true},
		AssigneeID:   leadID,
	}
	valid := "**Що було не так:** transient Google API error became 500\n\n" +
		"**Що змінили:** return 503 after retries\n\n" +
		"**Що перевірили:** 14 tests passed\n\n" +
		"**Результат:** deploy is ready\n\n" +
		"**Ризик / наступне:** немає відомого"

	tests := map[string]struct {
		comment db.Comment
		want    bool
	}{
		"lead summary qualifies": {
			comment: db.Comment{AuthorType: "agent", AuthorID: leadID, Content: valid},
			want:    true,
		},
		"missing heading fails": {
			comment: db.Comment{AuthorType: "agent", AuthorID: leadID, Content: strings.Replace(valid, "**Результат:**", "**Підсумок:**", 1)},
			want:    false,
		},
		"specialist cannot satisfy root summary": {
			comment: db.Comment{AuthorType: "agent", AuthorID: parseUUID("22222222-2222-4222-8222-222222222222"), Content: valid},
			want:    false,
		},
		"member cannot impersonate lead": {
			comment: db.Comment{AuthorType: "member", AuthorID: leadID, Content: valid},
			want:    false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := hasBPAFinalRootSummary(root, []db.Comment{tt.comment}); got != tt.want {
				t.Errorf("hasBPAFinalRootSummary() = %v, want %v", got, tt.want)
			}
		})
	}
}
