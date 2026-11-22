package detect

import (
	"reflect"
	"strings"
	"testing"

	"github.com/praetorian-inc/titus/pkg/matcher"
	"github.com/praetorian-inc/titus/pkg/rule"
	"github.com/praetorian-inc/titus/pkg/types"
)

func TestRequiredLiterals(t *testing.T) {
	cases := []struct {
		pattern string
		want    []string
	}{
		{`AKIA[0-9A-Z]{16}`, []string{"akia"}},
		{`(?i)(?:slack|discord)_k\w+`, []string{"slack", "discord"}},
		{`(?i)(?:slack|discord)_apikey=\w+`, []string{"_apikey="}},
		{"(?xi)\n\\b myshopify \\. com # host\n[a-z]+", []string{"myshopify.com"}},
		{"(?x) [ ]abc", []string{" abc"}},
		{`(?:abc)?def`, []string{"def"}},
		{`(?:abc)?de`, nil},
		{`ab\d+`, nil},
		{`(?:abc|x)\d`, nil},
		{`a(?x) b c`, nil},
		{`abc(?#comment)`, nil},
		{`(abc)\1`, nil},
	}
	for _, c := range cases {
		if got := requiredLiterals(c.pattern); !reflect.DeepEqual(got, c.want) {
			t.Errorf("requiredLiterals(%q) = %q, want %q", c.pattern, got, c.want)
		}
	}
}

// Every builtin rule example must get the same verdict from the prefiltered
// rules as from one Titus matcher over all rules, and every matching example
// must contain one of its rule's required literals.
func TestPrefilterAgreesWithFullMatcherOnRuleExamples(t *testing.T) {
	rules, err := rule.NewLoader().LoadBuiltinRules()
	if err != nil {
		t.Fatal(err)
	}
	rules = rule.FilterNoisy(rules, false)
	full, err := matcher.New(matcher.Config{Rules: rules, ContextLines: titusContextLines})
	if err != nil {
		t.Fatal(err)
	}
	defer full.Close()
	filtered, err := newTitusRules(rules, true)
	if err != nil {
		t.Fatal(err)
	}
	defer filtered.close()
	prefiltered, examples := 0, 0
	for _, r := range rules {
		literals := requiredLiterals(r.Pattern)
		if literals != nil {
			prefiltered++
		}
		single, err := matcher.New(matcher.Config{Rules: []*types.Rule{r}, ContextLines: titusContextLines})
		if err != nil {
			t.Fatal(err)
		}
		for _, example := range append(append([]string{}, r.Examples...), r.NegativeExamples...) {
			examples++
			content := []byte(example)
			want, err := full.Match(content)
			if err != nil {
				t.Fatal(err)
			}
			got, err := filtered.match(content)
			if err != nil {
				t.Fatal(err)
			}
			if got != (len(want) > 0) {
				t.Errorf("rule %s example %q: prefiltered = %v, full = %v", r.ID, example, got, len(want) > 0)
			}
			own, err := single.Match(content)
			if err != nil {
				t.Fatal(err)
			}
			if len(own) > 0 && literals != nil && !containsAny(strings.ToLower(example), literals) {
				t.Errorf("rule %s matched %q without any of %q", r.ID, example, literals)
			}
		}
		single.Close()
	}
	if prefiltered*10 < len(rules)*9 {
		t.Errorf("prefiltered %d of %d rules; literal extraction regressed", prefiltered, len(rules))
	}
	if examples == 0 {
		t.Fatal("no rule examples")
	}
}

func containsAny(text string, literals []string) bool {
	for _, literal := range literals {
		if strings.Contains(text, literal) {
			return true
		}
	}
	return false
}
