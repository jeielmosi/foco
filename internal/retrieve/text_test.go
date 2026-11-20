package retrieve

import "testing"

func TestTextIndexLineAndByteCoordinates(t *testing.T) {
	index := newTextIndex("a\nbb\n\nccc")
	if len(index.lines) != 4 {
		t.Fatalf("lines = %d, want 4", len(index.lines))
	}
	if index.offsets[0] != 0 || index.offsets[1] != 2 || index.offsets[2] != 5 || index.offsets[3] != 6 || index.offsets[4] != 9 {
		t.Fatalf("offsets = %v", index.offsets)
	}
	if got := index.lineAt(0); got != 1 {
		t.Fatalf("lineAt(0) = %d", got)
	}
	if got := index.lineAt(2); got != 2 {
		t.Fatalf("lineAt(2) = %d", got)
	}
	if got := index.lineAt(6); got != 4 {
		t.Fatalf("lineAt(6) = %d", got)
	}
	whole := index.rangeForSpan(0, 9)
	if whole.StartLine != 1 || whole.EndLine != 4 || whole.SourceByteStart != nil {
		t.Fatalf("whole range = %+v", whole)
	}
	partial := index.rangeForSpan(1, 3)
	if partial.SourceByteStart == nil || *partial.SourceByteStart != 1 || *partial.SourceByteEnd != 3 {
		t.Fatalf("partial range = %+v", partial)
	}
}

func TestCommentRangesAreLineBased(t *testing.T) {
	ranges := commentRanges([]string{"code", "# one", "# two", "", "// three", "code"})
	want := []Range{{2, 3}, {5, 5}}
	if len(ranges) != len(want) {
		t.Fatalf("ranges = %+v", ranges)
	}
	for i := range want {
		if ranges[i] != want[i] {
			t.Fatalf("range[%d] = %+v, want %+v", i, ranges[i], want[i])
		}
	}
}

func TestRangeForSpanEmptySpan(t *testing.T) {
	index := newTextIndex("a\nbb\n\nccc")
	for _, start := range []int{0, 1, 2, 3, 5, 6, 9} {
		got := index.rangeForSpan(start, start)
		if got.StartLine != index.lineAt(start) || got.EndLine != got.StartLine {
			t.Errorf("span(%d, %d) lines = %d..%d, want %d..%d", start, start, got.StartLine, got.EndLine, index.lineAt(start), index.lineAt(start))
		}
		if got.SourceByteStart == nil || got.SourceByteEnd == nil || *got.SourceByteStart != start || *got.SourceByteEnd != start {
			t.Errorf("span(%d, %d) bytes = %+v, want %d..%d", start, start, got, start, start)
		}
	}
}

func TestBlankLines(t *testing.T) {
	tests := []struct {
		name   string
		source string
		first  int
		last   int
		want   bool
	}{
		{"normal blank range", "a\n\n\nb", 2, 3, true},
		{"normal non-blank range", "a\n\nb", 1, 2, false},
		{"reversed range", "a\nb", 2, 1, true},
		{"first past end", "a\nb", 100, 200, true},
		{"both negative", "a\nb", -5, -1, true},
		{"empty file", "", 1, 1, true},
	}
	for _, test := range tests {
		index := newTextIndex(test.source)
		if got := index.blankLines(test.first, test.last); got != test.want {
			t.Errorf("%s: blankLines(%d, %d) = %v, want %v", test.name, test.first, test.last, got, test.want)
		}
	}
}

func TestCommentRangesTrailingBlock(t *testing.T) {
	ranges := commentRanges([]string{"code", "/* one", "*/ two"})
	want := []Range{{2, 3}}
	if len(ranges) != len(want) {
		t.Fatalf("ranges = %+v", ranges)
	}
	for i := range want {
		if ranges[i] != want[i] {
			t.Fatalf("range[%d] = %+v, want %+v", i, ranges[i], want[i])
		}
	}
}

func TestCommentRangesWholeFile(t *testing.T) {
	ranges := commentRanges([]string{"# one", "// two"})
	want := []Range{{1, 2}}
	if len(ranges) != len(want) {
		t.Fatalf("ranges = %+v", ranges)
	}
	for i := range want {
		if ranges[i] != want[i] {
			t.Fatalf("range[%d] = %+v, want %+v", i, ranges[i], want[i])
		}
	}
}

func TestCommentRangesEmptySource(t *testing.T) {
	for _, lines := range [][]string{nil, {}} {
		if got := commentRanges(lines); len(got) != 0 {
			t.Fatalf("commentRanges(%v) = %+v, want none", lines, got)
		}
	}
}
