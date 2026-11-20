package cli

import (
	"testing"

	"foco/internal/enums"
	"foco/internal/retrieve"
)

func TestExitCodeForResult(t *testing.T) {
	tests := []struct {
		name   string
		status enums.RetrievalStatus
		issues []retrieve.Issue
		want   int
	}{
		{name: "complete", status: enums.RetrievalStatusComplete, want: 0},
		{name: "resource_limit only", status: enums.RetrievalStatusIncomplete, issues: []retrieve.Issue{{Kind: enums.IssueCodeResourceLimit, Count: 2}}, want: 0},
		{name: "changed", status: enums.RetrievalStatusIncomplete, issues: []retrieve.Issue{{Kind: enums.IssueCodeChanged, Count: 1}}, want: 2},
		{name: "unreadable", status: enums.RetrievalStatusIncomplete, issues: []retrieve.Issue{{Kind: enums.IssueCodeUnreadable, Count: 1}}, want: 2},
		{name: "provider", status: enums.RetrievalStatusIncomplete, issues: []retrieve.Issue{{Kind: enums.IssueCodeProvider, Count: 1}}, want: 2},
		{name: "resource_limit and changed", status: enums.RetrievalStatusIncomplete, issues: []retrieve.Issue{{Kind: enums.IssueCodeResourceLimit, Count: 1}, {Kind: enums.IssueCodeChanged, Count: 1}}, want: 2},
		{name: "authentication", status: enums.RetrievalStatusIncomplete, issues: []retrieve.Issue{{Kind: enums.IssueCodeAuthentication, Count: 1}}, want: 1},
		{name: "request-limit", status: enums.RetrievalStatusIncomplete, issues: []retrieve.Issue{{Kind: enums.IssueCodeRequestLimit, Count: 1}}, want: 1},
		{name: "cancelled", status: enums.RetrievalStatusIncomplete, issues: []retrieve.Issue{{Kind: enums.IssueCodeCancelled, Count: 1}}, want: 1},
		{name: "fatal with changed", status: enums.RetrievalStatusIncomplete, issues: []retrieve.Issue{{Kind: enums.IssueCodeChanged, Count: 1}, {Kind: enums.IssueCodeAuthentication, Count: 1}}, want: 1},
		{name: "interrupted status", status: enums.RetrievalStatusInterrupted, issues: []retrieve.Issue{{Kind: enums.IssueCodeAuthentication, Count: 1}}, want: 130},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := exitCodeForResult(retrieve.RetrievalResult{Status: test.status, Issues: test.issues}); got != test.want {
				t.Fatalf("exit code = %d, want %d", got, test.want)
			}
		})
	}
}
