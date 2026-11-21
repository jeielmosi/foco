package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"

	"foco/internal/detect"
)

// scanFinding is one secret reported by foco scan. Start and End are UTF-8
// byte offsets [start, end) of the secret value; Line and Column are 1-based.
type scanFinding struct {
	Path   string `json:"path"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Rule   string `json:"rule"`
	Engine string `json:"engine"`
}

// runScan reports the secrets foco's detector finds in files, the ones a
// search would exclude. Exit status: 0 none found, 1 found, 2 error.
func runScan(ctx context.Context, opts ScanOptions, stdout, stderr io.Writer) int {
	// Scanning allocates many short-lived strings per file; collecting less
	// often trades a little memory for less GC work.
	defer debug.SetGCPercent(debug.SetGCPercent(400))
	engine, err := newDetector()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	defer engine.Close()
	scanner, ok := engine.(detect.SpanDetector)
	if !ok {
		fmt.Fprintln(stderr, "detector cannot report spans")
		return 2
	}
	paths, err := scanPaths(opts.Paths)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	results := make([][]scanFinding, len(paths))
	failures := make([]error, len(paths))
	var next sync.Mutex
	position := 0
	var wait sync.WaitGroup
	for range max(1, runtime.GOMAXPROCS(0)) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for {
				next.Lock()
				index := position
				position++
				next.Unlock()
				if index >= len(paths) || ctx.Err() != nil {
					return
				}
				results[index], failures[index] = scanFile(ctx, scanner, paths[index])
			}
		}()
	}
	wait.Wait()
	if ctx.Err() != nil {
		return 130
	}
	found, failed := false, false
	encoder := json.NewEncoder(stdout)
	for index, findings := range results {
		if failures[index] != nil {
			failed = true
			fmt.Fprintf(stderr, "%s: %v\n", paths[index], failures[index])
		}
		for _, finding := range findings {
			found = true
			if opts.JSON {
				_ = encoder.Encode(finding)
			} else {
				fmt.Fprintf(stdout, "%s:%d:%d: %s (%s)\n", finding.Path, finding.Line, finding.Column, finding.Rule, finding.Engine)
			}
		}
	}
	switch {
	case failed:
		return 2
	case found:
		return 1
	}
	return 0
}

func scanFile(ctx context.Context, scanner detect.SpanDetector, path string) ([]scanFinding, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	spans, err := scanner.Spans(ctx, path, content)
	if err != nil {
		return nil, err
	}
	out := make([]scanFinding, 0, len(spans))
	for _, span := range spans {
		line, column := 1, 1
		for _, b := range content[:span.Start] {
			if b == '\n' {
				line, column = line+1, 1
			} else {
				column++
			}
		}
		out = append(out, scanFinding{
			Path: path, Start: span.Start, End: span.End, Line: line, Column: column,
			Rule: span.Rule, Engine: span.Engine,
		})
	}
	return out, nil
}

// scanPaths expands directories to their regular files, skipping .git, in
// sorted order.
func scanPaths(roots []string) ([]string, error) {
	var out []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == ".git" && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type().IsRegular() {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}
