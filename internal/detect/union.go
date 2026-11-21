package detect

import (
	"context"
	"path/filepath"
	"strings"

	"foco/internal/detect/catalog"
)

type Union struct {
	titus       Engine
	credentials *Credentials
	better      Engine
	catalog     *catalog.Catalog
}

func NewUnion(titus, better Engine, catalog *catalog.Catalog) *Union {
	return &Union{titus: titus, credentials: NewCredentials(), better: better, catalog: catalog}
}

func (u *Union) DetectPath(ctx context.Context, path string) (bool, error) {
	if excluded, _ := u.credentials.DetectPath(ctx, path); excluded {
		return true, nil
	}
	excluded, err := u.titus.DetectPath(ctx, path)
	if err != nil || excluded {
		return excluded, err
	}
	if !needsSecondPass(path, u.catalog) {
		return false, nil
	}
	return u.better.DetectPath(ctx, path)
}

// Engines that cannot scan bytes fall back to reading path themselves.
func (u *Union) DetectContent(ctx context.Context, path string, content []byte) (bool, error) {
	// The credential heuristics are cheap regular expressions and need no
	// setup, so they run first and can answer while Titus is still loading.
	if excluded, _ := u.credentials.DetectContent(ctx, path, content); excluded {
		return true, nil
	}
	excluded, err := detectContent(ctx, u.titus, path, content)
	if err != nil || excluded {
		return excluded, err
	}
	if !needsSecondPass(path, u.catalog) {
		return false, nil
	}
	return detectContent(ctx, u.better, path, content)
}

func detectContent(ctx context.Context, engine Engine, path string, content []byte) (bool, error) {
	if scanner, ok := engine.(ContentDetector); ok {
		return scanner.DetectContent(ctx, path, content)
	}
	return engine.DetectPath(ctx, path)
}

// Spans runs every engine on content (no short-circuit, and Betterleaks on
// every path, not only config paths) and returns their merged findings.
func (u *Union) Spans(ctx context.Context, path string, content []byte) ([]Span, error) {
	var out []Span
	for _, engine := range []any{u.credentials, u.titus, u.better} {
		detector, ok := engine.(SpanDetector)
		if !ok {
			continue
		}
		spans, err := detector.Spans(ctx, path, content)
		if err != nil {
			return nil, err
		}
		for _, span := range spans {
			if keepSpan(content, span) {
				out = append(out, span)
			}
		}
	}
	return mergeSpans(out), nil
}

// Ready reports whether the engines that load in the background loaded. A
// failure there makes every scan fail, so callers check it to fail the whole
// search instead of reporting files as unreadable.
func (u *Union) Ready(ctx context.Context) error {
	if loader, ok := u.titus.(interface{ Ready(context.Context) error }); ok {
		return loader.Ready(ctx)
	}
	return nil
}

func (u *Union) Close() error {
	var first error
	if err := u.titus.Close(); err != nil {
		first = err
	}
	if err := u.better.Close(); err != nil && first == nil {
		first = err
	}
	return first
}

var secondPassExtensions = map[string]struct{}{
	".env": {}, ".json": {}, ".yaml": {}, ".yml": {}, ".toml": {}, ".ini": {},
	".cfg": {}, ".pem": {}, ".key": {}, ".properties": {}, ".tfstate": {},
}

var _ Engine = (*Union)(nil)
var _ ContentDetector = (*Union)(nil)
var _ SpanDetector = (*Union)(nil)

func needsSecondPass(path string, c *catalog.Catalog) bool {
	lower := strings.ToLower(path)
	ext := filepath.Ext(lower)
	if _, ok := secondPassExtensions[ext]; ok {
		return true
	}
	if c != nil && c.Match(path) {
		return true
	}
	return false
}
