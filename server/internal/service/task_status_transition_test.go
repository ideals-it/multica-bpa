package service

import "testing"

func TestShouldMoveQueuedIssueToInProgress(t *testing.T) {
	tests := map[string]struct {
		status string
		want   bool
	}{
		"todo work starts":          {status: "todo", want: true},
		"backlog work starts":       {status: "backlog", want: true},
		"active work stays active":  {status: "in_progress", want: false},
		"human review is preserved": {status: "in_review", want: false},
		"done is preserved":         {status: "done", want: false},
		"blocked is preserved":      {status: "blocked", want: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := shouldMoveQueuedIssueToInProgress(tt.status); got != tt.want {
				t.Errorf("shouldMoveQueuedIssueToInProgress(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
