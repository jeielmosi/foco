package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"foco/internal/enums"
	"foco/internal/provider"
	"foco/internal/retrieve"
)

func TestCategorySet(t *testing.T) {
	if got := categorySet(nil); got != nil {
		t.Fatalf("categorySet(nil) = %v, want nil", got)
	}

	want := map[enums.FileCategory]struct{}{
		enums.FileCategoryCode: {},
		enums.FileCategoryDocs: {},
	}
	got := categorySet([]enums.FileCategory{
		enums.FileCategoryCode,
		enums.FileCategoryDocs,
		enums.FileCategoryCode,
	})
	if len(got) != len(want) {
		t.Fatalf("categorySet() = %v, want %v", got, want)
	}
	for category := range want {
		if _, ok := got[category]; !ok {
			t.Errorf("categorySet() is missing %q", category)
		}
	}
}

func TestRunSearchMissingCredentials(t *testing.T) {
	setConfigHome(t)
	var output, errorOutput bytes.Buffer
	code := runSearch(context.Background(), SearchOptions{
		Query: "find the auth handler",
		Root:  t.TempDir(),
	}, &output, &errorOutput)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(output.String(), "Run foco auth or use foco auth --schema NAME --endpoint URL --model ID --stdin.") {
		t.Fatalf("output: %q", output.String())
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr: %q", errorOutput.String())
	}
}

func TestFinishSearchInterruptedAuthFailureReturns130(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status := enums.RetrievalStatusComplete
	if ctx.Err() != nil {
		status = enums.RetrievalStatusInterrupted
	}
	result := retrieve.RetrievalResult{
		Root:            ".",
		Query:           "find the auth handler",
		Status:          status,
		Issues:          []retrieve.Issue{{Kind: enums.IssueCodeAuthentication, Count: 1}},
		ProviderFailure: "API authentication failed.",
	}
	failure := &provider.Failure{
		Kind:    enums.FailureAuthentication,
		Message: "API authentication failed.",
	}
	var output, errorOutput bytes.Buffer
	progress := newProgress(true, &errorOutput)
	progress.Start()
	code := finishSearch(result, failure, SearchOptions{Query: result.Query, Root: result.Root}, &output, progress)
	if code != 130 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(output.String(), "Interrupted.") {
		t.Fatalf("output missing interrupted line: %q", output.String())
	}
	if !strings.Contains(output.String(), "API error:") {
		t.Fatalf("output missing provider failure: %q", output.String())
	}
	progress.mu.Lock()
	active := progress.active
	progress.mu.Unlock()
	if active {
		t.Fatal("progress still active")
	}
}

func TestFinishSearchAuthFailureReturns1(t *testing.T) {
	failure := &provider.Failure{
		Kind:    enums.FailureAuthentication,
		Message: "API authentication failed.",
	}
	var output bytes.Buffer
	code := finishSearch(retrieve.RetrievalResult{Status: enums.RetrievalStatusIncomplete}, failure, SearchOptions{}, &output, newProgress(false, &output))
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(output.String(), "API authentication failed.") {
		t.Fatalf("output: %q", output.String())
	}
	if strings.Contains(output.String(), "Foco:") {
		t.Fatalf("result rendered on fatal error: %q", output.String())
	}
}

func TestFinishSearchWrappedContextErrorReturns130(t *testing.T) {
	result := retrieve.RetrievalResult{Status: enums.RetrievalStatusIncomplete}
	var output bytes.Buffer
	code := finishSearch(result, fmt.Errorf("retrieval: %w", context.Canceled), SearchOptions{}, &output, newProgress(false, &output))
	if code != 130 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(output.String(), "Foco:") {
		t.Fatalf("partial result not rendered: %q", output.String())
	}
	if !strings.Contains(output.String(), "Interrupted.") {
		t.Fatalf("interrupted line missing: %q", output.String())
	}
	if !strings.HasSuffix(output.String(), "\n\nEnd context.\n") {
		t.Fatalf("missing terminator: %q", output.String())
	}
}

func TestFinishSearchIncompleteOperationalIssueReturnsTwo(t *testing.T) {
	result := retrieve.RetrievalResult{
		Status: enums.RetrievalStatusIncomplete,
		Issues: []retrieve.Issue{{Kind: enums.IssueCodeChanged, Count: 1}},
	}
	var output bytes.Buffer
	code := finishSearch(result, nil, SearchOptions{}, &output, newProgress(false, &output))
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(output.String(), `Issue: "changed": 1`) {
		t.Fatalf("missing issue: %q", output.String())
	}
	if !strings.HasSuffix(output.String(), "\n\nEnd context.\n") {
		t.Fatalf("missing terminator: %q", output.String())
	}
}

func TestFinishSearchStopsProgressBeforeRendering(t *testing.T) {
	var stream bytes.Buffer
	progress := newProgress(true, &stream)
	progress.Start()
	code := finishSearch(retrieve.RetrievalResult{Status: enums.RetrievalStatusComplete}, nil, SearchOptions{}, &stream, progress)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	rendered := strings.Index(stream.String(), "Foco:")
	cleared := strings.LastIndex(stream.String(), "\r\033[2K")
	if rendered < 0 || cleared < 0 {
		t.Fatalf("missing render or progress clear: %q", stream.String())
	}
	if cleared > rendered {
		t.Fatalf("progress cleared after rendering: %q", stream.String())
	}
}
