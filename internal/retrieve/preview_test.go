package retrieve

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"foco/internal/enums"
	"foco/internal/files"
	"foco/internal/requests"
)

func TestPreviewFileCapsBytesAndJSONWithoutDeclarations(t *testing.T) {
	source := strings.Repeat("x", 40000)
	preview := previewFile(files.Snapshot{Path: "a.ts", Source: source}, false)
	if preview.SizeBytes != 40000 || !preview.Truncated {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.Text) > PreviewBytes {
		t.Fatalf("text = %d bytes", len(preview.Text))
	}
	blob, _ := jsonMarshal(preview)
	if len(blob) > PreviewJSONBytes {
		t.Fatalf("json = %d bytes", len(blob))
	}
	if preview.Range != "opening bytes" {
		t.Fatalf("range = %q", preview.Range)
	}
}

func TestPreviewDirectoryCapsEntriesAndSamplesExtensions(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 80; index++ {
		name := filepath.Join(root, fmt.Sprintf("f%03d.ts", index))
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	preview := previewDirectory(context.Background(), reader, root, func(enums.RetrievalIssueCode) {})
	if preview == nil || !preview.Truncated {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.Entries) != 64 {
		t.Fatalf("entries = %d, want 64", len(preview.Entries))
	}
	if preview.SampledFiles != 64 || preview.SampledExtensions[".ts"] != 64 {
		t.Fatalf("samples = %+v", preview)
	}
}

func TestExtensionMatchesNodePathExtnameForDotfiles(t *testing.T) {
	cases := map[string]string{
		"a.ts": ".ts", "tool": "", ".env": "", ".env.local": ".local", "a/b.c.d": ".d",
		"a.": ".", "..": "", "...": ".", "a.b.": ".", ".": "",
	}
	for path, want := range cases {
		if got := extension(path); got != want {
			t.Errorf("extension(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestPreviewFileKeepsRuneBoundaryAtByteCap(t *testing.T) {
	source := strings.Repeat("a", PreviewBytes-1) + "é" + "tail"
	preview := previewFile(files.Snapshot{Path: "a.txt", Source: source}, false)
	if preview.PreviewBytes != PreviewBytes-1 || !preview.Truncated {
		t.Fatalf("preview = %+v", preview)
	}
	if !utf8.ValidString(preview.Text) {
		t.Fatalf("text ends mid-rune: %q", preview.Text[len(preview.Text)-4:])
	}
}

func TestPreviewDirectoryReportsPageIssue(t *testing.T) {
	var issues []enums.RetrievalIssueCode
	reader := pageIssueReader{page: files.Page{Issue: enums.FileIssueResourceLimit}}
	preview := previewDirectory(context.Background(), reader, "pkg", func(kind enums.RetrievalIssueCode) {
		issues = append(issues, kind)
	})
	if preview == nil || len(preview.Entries) != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	if len(issues) != 1 || issues[0] != enums.IssueCodeResourceLimit {
		t.Fatalf("issues = %v", issues)
	}
}

func TestSplitTextCapsBytesPreservesLinesAndRunes(t *testing.T) {
	source := "line one\nline two\nline three\n"
	chunks := splitText(source, 14)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %+v", chunks)
	}
	want := []textChunk{
		{Text: "line one\n", StartLine: 1, EndLine: 1},
		{Text: "line two\n", StartLine: 2, EndLine: 2},
		{Text: "line three\n", StartLine: 3, EndLine: 3},
	}
	joined := strings.Builder{}
	for index, chunk := range chunks {
		if chunk != want[index] {
			t.Errorf("chunk %d = %+v, want %+v", index, chunk, want[index])
		}
		joined.WriteString(chunk.Text)
	}
	if joined.String() != source {
		t.Fatalf("joined = %q", joined.String())
	}

	multibyte := strings.Repeat("é", 10)
	joined.Reset()
	for _, chunk := range splitText(multibyte, 5) {
		if len(chunk.Text) > 5 {
			t.Fatalf("chunk = %d bytes", len(chunk.Text))
		}
		if !utf8.ValidString(chunk.Text) {
			t.Fatalf("chunk = %q", chunk.Text)
		}
		joined.WriteString(chunk.Text)
	}
	if joined.String() != multibyte {
		t.Fatalf("joined = %q", joined.String())
	}
}

func TestWithDirectoryContentSamplesFilesAndReportsSnapshots(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "pkg")
	if err := os.MkdirAll(filepath.Join(directory, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "a.ts"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "b.ts"), []byte("beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	item := requests.NavigationItem{
		Path: "pkg", Kind: enums.NavigationKindDirectory,
		ChildPreview: &requests.DirectoryPreview{Entries: []requests.DirectoryEntry{
			{Name: "a.ts", Kind: enums.NavigationKindFile},
			{Name: "b.ts", Kind: enums.NavigationKindFile},
			{Name: "sub", Kind: enums.NavigationKindDirectory},
		}},
	}
	reported := map[string]string{}
	got, donors := withDirectoryContent(
		context.Background(), reader, item,
		func(kind enums.RetrievalIssueCode) { t.Errorf("unexpected issue %q", kind) },
		func(relative string, snapshot *files.Snapshot) {
			reported[relative] = snapshot.ContentHash
		},
		nil,
	)
	if got.Path != "pkg" || got.Kind != enums.NavigationKindDirectory || got.ChildPreview == nil {
		t.Fatalf("item = %+v", got)
	}
	samples := got.ChildPreview.ContentSamples
	if len(samples) != 2 {
		t.Fatalf("samples = %+v", samples)
	}
	for index, name := range []string{"a.ts", "b.ts"} {
		if samples[index].Name != name || samples[index].Truncated {
			t.Fatalf("sample %d = %+v", index, samples[index])
		}
	}
	if samples[0].Source != "alpha\n" || samples[1].Source != "beta\n" {
		t.Fatalf("sources = %q, %q", samples[0].Source, samples[1].Source)
	}
	if len(donors) != 2 || donors[0].Path != "pkg/a.ts" || donors[1].Path != "pkg/b.ts" {
		t.Fatalf("donors = %+v", donors)
	}
	for _, donor := range donors {
		if donor.ContentHash == "" || donor.ContentHash != reported[donor.Path] {
			t.Fatalf("donor = %+v, reported = %v", donor, reported)
		}
	}
	if len(reported) != 2 {
		t.Fatalf("reported = %v", reported)
	}
}

func TestWithDirectoryContentMarksLongSamplesTruncated(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("abcdefghij", 2000)
	if err := os.WriteFile(filepath.Join(root, "big.ts"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	item := requests.NavigationItem{
		Path: ".", Kind: enums.NavigationKindDirectory,
		ChildPreview: &requests.DirectoryPreview{Entries: []requests.DirectoryEntry{
			{Name: "big.ts", Kind: enums.NavigationKindFile},
		}},
	}
	got, donors := withDirectoryContent(
		context.Background(), reader, item, func(enums.RetrievalIssueCode) {}, nil, nil,
	)
	if len(donors) != 1 || len(got.ChildPreview.ContentSamples) != 1 {
		t.Fatalf("donors = %+v, preview = %+v", donors, got.ChildPreview)
	}
	sample := got.ChildPreview.ContentSamples[0]
	if !sample.Truncated {
		t.Fatalf("sample = %+v", sample)
	}
	for _, offset := range []string{"[character offset 0]", "[character offset 7334]", "[character offset 14667]"} {
		if !strings.Contains(sample.Source, offset) {
			t.Fatalf("sample missing %q: %q", offset, sample.Source)
		}
	}
	if strings.Contains(sample.Source, content) {
		t.Fatal("sample contains the whole source")
	}
}

func TestWithDirectoryContentCapsSampleJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "quotes.ts"), []byte(strings.Repeat(`"`, 16000)), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	item := requests.NavigationItem{
		Path: ".", Kind: enums.NavigationKindDirectory,
		ChildPreview: &requests.DirectoryPreview{Entries: []requests.DirectoryEntry{
			{Name: "quotes.ts", Kind: enums.NavigationKindFile},
		}},
	}
	got, _ := withDirectoryContent(context.Background(), reader, item, func(enums.RetrievalIssueCode) {}, nil, nil)
	blob, err := jsonMarshal(got.ChildPreview)
	if err != nil {
		t.Fatal(err)
	}
	if len(blob) > PreviewSampleJSONBytes {
		t.Fatalf("json = %d bytes", len(blob))
	}
	if !got.ChildPreview.ContentSamples[0].Truncated {
		t.Fatalf("sample = %+v", got.ChildPreview.ContentSamples[0])
	}
}

func TestWithDirectoryContentWithoutPreviewIsUnchanged(t *testing.T) {
	item := requests.NavigationItem{Path: "pkg", Kind: enums.NavigationKindDirectory}
	got, donors := withDirectoryContent(context.Background(), nil, item, func(enums.RetrievalIssueCode) {}, nil, nil)
	if got.Path != "pkg" || got.ChildPreview != nil || donors != nil {
		t.Fatalf("item = %+v, donors = %+v", got, donors)
	}
}

func TestWithDirectoryContentReportsChildReadIssue(t *testing.T) {
	var issues []enums.RetrievalIssueCode
	reader := snapshotIssueReader{result: files.SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueChanged}}
	item := requests.NavigationItem{
		Path: "pkg", Kind: enums.NavigationKindDirectory,
		ChildPreview: &requests.DirectoryPreview{Entries: []requests.DirectoryEntry{
			{Name: "a.ts", Kind: enums.NavigationKindFile},
		}},
	}
	got, donors := withDirectoryContent(
		context.Background(), reader, item,
		func(kind enums.RetrievalIssueCode) { issues = append(issues, kind) }, nil, nil,
	)
	if len(issues) != 1 || issues[0] != enums.IssueCodeChanged {
		t.Fatalf("issues = %v", issues)
	}
	if len(donors) != 0 || len(got.ChildPreview.ContentSamples) != 0 {
		t.Fatalf("donors = %+v, samples = %+v", donors, got.ChildPreview.ContentSamples)
	}
}

func TestWithDirectoryContentKeepsSamplesValidUTF8(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("é", 12000)
	if err := os.WriteFile(filepath.Join(root, "big.ts"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	item := requests.NavigationItem{
		Path: ".", Kind: enums.NavigationKindDirectory,
		ChildPreview: &requests.DirectoryPreview{Entries: []requests.DirectoryEntry{
			{Name: "big.ts", Kind: enums.NavigationKindFile},
		}},
	}
	got, _ := withDirectoryContent(context.Background(), reader, item, func(enums.RetrievalIssueCode) {}, nil, nil)
	samples := got.ChildPreview.ContentSamples
	if len(samples) != 1 || !samples[0].Truncated {
		t.Fatalf("samples = %+v", samples)
	}
	if !utf8.ValidString(samples[0].Source) {
		t.Fatalf("sample is not valid UTF-8: %q", samples[0].Source)
	}
	if strings.ContainsRune(samples[0].Source, utf8.RuneError) {
		t.Fatalf("sample contains a replacement character: %q", samples[0].Source)
	}
	if !strings.Contains(samples[0].Source, "[character offset 18666]") {
		t.Fatalf("tail offset was not snapped: %q", samples[0].Source)
	}
}

func TestPreviewDirectoryReportsListError(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		want enums.RetrievalIssueCode
	}{
		{name: "error", ctx: context.Background(), want: enums.IssueCodeUnreadable},
		{name: "cancelled", ctx: cancelled, want: enums.IssueCodeInterrupted},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var issues []enums.RetrievalIssueCode
			reader := listErrorReader{err: errors.New("boom")}
			preview := previewDirectory(testCase.ctx, reader, "pkg", func(kind enums.RetrievalIssueCode) {
				issues = append(issues, kind)
			})
			if preview == nil || len(preview.Entries) != 0 {
				t.Fatalf("preview = %+v", preview)
			}
			if len(issues) != 1 || issues[0] != testCase.want {
				t.Fatalf("issues = %v, want %q", issues, testCase.want)
			}
		})
	}
}

func TestPreviewDirectoryStopsAtByteBudget(t *testing.T) {
	root := t.TempDir()
	longName := strings.Repeat("n", 200) + ".ts"
	const count = 20
	for index := 0; index < count; index++ {
		name := filepath.Join(root, fmt.Sprintf("%02d-%s", index, longName))
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	preview := previewDirectory(context.Background(), reader, root, func(kind enums.RetrievalIssueCode) {
		t.Errorf("unexpected issue %q", kind)
	})
	if preview == nil || !preview.Truncated {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.Entries) == 0 || len(preview.Entries) >= count {
		t.Fatalf("entries = %d, want between 1 and %d", len(preview.Entries), count-1)
	}
	total := 0
	for _, entry := range preview.Entries {
		blob, err := jsonMarshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		total += len(blob)
	}
	if total > 4096 {
		t.Fatalf("entries use %d bytes, want <= 4096", total)
	}
	next, err := jsonMarshal(preview.Entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if total+len(next) <= 4096 {
		t.Fatalf("byte budget not reached: total=%d next=%d", total, len(next))
	}
}

func TestSplitTextNormalizesSmallBudgets(t *testing.T) {
	for _, budget := range []int{-5, 0, 4} {
		chunks := splitText("abcdefgh", budget)
		joined := strings.Builder{}
		for _, chunk := range chunks {
			if chunk.Text == "" {
				t.Fatalf("budget %d produced an empty chunk", budget)
			}
			if len(chunk.Text) > 4 {
				t.Fatalf("budget %d chunk = %d bytes", budget, len(chunk.Text))
			}
			joined.WriteString(chunk.Text)
		}
		if joined.String() != "abcdefgh" {
			t.Fatalf("budget %d joined = %q", budget, joined.String())
		}
	}

	multibyte := strings.Repeat("é", 6)
	for _, budget := range []int{0, 4} {
		chunks := splitText(multibyte, budget)
		if len(chunks) == 0 {
			t.Fatalf("budget %d produced no chunks", budget)
		}
		joined := strings.Builder{}
		for _, chunk := range chunks {
			if !utf8.ValidString(chunk.Text) {
				t.Fatalf("budget %d chunk is not valid UTF-8: %q", budget, chunk.Text)
			}
			if len(chunk.Text) > 4 {
				t.Fatalf("budget %d chunk = %d bytes", budget, len(chunk.Text))
			}
			joined.WriteString(chunk.Text)
		}
		if joined.String() != multibyte {
			t.Fatalf("budget %d joined = %q", budget, joined.String())
		}
	}
}

func TestPreviewFileShrinkLoopHandlesSingleByteText(t *testing.T) {
	path := "a." + strings.Repeat("x", PreviewJSONBytes+1024)
	preview := previewFile(files.Snapshot{Path: path, Source: "z"}, false)
	if !preview.Truncated || preview.Text != "" || preview.PreviewBytes != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	if !utf8.ValidString(preview.Text) {
		t.Fatalf("text is not valid UTF-8: %q", preview.Text)
	}
}

type snapshotIssueReader struct {
	result files.SnapshotResult
}

func (r snapshotIssueReader) Root() string { return "/" }

func (r snapshotIssueReader) ListPage(context.Context, string, int) (files.Page, error) {
	return files.Page{}, nil
}

func (r snapshotIssueReader) ReadSnapshotReuse(context.Context, string) files.SnapshotResult {
	return r.result
}

func (r snapshotIssueReader) LookupFile(context.Context, string) (files.LookupResult, error) {
	return files.LookupResult{}, nil
}

type listErrorReader struct {
	err error
}

func (r listErrorReader) Root() string { return "/" }

func (r listErrorReader) ListPage(context.Context, string, int) (files.Page, error) {
	return files.Page{}, r.err
}

func (r listErrorReader) ReadSnapshotReuse(context.Context, string) files.SnapshotResult {
	return files.SnapshotResult{}
}

func (r listErrorReader) LookupFile(context.Context, string) (files.LookupResult, error) {
	return files.LookupResult{}, nil
}

type pageIssueReader struct {
	page files.Page
}

func (r pageIssueReader) Root() string { return "/" }

func (r pageIssueReader) ListPage(context.Context, string, int) (files.Page, error) {
	return r.page, nil
}

func (r pageIssueReader) ReadSnapshotReuse(context.Context, string) files.SnapshotResult {
	return files.SnapshotResult{}
}

func (r pageIssueReader) LookupFile(context.Context, string) (files.LookupResult, error) {
	return files.LookupResult{}, nil
}
