package files

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

type scopePatterns struct {
	git    []gitignore.Pattern
	search []gitignore.Pattern
}

type IgnoreMatcher struct {
	root     string
	noIgnore bool
	exclude  []string
	maxBytes int64
	mu       sync.Mutex
	patterns map[string]scopePatterns
	matchers map[string]gitignore.Matcher
}

func NewIgnoreMatcher(root string, noIgnore bool, excludes []string) (*IgnoreMatcher, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &IgnoreMatcher{
		root: abs, noIgnore: noIgnore, exclude: excludes, maxBytes: 1 << 20,
		patterns: map[string]scopePatterns{}, matchers: map[string]gitignore.Matcher{},
	}, nil
}

func (m *IgnoreMatcher) Ignored(rel string, isDir bool) bool {
	rel = filepath.ToSlash(rel)
	if ExcludedByPattern(m.exclude, rel, isDir) {
		return true
	}
	if m.noIgnore {
		return false
	}
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		if m.matcherFor(strings.Join(parts[:i], "/")).Match(parts[:i], true) {
			return true
		}
	}
	return m.matcherFor(filepath.Dir(rel)).Match(parts, isDir)
}

func (m *IgnoreMatcher) matcherFor(dir string) gitignore.Matcher {
	if dir == "." {
		dir = ""
	}
	m.mu.Lock()
	matcher, ok := m.matchers[dir]
	m.mu.Unlock()
	if ok {
		return matcher
	}
	scopes := []string{""}
	if dir != "" {
		parts := strings.Split(dir, "/")
		for i := range parts {
			scopes = append(scopes, strings.Join(parts[:i+1], "/"))
		}
	}
	deepestGit := -1
	for index := 1; index < len(scopes); index++ {
		if scopeHasGit(m.root, scopes[index]) {
			deepestGit = index
		}
	}
	var patterns []gitignore.Pattern
	for index, scope := range scopes {
		loaded := m.patternsFor(scope)
		if deepestGit < 0 || index >= deepestGit {
			patterns = append(patterns, loaded.git...)
		}
		patterns = append(patterns, loaded.search...)
	}
	matcher = gitignore.NewMatcher(patterns)
	m.mu.Lock()
	m.matchers[dir] = matcher
	m.mu.Unlock()
	return matcher
}

func scopeHasGit(root, scope string) bool {
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(scope), ".git"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}

func (m *IgnoreMatcher) patternsFor(scope string) scopePatterns {
	m.mu.Lock()
	patterns, ok := m.patterns[scope]
	m.mu.Unlock()
	if ok {
		return patterns
	}
	abs := filepath.Join(m.root, filepath.FromSlash(scope))
	domain := []string(nil)
	if scope != "" {
		domain = strings.Split(scope, "/")
	}
	loaded := scopePatterns{
		git:    readPatterns(filepath.Join(abs, ".gitignore"), domain, m.maxBytes),
		search: readPatterns(filepath.Join(abs, ".ignore"), domain, m.maxBytes),
	}
	m.mu.Lock()
	m.patterns[scope] = loaded
	m.mu.Unlock()
	return loaded
}

func readPatterns(path string, domain []string, maxBytes int64) []gitignore.Pattern {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var patterns []gitignore.Pattern
	scanner := bufio.NewScanner(io.LimitReader(file, maxBytes))
	scanner.Buffer(make([]byte, 4096), int(maxBytes))
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, domain))
	}
	return patterns
}

func matchPattern(pattern, candidate string) bool {
	anchored := strings.HasPrefix(pattern, "/")
	if anchored {
		pattern = strings.TrimPrefix(pattern, "/")
	}
	ok, err := doublestar.Match(pattern, candidate)
	if err != nil {
		return false
	}
	if ok {
		return true
	}
	if anchored {
		return doublestar.MatchUnvalidated(pattern, strings.TrimSuffix(candidate, "/"))
	}
	if strings.HasSuffix(pattern, "/") {
		return doublestar.MatchUnvalidated(strings.TrimSuffix(pattern, "/")+"/**", candidate)
	}
	return doublestar.MatchUnvalidated("**/"+pattern, candidate) ||
		doublestar.MatchUnvalidated(pattern+"/**", candidate)
}
