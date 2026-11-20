package cli

import (
	"foco/internal/enums"
	"foco/internal/retrieve"
)

func exitCodeForResult(result retrieve.RetrievalResult) int {
	if result.Status == enums.RetrievalStatusInterrupted {
		return 130
	}
	if result.Status == enums.RetrievalStatusIncomplete {
		fatal, incomplete := false, false
		for _, issue := range result.Issues {
			if fatalExitIssue(issue.Kind) {
				fatal = true
				continue
			}
			if issue.Kind != enums.IssueCodeResourceLimit {
				incomplete = true
			}
		}
		if fatal {
			return 1
		}
		if incomplete {
			return 2
		}
	}
	return 0
}

func fatalExitIssue(kind enums.RetrievalIssueCode) bool {
	switch kind {
	case enums.IssueCodeAuthentication, enums.IssueCodeRequestLimit, enums.IssueCodeCancelled, enums.IssueCodeInterrupted:
		return true
	}
	return false
}
