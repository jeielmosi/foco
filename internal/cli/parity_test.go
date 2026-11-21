package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"foco/internal/enums"
	"foco/internal/files"
	"foco/internal/provider"
	"foco/internal/requests"
	"foco/internal/retrieve"
)

func itoa(value int) string { return strconv.Itoa(value) }

type parityEvaluator struct {
	mu       sync.Mutex
	requests int
	sent     []string
}

func (e *parityEvaluator) Requests() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.requests
}

func (e *parityEvaluator) Evaluate(_ context.Context, request provider.Request, _ provider.EvalOptions) (map[string]float64, error) {
	blob, _ := json.Marshal(request)
	e.mu.Lock()
	e.requests++
	e.sent = append(e.sent, string(blob))
	e.mu.Unlock()
	out := map[string]float64{}
	switch state := request.State.(type) {
	case requests.NavigationRequestState:
		for index, item := range state.Items {
			value := 0.1
			if item.Kind == enums.NavigationKindFile && strings.HasSuffix(item.Path, ".ts") {
				value = 0.9
			}
			out["q"+itoa(index)] = value
		}
	case requests.AssessmentState:
		out["implementation"], out["priority"] = 0.9, 0.9
		out["caller"], out["test"], out["fixture"], out["helper"] = 0.1, 0.1, 0.1, 0.1
	case requests.EvidenceState:
		for index := range state.Declarations {
			out["q"+itoa(index)] = 0.9
			out["scope"+itoa(index)] = 0.9
			if state.SelectedEvidence != nil {
				out["ref"+itoa(index)] = 0.9
			}
		}
	}
	return out, nil
}

func parityTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src/events.ts"), []byte("export function record(name: string) { return name; }"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("skipped.ts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skipped.ts"), []byte("DO_NOT_UPLOAD_IGNORED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DO_NOT_UPLOAD_SECRET\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runParity(t *testing.T, query string) (retrieve.RetrievalResult, *parityEvaluator) {
	t.Helper()
	root := parityTree(t)
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	evaluator := &parityEvaluator{}
	result, err := retrieve.Run(context.Background(), retrieve.Input{Query: query}, evaluator, reader)
	if err != nil {
		t.Fatal(err)
	}
	return result, evaluator
}

func TestParityBasicOutputMatchesGolden(t *testing.T) {
	result, _ := runParity(t, "event recording")
	golden, err := os.ReadFile("../../testdata/search/goldens/basic.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := RenderResult(result, 0, 0, false); got != string(golden) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, string(golden))
	}
}

func TestParityExcludedFilesNeverReachProvider(t *testing.T) {
	_, evaluator := runParity(t, "event recording")
	if evaluator.Requests() == 0 {
		t.Fatal("evaluator received no requests; exclusion check would be vacuous")
	}
	payloads := strings.Join(evaluator.sent, "\n")
	if !strings.Contains(payloads, "src/events.ts") {
		t.Fatalf("included file src/events.ts missing from provider payloads")
	}
	for _, forbidden := range []string{"DO_NOT_UPLOAD", "skipped.ts", ".env"} {
		if strings.Contains(payloads, forbidden) {
			t.Fatalf("%q leaked into provider payloads", forbidden)
		}
	}
}

// Spec §11 orders "Source omitted: N file(s)." (item 7), then each
// `Issue: "<kind>": <count>` line (item 8), then the file list (item 10),
// and always closes with "\n\nEnd context.\n".
func TestParityIssueAndSourceOmittedLinesMatchSpecOrder(t *testing.T) {
	result := retrieve.RetrievalResult{
		Status: enums.RetrievalStatusIncomplete,
		Issues: []retrieve.Issue{
			{Kind: enums.IssueCodeChanged, Count: 2},
			{Kind: enums.IssueCodeResourceLimit, Count: 1},
		},
		Files: []retrieve.FileEvidence{
			{
				Path: "src/kept.ts", Score: 0.9, Roles: []enums.AssessmentRole{enums.AssessmentRoleImplementation},
				PresentationExcerpts: []retrieve.Excerpt{{
					Range:  retrieve.EvidenceRange{StartLine: 1, EndLine: 1},
					Source: "kept",
				}},
			},
			{
				Path: "src/omitted.ts", Score: 0.8, Roles: []enums.AssessmentRole{enums.AssessmentRoleHelper},
				SourceOmitted: true,
			},
		},
	}
	want := `Foco: 2 relevant files; discovery incomplete.
Roles are estimates. Declaration locations omitted.
AGENTS.md lookup (root and returned-file ancestors): none found.
Source omitted: 1 file(s).
Issue: "changed": 2
Issue: "resource_limit": 1
- "src/kept.ts" — implementation; source below
- "src/omitted.ts" — helper; source omitted
End file list.

Source block "src/kept.ts" lines 1-1:
` + "```" + `
kept
` + "```" + `

End context.
`
	if got := RenderResult(result, 0, 0, false); got != want {
		t.Fatalf("parity output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// Upstream counts a newline-terminated one-line file as two lines. This locks
// `lines 1-2` for the normal trailing-newline case while basic.txt keeps the
// no-trailing-newline `lines 1-1` case.
func TestParityNewlineTerminatedSourceCountsTrailingLine(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	const source = "export function record(name: string) { return name; }\n"
	if err := os.WriteFile(filepath.Join(root, "src/events.ts"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := files.New(root, files.DefaultPolicy(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	result, err := retrieve.Run(context.Background(), retrieve.Input{Query: "event recording"}, &parityEvaluator{}, reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) == 0 {
		t.Fatal("fixture produced no files")
	}
	output := RenderResult(result, 0, 0, false)
	want := "Source block \"src/events.ts\" lines 1-2:\n" +
		"```\n" +
		"export function record(name: string) { return name; }\n" +
		"\n" +
		"```"
	if !strings.Contains(output, want) {
		t.Fatalf("trailing-newline line count mismatch:\n%s", output)
	}
	if !strings.HasSuffix(output, "\n\nEnd context.\n") {
		t.Fatalf("missing terminator:\n%s", output)
	}
}

func TestParityTruncationSuffixIsExact(t *testing.T) {
	result, _ := runParity(t, "event recording")
	if len(result.Files) == 0 {
		t.Fatal("fixture produced no files")
	}
	long := strings.Repeat("x\n", 200000)
	result.Files[0].PresentationExcerpts = []retrieve.Excerpt{{
		Range: retrieve.EvidenceRange{StartLine: 1, EndLine: 200001}, Source: long,
	}}
	full := RenderResult(result, 0, 0, false)
	const limit = 4096
	output := RenderResult(result, 0, limit, false)
	if len(output) > limit {
		t.Fatalf("output = %d bytes", len(output))
	}
	if strings.Count(output, "Output truncated at the byte limit (") != 1 {
		t.Fatalf("truncation marker not unique:\n%s", output[max(0, len(output)-200):])
	}
	const marker = "\nOutput truncated at the byte limit (4096 bytes); "
	index := strings.Index(output, marker)
	if index < 0 {
		t.Fatalf("missing truncation marker")
	}
	prefix := output[:index]
	prefixBytes := len(prefix)
	trimmed := strings.TrimSuffix(prefix, "\n")
	lastLine := trimmed[strings.LastIndex(trimmed, "\n")+1:]
	if isFenceLine(lastLine) {
		prefixBytes -= len(lastLine) + 1
	}
	wantSuffix := fmt.Sprintf("%s%d original bytes omitted.\n\nEnd context.\n", marker, len(full)-prefixBytes)
	if !strings.HasSuffix(output, wantSuffix) {
		t.Fatalf("truncation suffix mismatch:\n--- got tail ---\n%s\n--- want tail ---\n%s", output[max(0, len(output)-200):], wantSuffix)
	}
}
