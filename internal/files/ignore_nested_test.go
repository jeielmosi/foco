package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIgnoreMatcherDiscardsOuterGitScopeAtNestedRepository(t *testing.T) {
	root := t.TempDir()
	writeIgnoreFile(t, root, ".gitignore", "*.log\n")
	writeIgnoreFile(t, root, ".ignore", "*.secret\n")
	if err := os.MkdirAll(filepath.Join(root, "sub", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeIgnoreFile(t, root, "sub/.gitignore", "*.tmp\n")

	matcher, err := NewIgnoreMatcher(root, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if matcher.Ignored("sub/a.log", false) {
		t.Fatal("outer .gitignore must be discarded inside a nested repository")
	}
	if !matcher.Ignored("sub/b.tmp", false) {
		t.Fatal("nested .gitignore must apply inside the nested repository")
	}
	if !matcher.Ignored("sub/c.secret", false) {
		t.Fatal(".ignore scopes survive nested repository boundaries")
	}
	if !matcher.Ignored("root.log", false) {
		t.Fatal("outer .gitignore must still apply outside the nested repository")
	}
}

func TestIgnoreMatcherHonorsIgnoreByteCap(t *testing.T) {
	root := t.TempDir()
	content := "*early.log\n" + strings.Repeat("#", 60) + "\n*.tmp\n"
	writeIgnoreFile(t, root, ".gitignore", content)

	reader, err := NewWithLimits(root, DefaultPolicy(), nil, nil, Limits{MaxIgnoreBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	if !reader.ignore.Ignored("early.log", false) {
		t.Fatal("patterns within MaxIgnoreBytes must apply")
	}
	if reader.ignore.Ignored("late.tmp", false) {
		t.Fatal("patterns beyond MaxIgnoreBytes must not be loaded")
	}
}
