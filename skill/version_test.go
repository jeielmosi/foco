package skill

import (
	"strings"
	"testing"
)

func TestPathResolvesCurrentVersion(t *testing.T) {
	if Path != Version+"/SKILL.md" {
		t.Fatalf("Path = %q, want %q", Path, Version+"/SKILL.md")
	}
	if _, err := files.ReadFile(Path); err != nil {
		t.Fatalf("embedded files do not contain Path: %v", err)
	}
	if !strings.Contains(Content, "name: foco") {
		t.Fatalf("Content does not describe the foco skill:\n%s", Content)
	}
}
