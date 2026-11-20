package notices

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var versionSuffix = regexp.MustCompile(`/v[0-9]+$`)

const pinnedShhgitCommit = "bac0c7d39519203d230b6c9a2c6e3eba18346aba"

func directModules(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	var modules []string
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "require (":
			inBlock = true
		case inBlock && trimmed == ")":
			return modules
		case inBlock && trimmed != "" && !strings.HasPrefix(trimmed, "//"):
			fields := strings.Fields(trimmed)
			if len(fields) > 0 {
				modules = append(modules, fields[0])
			}
		}
	}
	t.Fatal("go.mod has no direct require block")
	return nil
}

func moduleKey(module string) string {
	return versionSuffix.ReplaceAllString(module, "")
}

func TestNoticesCoverDirectRequirementsAndLicenseFiles(t *testing.T) {
	root := filepath.Join("..", "..")
	noticeData, err := os.ReadFile(filepath.Join(root, "THIRD_PARTY_NOTICES"))
	if err != nil {
		t.Fatal(err)
	}
	notice := string(noticeData)
	if strings.TrimSpace(notice) == "" {
		t.Fatal("THIRD_PARTY_NOTICES is empty")
	}
	modules := directModules(t)
	if len(modules) == 0 {
		t.Fatal("go.mod direct require block is empty")
	}
	for _, module := range modules {
		key := moduleKey(module)
		if !strings.Contains(notice, key) {
			t.Errorf("missing notice for %s (key %s)", module, key)
		}
	}
	licenseLine := regexp.MustCompile(`(?m)^License text: (.+)$`)
	matches := licenseLine.FindAllStringSubmatch(notice, -1)
	if len(matches) == 0 {
		t.Fatal("THIRD_PARTY_NOTICES has no License text entries")
	}
	for _, match := range matches {
		target := strings.TrimSpace(match[1])
		if _, err := os.Stat(filepath.Join(root, target)); err != nil {
			t.Errorf("license text %q: %v", target, err)
		}
	}
}

func TestEmbeddedCatalogIsAttributed(t *testing.T) {
	root := filepath.Join("..", "..")
	catalogData, err := os.ReadFile(filepath.Join(root, "internal", "detect", "catalog", "paths.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	catalog := string(catalogData)
	for _, want := range []string{"commit:", "source:", "license:", "shhgit", pinnedShhgitCommit} {
		if !strings.Contains(catalog, want) {
			t.Errorf("paths.yaml missing %q", want)
		}
	}
	noticeData, err := os.ReadFile(filepath.Join(root, "THIRD_PARTY_NOTICES"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(noticeData), "shhgit") {
		t.Error("THIRD_PARTY_NOTICES does not mention shhgit")
	}
}
