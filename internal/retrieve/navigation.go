package retrieve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"

	"foco/internal/enums"
	"foco/internal/provider"
	"foco/internal/requests"
)

type scoredItem struct {
	item  requests.NavigationItem
	score float64
}

func navigationAdmissions(classified []scoredItem, hasStrongEvidence bool) (admitted []scoredItem, suppressWeak bool) {
	weak := 0
	for _, entry := range classified {
		if entry.score > RelevanceBoundary && entry.score < WeakScoreCeiling {
			weak++
		}
	}
	suppressWeak = !hasStrongEvidence && weak >= MinimumPositiveScores
	for _, entry := range classified {
		if entry.score >= WeakScoreCeiling || (!suppressWeak && entry.score > RelevanceBoundary) {
			admitted = append(admitted, entry)
		}
	}
	return admitted, suppressWeak
}

type navigator struct {
	ctx       context.Context
	query     string
	evaluator Evaluator
	issues    *issueSet
	donors    func(requests.NavigationItem) []Donor
	validate  func(context.Context, []Donor) bool
	probe     func(context.Context, string) probeOutcome

	mu        sync.Mutex
	bytesSeen int
	positive  int
	strong    bool
	confident bool
	paused    bool
	stopped   bool
	probes    map[string]struct{}
	settle    sync.Mutex

	batchSize  int
	batchBytes int
}

func newNavigator(
	ctx context.Context, query string, evaluator Evaluator, issues *issueSet,
	donors func(requests.NavigationItem) []Donor,
	validate func(context.Context, []Donor) bool,
	probe func(context.Context, string) probeOutcome,
) *navigator {
	return &navigator{
		ctx: ctx, query: query, evaluator: evaluator, issues: issues,
		donors: donors, validate: validate, probe: probe, probes: map[string]struct{}{},
		batchSize: NavigationBatchSize, batchBytes: NavigationBatchBytes,
	}
}

func requestSize(request provider.Request) int {
	blob, err := json.Marshal(request)
	if err != nil {
		return math.MaxInt
	}
	return len(blob)
}

func groupNavigation(query string, items []requests.NavigationItem, anchor *requests.RelationAnchor, batchSize, batchBytes int, issue func(enums.RetrievalIssueCode)) [][]requests.NavigationItem {
	var groups [][]requests.NavigationItem
	current := []requests.NavigationItem{}
	flush := func() {
		if len(current) > 0 {
			groups = append(groups, current)
			current = []requests.NavigationItem{}
		}
	}
	for _, item := range items {
		if requestSize(requests.NavigationRequest(query, []requests.NavigationItem{item}, anchor)) > batchBytes {
			issue(enums.IssueCodeRequestSize)
			continue
		}
		if len(current) > 0 {
			candidate := append(append([]requests.NavigationItem{}, current...), item)
			if len(current) >= min(batchSize, MaxQuestionsPerRequest) ||
				requestSize(requests.NavigationRequest(query, candidate, anchor)) > batchBytes {
				flush()
			}
		}
		current = append(current, item)
	}
	flush()
	return groups
}

type groupOutcome struct {
	scores  map[int]float64
	failure *provider.Failure
	paused  bool
}

// charge reserves a request's serialized size before it is sent. The budget is
// only enforced while there is positive evidence but no confirmed source. When
// the request would exceed the budget, navigation pauses and final confirmation
// decides whether the request may still be sent; otherwise the navigator stops
// and reports low_confidence_budget exactly once.
func (n *navigator) charge(size int, finalChoices func() []string) bool {
	n.mu.Lock()
	if n.stopped {
		n.mu.Unlock()
		return false
	}
	if n.confident || n.positive < MinimumPositiveScores || n.bytesSeen+size <= NavigationByteBudget {
		n.bytesSeen += size
		n.mu.Unlock()
		return true
	}
	n.paused = true
	n.mu.Unlock()

	n.settle.Lock()
	defer n.settle.Unlock()
	n.mu.Lock()
	if n.confident {
		n.bytesSeen += size
		n.paused = false
		n.mu.Unlock()
		return true
	}
	if n.stopped {
		n.mu.Unlock()
		return false
	}
	n.mu.Unlock()
	if finalChoices != nil {
		n.confirmFinal(finalChoices())
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.confident {
		n.bytesSeen += size
		n.paused = false
		return true
	}
	n.stopped = true
	n.issues.add(enums.IssueCodeLowConfidenceBudget)
	return false
}

func (n *navigator) evalGroup(group []requests.NavigationItem, anchor *requests.RelationAnchor, finalChoices func() []string) groupOutcome {
	if n.ctx.Err() != nil {
		return groupOutcome{failure: &provider.Failure{Kind: enums.FailureCancelled}}
	}
	request := requests.NavigationRequest(n.query, group, anchor)
	if !n.charge(requestSize(request), finalChoices) {
		return groupOutcome{paused: true}
	}
	sources := []Donor{}
	if n.donors != nil {
		for _, item := range group {
			sources = append(sources, n.donors(item)...)
		}
		if anchor != nil {
			sources = append(sources, n.donors(requests.NavigationItem{Path: anchor.Path})...)
		}
	}
	if n.validate != nil && !n.validate(n.ctx, sources) {
		if len(group) > 1 {
			return n.evalSplit(group, anchor, finalChoices)
		}
		return groupOutcome{failure: &provider.Failure{Kind: enums.FailureSourceInvalid}}
	}
	scores, err := n.evaluator.Evaluate(n.ctx, request, provider.EvalOptions{Navigation: true})
	if err != nil {
		failure := asFailure(err)
		if failure.Kind == enums.FailureProvider && failure.SplitEligible && len(group) > 1 {
			return n.evalSplit(group, anchor, finalChoices)
		}
		return groupOutcome{failure: failure}
	}
	out := make(map[int]float64, len(group))
	for index := range group {
		value, ok := scores[fmt.Sprintf("q%d", index)]
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return groupOutcome{failure: &provider.Failure{
				Kind: enums.FailureProvider, Message: "Invalid or incomplete API response.",
			}}
		}
		out[index] = value
	}
	return groupOutcome{scores: out}
}

func (n *navigator) evalSplit(group []requests.NavigationItem, anchor *requests.RelationAnchor, finalChoices func() []string) groupOutcome {
	middle := (len(group) + 1) / 2
	left := n.evalGroup(group[:middle], anchor, finalChoices)
	right := n.evalGroup(group[middle:], anchor, finalChoices)
	merged := map[int]float64{}
	for index, value := range left.scores {
		merged[index] = value
	}
	for index, value := range right.scores {
		merged[index+middle] = value
	}
	failure := left.failure
	switch {
	case left.failure != nil && right.failure != nil:
		if !fatalIssue(retrievalIssueForProviderFailure(left.failure.Kind)) && fatalIssue(retrievalIssueForProviderFailure(right.failure.Kind)) {
			failure = right.failure
		}
	case left.failure == nil:
		failure = right.failure
	}
	return groupOutcome{scores: merged, failure: failure, paused: left.paused || right.paused}
}

func (n *navigator) score(
	items []requests.NavigationItem,
	anchor *requests.RelationAnchor,
	finalChoices func() []string,
	consume func(requests.NavigationItem, float64),
) error {
	groups := groupNavigation(n.query, items, anchor, n.batchSize, n.batchBytes, n.issues.add)
	for start := 0; start < len(groups); start += StageWorkers {
		if n.issues.fatal() {
			return nil
		}
		end := min(start+StageWorkers, len(groups))
		outcomes := make([]groupOutcome, end-start)
		var wait sync.WaitGroup
		for index := start; index < end; index++ {
			wait.Add(1)
			go func(slot int, group []requests.NavigationItem) {
				defer wait.Done()
				outcomes[slot-start] = n.evalGroup(group, anchor, finalChoices)
			}(index, groups[index])
		}
		wait.Wait()
		fatal, paused := false, false
		waveClassified := make([][]scoredItem, end-start)
		for index := start; index < end; index++ {
			group := groups[index]
			outcome := outcomes[index-start]
			classified := make([]scoredItem, 0, len(group))
			for offset, item := range group {
				if value, ok := outcome.scores[offset]; ok {
					classified = append(classified, scoredItem{item: item, score: value})
				}
			}
			waveClassified[index-start] = classified
		}
		probes := n.probeWave(waveClassified)
		for index := start; index < end; index++ {
			outcome := outcomes[index-start]
			classified := waveClassified[index-start]
			positive, strong := 0, false
			for _, entry := range classified {
				if entry.score >= WeakScoreCeiling {
					strong = true
				}
				if entry.score > RelevanceBoundary {
					positive++
				}
			}
			if positive > 0 || strong {
				n.mu.Lock()
				n.positive += positive
				n.strong = n.strong || strong
				n.mu.Unlock()
			}
			for _, entry := range classified {
				consume(entry.item, entry.score)
			}
			n.settleProbe(probes[index-start])
			if outcome.paused {
				paused = true
			}
			if outcome.failure != nil {
				if outcome.failure.Kind == enums.FailureSourceInvalid {
					continue
				}
				kind := retrievalIssueForProviderFailure(outcome.failure.Kind)
				if outcome.failure.Kind == enums.FailureProvider && outcome.failure.Message != "" {
					n.issues.addProvider(outcome.failure.Message)
				} else {
					n.issues.add(kind)
				}
				if fatalIssue(kind) {
					fatal = true
				}
			}
		}
		if fatal || paused {
			return nil
		}
	}
	return nil
}

// probeOutcome is the result of confirming one file. apply records the
// probe's side effects (issues) and is only called for probes the sequential
// flow would have made.
type probeOutcome struct {
	confident bool
	apply     func()
}

type plannedProbe struct {
	path    string
	planned bool
	done    chan struct{}
	outcome probeOutcome
}

// probeWave confirms the first unprobed strong file of each group in a wave.
// Sequentially, groups are confirmed one after another until one probe makes
// the navigator confident, with at most four probes per search. Probes never
// depend on each other's answers, so the wave's probes start together and
// settleProbe applies them in group order, discarding any that follow the
// first confident one. Discarded probes cost requests, not results, and their
// requests are the ones later source selection sends for the same file.
func (n *navigator) probeWave(groups [][]scoredItem) []*plannedProbe {
	planned := make([]*plannedProbe, len(groups))
	if n.probe == nil {
		return planned
	}
	n.mu.Lock()
	if n.confident {
		n.mu.Unlock()
		return planned
	}
	taken := map[string]struct{}{}
	budget := 4 - len(n.probes)
	for index, items := range groups {
		if budget <= 0 {
			break
		}
		for _, entry := range items {
			if entry.item.Kind != enums.NavigationKindFile || entry.score < WeakScoreCeiling {
				continue
			}
			if _, ok := n.probes[entry.item.Path]; ok {
				continue
			}
			if _, ok := taken[entry.item.Path]; ok {
				continue
			}
			taken[entry.item.Path] = struct{}{}
			planned[index] = &plannedProbe{path: entry.item.Path, planned: true, done: make(chan struct{})}
			budget--
			break
		}
	}
	n.mu.Unlock()
	for _, probe := range planned {
		if probe == nil {
			continue
		}
		go func() {
			defer close(probe.done)
			probe.outcome = n.probe(n.ctx, probe.path)
		}()
	}
	return planned
}

func (n *navigator) settleProbe(probe *plannedProbe) {
	if probe == nil {
		return
	}
	<-probe.done
	n.mu.Lock()
	if n.confident || len(n.probes) >= 4 {
		n.mu.Unlock()
		return
	}
	n.probes[probe.path] = struct{}{}
	n.mu.Unlock()
	if probe.outcome.apply != nil {
		probe.outcome.apply()
	}
	if probe.outcome.confident {
		n.mu.Lock()
		n.confident = true
		n.mu.Unlock()
	}
}

func (n *navigator) confirmFinal(paths []string) {
	if n.probe == nil {
		return
	}
	for _, path := range paths {
		n.mu.Lock()
		if n.confident {
			n.mu.Unlock()
			return
		}
		if _, ok := n.probes[path]; ok {
			n.mu.Unlock()
			continue
		}
		n.probes[path] = struct{}{}
		n.mu.Unlock()
		outcome := n.probe(n.ctx, path)
		if outcome.apply != nil {
			outcome.apply()
		}
		if outcome.confident {
			n.mu.Lock()
			n.confident = true
			n.mu.Unlock()
			return
		}
	}
}

func asFailure(err error) *provider.Failure {
	var failure *provider.Failure
	if errors.As(err, &failure) {
		return failure
	}
	return &provider.Failure{Kind: enums.FailureProvider, Message: err.Error()}
}
