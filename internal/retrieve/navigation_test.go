package retrieve

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"foco/internal/enums"
	"foco/internal/provider"
	"foco/internal/requests"
)

type scriptedEvaluator struct {
	mu       sync.Mutex
	requests int
	answers  func(provider.Request) (map[string]float64, error)
}

func (e *scriptedEvaluator) Requests() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.requests
}

func (e *scriptedEvaluator) Evaluate(_ context.Context, request provider.Request, _ provider.EvalOptions) (map[string]float64, error) {
	e.mu.Lock()
	e.requests++
	e.mu.Unlock()
	return e.answers(request)
}

func TestNavigationAdmissions(t *testing.T) {
	items := []scoredItem{
		{item: requests.NavigationItem{Path: "strong", Kind: enums.NavigationKindFile}, score: 0.9},
		{item: requests.NavigationItem{Path: "boundary", Kind: enums.NavigationKindFile}, score: 0.6},
		{item: requests.NavigationItem{Path: "positive", Kind: enums.NavigationKindFile}, score: 0.5},
		{item: requests.NavigationItem{Path: "weak", Kind: enums.NavigationKindFile}, score: 0.55},
	}
	admitted, suppress := navigationAdmissions(items, true)
	if suppress {
		t.Fatal("strong evidence must not suppress weak scores")
	}
	if len(admitted) != 3 || admitted[0].item.Path != "strong" || admitted[1].item.Path != "boundary" || admitted[2].item.Path != "weak" {
		t.Fatalf("admitted = %+v", admitted)
	}
	weak := make([]scoredItem, MinimumPositiveScores)
	for index := range weak {
		weak[index] = scoredItem{item: requests.NavigationItem{Path: "weak", Kind: enums.NavigationKindFile}, score: 0.55}
	}
	admitted, suppress = navigationAdmissions(weak, false)
	if !suppress || len(admitted) != 0 {
		t.Fatalf("suppress=%v admitted=%d, want true/0", suppress, len(admitted))
	}
}

func TestNavigatorConsumesDiscoveryOrderAcrossBatches(t *testing.T) {
	items := make([]requests.NavigationItem, 130)
	for index := range items {
		items[index] = requests.NavigationItem{Path: fmt.Sprintf("f%03d.ts", index), Kind: enums.NavigationKindFile}
	}
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		out := map[string]float64{}
		for _, question := range request.Questions {
			out[question.ID] = 0.9
		}
		return out, nil
	}}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	navigator := newNavigator(context.Background(), "q", evaluator, issues, nil, nil, nil)
	var seen []string
	if err := navigator.score(items, nil, nil, func(item requests.NavigationItem, score float64) {
		if score != 0.9 {
			t.Fatalf("score = %v", score)
		}
		seen = append(seen, item.Path)
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 130 {
		t.Fatalf("seen = %d", len(seen))
	}
	for index := range items {
		if seen[index] != items[index].Path {
			t.Fatalf("seen[%d] = %q, want %q", index, seen[index], items[index].Path)
		}
	}
	if evaluator.requests != 2 {
		t.Fatalf("requests = %d, want 2 batches", evaluator.requests)
	}
}

func TestNavigatorSplitsFreshnessFailure(t *testing.T) {
	items := []requests.NavigationItem{{Path: "a.ts", Kind: enums.NavigationKindFile}, {Path: "b.ts", Kind: enums.NavigationKindFile}}
	attempts := 0
	validator := func(context.Context, []Donor) bool {
		attempts++
		return attempts > 1
	}
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		out := map[string]float64{}
		for _, question := range request.Questions {
			out[question.ID] = 0.7
		}
		return out, nil
	}}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	navigator := newNavigator(context.Background(), "q", evaluator, issues, nil, validator, nil)
	var seen []string
	_ = navigator.score(items, nil, nil, func(item requests.NavigationItem, _ float64) {
		seen = append(seen, item.Path)
	})
	if len(seen) != 2 {
		t.Fatalf("seen = %+v", seen)
	}
	if attempts != 3 {
		t.Fatalf("validations = %d, want split + two items", attempts)
	}
}

func TestNavigatorIssuesLowConfidenceBudgetOnce(t *testing.T) {
	items := []requests.NavigationItem{{Path: "a.ts", Kind: enums.NavigationKindFile}}
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		return map[string]float64{"q0": 0.55}, nil
	}}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	navigator := newNavigator(context.Background(), "q", evaluator, issues, nil, nil, func(context.Context, string) probeOutcome { return probeOutcome{} })
	navigator.positive = MinimumPositiveScores
	navigator.bytesSeen = NavigationByteBudget
	_ = navigator.score(items, nil, func() []string { return []string{"a.ts"} }, func(requests.NavigationItem, float64) {})
	if issues.counts[enums.IssueCodeLowConfidenceBudget] != 1 {
		t.Fatalf("issues = %+v", issues.counts)
	}
}

func TestRequestSizeMarshalFailureForcesRequestSizeSkip(t *testing.T) {
	size := requestSize(provider.Request{State: make(chan int)})
	if size <= NavigationBatchBytes {
		t.Fatalf("size = %d, want greater than %d", size, NavigationBatchBytes)
	}
}

func TestNavigatorBudgetResumesAfterConfirmation(t *testing.T) {
	items := []requests.NavigationItem{{Path: "a.ts", Kind: enums.NavigationKindFile}}
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		return map[string]float64{"q0": 0.9}, nil
	}}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	probe := func(context.Context, string) probeOutcome { return probeOutcome{confident: true} }
	navigator := newNavigator(context.Background(), "q", evaluator, issues, nil, nil, probe)
	navigator.positive = MinimumPositiveScores
	navigator.bytesSeen = NavigationByteBudget
	var seen []string
	if err := navigator.score(items, nil, func() []string { return []string{"a.ts"} }, func(item requests.NavigationItem, _ float64) {
		seen = append(seen, item.Path)
	}); err != nil {
		t.Fatal(err)
	}
	if evaluator.Requests() != 1 || len(seen) != 1 {
		t.Fatalf("requests = %d, seen = %+v", evaluator.Requests(), seen)
	}
	if issues.counts[enums.IssueCodeLowConfidenceBudget] != 0 {
		t.Fatalf("issues = %+v", issues.counts)
	}
	if want := NavigationByteBudget + requestSize(requests.NavigationRequest("q", items, nil)); navigator.bytesSeen != want {
		t.Fatalf("bytesSeen = %d, want %d", navigator.bytesSeen, want)
	}
}

func TestNavigatorProbesWaveConcurrentlyButAppliesInOrder(t *testing.T) {
	large := strings.Repeat("x", 30_000)
	items := []requests.NavigationItem{
		{Path: "a.ts", Kind: enums.NavigationKindFile, FilePreview: &requests.FilePreview{Text: large}},
		{Path: "b.ts", Kind: enums.NavigationKindFile, FilePreview: &requests.FilePreview{Text: large}},
		{Path: "c.ts", Kind: enums.NavigationKindFile, FilePreview: &requests.FilePreview{Text: large}},
	}
	if groups := groupNavigation("q", items, nil, func(enums.RetrievalIssueCode) {}); len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		return map[string]float64{"q0": 0.9}, nil
	}}
	var mu sync.Mutex
	started := map[string]bool{}
	applied := []string{}
	all := make(chan struct{})
	probe := func(_ context.Context, path string) probeOutcome {
		mu.Lock()
		started[path] = true
		if len(started) == 3 {
			close(all)
		}
		mu.Unlock()
		<-all
		return probeOutcome{confident: path == "b.ts", apply: func() { applied = append(applied, path) }}
	}
	navigator := newNavigator(context.Background(), "q", evaluator, &issueSet{counts: map[enums.RetrievalIssueCode]int{}}, nil, nil, probe)
	if err := navigator.score(items, nil, nil, func(requests.NavigationItem, float64) {}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(applied, ",") != "a.ts,b.ts" || !navigator.confident {
		t.Fatalf("applied = %v, confident = %v", applied, navigator.confident)
	}
	if _, ok := navigator.probes["c.ts"]; ok || len(navigator.probes) != 2 {
		t.Fatalf("probes = %v", navigator.probes)
	}
}

func TestNavigatorChargesEverySplitRequest(t *testing.T) {
	items := []requests.NavigationItem{{Path: "a.ts", Kind: enums.NavigationKindFile}, {Path: "b.ts", Kind: enums.NavigationKindFile}}
	attempts := 0
	validator := func(context.Context, []Donor) bool {
		attempts++
		return attempts > 1
	}
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		out := map[string]float64{}
		for _, question := range request.Questions {
			out[question.ID] = 0.7
		}
		return out, nil
	}}
	navigator := newNavigator(context.Background(), "q", evaluator, &issueSet{counts: map[enums.RetrievalIssueCode]int{}}, nil, validator, nil)
	if err := navigator.score(items, nil, nil, func(requests.NavigationItem, float64) {}); err != nil {
		t.Fatal(err)
	}
	want := requestSize(requests.NavigationRequest("q", items, nil)) +
		requestSize(requests.NavigationRequest("q", items[:1], nil)) +
		requestSize(requests.NavigationRequest("q", items[1:], nil))
	if navigator.bytesSeen != want {
		t.Fatalf("bytesSeen = %d, want %d", navigator.bytesSeen, want)
	}
}

func TestNavigatorBudgetRejectsUnchargedSplitRequest(t *testing.T) {
	items := []requests.NavigationItem{{Path: "a.ts", Kind: enums.NavigationKindFile}, {Path: "b.ts", Kind: enums.NavigationKindFile}}
	attempts := 0
	validator := func(context.Context, []Donor) bool {
		attempts++
		return attempts > 1
	}
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		return map[string]float64{"q0": 0.7}, nil
	}}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	navigator := newNavigator(context.Background(), "q", evaluator, issues, nil, validator, nil)
	navigator.positive = MinimumPositiveScores
	parent := requestSize(requests.NavigationRequest("q", items, nil))
	navigator.bytesSeen = NavigationByteBudget - parent
	var seen []string
	_ = navigator.score(items, nil, nil, func(item requests.NavigationItem, _ float64) {
		seen = append(seen, item.Path)
	})
	if evaluator.Requests() != 0 || len(seen) != 0 {
		t.Fatalf("requests = %d, seen = %+v", evaluator.Requests(), seen)
	}
	if navigator.bytesSeen != NavigationByteBudget {
		t.Fatalf("bytesSeen = %d, want %d", navigator.bytesSeen, NavigationByteBudget)
	}
	if issues.counts[enums.IssueCodeLowConfidenceBudget] != 1 {
		t.Fatalf("issues = %+v", issues.counts)
	}
}

func TestNavigatorAnchorDonorsReachValidation(t *testing.T) {
	item := requests.NavigationItem{Path: "child.ts", Kind: enums.NavigationKindFile}
	anchor := &requests.RelationAnchor{Path: "anchor.ts", Classes: []string{"Widget"}}
	donors := func(item requests.NavigationItem) []Donor {
		switch item.Path {
		case "child.ts":
			return []Donor{{Path: "child.ts", ContentHash: "child"}}
		case "anchor.ts":
			return []Donor{{Path: "anchor.ts", ContentHash: "anchor"}}
		}
		return nil
	}
	var validated [][]Donor
	validator := func(_ context.Context, sources []Donor) bool {
		validated = append(validated, append([]Donor(nil), sources...))
		return false
	}
	evaluator := &scriptedEvaluator{answers: func(provider.Request) (map[string]float64, error) {
		return nil, fmt.Errorf("stale anchor must block evaluation")
	}}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	navigator := newNavigator(context.Background(), "q", evaluator, issues, donors, validator, nil)
	var seen []string
	if err := navigator.score([]requests.NavigationItem{item}, anchor, nil, func(item requests.NavigationItem, _ float64) {
		seen = append(seen, item.Path)
	}); err != nil {
		t.Fatal(err)
	}
	if evaluator.Requests() != 0 || len(seen) != 0 {
		t.Fatalf("requests = %d, seen = %+v", evaluator.Requests(), seen)
	}
	if len(validated) != 1 || len(validated[0]) != 2 || validated[0][0].Path != "child.ts" || validated[0][1].Path != "anchor.ts" {
		t.Fatalf("validated = %+v", validated)
	}
	if len(issues.counts) != 0 {
		t.Fatalf("issues = %+v", issues.counts)
	}
}

func TestNavigatorPreservesSiblingScoresAfterFatalFailure(t *testing.T) {
	items := make([]requests.NavigationItem, NavigationBatchSize+1)
	for index := range items {
		items[index] = requests.NavigationItem{Path: fmt.Sprintf("f%03d.ts", index), Kind: enums.NavigationKindFile}
	}
	evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
		state := request.State.(requests.NavigationRequestState)
		for _, item := range state.Items {
			if item.Path == items[0].Path {
				return nil, &provider.Failure{Kind: enums.FailureAuthentication}
			}
		}
		out := map[string]float64{}
		for _, question := range request.Questions {
			out[question.ID] = 0.9
		}
		return out, nil
	}}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	navigator := newNavigator(context.Background(), "q", evaluator, issues, nil, nil, nil)
	var seen []string
	if err := navigator.score(items, nil, nil, func(item requests.NavigationItem, _ float64) {
		seen = append(seen, item.Path)
	}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, path := range seen {
		if path == items[NavigationBatchSize].Path {
			found = true
		}
		if path == items[0].Path {
			t.Fatalf("consumed score from failed group: %+v", seen)
		}
	}
	if !found {
		t.Fatalf("sibling scores were not consumed: %+v", seen)
	}
	if issues.counts[enums.IssueCodeAuthentication] != 1 {
		t.Fatalf("issues = %+v", issues.counts)
	}
}

func TestNavigatorSplitFailureSelection(t *testing.T) {
	cases := []struct {
		name    string
		left    error
		right   error
		kind    enums.ProviderFailureKind
		message string
	}{
		{"prefers fatal right", &provider.Failure{Kind: enums.FailureProvider}, &provider.Failure{Kind: enums.FailureAuthentication}, enums.FailureAuthentication, ""},
		{"keeps fatal left", &provider.Failure{Kind: enums.FailureAuthentication}, &provider.Failure{Kind: enums.FailureProvider}, enums.FailureAuthentication, ""},
		{"reports left", &provider.Failure{Kind: enums.FailureProvider, Message: "left"}, nil, enums.FailureProvider, "left"},
		{"reports right", nil, &provider.Failure{Kind: enums.FailureProvider, Message: "right"}, enums.FailureProvider, "right"},
		{"prefers left when both non fatal", &provider.Failure{Kind: enums.FailureProvider, Message: "left"}, &provider.Failure{Kind: enums.FailureProvider, Message: "right"}, enums.FailureProvider, "left"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evaluator := &scriptedEvaluator{answers: func(request provider.Request) (map[string]float64, error) {
				state, ok := request.State.(requests.NavigationRequestState)
				if !ok || len(state.Items) != 1 {
					return nil, fmt.Errorf("unexpected split request")
				}
				failure := tc.left
				if state.Items[0].Path == "right.ts" {
					failure = tc.right
				}
				if failure != nil {
					return nil, failure
				}
				return map[string]float64{"q0": 0.7}, nil
			}}
			navigator := newNavigator(context.Background(), "q", evaluator, &issueSet{counts: map[enums.RetrievalIssueCode]int{}}, nil, nil, nil)
			outcome := navigator.evalSplit([]requests.NavigationItem{
				{Path: "left.ts", Kind: enums.NavigationKindFile},
				{Path: "right.ts", Kind: enums.NavigationKindFile},
			}, nil, nil)
			if outcome.failure == nil || outcome.failure.Kind != tc.kind || outcome.failure.Message != tc.message {
				t.Fatalf("failure = %+v, want kind %q message %q", outcome.failure, tc.kind, tc.message)
			}
		})
	}
}

func TestNavigatorCancellationStopsAndIssues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	evaluator := &scriptedEvaluator{answers: func(provider.Request) (map[string]float64, error) {
		return nil, fmt.Errorf("cancelled context must block evaluation")
	}}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	navigator := newNavigator(ctx, "q", evaluator, issues, nil, nil, nil)
	var seen []string
	if err := navigator.score([]requests.NavigationItem{{Path: "a.ts", Kind: enums.NavigationKindFile}}, nil, nil, func(item requests.NavigationItem, _ float64) {
		seen = append(seen, item.Path)
	}); err != nil {
		t.Fatal(err)
	}
	if evaluator.Requests() != 0 || len(seen) != 0 {
		t.Fatalf("requests = %d, seen = %+v", evaluator.Requests(), seen)
	}
	if issues.counts[enums.IssueCodeCancelled] != 1 {
		t.Fatalf("issues = %+v", issues.counts)
	}
}

func TestNavigatorDiscardsSourceInvalidFailure(t *testing.T) {
	evaluator := &scriptedEvaluator{answers: func(provider.Request) (map[string]float64, error) {
		return nil, &provider.Failure{Kind: enums.FailureSourceInvalid}
	}}
	issues := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	navigator := newNavigator(context.Background(), "q", evaluator, issues, nil, nil, nil)
	if err := navigator.score([]requests.NavigationItem{{Path: "a.ts", Kind: enums.NavigationKindFile}}, nil, nil, func(requests.NavigationItem, float64) {}); err != nil {
		t.Fatal(err)
	}
	if got := issues.list(); len(got) != 0 {
		t.Fatalf("issues = %+v, want source-invalid discarded", got)
	}
}
