package files

import (
	"path"
	"strings"

	"foco/internal/enums"
)

type PathMatcher interface {
	Match(path string) bool
}

type Policy struct {
	Hidden              bool
	NoIgnore            bool
	IncludeDependencies bool
	IncludeSensitive    bool
	Exclude             []string
	Only                map[enums.FileCategory]struct{}
	ExcludeTypes        map[enums.FileCategory]struct{}
	MaxFileBytes        int64
	SensitivePaths      PathMatcher
}

func DefaultPolicy() Policy {
	return Policy{MaxFileBytes: 16 * 1024 * 1024}
}

var dependencyDirectories = map[string]struct{}{
	"node_modules": {}, "vendor": {}, "venv": {}, ".venv": {}, ".tox": {},
	"__pycache__": {}, "dist": {}, "build": {}, "coverage": {}, "target": {},
	".next": {}, ".nuxt": {}, ".turbo": {},
}

var sensitiveNames = map[string]struct{}{
	"credentials": {}, "credentials.json": {}, "secrets.json": {}, "secrets.yaml": {},
	"secrets.yml": {}, "id_rsa": {}, "id_dsa": {}, "id_ecdsa": {}, "id_ed25519": {},
	".netrc": {}, ".npmrc": {}, ".pypirc": {},
}

var sensitiveSuffixes = []string{".env", ".pem", ".key", ".p12", ".pfx"}

func IsSensitiveName(name string) bool {
	lower := strings.ToLower(name)
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return true
	}
	if _, ok := sensitiveNames[lower]; ok {
		return true
	}
	for _, suffix := range sensitiveSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func IsDependencyName(name string) bool {
	_, ok := dependencyDirectories[name]
	return ok
}

func CategoryAllowed(p Policy, category enums.FileCategory) bool {
	if len(p.Only) > 0 {
		if _, ok := p.Only[category]; !ok {
			return false
		}
	}
	if _, ok := p.ExcludeTypes[category]; ok {
		return false
	}
	return true
}

func ExcludedByPattern(patterns []string, rel string, isDir bool) bool {
	candidate := strings.TrimPrefix(path.Clean(rel), "./")
	if isDir {
		candidate += "/"
	}
	for _, pattern := range patterns {
		if matchPattern(pattern, candidate) {
			return true
		}
	}
	return false
}
