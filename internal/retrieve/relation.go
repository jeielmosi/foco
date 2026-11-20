package retrieve

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"foco/internal/enums"
	"foco/internal/files"
	"foco/internal/requests"
)

var classPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)`),
	regexp.MustCompile(`^\s*type\s+([A-Za-z_]\w*)\s+(?:struct|interface)\b`),
	regexp.MustCompile(`^\s*interface\s+([A-Za-z_]\w*)`),
	regexp.MustCompile(`^\s*impl(?:<[^>]*>)?\s+(?:[A-Za-z_][\w:<>, ]*\s+for\s+)?([A-Za-z_]\w*)`),
	regexp.MustCompile(`^\s*(?:pub\s+)?(?:struct|enum|trait)\s+([A-Za-z_]\w*)`),
}

func indentOf(line string) int {
	count := 0
	for _, r := range line {
		if r == ' ' {
			count++
			continue
		}
		if r == '\t' {
			count++
			continue
		}
		break
	}
	return count
}

func nextNonBlank(lines []string, index int) (string, bool) {
	for cursor := index + 1; cursor < len(lines); cursor++ {
		if strings.TrimSpace(lines[cursor]) != "" {
			return lines[cursor], true
		}
	}
	return "", false
}

func anchorClasses(source string) []string {
	lines := strings.Split(source, "\n")
	seen := map[string]bool{}
	var out []string
	for index, line := range lines {
		for _, pattern := range classPatterns {
			match := pattern.FindStringSubmatch(line)
			if len(match) < 2 {
				continue
			}
			name := match[1]
			if seen[name] {
				break
			}
			next, ok := nextNonBlank(lines, index)
			if !ok || indentOf(next) <= indentOf(line) {
				break
			}
			seen[name] = true
			out = append(out, name)
			break
		}
	}
	return out
}

func findAnchor(ctx context.Context, reader Reader, root string, ordered []candidate) (*requests.RelationAnchor, Donor, bool) {
	for _, entry := range ordered {
		if entry.Score <= RelevanceBoundary || ctx.Err() != nil {
			break
		}
		result := reader.ReadSnapshotReuse(ctx, absPath(root, entry.Path))
		if result.Status != enums.SnapshotStatusSnapshot || result.Snapshot == nil {
			continue
		}
		classes := anchorClasses(result.Snapshot.Source)
		if len(classes) == 0 {
			continue
		}
		blob, _ := json.Marshal(classes)
		if len(blob) >= 4000 {
			continue
		}
		return &requests.RelationAnchor{Path: entry.Path, Classes: classes}, Donor{
			Path: entry.Path, ContentHash: entry.ContentHash,
		}, true
	}
	return nil, Donor{}, false
}

func reconsider(
	ctx context.Context, reader Reader, navigator *navigator, root string,
	pruned map[string]requests.NavigationItem, anchor *requests.RelationAnchor,
	donors map[string][]Donor, issue func(enums.RetrievalIssueCode), report func(string, *files.Snapshot),
) ([]string, error) {
	if anchor == nil || len(pruned) == 0 {
		return nil, nil
	}
	paths := make([]string, 0, len(pruned))
	for path := range pruned {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	items := make([]requests.NavigationItem, 0, len(paths))
	for _, path := range paths {
		item := pruned[path]
		var sampled []Donor
		item, sampled = withDirectoryContent(ctx, reader, item, issue, report)
		if donors != nil {
			donors[path] = sampled
		}
		items = append(items, item)
	}
	classified := []scoredItem{}
	if err := navigator.score(items, anchor, nil, func(item requests.NavigationItem, score float64) {
		classified = append(classified, scoredItem{item: item, score: score})
	}); err != nil {
		return nil, err
	}
	admitted, _ := navigationAdmissions(classified, navigator.strong)
	seeds := make([]string, 0, len(admitted))
	for _, entry := range admitted {
		seeds = append(seeds, entry.item.Path)
	}
	return seeds, nil
}
