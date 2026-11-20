package retrieve

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"foco/internal/enums"
	"foco/internal/files"
)

type repositoryContextReader struct {
	root   string
	lookup func(ctx context.Context, path string) (files.LookupResult, error)
}

func (r *repositoryContextReader) Root() string { return r.root }

func (r *repositoryContextReader) ListPage(context.Context, string, int) (files.Page, error) {
	return files.Page{}, nil
}

func (r *repositoryContextReader) ReadSnapshotReuse(context.Context, string) files.SnapshotResult {
	return files.SnapshotResult{}
}

func (r *repositoryContextReader) LookupFile(ctx context.Context, path string) (files.LookupResult, error) {
	return r.lookup(ctx, path)
}

func newInstructionReader(t *testing.T) *files.Reader {
	t.Helper()
	reader, err := files.New(t.TempDir(), files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close reader: %v", err)
		}
	})
	return reader
}

func writeInstructionFile(t *testing.T, root, relative string) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte("guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGatherRepositoryContextOrderAndIncomplete(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"a/b", "unrelated"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"AGENTS.md", "a/AGENTS.md", "a/b/AGENTS.md", "unrelated/AGENTS.md"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte("guidance\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	result, err := gatherRepositoryContext(context.Background(), reader, []FileEvidence{{Path: "a/b/test_behavior.py"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"AGENTS.md", "a/b/AGENTS.md", "a/AGENTS.md"}
	if len(result.InstructionFiles) != len(want) {
		t.Fatalf("files = %v", result.InstructionFiles)
	}
	for index := range want {
		if result.InstructionFiles[index] != want[index] {
			t.Fatalf("files = %v, want %v", result.InstructionFiles, want)
		}
	}
	if result.InstructionLookupIncomplete {
		t.Fatal("lookup must be complete")
	}

	if err := os.Remove(filepath.Join(root, "a/b/AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "unrelated/AGENTS.md"), filepath.Join(root, "a/b/AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	unsafe, err := gatherRepositoryContext(context.Background(), reader, []FileEvidence{{Path: "a/b/test_behavior.py"}})
	if err != nil {
		t.Fatal(err)
	}
	if !unsafe.InstructionLookupIncomplete {
		t.Fatal("symlinked AGENTS.md must mark lookup incomplete")
	}
	if len(unsafe.InstructionFiles) != 2 {
		t.Fatalf("files = %v", unsafe.InstructionFiles)
	}
}

func TestGatherRepositoryContextDeduplicatesAcrossEvidence(t *testing.T) {
	reader := newInstructionReader(t)
	for _, relative := range []string{"AGENTS.md", "a/AGENTS.md", "a/b/AGENTS.md", "a/c/AGENTS.md"} {
		writeInstructionFile(t, reader.Root(), relative)
	}
	entries := []FileEvidence{{Path: "a/b/one.py"}, {Path: "a/c/two.py"}, {Path: "a/b/three.py"}}
	result, err := gatherRepositoryContext(context.Background(), reader, entries)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"AGENTS.md", "a/b/AGENTS.md", "a/AGENTS.md", "a/c/AGENTS.md"}
	if len(result.InstructionFiles) != len(want) {
		t.Fatalf("files = %v, want %v", result.InstructionFiles, want)
	}
	for index := range want {
		if result.InstructionFiles[index] != want[index] {
			t.Fatalf("files = %v, want %v", result.InstructionFiles, want)
		}
	}
	if result.InstructionLookupIncomplete {
		t.Fatal("lookup must be complete")
	}
}

func TestGatherRepositoryContextIgnoresNilAndEmptyEntries(t *testing.T) {
	reader := newInstructionReader(t)
	writeInstructionFile(t, reader.Root(), "AGENTS.md")
	entries := []FileEvidence{{}, {Path: ""}, {Path: "."}, {Path: "/"}}
	result, err := gatherRepositoryContext(context.Background(), reader, entries)
	if err != nil {
		t.Fatal(err)
	}
	assertInstructionFiles(t, result.InstructionFiles, []string{"AGENTS.md"})
	if result.InstructionLookupIncomplete {
		t.Fatal("lookup must be complete")
	}
}

func TestGatherRepositoryContextMissingOnlyIsComplete(t *testing.T) {
	reader := newInstructionReader(t)
	result, err := gatherRepositoryContext(context.Background(), reader, []FileEvidence{{Path: "a/b/test_behavior.py"}})
	if err != nil {
		t.Fatal(err)
	}
	assertInstructionFiles(t, result.InstructionFiles, nil)
	if result.InstructionLookupIncomplete {
		t.Fatal("missing-only lookup must be complete")
	}
}

func TestGatherRepositoryContextLookupErrorIsIncomplete(t *testing.T) {
	reader := &repositoryContextReader{
		root: "/repo",
		lookup: func(_ context.Context, path string) (files.LookupResult, error) {
			if path == filepath.Join("/repo", "AGENTS.md") {
				return files.LookupResult{Status: enums.LookupStatusFile}, nil
			}
			return files.LookupResult{}, errors.New("lookup exploded")
		},
	}
	result, err := gatherRepositoryContext(context.Background(), reader, []FileEvidence{{Path: "a/b/test_behavior.py"}})
	if err != nil {
		t.Fatalf("non-context lookup error must not fail: %v", err)
	}
	assertInstructionFiles(t, result.InstructionFiles, []string{"AGENTS.md"})
	if !result.InstructionLookupIncomplete {
		t.Fatal("non-context lookup error must mark lookup incomplete")
	}
}

func TestGatherRepositoryContextCancellationReturnsContextError(t *testing.T) {
	t.Run("pre-cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		reader := &repositoryContextReader{
			root: "/repo",
			lookup: func(ctx context.Context, _ string) (files.LookupResult, error) {
				if err := ctx.Err(); err != nil {
					return files.LookupResult{}, err
				}
				return files.LookupResult{Status: enums.LookupStatusFile}, nil
			},
		}
		result, err := gatherRepositoryContext(ctx, reader, []FileEvidence{{Path: "a/b/test_behavior.py"}})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		assertInstructionFiles(t, result.InstructionFiles, nil)
		if !result.InstructionLookupIncomplete {
			t.Fatal("cancelled lookup must mark lookup incomplete")
		}
	})

	t.Run("wrapped-error-mid-run", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		reader := &repositoryContextReader{
			root: "/repo",
			lookup: func(_ context.Context, path string) (files.LookupResult, error) {
				if path == filepath.Join("/repo", "AGENTS.md") {
					return files.LookupResult{Status: enums.LookupStatusFile}, nil
				}
				cancel()
				return files.LookupResult{}, fmt.Errorf("read: %w", context.Canceled)
			},
		}
		result, err := gatherRepositoryContext(ctx, reader, []FileEvidence{{Path: "a/b/test_behavior.py"}})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		assertInstructionFiles(t, result.InstructionFiles, []string{"AGENTS.md"})
		if !result.InstructionLookupIncomplete {
			t.Fatal("cancelled lookup must mark lookup incomplete")
		}
	})
}

func assertInstructionFiles(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("files = %v, want %v", got, want)
		}
	}
}
