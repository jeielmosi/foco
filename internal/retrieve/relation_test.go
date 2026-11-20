package retrieve

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"foco/internal/enums"
	"foco/internal/files"
	"foco/internal/provider"
	"foco/internal/requests"
)

func testReader(t *testing.T) (string, *files.Reader) {
	t.Helper()
	root := t.TempDir()
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close() })
	return root, reader
}

func writeFile(t *testing.T, root, relative, source string) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAnchorClassesRequiresIndentedBody(t *testing.T) {
	if got := anchorClasses("export class Widget {\n  context() { return 1; }\n}\n"); len(got) != 1 || got[0] != "Widget" {
		t.Fatalf("multi-line class = %v", got)
	}
	if got := anchorClasses("export class Widget { context() { return 1; } }\n"); len(got) != 0 {
		t.Fatalf("one-line class must not anchor: %v", got)
	}
	if got := anchorClasses("type Widget struct {\n  Value int\n}\n"); len(got) != 1 || got[0] != "Widget" {
		t.Fatalf("go struct = %v", got)
	}
	if got := anchorClasses("impl Widget {\n    fn context(&self) -> u32 { 1 }\n}\n"); len(got) != 1 || got[0] != "Widget" {
		t.Fatalf("rust impl = %v", got)
	}
}

func TestReconsiderScoresPrunedDirectoriesWithAnchor(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "deep/nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deep/nested/sample.ts"), []byte("export class Child extends Anchor { method() { return 1; } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		out := map[string]float64{}
		for _, question := range request.Questions {
			out[question.ID] = 0.9
		}
		return out, nil
	}}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	navigator := newNavigator(context.Background(), "widget", evaluator, issues, nil, nil, nil)
	pruned := map[string]requests.NavigationItem{
		"deep": {Path: "deep", Kind: enums.NavigationKindDirectory, ChildPreview: &requests.DirectoryPreview{
			Entries: []requests.DirectoryEntry{{Name: "nested", Kind: enums.NavigationKindDirectory}},
		}},
	}
	seeds, err := reconsider(context.Background(), reader, navigator, root, pruned, &requests.RelationAnchor{Path: "anchor.ts", Classes: []string{"Anchor"}}, map[string][]Donor{}, issues.add, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 1 || seeds[0] != "deep" {
		t.Fatalf("seeds = %v", seeds)
	}
}

func constantEvaluator(score float64) *scriptedEvaluator {
	return &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		out := map[string]float64{}
		for _, question := range request.Questions {
			out[question.ID] = score
		}
		return out, nil
	}}
}

func TestFindAnchorSkipsCandidatesWithoutClasses(t *testing.T) {
	root, reader := testReader(t)
	writeFile(t, root, "plain.ts", "const value = 1;\n")
	writeFile(t, root, "widget.ts", "export class Widget {\n  value() { return 1; }\n}\n")
	ordered := []candidate{
		{Path: "plain.ts", ContentHash: "plain-hash", Score: 0.9},
		{Path: "missing.ts", ContentHash: "missing-hash", Score: 0.8},
		{Path: "widget.ts", ContentHash: "widget-hash", Score: 0.7},
	}
	anchor, donor, ok := findAnchor(context.Background(), reader, root, ordered)
	if !ok || anchor == nil {
		t.Fatalf("ok=%v anchor=%+v", ok, anchor)
	}
	if anchor.Path != "widget.ts" || len(anchor.Classes) != 1 || anchor.Classes[0] != "Widget" {
		t.Fatalf("anchor = %+v", anchor)
	}
	if donor.Path != "widget.ts" || donor.ContentHash != "widget-hash" {
		t.Fatalf("donor = %+v", donor)
	}
	if anchor, donor, ok := findAnchor(context.Background(), reader, root, []candidate{{Path: "widget.ts", Score: RelevanceBoundary}}); ok || anchor != nil || donor != (Donor{}) {
		t.Fatalf("boundary candidate anchored: ok=%v anchor=%+v donor=%+v", ok, anchor, donor)
	}
}

func TestReconsiderRequiresAnchorAndPrunedDirectories(t *testing.T) {
	root, reader := testReader(t)
	evaluator := constantEvaluator(0.9)
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	navigator := newNavigator(context.Background(), "widget", evaluator, issues, nil, nil, nil)
	pruned := map[string]requests.NavigationItem{"deep": {Path: "deep", Kind: enums.NavigationKindDirectory}}
	if seeds, err := reconsider(context.Background(), reader, navigator, root, pruned, nil, map[string][]Donor{}, issues.add, nil); err != nil || seeds != nil {
		t.Fatalf("nil anchor: seeds=%v err=%v", seeds, err)
	}
	if seeds, err := reconsider(context.Background(), reader, navigator, root, map[string]requests.NavigationItem{}, &requests.RelationAnchor{Path: "anchor.ts"}, map[string][]Donor{}, issues.add, nil); err != nil || seeds != nil {
		t.Fatalf("empty pruned: seeds=%v err=%v", seeds, err)
	}
	if requests := evaluator.Requests(); requests != 0 {
		t.Fatalf("evaluator requests = %d, want 0", requests)
	}
}

func TestReconsiderForwardsReportAndDonors(t *testing.T) {
	root, reader := testReader(t)
	writeFile(t, root, "pkg/a.ts", "export class Anchor {\n  value() { return 1; }\n}\n")
	navigator := newNavigator(context.Background(), "widget", constantEvaluator(0.9), &issueSet{counts: map[enums.RetrievalIssueCode]int{}}, nil, nil, nil)
	pruned := map[string]requests.NavigationItem{
		"pkg": {Path: "pkg", Kind: enums.NavigationKindDirectory, ChildPreview: &requests.DirectoryPreview{
			Entries: []requests.DirectoryEntry{{Name: "a.ts", Kind: enums.NavigationKindFile}},
		}},
	}
	reported := map[string]bool{}
	report := func(path string, snapshot *files.Snapshot) {
		if snapshot == nil {
			t.Errorf("report(%q) got nil snapshot", path)
			return
		}
		reported[path] = true
	}
	donors := map[string][]Donor{}
	seeds, err := reconsider(context.Background(), reader, navigator, root, pruned, &requests.RelationAnchor{Path: "anchor.ts", Classes: []string{"Anchor"}}, donors, nil, report)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 1 || seeds[0] != "pkg" {
		t.Fatalf("seeds = %v", seeds)
	}
	if !reported["pkg/a.ts"] {
		t.Fatalf("report paths = %v", reported)
	}
	sampled := donors["pkg"]
	if len(sampled) != 1 || sampled[0].Path != "pkg/a.ts" || sampled[0].ContentHash == "" {
		t.Fatalf("donors = %+v", sampled)
	}
}

func TestReconsiderSeedsFollowSortedPrunedPaths(t *testing.T) {
	root, reader := testReader(t)
	navigator := newNavigator(context.Background(), "widget", constantEvaluator(0.9), &issueSet{counts: map[enums.RetrievalIssueCode]int{}}, nil, nil, nil)
	pruned := map[string]requests.NavigationItem{
		"zeta":  {Path: "zeta", Kind: enums.NavigationKindDirectory, ChildPreview: &requests.DirectoryPreview{}},
		"alpha": {Path: "alpha", Kind: enums.NavigationKindDirectory, ChildPreview: &requests.DirectoryPreview{}},
	}
	seeds, err := reconsider(context.Background(), reader, navigator, root, pruned, &requests.RelationAnchor{Path: "anchor.ts", Classes: []string{"Anchor"}}, map[string][]Donor{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 2 || seeds[0] != "alpha" || seeds[1] != "zeta" {
		t.Fatalf("seeds = %v", seeds)
	}
}
