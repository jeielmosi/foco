package retrieve

import "strings"

type textIndex struct {
	source   string
	bytes    []byte
	lines    []string
	offsets  []int
	nonBlank []int
}

func newTextIndex(source string) *textIndex {
	lines := strings.Split(source, "\n")
	offsets := make([]int, len(lines)+1)
	position := 0
	for index, line := range lines {
		offsets[index] = position
		position += len(line) + 1
		if position > len(source) {
			position = len(source)
		}
	}
	offsets[len(lines)] = len(source)
	nonBlank := make([]int, len(lines)+1)
	for index, line := range lines {
		increment := 0
		if strings.TrimSpace(line) != "" {
			increment = 1
		}
		nonBlank[index+1] = nonBlank[index] + increment
	}
	return &textIndex{
		source: source, bytes: []byte(source), lines: lines,
		offsets: offsets, nonBlank: nonBlank,
	}
}

func (t *textIndex) lineAt(position int) int {
	if position < 0 {
		position = 0
	}
	low, high := 0, len(t.lines)
	for low+1 < high {
		middle := (low + high) / 2
		if t.offsets[middle] <= position {
			low = middle
		} else {
			high = middle
		}
	}
	return low + 1
}

func (t *textIndex) rangeForSpan(start, end int) EvidenceRange {
	startLine := t.lineAt(start)
	if end <= start {
		s, e := start, start
		return EvidenceRange{StartLine: startLine, EndLine: startLine, SourceByteStart: &s, SourceByteEnd: &e}
	}
	endLine := t.lineAt(end - 1)
	result := EvidenceRange{StartLine: startLine, EndLine: endLine}
	if start != t.offsets[startLine-1] || end != t.offsets[endLine] {
		s, e := start, end
		result.SourceByteStart = &s
		result.SourceByteEnd = &e
	}
	return result
}

func (t *textIndex) blankLines(first, last int) bool {
	if last < first {
		return true
	}
	begin := min(max(first-1, 0), len(t.lines))
	end := min(max(last, 0), len(t.lines))
	return t.nonBlank[end] == t.nonBlank[begin]
}

func isCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	for _, prefix := range []string{"#", "//", "/*", "*", "--", "<!--", ";"} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func commentRanges(lines []string) []Range {
	var out []Range
	start := 0
	for index, line := range lines {
		if !isCommentLine(line) {
			if start > 0 {
				out = append(out, Range{StartLine: start, EndLine: index})
				start = 0
			}
			continue
		}
		if start == 0 {
			start = index + 1
		}
	}
	if start > 0 {
		out = append(out, Range{StartLine: start, EndLine: len(lines)})
	}
	return out
}
