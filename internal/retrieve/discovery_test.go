package retrieve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"foco/internal/enums"
	"foco/internal/files"
	"foco/internal/provider"
	"foco/internal/requests"
)

func TestDiscoverAdmitsFilesAndPrunesDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"root.ts", "sub/child.ts"} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.WriteFile(full, []byte("export function event() { return true; }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	navigator := discoveryNavigator(fileScores)
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	state := newDiscoveryState()
	if err := discover(context.Background(), root, reader, "event", []string{"."}, nil, navigator, issues.add, issues.fatal, nil, state); err != nil {
		t.Fatal(err)
	}
	if len(state.Candidates) != 2 {
		t.Fatalf("candidates = %+v", state.Candidates)
	}
	if _, ok := state.Candidates["root.ts"]; !ok {
		t.Fatal("root.ts missing")
	}
	if _, ok := state.Candidates["sub/child.ts"]; !ok {
		t.Fatal("sub/child.ts missing")
	}
	if len(state.Pruned) == 0 {
		t.Fatal("expected a pruned directory")
	}
	if _, ok := state.Pruned["sub/nested"]; !ok {
		t.Fatalf("pruned = %+v, want sub/nested", state.Pruned)
	}
}

func TestDiscoverStopsPagingAtDiscoveryLimit(t *testing.T) {
	const pageSize = 10_000
	const total = 250_000
	reader := &discoveryReader{
		root: "/root",
		listPage: func(_ context.Context, directory string, offset int) (files.Page, error) {
			if directory != "/root" {
				t.Fatalf("unexpected directory %q", directory)
			}
			end := offset + pageSize
			if end > total {
				end = total
			}
			entries := make([]files.Entry, 0, end-offset)
			for index := offset; index < end; index++ {
				entries = append(entries, files.Entry{Path: fmt.Sprintf("/root/f%06d.ts", index)})
			}
			next := 0
			if end < total {
				next = end
			}
			return files.Page{Path: directory, Entries: entries, NextOffset: next}, nil
		},
		snapshot: func(context.Context, string) files.SnapshotResult {
			return files.SnapshotResult{Status: enums.SnapshotStatusIssue}
		},
	}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	state := newDiscoveryState()
	if err := discover(context.Background(), "/root", reader, "event", []string{"."}, nil, discoveryNavigator(fileScores), issues.add, issues.fatal, nil, state); err != nil {
		t.Fatal(err)
	}
	if state.EntriesSeen != DiscoveryLimit {
		t.Fatalf("entries seen = %d, want %d", state.EntriesSeen, DiscoveryLimit)
	}
	if want := DiscoveryLimit / pageSize; len(reader.lists) != want {
		t.Fatalf("list calls = %d (%v), want %d", len(reader.lists), reader.lists, want)
	}
	if last := reader.lists[len(reader.lists)-1]; last != fmt.Sprintf("/root@%d", DiscoveryLimit-pageSize) {
		t.Fatalf("last list call = %q, want cap boundary", last)
	}
	if got := issues.counts[enums.IssueCodeResourceLimit]; got != 1 {
		t.Fatalf("resource_limit = %d, want 1", got)
	}
}

func TestDiscoverCancellationStopsEntryProcessing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &discoveryReader{
		root: "/root",
		listPage: func(_ context.Context, directory string, _ int) (files.Page, error) {
			return files.Page{Path: directory, Entries: []files.Entry{
				{Path: "/root/a.ts"},
				{Path: "/root/b.ts"},
				{Path: "/root/c.ts"},
			}}, nil
		},
		snapshot: func(_ context.Context, path string) files.SnapshotResult {
			if path == "/root/a.ts" {
				cancel()
			}
			return files.SnapshotResult{Status: enums.SnapshotStatusSnapshot, Snapshot: &files.Snapshot{
				Path: path, Source: "export const event = true;\n", ContentHash: "hash",
			}}
		},
	}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	reported := []string{}
	report := func(path string, _ *files.Snapshot) { reported = append(reported, path) }
	state := newDiscoveryState()
	if err := discover(ctx, "/root", reader, "event", []string{"."}, nil, discoveryNavigator(fileScores), issues.add, issues.fatal, report, state); err != nil {
		t.Fatal(err)
	}
	// Snapshots are prefetched concurrently, so later entries may already be
	// read when cancellation lands; they must not be processed or reported.
	if len(reader.snapshots) == 0 || len(reader.snapshots) > 3 {
		t.Fatalf("snapshots = %v", reader.snapshots)
	}
	if len(reported) != 1 || reported[0] != "a.ts" {
		t.Fatalf("reported = %v, want [a.ts]", reported)
	}
	if state.EntriesSeen != 1 {
		t.Fatalf("entries seen = %d, want 1", state.EntriesSeen)
	}
	if got := issues.counts[enums.IssueCodeResourceLimit]; got != 0 {
		t.Fatalf("resource_limit = %d, want 0", got)
	}
}

func TestDiscoverCancellationDoesNotReportResourceLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &discoveryReader{
		root: "/root",
		listPage: func(_ context.Context, directory string, _ int) (files.Page, error) {
			switch directory {
			case "/root":
				return files.Page{Path: directory, Entries: []files.Entry{{Path: "/root/sub", IsDir: true}}}, nil
			case "/root/sub":
				return files.Page{Path: directory, Entries: []files.Entry{{Path: "/root/sub/inner", IsDir: true}}}, nil
			case "/root/sub/inner":
				cancel()
				return files.Page{Path: directory}, nil
			}
			return files.Page{Path: directory}, nil
		},
	}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	state := newDiscoveryState()
	if err := discover(ctx, "/root", reader, "event", []string{"."}, nil, discoveryNavigator(directoryScores), issues.add, issues.fatal, nil, state); err != nil {
		t.Fatal(err)
	}
	if len(state.Pruned) != 0 {
		t.Fatalf("pruned = %+v, want none", state.Pruned)
	}
	if got := issues.counts[enums.IssueCodeResourceLimit]; got != 0 {
		t.Fatalf("resource_limit = %d, want 0 after cancellation", got)
	}
}

func TestDiscoverStopsOnNonAdvancingOffset(t *testing.T) {
	reader := &discoveryReader{
		root: "/root",
		listPage: func(_ context.Context, directory string, offset int) (files.Page, error) {
			next := 5
			if offset == 0 {
				return files.Page{Path: directory, NextOffset: next}, nil
			}
			return files.Page{Path: directory, NextOffset: offset}, nil
		},
	}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	state := newDiscoveryState()
	if err := discover(context.Background(), "/root", reader, "event", []string{"."}, nil, discoveryNavigator(fileScores), issues.add, issues.fatal, nil, state); err != nil {
		t.Fatal(err)
	}
	if len(reader.lists) != 2 || reader.lists[0] != "/root@0" || reader.lists[1] != "/root@5" {
		t.Fatalf("list calls = %v, want [/root@0 /root@5]", reader.lists)
	}
	if got := issues.counts[enums.IssueCodeUnreadable]; got != 1 {
		t.Fatalf("unreadable = %d, want 1", got)
	}
	if got := issues.counts[enums.IssueCodeResourceLimit]; got != 0 {
		t.Fatalf("resource_limit = %d, want 0", got)
	}
}

func TestDiscoverStopsAfterFatalNavigation(t *testing.T) {
	reader := &discoveryReader{
		root: "/root",
		listPage: func(_ context.Context, directory string, _ int) (files.Page, error) {
			switch directory {
			case "/root":
				return files.Page{Path: directory, Entries: []files.Entry{{Path: "/root/buckets", IsDir: true}}}, nil
			case "/root/buckets":
				entries := make([]files.Entry, 0, 130)
				for index := 0; index < 130; index++ {
					entries = append(entries, files.Entry{Path: fmt.Sprintf("/root/buckets/d%03d", index), IsDir: true})
				}
				return files.Page{Path: directory, Entries: entries}, nil
			}
			return files.Page{Path: directory}, nil
		},
	}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		state := request.State.(requests.NavigationRequestState)
		for _, item := range state.Items {
			if item.Path >= "buckets/d128" {
				return nil, &provider.Failure{Kind: enums.FailureAuthentication, Message: "API authentication failed."}
			}
		}
		out := map[string]float64{}
		for index := range state.Items {
			out["q"+itoa(index)] = 0.9
		}
		return out, nil
	}}
	navigator := newNavigator(context.Background(), "event", evaluator, issues, stateDonors, nil, nil)
	state := newDiscoveryState()
	if err := discover(context.Background(), "/root", reader, "event", []string{"."}, nil, navigator, issues.add, issues.fatal, nil, state); err != nil {
		t.Fatal(err)
	}
	if got := issues.counts[enums.IssueCodeAuthentication]; got != 1 {
		t.Fatalf("authentication = %d, want 1", got)
	}
	if len(reader.lists) != 132 {
		t.Fatalf("list calls = %d, want 132 (root, buckets, one preview per directory)", len(reader.lists))
	}
	seen := map[string]int{}
	for _, call := range reader.lists {
		seen[call]++
	}
	for call, count := range seen {
		if count > 1 {
			t.Fatalf("directory %q listed %d times after fatal issue", call, count)
		}
	}
	if len(reader.snapshots) != 0 {
		t.Fatalf("snapshots = %v, want none", reader.snapshots)
	}
}

func TestDiscoverForwardsPageIssues(t *testing.T) {
	reader := &discoveryReader{
		root: "/root",
		listPage: func(_ context.Context, directory string, _ int) (files.Page, error) {
			return files.Page{Path: directory, Issue: enums.FileIssueUnreadable}, nil
		},
	}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	state := newDiscoveryState()
	if err := discover(context.Background(), "/root", reader, "event", []string{"."}, nil, discoveryNavigator(fileScores), issues.add, issues.fatal, nil, state); err != nil {
		t.Fatal(err)
	}
	if got := issues.counts[enums.IssueCodeUnreadable]; got != 1 {
		t.Fatalf("unreadable = %d, want 1", got)
	}
}

func TestDiscoverReportsFileSnapshots(t *testing.T) {
	reader := &discoveryReader{
		root: "/root",
		listPage: func(_ context.Context, directory string, _ int) (files.Page, error) {
			return files.Page{Path: directory, Entries: []files.Entry{
				{Path: "/root/b.ts"},
				{Path: "/root/a.ts"},
			}}, nil
		},
		snapshot: func(_ context.Context, path string) files.SnapshotResult {
			return files.SnapshotResult{Status: enums.SnapshotStatusSnapshot, Snapshot: &files.Snapshot{
				Path: path, Source: "export const event = true;\n", ContentHash: "hash-" + path,
			}}
		},
	}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	reported := []string{}
	report := func(path string, _ *files.Snapshot) { reported = append(reported, path) }
	state := newDiscoveryState()
	if err := discover(context.Background(), "/root", reader, "event", []string{"."}, nil, discoveryNavigator(fileScores), issues.add, issues.fatal, report, state); err != nil {
		t.Fatal(err)
	}
	if len(reported) != 2 || reported[0] != "a.ts" || reported[1] != "b.ts" {
		t.Fatalf("reported = %v, want [a.ts b.ts]", reported)
	}
	if len(state.Previews) != 2 {
		t.Fatalf("previews = %d, want 2", len(state.Previews))
	}
}

type discoveryReader struct {
	root      string
	listPage  func(context.Context, string, int) (files.Page, error)
	snapshot  func(context.Context, string) files.SnapshotResult
	lists     []string
	mu        sync.Mutex
	snapshots []string
}

func (r *discoveryReader) Root() string { return r.root }

func (r *discoveryReader) ListPage(ctx context.Context, directory string, offset int) (files.Page, error) {
	r.mu.Lock()
	r.lists = append(r.lists, fmt.Sprintf("%s@%d", directory, offset))
	r.mu.Unlock()
	if r.listPage != nil {
		return r.listPage(ctx, directory, offset)
	}
	return files.Page{Path: directory}, nil
}

func (r *discoveryReader) ReadSnapshotReuse(ctx context.Context, path string) files.SnapshotResult {
	r.mu.Lock()
	r.snapshots = append(r.snapshots, path)
	r.mu.Unlock()
	if r.snapshot != nil {
		return r.snapshot(ctx, path)
	}
	return files.SnapshotResult{}
}

func (r *discoveryReader) LookupFile(context.Context, string) (files.LookupResult, error) {
	return files.LookupResult{}, nil
}

func discoveryNavigator(answers func(provider.Request) (map[string]float64, error)) *navigator {
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	return newNavigator(context.Background(), "event", &scriptedEvaluator{answers: answers}, issues, stateDonors, nil, nil)
}

func newDiscoveryState() *discoveryState {
	return &discoveryState{
		Visited: map[string]bool{}, Candidates: map[string]candidate{},
		Pruned: map[string]requests.NavigationItem{}, Previews: map[string]*requests.FilePreview{},
		Donors: map[string][]Donor{},
	}
}

func fileScores(request provider.Request) (map[string]float64, error) {
	state := request.State.(requests.NavigationRequestState)
	out := map[string]float64{}
	for index, item := range state.Items {
		if item.Kind == enums.NavigationKindFile {
			out["q"+itoa(index)] = 0.9
		} else {
			out["q"+itoa(index)] = 0.1
		}
	}
	return out, nil
}

func directoryScores(request provider.Request) (map[string]float64, error) {
	state := request.State.(requests.NavigationRequestState)
	out := map[string]float64{}
	for index, item := range state.Items {
		if item.Kind == enums.NavigationKindDirectory {
			out["q"+itoa(index)] = 0.9
		} else {
			out["q"+itoa(index)] = 0.1
		}
	}
	return out, nil
}

func stateDonors(item requests.NavigationItem) []Donor { return nil }
