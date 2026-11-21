package detect

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/cloudflare/ahocorasick"
	"github.com/praetorian-inc/titus/pkg/matcher"
	"github.com/praetorian-inc/titus/pkg/types"
)

// titusContextLines matches the Titus scanner default.
const titusContextLines = 2

// titusRules runs Titus rules with a literal prefilter.
//
// The portable Titus matcher runs every rule's backtracking regex over every
// file. Most rules require a fixed string (a provider name, a host, a key
// prefix), so a file lacking all of a rule's required literals cannot match it.
// One Aho-Corasick pass over the lowercased content selects the rules that can
// match; only those run. Rules without a known literal always run.
//
// A file is sensitive when any rule reports a match after Titus's per-match
// filters. Titus applies those filters per match and its cross-rule dedup
// never empties a non-empty set, so running rules separately gives the same
// verdict as running them together.
type titusRules struct {
	all      []*types.Rule
	always   matcher.Matcher
	rules    []*types.Rule
	matchers []lazyMatcher
	words    *ahocorasick.Matcher
	wordRule [][]int
}

type lazyMatcher struct {
	once    sync.Once
	matcher matcher.Matcher
	err     error
}

func newTitusRules(rules []*types.Rule, prefilter bool) (*titusRules, error) {
	out := &titusRules{all: rules}
	var always []*types.Rule
	wordIndex := map[string]int{}
	var words []string
	for _, rule := range rules {
		var literals []string
		if prefilter {
			literals = requiredLiterals(rule.Pattern)
		}
		if literals == nil {
			always = append(always, rule)
			continue
		}
		position := len(out.rules)
		out.rules = append(out.rules, rule)
		for _, literal := range literals {
			index, ok := wordIndex[literal]
			if !ok {
				index = len(words)
				wordIndex[literal] = index
				words = append(words, literal)
				out.wordRule = append(out.wordRule, nil)
			}
			out.wordRule[index] = append(out.wordRule[index], position)
		}
	}
	out.matchers = make([]lazyMatcher, len(out.rules))
	if len(always) > 0 {
		compiled, err := matcher.New(matcher.Config{Rules: always, ContextLines: titusContextLines})
		if err != nil {
			return nil, fmt.Errorf("detect: creating titus matcher: %w", err)
		}
		out.always = compiled
	}
	if len(words) > 0 {
		out.words = ahocorasick.NewStringMatcher(words)
	}
	return out, nil
}

// Rules returns the loaded rules.
func (t *titusRules) Rules() []*types.Rule {
	return append([]*types.Rule(nil), t.all...)
}

// match reports whether any rule matches content.
func (t *titusRules) match(content []byte) (bool, error) {
	if t.always != nil {
		found, err := t.always.Match(content)
		if err != nil {
			return false, err
		}
		if len(found) > 0 {
			return true, nil
		}
	}
	if t.words == nil {
		return false, nil
	}
	candidates := map[int]struct{}{}
	for _, word := range t.words.MatchThreadSafe(bytes.ToLower(content)) {
		for _, position := range t.wordRule[word] {
			candidates[position] = struct{}{}
		}
	}
	for position := range t.rules {
		if _, ok := candidates[position]; !ok {
			continue
		}
		compiled, err := t.matcher(position)
		if err != nil {
			return false, err
		}
		found, err := compiled.Match(content)
		if err != nil {
			return false, err
		}
		if len(found) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// matcher compiles a prefiltered rule on its first candidate file, so rules
// whose literals never appear cost nothing.
func (t *titusRules) matcher(position int) (matcher.Matcher, error) {
	lazy := &t.matchers[position]
	lazy.once.Do(func() {
		lazy.matcher, lazy.err = matcher.New(matcher.Config{
			Rules: []*types.Rule{t.rules[position]}, ContextLines: titusContextLines,
		})
		if lazy.err != nil {
			lazy.err = fmt.Errorf("detect: creating titus matcher: %w", lazy.err)
		}
	})
	return lazy.matcher, lazy.err
}

func (t *titusRules) close() error {
	var first error
	if t.always != nil {
		first = t.always.Close()
	}
	for index := range t.matchers {
		lazy := &t.matchers[index]
		lazy.once.Do(func() {})
		if lazy.matcher != nil {
			if err := lazy.matcher.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}
