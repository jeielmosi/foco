package files

import (
	"os"
	"path/filepath"
	"testing"
)

func writeIgnoreFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIgnoreMatcher(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n!keep.log\nbuild/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewIgnoreMatcher(root, false, []string{"**/*.tmp"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		rel    string
		dir    bool
		ignore bool
	}{
		{"a.log", false, true},
		{"keep.log", false, false},
		{"build", true, true},
		{"src/a.tmp", false, true},
		{"src/a.go", false, false},
	}
	for _, tc := range cases {
		if got := m.Ignored(tc.rel, tc.dir); got != tc.ignore {
			t.Errorf("%s: got %v, want %v", tc.rel, got, tc.ignore)
		}
	}
}

func TestIgnoreMatcherDisabled(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n"), 0o644)
	m, err := NewIgnoreMatcher(root, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Ignored("a.log", false) {
		t.Fatal("no-ignore must disable .gitignore")
	}
}

func TestIgnoreMatcherNestedCascade(t *testing.T) {
	root := t.TempDir()
	writeIgnoreFile(t, root, ".gitignore", "build/\n")
	m, err := NewIgnoreMatcher(root, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Ignored("build", true) {
		t.Fatal("build directory should be ignored")
	}
	if !m.Ignored("build/a.go", false) {
		t.Fatal("file below ignored directory should be ignored")
	}
	if m.Ignored("src/a.go", false) {
		t.Fatal("unrelated file should not be ignored")
	}
}

func TestIgnoreMatcherCRLF(t *testing.T) {
	root := t.TempDir()
	writeIgnoreFile(t, root, ".gitignore", "*.log\r\n!keep.log\r\n")
	m, err := NewIgnoreMatcher(root, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Ignored("a.log", false) {
		t.Fatal("CRLF rule should still ignore a.log")
	}
	if m.Ignored("keep.log", false) {
		t.Fatal("CRLF negation should still unignore keep.log")
	}
}

func TestGitignoreAnchoredPattern(t *testing.T) {
	anchoredRoot := t.TempDir()
	writeIgnoreFile(t, anchoredRoot, ".gitignore", "/build\n")
	anchored, err := NewIgnoreMatcher(anchoredRoot, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !anchored.Ignored("build", false) {
		t.Fatal("anchored /build should match root build")
	}
	if anchored.Ignored("a/build", false) {
		t.Fatal("anchored /build must not match a/build")
	}

	unanchoredRoot := t.TempDir()
	writeIgnoreFile(t, unanchoredRoot, ".gitignore", "build\n")
	unanchored, err := NewIgnoreMatcher(unanchoredRoot, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !unanchored.Ignored("a/build", false) {
		t.Fatal("unanchored build should match a/build")
	}
}

func TestExcludedByPatternAnchored(t *testing.T) {
	root := t.TempDir()
	anchored, err := NewIgnoreMatcher(root, false, []string{"/build"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		rel    string
		dir    bool
		ignore bool
	}{
		{"build", false, true},
		{"build", true, true},
		{"a/build", false, false},
	}
	for _, tc := range cases {
		if got := anchored.Ignored(tc.rel, tc.dir); got != tc.ignore {
			t.Errorf("anchored /build %s (dir=%v): got %v, want %v", tc.rel, tc.dir, got, tc.ignore)
		}
	}

	unanchored, err := NewIgnoreMatcher(root, false, []string{"build"})
	if err != nil {
		t.Fatal(err)
	}
	if !unanchored.Ignored("a/build", false) {
		t.Fatal("unanchored exclude build should match a/build")
	}
}

func TestIgnoreMatcherSubdirectoryScope(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeIgnoreFile(t, root, "sub/.gitignore", "*.log\n!keep.log\n")
	m, err := NewIgnoreMatcher(root, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Ignored("sub/a.log", false) {
		t.Fatal("sub/.gitignore *.log should ignore sub/a.log")
	}
	if m.Ignored("sub/a.go", false) {
		t.Fatal("sub/.gitignore *.log must not ignore sub/a.go")
	}
	if m.Ignored("sub/keep.log", false) {
		t.Fatal("nested negation !keep.log should unignore sub/keep.log")
	}
	if m.Ignored("other/a.log", false) {
		t.Fatal("sub/.gitignore *.log must not ignore other/a.log")
	}
	if m.Ignored("a.log", false) {
		t.Fatal("sub/.gitignore *.log must not ignore root a.log")
	}
}

func TestIgnoreMatcherNoIgnoreHonorsExcludes(t *testing.T) {
	root := t.TempDir()
	writeIgnoreFile(t, root, ".gitignore", "*.log\n")
	m, err := NewIgnoreMatcher(root, true, []string{"**/*.tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Ignored("a.tmp", false) {
		t.Fatal("--exclude pattern must still apply with no-ignore")
	}
	if !m.Ignored("src/a.tmp", false) {
		t.Fatal("--exclude pattern must still apply to nested paths with no-ignore")
	}
	if m.Ignored("a.log", false) {
		t.Fatal("no-ignore must disable .gitignore")
	}
}
