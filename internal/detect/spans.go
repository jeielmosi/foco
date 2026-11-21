package detect

import (
	"bytes"
	"context"
	"sort"
	"strings"
)

// Span is one secret found in a file: the byte range [Start, End) of the
// secret value, the rule that found it and the engine that ran the rule.
type Span struct {
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Rule   string `json:"rule"`
	Engine string `json:"engine"`
}

// SpanDetector reports every secret in content with its byte range, instead
// of stopping at the first one.
type SpanDetector interface {
	Spans(ctx context.Context, path string, content []byte) ([]Span, error)
}

// locate finds value in content, preferring the occurrence closest to offset
// hint (or on line hint when hint < 0 means "line -hint"). It returns -1 when
// value does not occur.
func locate(content, value []byte, near int) int {
	if len(value) == 0 {
		return -1
	}
	best, bestDistance := -1, -1
	for offset := 0; ; {
		index := bytes.Index(content[offset:], value)
		if index < 0 {
			break
		}
		position := offset + index
		distance := position - near
		if distance < 0 {
			distance = -distance
		}
		if best < 0 || distance < bestDistance {
			best, bestDistance = position, distance
		}
		offset = position + 1
	}
	return best
}

// lineOffset returns the byte offset of the start of 1-based line.
func lineOffset(content []byte, line int) int {
	offset := 0
	for current := 1; current < line; current++ {
		next := bytes.IndexByte(content[offset:], '\n')
		if next < 0 {
			return offset
		}
		offset += next + 1
	}
	return offset
}

// mergeSpans sorts spans and drops exact duplicates. Overlapping and nested
// spans are kept: each is a distinct finding, and a narrow finding inside a
// broad one (a password inside a connection URI) is the more precise range.
func mergeSpans(spans []Span) []Span {
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End > spans[j].End
	})
	out := spans[:0]
	for _, span := range spans {
		if len(out) > 0 && out[len(out)-1].Start == span.Start && out[len(out)-1].End == span.End {
			continue
		}
		out = append(out, span)
	}
	return out
}

// nonSecretRules report identifiers, not credentials: an Amazon Resource
// Name names a resource and grants nothing.
var nonSecretRules = map[string]bool{"np.arn.1": true}

var referenceMarkers = []string{
	"op://", "vault:", "vault://", "secretsmanager:", "arn:aws:", "${", "{{", "%(", "$(", "env:", "<", ">",
	"redacted", "[hidden]", "[filtered]", "your_", "your-", "yourpass", "your password", "placeholder",
	"changeme", "change_me", "change-me", "dummy",
}

// defaultValues are well-known defaults and words used as passwords in
// documentation; a credential equal to one is not a leaked secret.
var defaultValues = map[string]bool{
	"password": true, "passwd": true, "secret": true, "postgres": true, "root": true, "admin": true,
	"changeme": true, "letmein": true, "default": true, "guest": true, "test": true, "token": true,
	"username": true, "user": true, "pass": true, "mysql": true, "redis": true, "none": true, "null": true,
}

// plausibleSecret reports whether value could be a real secret rather than a
// reference to one, a placeholder, a mask or a documented default.
func plausibleSecret(value []byte) bool {
	text := strings.ToLower(string(value))
	for _, marker := range referenceMarkers {
		if strings.Contains(text, marker) {
			return false
		}
	}
	if defaultValues[strings.Trim(text, "\"' ")] {
		return false
	}
	// user:password pairs of defaults, as in postgres:postgres.
	if user, password, ok := strings.Cut(text, ":"); ok && (defaultValues[password] || user == password) {
		return false
	}
	// Masks: one character repeated, possibly around a short visible tail.
	distinct := map[rune]int{}
	for _, r := range text {
		distinct[r]++
	}
	for r, count := range distinct {
		if (r == 'x' || r == '*' || r == '•' || r == '.' || r == '#') && count*2 >= len([]rune(text)) {
			return false
		}
	}
	return len(distinct) > 1
}

// keepSpan filters findings that are not secrets.
func keepSpan(content []byte, span Span) bool {
	if nonSecretRules[span.Rule] {
		return false
	}
	return plausibleSecret(content[span.Start:span.End])
}
