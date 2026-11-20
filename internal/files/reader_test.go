package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"foco/internal/enums"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestEligibilityReasons(t *testing.T) {
	root := writeTree(t, map[string]string{
		"main.go":           "package main\n",
		"server.pem":        "x\n",
		".env":              "A=1\n",
		"node_modules/a.js": "x\n",
		"build/out.txt":     "x\n",
		".hidden.txt":       "x\n",
	})
	reader, err := New(root, DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	cases := map[string]enums.FileReason{
		"main.go":           enums.UnknownFileReason,
		"server.pem":        enums.FileReasonSensitiveName,
		".env":              enums.FileReasonHidden,
		"node_modules/a.js": enums.FileReasonDependency,
		"build/out.txt":     enums.FileReasonDependency,
		".hidden.txt":       enums.FileReasonHidden,
		"../outside":        enums.FileReasonOutsideRoot,
	}
	for rel, want := range cases {
		decision, err := reader.Eligibility(context.Background(), filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if decision.Reason != want {
			t.Errorf("%s: got %q, want %q", rel, decision.Reason, want)
		}
	}
}

func TestListPage(t *testing.T) {
	root := writeTree(t, map[string]string{"a.go": "x", "b.go": "x", "c.go": "x"})
	reader, err := New(root, DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	page, err := reader.ListPage(context.Background(), root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 3 || page.NextOffset != 0 {
		t.Fatalf("page: %+v", page)
	}
}

func TestIgnoreAndExclude(t *testing.T) {
	root := writeTree(t, map[string]string{
		".gitignore": "*.log\n",
		"a.log":      "x",
		"b.go":       "x",
		"c.tmp":      "x",
	})
	policy := DefaultPolicy()
	policy.Exclude = []string{"**/*.tmp"}
	reader, err := New(root, policy, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for rel, want := range map[string]enums.FileReason{
		"a.log": enums.FileReasonIgnored, "b.go": enums.UnknownFileReason, "c.tmp": enums.FileReasonExcludePattern,
	} {
		decision, err := reader.Eligibility(context.Background(), filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if decision.Reason != want {
			t.Errorf("%s: got %q, want %q", rel, decision.Reason, want)
		}
	}
}

func TestEligibilityEligibleFile(t *testing.T) {
	root := writeTree(t, map[string]string{"main.go": "package main\n"})
	reader, err := New(root, DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decision, err := reader.Eligibility(context.Background(), filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Eligible || decision.Reason != enums.UnknownFileReason {
		t.Fatalf("decision: %+v", decision)
	}
}

func TestEligibilityTypedCategoryFilters(t *testing.T) {
	root := writeTree(t, map[string]string{
		"main.go":   "package main\n",
		"README.md": "docs\n",
		"data.json": "{}\n",
	})
	policy := DefaultPolicy()
	policy.Only = map[enums.FileCategory]struct{}{
		enums.FileCategoryCode: {},
		enums.FileCategoryDocs: {},
	}
	policy.ExcludeTypes = map[enums.FileCategory]struct{}{
		enums.FileCategoryDocs: {},
	}
	reader, err := New(root, policy, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	for rel, want := range map[string]enums.FileReason{
		"main.go":   enums.UnknownFileReason,
		"README.md": enums.FileReasonFileTypeDocs,
		"data.json": enums.FileReasonFileTypeData,
	} {
		decision, err := reader.Eligibility(context.Background(), filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if decision.Reason != want {
			t.Errorf("%s: got %q, want %q", rel, decision.Reason, want)
		}
	}
}

func TestLookupFile(t *testing.T) {
	root := writeTree(t, map[string]string{"file.txt": "x"})
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	reader, err := New(root, DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	cases := []struct {
		name       string
		path       string
		wantStatus enums.LookupStatus
		wantReason enums.FileReason
	}{
		{"file", filepath.Join(root, "file.txt"), enums.LookupStatusFile, enums.UnknownFileReason},
		{"missing", filepath.Join(root, "missing.txt"), enums.LookupStatusExcluded, enums.FileReasonMissing},
		{"directory", filepath.Join(root, "sub"), enums.LookupStatusExcluded, enums.FileReasonNotFile},
	}
	for _, tc := range cases {
		result, err := reader.LookupFile(context.Background(), tc.path)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if result.Status != tc.wantStatus || result.Reason != tc.wantReason {
			t.Errorf("%s: got %+v, want status %q reason %q", tc.name, result, tc.wantStatus, tc.wantReason)
		}
	}
}

func TestListPagePaging(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 130; i++ {
		files[fmt.Sprintf("f%03d.txt", i)] = "x"
	}
	root := writeTree(t, files)
	reader, err := New(root, DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	first, err := reader.ListPage(context.Background(), root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 128 || first.NextOffset != 128 {
		t.Fatalf("first page: %d entries, next %d", len(first.Entries), first.NextOffset)
	}
	if first.Entries[0].Name != "f000.txt" || first.Entries[127].Name != "f127.txt" {
		t.Fatalf("first page ordering: %q .. %q", first.Entries[0].Name, first.Entries[127].Name)
	}
	second, err := reader.ListPage(context.Background(), root, first.NextOffset)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 2 || second.NextOffset != 0 {
		t.Fatalf("second page: %d entries, next %d", len(second.Entries), second.NextOffset)
	}
	if second.Entries[0].Name != "f128.txt" || second.Entries[1].Name != "f129.txt" {
		t.Fatalf("second page ordering: %q, %q", second.Entries[0].Name, second.Entries[1].Name)
	}
}

func TestNewRejectsFileRoot(t *testing.T) {
	root := writeTree(t, map[string]string{"a.go": "x"})
	if _, err := New(filepath.Join(root, "a.go"), DefaultPolicy(), nil, nil); err == nil {
		t.Fatal("expected error for regular-file root")
	}
}

func TestSymlinkedRoot(t *testing.T) {
	root := writeTree(t, map[string]string{"main.go": "package main\n"})
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	reader, err := New(link, DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decision, err := reader.Eligibility(context.Background(), filepath.Join(link, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Eligible || decision.Reason != enums.UnknownFileReason {
		t.Fatalf("decision through symlinked root: %+v", decision)
	}
	page, err := reader.ListPage(context.Background(), link, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Name != "main.go" {
		t.Fatalf("page through symlinked root: %+v", page)
	}
}

func TestSymlinkedFileStillDetected(t *testing.T) {
	root := writeTree(t, map[string]string{"real.txt": "x"})
	outside := writeTree(t, map[string]string{"target.txt": "x"})
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(filepath.Join(outside, "target.txt"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	reader, err := New(root, DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decision, err := reader.Eligibility(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Reason != enums.FileReasonSymlink {
		t.Fatalf("got %q, want %q", decision.Reason, enums.FileReasonSymlink)
	}
}

type fakeMatcher map[string]bool

func (m fakeMatcher) Match(path string) bool { return m[path] }

func TestSensitivePathMatcher(t *testing.T) {
	root := writeTree(t, map[string]string{"notes.txt": "x\n", "terraform.tfstate": "x\n"})
	policy := DefaultPolicy()
	policy.SensitivePaths = fakeMatcher{"notes.txt": true}
	reader, err := New(root, policy, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	decision, err := reader.Eligibility(context.Background(), filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Reason != enums.FileReasonSensitiveName {
		t.Fatalf("notes.txt: got %q, want %q", decision.Reason, enums.FileReasonSensitiveName)
	}

	decision, err = reader.Eligibility(context.Background(), filepath.Join(root, "terraform.tfstate"))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Eligible || decision.Reason != enums.UnknownFileReason {
		t.Fatalf("terraform.tfstate: got %+v, want eligible", decision)
	}

	include := DefaultPolicy()
	include.IncludeSensitive = true
	include.SensitivePaths = fakeMatcher{"notes.txt": true}
	reader2, err := New(root, include, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader2.Close()
	decision, err = reader2.Eligibility(context.Background(), filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Eligible || decision.Reason != enums.UnknownFileReason {
		t.Fatalf("notes.txt with --include-sensitive: got %+v, want eligible", decision)
	}
}

func TestFileTypeReason(t *testing.T) {
	cases := []struct {
		category enums.FileCategory
		want     enums.FileReason
	}{
		{enums.FileCategoryCode, enums.FileReasonFileTypeCode},
		{enums.FileCategoryConfig, enums.FileReasonFileTypeConfig},
		{enums.FileCategoryDocs, enums.FileReasonFileTypeDocs},
		{enums.FileCategoryData, enums.FileReasonFileTypeData},
		{enums.FileCategoryAudio, enums.FileReasonFileTypeAudio},
		{enums.FileCategoryVideo, enums.FileReasonFileTypeVideo},
		{enums.FileCategoryArchives, enums.FileReasonFileTypeArchives},
		{enums.FileCategoryLogs, enums.FileReasonFileTypeLogs},
		{enums.FileCategoryTemp, enums.FileReasonFileTypeTemp},
		{enums.FileCategoryUnknown, enums.FileReasonFileTypeUnknown},
	}
	for _, test := range cases {
		got := fileTypeReason(test.category)
		if got != test.want {
			t.Errorf("fileTypeReason(%q) = %q, want %q", test.category, got, test.want)
		}
		if got.String() != string(test.want) {
			t.Errorf("fileTypeReason(%q).String() = %q, want %q", test.category, got.String(), test.want)
		}
	}
	if got := fileTypeReason(enums.FileCategory("invalid")); got != enums.UnknownFileReason {
		t.Errorf("fileTypeReason(invalid) = %q, want unknown reason", got)
	}
}
