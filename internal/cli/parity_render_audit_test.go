package cli

import (
	"strings"
	"testing"

	"foco/internal/enums"
	"foco/internal/retrieve"
)

func TestParityNoMatchOutputIsExact(t *testing.T) {
	want := "Foco: 0 relevant files.\n" +
		"No confident match found under this root.\n" +
		"Roles are estimates. Declaration locations omitted.\n" +
		"AGENTS.md lookup (root and returned-file ancestors): none found.\n" +
		"End file list.\n\nEnd context.\n"
	got := RenderResult(retrieve.RetrievalResult{Status: enums.RetrievalStatusComplete}, 0, 0, false)
	if got != want {
		t.Fatalf("no-match output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestParityLowConfidenceWarningAndIssue(t *testing.T) {
	result := retrieve.RetrievalResult{
		Status: enums.RetrievalStatusComplete,
		Issues: []retrieve.Issue{{Kind: enums.IssueCodeLowConfidenceBudget, Count: 1}},
		Files: []retrieve.FileEvidence{{
			Path: "src/a.go", Score: 0.9, Roles: []enums.AssessmentRole{enums.AssessmentRoleImplementation},
		}},
	}
	output := RenderResult(result, 0, 0, false)
	const warning = "No confident match within the navigation budget (8000000 serialized bytes); narrow the root to continue."
	if !strings.Contains(output, warning) {
		t.Fatalf("missing low-confidence warning:\n%s", output)
	}
	if !strings.Contains(output, `Issue: "low_confidence_budget": 1`) {
		t.Fatalf("missing low-confidence issue:\n%s", output)
	}
}

func TestParityMultipleInstructionFilesAndIncompleteLookup(t *testing.T) {
	result := retrieve.RetrievalResult{
		Status: enums.RetrievalStatusComplete,
		RepositoryContext: retrieve.RepositoryContext{
			InstructionFiles:            []string{"AGENTS.md", "tests/AGENTS.md"},
			InstructionLookupIncomplete: true,
		},
	}
	output := RenderResult(result, 0, 0, false)
	const want = `AGENTS.md lookup (root and returned-file ancestors): "AGENTS.md", "tests/AGENTS.md"; lookup incomplete.`
	if !strings.Contains(output, want) {
		t.Fatalf("instruction lookup line mismatch:\n--- got ---\n%s\n--- want ---\n%s", output, want)
	}
}

func TestParityPathOnlyAndUncertainRole(t *testing.T) {
	result := retrieve.RetrievalResult{
		Status: enums.RetrievalStatusComplete,
		Files: []retrieve.FileEvidence{
			{Path: "src/a.go", Score: 0.9},
			{Path: "src/b.go", Score: 0.8, Roles: []enums.AssessmentRole{enums.AssessmentRoleImplementation}},
		},
	}
	output := RenderResult(result, 0, 0, false)
	if !strings.Contains(output, `- "src/a.go" — relevant; role uncertain; path only`) {
		t.Fatalf("missing uncertain-role path-only line:\n%s", output)
	}
	if !strings.Contains(output, `- "src/b.go" — implementation; path only`) {
		t.Fatalf("missing role path-only line:\n%s", output)
	}
}

func TestParityPriorityControlsFileOrder(t *testing.T) {
	low, high := 0.1, 0.95
	result := retrieve.RetrievalResult{
		Status: enums.RetrievalStatusComplete,
		Files: []retrieve.FileEvidence{
			{Path: "specs/telemetry.md", Score: 0.99, Priority: &low},
			{Path: "src/telemetry.ts", Score: 0.10, Priority: &high},
		},
	}
	output := RenderResult(result, 0, 0, false)
	highAt := strings.Index(output, `- "src/telemetry.ts"`)
	lowAt := strings.Index(output, `- "specs/telemetry.md"`)
	if highAt < 0 || lowAt < 0 {
		t.Fatalf("priority markers missing (high=%d low=%d):\n%s", highAt, lowAt, output)
	}
	if highAt > lowAt {
		t.Fatalf("priority did not order files:\n%s", output)
	}
}

func TestParityProviderErrorIsQuoted(t *testing.T) {
	result := retrieve.RetrievalResult{
		Status:          enums.RetrievalStatusIncomplete,
		Issues:          []retrieve.Issue{{Kind: enums.IssueCodeProvider, Count: 1}},
		ProviderFailure: `bad "thing"`,
	}
	output := RenderResult(result, 0, 0, false)
	if !strings.Contains(output, `API error: "bad \"thing\""`) {
		t.Fatalf("provider error not quoted:\n%s", output)
	}
	if !strings.Contains(output, `Issue: "provider": 1`) {
		t.Fatalf("provider issue missing:\n%s", output)
	}
}

func TestParityInterruptedStatusRendersAndExits130(t *testing.T) {
	result := retrieve.RetrievalResult{
		Status: enums.RetrievalStatusInterrupted,
		Issues: []retrieve.Issue{{Kind: enums.IssueCodeInterrupted, Count: 1}},
	}
	output := RenderResult(result, 0, 0, false)
	if !strings.Contains(output, "Foco: 0 relevant files; discovery incomplete.") {
		t.Fatalf("missing incomplete header:\n%s", output)
	}
	if !strings.Contains(output, "Interrupted.") {
		t.Fatalf("missing interrupted line:\n%s", output)
	}
	if !strings.Contains(output, `Issue: "interrupted": 1`) {
		t.Fatalf("missing interrupted issue:\n%s", output)
	}
	if code := exitCodeForResult(result); code != 130 {
		t.Fatalf("exit code = %d, want 130", code)
	}
}
