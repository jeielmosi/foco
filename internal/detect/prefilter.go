package detect

import (
	"fmt"
	"regexp/syntax"
	"strings"
	"unicode/utf8"
)

// minLiteralRunes is the shortest required literal worth prefiltering on.
// Shorter literals occur in almost every file and only add scanning work.
const minLiteralRunes = 3

// requiredLiterals returns lowercase strings such that every match of pattern
// contains at least one of them, compared case-insensitively. It returns nil
// when no such set is known, and the rule must then always run.
//
// The analysis is conservative: it only reads literals the regex syntax tree
// requires (concatenations, alternations whose every branch has a literal,
// groups and repeats with a minimum of one) and gives up on any syntax it does
// not fully understand.
func requiredLiterals(pattern string) []string {
	plain, ok := expandFreeSpacing(pattern)
	if !ok {
		return nil
	}
	parsed, err := syntax.Parse(plain, syntax.Perl)
	if err != nil {
		return nil
	}
	return literalSet(parsed.Simplify())
}

func literalSet(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpLiteral:
		literal := strings.ToLower(string(re.Rune))
		if utf8.RuneCountInString(literal) < minLiteralRunes {
			return nil
		}
		return []string{literal}
	case syntax.OpCapture, syntax.OpPlus:
		return literalSet(re.Sub[0])
	case syntax.OpRepeat:
		if re.Min >= 1 {
			return literalSet(re.Sub[0])
		}
		return nil
	case syntax.OpConcat:
		var best []string
		for _, sub := range re.Sub {
			if set := literalSet(sub); set != nil && (best == nil || shortest(set) > shortest(best)) {
				best = set
			}
		}
		return best
	case syntax.OpAlternate:
		var out []string
		for _, sub := range re.Sub {
			set := literalSet(sub)
			if set == nil {
				return nil
			}
			out = append(out, set...)
		}
		return out
	}
	return nil
}

func shortest(set []string) int {
	out := -1
	for _, literal := range set {
		if out < 0 || len(literal) < out {
			out = len(literal)
		}
	}
	return out
}

// expandFreeSpacing rewrites a pattern that opens with a free-spacing flag
// group, such as (?x) or (?xi), into equivalent plain syntax following .NET
// IgnorePatternWhitespace rules, which regexp2 implements: unescaped
// whitespace and #-comments outside character classes are dropped, and
// (?#...) comments are removed. ok is false for patterns it cannot rewrite
// exactly, including free spacing enabled anywhere but the start.
func expandFreeSpacing(pattern string) (string, bool) {
	prefix := ""
	body := pattern
	if strings.HasPrefix(pattern, "(?") {
		end := strings.IndexByte(pattern, ')')
		if end > 2 {
			flags := pattern[2:end]
			if strings.Trim(flags, "imsx") == "" && strings.Contains(flags, "x") {
				if rest := strings.ReplaceAll(flags, "x", ""); rest != "" {
					prefix = "(?" + rest + ")"
				}
				body = pattern[end+1:]
				return stripFreeSpacing(prefix, body)
			}
		}
	}
	if strings.Contains(pattern, "(?#") || enablesFreeSpacing(pattern) {
		return "", false
	}
	return pattern, true
}

// enablesFreeSpacing reports any inline flag group that turns on x.
func enablesFreeSpacing(pattern string) bool {
	for index := 0; index+2 < len(pattern); index++ {
		if pattern[index] != '(' || pattern[index+1] != '?' {
			continue
		}
		for cursor := index + 2; cursor < len(pattern) && strings.IndexByte("imsxn-", pattern[cursor]) >= 0; cursor++ {
			if pattern[cursor] == 'x' {
				return true
			}
		}
	}
	return false
}

func stripFreeSpacing(prefix, body string) (string, bool) {
	runes := []rune(body)
	var out strings.Builder
	out.WriteString(prefix)
	inClass := false
	for index := 0; index < len(runes); index++ {
		current := runes[index]
		if current == '\\' {
			if index+1 >= len(runes) {
				return "", false
			}
			index++
			escaped := runes[index]
			if escaped == ' ' || escaped == '\t' || escaped == '\n' || escaped == '\r' {
				fmt.Fprintf(&out, `\x{%x}`, escaped)
			} else {
				out.WriteRune('\\')
				out.WriteRune(escaped)
			}
			continue
		}
		if inClass {
			if current == ']' {
				inClass = false
			}
			out.WriteRune(current)
			continue
		}
		switch {
		case current == '[':
			inClass = true
			out.WriteRune(current)
			if index+1 < len(runes) && runes[index+1] == '^' {
				index++
				out.WriteRune('^')
			}
			if index+1 < len(runes) && runes[index+1] == ']' {
				index++
				out.WriteString(`\]`)
			}
		case current == ' ' || current == '\t' || current == '\n' || current == '\r' || current == '\f' || current == '\v':
		case current == '#':
			for index < len(runes) && runes[index] != '\n' {
				index++
			}
		case current == '(' && index+2 < len(runes) && runes[index+1] == '?' && runes[index+2] == '#':
			for index < len(runes) && runes[index] != ')' {
				index++
			}
			if index >= len(runes) {
				return "", false
			}
		default:
			out.WriteRune(current)
		}
	}
	if inClass {
		return "", false
	}
	return out.String(), true
}
