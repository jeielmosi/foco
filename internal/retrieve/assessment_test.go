package retrieve

import (
	"context"
	"testing"

	"foco/internal/enums"
	"foco/internal/provider"
	"foco/internal/requests"
)

func TestAssessFileKeepsRoleOrderAndExcludesPriority(t *testing.T) {
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		return map[string]float64{
			"implementation": 0.8, "caller": 0.2, "test": 0.9, "fixture": 0.1,
			"helper": 0.6, "priority": 0.95,
		}, nil
	}}
	got, failure := assessFile(context.Background(), evaluator, "q", candidate{Path: "a.ts"}, &requests.FilePreview{Text: "x"})
	if failure != nil {
		t.Fatal(failure)
	}
	want := []enums.AssessmentRole{
		enums.AssessmentRoleImplementation, enums.AssessmentRoleTest, enums.AssessmentRoleHelper,
	}
	if len(got.Labels) != len(want) {
		t.Fatalf("labels = %v", got.Labels)
	}
	for index := range want {
		if got.Labels[index] != want[index] {
			t.Fatalf("labels = %v, want %v", got.Labels, want)
		}
	}
	if got.Priority != 0.95 {
		t.Fatalf("priority = %v", got.Priority)
	}
}
