package files

import (
	"context"
	"testing"

	"foco/internal/enums"
)

func TestListPageReportsResourceLimitWhenOpenDirectoryBudgetIsFull(t *testing.T) {
	root := writeTree(t, map[string]string{"a.txt": "x"})
	reader, err := NewWithLimits(root, DefaultPolicy(), nil, nil, Limits{
		MaxOpenDirectories: 1, MaxReusableBytes: 1 << 20, MaxIgnoreBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	reader.openDirs <- struct{}{}
	page, err := reader.ListPage(context.Background(), root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Issue != enums.FileIssueResourceLimit {
		t.Fatalf("issue = %q, want %q", page.Issue, enums.FileIssueResourceLimit)
	}
	if len(page.Entries) != 0 {
		t.Fatalf("entries = %d, want 0", len(page.Entries))
	}
}
