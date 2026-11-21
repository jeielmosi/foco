package detect

import (
	"bytes"
	"fmt"

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
	secretGroup map[string]string
	all         []*types.Rule
	always      matcher.Matcher
	rules       []*types.Rule
	matchers    []matcher.Matcher
	words       *ahocorasick.Matcher
	wordRule    [][]int
}

func newTitusRules(rules []*types.Rule, prefilter bool) (*titusRules, error) {
	out := &titusRules{all: rules, secretGroup: map[string]string{}}
	for _, rule := range rules {
		out.secretGroup[rule.ID] = rule.SecretGroup
	}
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
	out.matchers = make([]matcher.Matcher, len(out.rules))
	for position, rule := range out.rules {
		compiled, err := newTitusMatcher(matcher.Config{Rules: []*types.Rule{rule}, ContextLines: titusContextLines})
		if err != nil {
			_ = out.close()
			return nil, fmt.Errorf("detect: creating titus matcher: %w", err)
		}
		out.matchers[position] = compiled
	}
	if len(always) > 0 {
		compiled, err := newTitusMatcher(matcher.Config{Rules: always, ContextLines: titusContextLines})
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

func (t *titusRules) Rules() []*types.Rule {
	return append([]*types.Rule(nil), t.all...)
}

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
		found, err := t.matchers[position].Match(content)
		if err != nil {
			return false, err
		}
		if len(found) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// spans returns every match with the byte range of its secret capture.
func (t *titusRules) spans(content []byte) ([]Span, error) {
	var out []Span
	add := func(found []*types.Match) {
		for _, match := range found {
			start, end := int(match.Location.Offset.Start), int(match.Location.Offset.End)
			if start < 0 || end > len(content) || start >= end {
				continue
			}
			if secret := matcher.SecretCapture(match, t.secretGroup[match.RuleID]); len(secret) > 0 {
				if index := bytes.Index(content[start:end], secret); index >= 0 {
					start, end = start+index, start+index+len(secret)
				}
			}
			out = append(out, Span{Start: start, End: end, Rule: match.RuleID, Engine: "titus"})
		}
	}
	if t.always != nil {
		found, err := t.always.Match(content)
		if err != nil {
			return nil, err
		}
		add(found)
	}
	if t.words == nil {
		return out, nil
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
		found, err := t.matchers[position].Match(content)
		if err != nil {
			return nil, err
		}
		add(found)
	}
	return out, nil
}

func (t *titusRules) close() error {
	var first error
	if t.always != nil {
		first = t.always.Close()
	}
	for _, compiled := range t.matchers {
		if compiled != nil {
			if err := compiled.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}
