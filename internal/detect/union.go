package detect

import (
	"context"
	"path/filepath"
	"strings"

	"foco/internal/detect/catalog"
)

type Union struct {
	titus   Engine
	better  Engine
	catalog *catalog.Catalog
	all     bool
}

func NewUnion(titus, better Engine, catalog *catalog.Catalog) *Union {
	return &Union{titus: titus, better: better, catalog: catalog}
}

// NewUnionAll is the benchmark-only U3 union. It runs both engines for every
// path and ORs their verdicts instead of taking the U0 short path. A failure in
// one engine does not prevent the other from running; the first error is
// preserved so U3 stays a true both-engine OR. The CLI uses NewUnion;
// NewUnionAll exists so the acceptance harness can measure the recall/cost
// ceiling without a second detector implementation.
func NewUnionAll(titus, better Engine, catalog *catalog.Catalog) *Union {
	return &Union{titus: titus, better: better, catalog: catalog, all: true}
}

func (u *Union) DetectPath(ctx context.Context, path string) (bool, error) {
	if u.all {
		first, firstErr := u.titus.DetectPath(ctx, path)
		second, secondErr := u.better.DetectPath(ctx, path)
		if firstErr != nil {
			return false, firstErr
		}
		if secondErr != nil {
			return false, secondErr
		}
		return first || second, nil
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

// DetectContent applies the same union to content already read from path.
// Engines that cannot scan bytes fall back to reading path themselves.
func (u *Union) DetectContent(ctx context.Context, path string, content []byte) (bool, error) {
	if u.all {
		first, firstErr := detectContent(ctx, u.titus, path, content)
		second, secondErr := detectContent(ctx, u.better, path, content)
		if firstErr != nil {
			return false, firstErr
		}
		if secondErr != nil {
			return false, secondErr
		}
		return first || second, nil
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
