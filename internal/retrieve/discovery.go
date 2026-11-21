package retrieve

import (
	"context"
	"sort"

	"foco/internal/enums"
	"foco/internal/files"
	"foco/internal/requests"
)

type discoveryState struct {
	EntriesSeen int
	Visited     map[string]bool
	Candidates  map[string]candidate
	Pruned      map[string]requests.NavigationItem
	Previews    map[string]*requests.FilePreview
	Donors      map[string][]Donor
}

type levelDirectory struct {
	Path  string
	Depth int
}

func listEntries(ctx context.Context, list pageLister, directory string, remaining int, issue func(enums.RetrievalIssueCode), fatal func() bool) ([]files.Entry, bool, bool) {
	entries := []files.Entry{}
	offset := 0
	for {
		if fatal != nil && fatal() {
			return entries, false, false
		}
		page, err := list(ctx, directory, offset)
		if err != nil {
			if ctx.Err() != nil {
				issue(enums.IssueCodeInterrupted)
			} else {
				issue(enums.IssueCodeUnreadable)
			}
			return entries, false, false
		}
		if ctx.Err() != nil {
			return entries, false, false
		}
		if page.Issue != enums.UnknownFileIssueKind {
			issue(retrievalIssueForFileIssue(page.Issue))
			if fatal != nil && fatal() {
				return entries, false, false
			}
		}
		entries = append(entries, page.Entries...)
		hasNext := page.NextOffset != 0
		if remaining-len(entries) < 0 || (hasNext && remaining-len(entries) == 0) {
			return entries, true, true
		}
		if !hasNext {
			break
		}
		if page.NextOffset == offset {
			issue(enums.IssueCodeUnreadable)
			return entries, true, false
		}
		offset = page.NextOffset
	}
	return entries, true, false
}

func discover(
	ctx context.Context, root string, reader Reader, query string,
	seeds []string, anchor *requests.RelationAnchor, navigator *navigator,
	issue func(enums.RetrievalIssueCode), fatal func() bool, report func(string, *files.Snapshot), state *discoveryState,
) error {
	limitReported := false
	reportLimit := func() {
		if limitReported {
			return
		}
		limitReported = true
		issue(enums.IssueCodeResourceLimit)
	}
	stopped := func() bool {
		return ctx.Err() != nil || (fatal != nil && fatal())
	}
	directories := append([]string{}, seeds...)
	for len(directories) > 0 && state.EntriesSeen < DiscoveryLimit && !stopped() {
		level := make([]levelDirectory, 0, len(directories))
		for _, directory := range directories {
			level = append(level, levelDirectory{Path: directory, Depth: 0})
		}
		next := directories
		directories = []string{}
		items := []requests.NavigationItem{}
		ahead := startReadAhead(ctx, reader, root, next)
		for index := 0; index < len(level) && !stopped(); index++ {
			current := level[index]
			if state.EntriesSeen >= DiscoveryLimit {
				reportLimit()
				break
			}
			if state.Visited[current.Path] {
				continue
			}
			state.Visited[current.Path] = true
			full := absPath(root, current.Path)
			entries, ok, limited := listEntries(ctx, ahead.listPage, full, DiscoveryLimit-state.EntriesSeen, issue, fatal)
			if limited {
				reportLimit()
			}
			if !ok {
				continue
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
			for _, entry := range entries {
				if stopped() {
					break
				}
				if state.EntriesSeen >= DiscoveryLimit {
					reportLimit()
					break
				}
				state.EntriesSeen++
				relative := relPath(root, entry.Path)
				if entry.IsDir {
					if current.Depth == 0 {
						level = append(level, levelDirectory{Path: relative, Depth: 1})
						continue
					}
					preview := previewDirectory(ctx, reader, entry.Path, issue)
					if preview == nil {
						continue
					}
					item := requests.NavigationItem{Path: relative, Kind: enums.NavigationKindDirectory, ChildPreview: preview}
					if anchor != nil {
						var donors []Donor
						item, donors = withDirectoryContent(ctx, reader, item, issue, report, nil)
						state.Donors[relative] = donors
					}
					items = append(items, item)
					continue
				}
				result := ahead.get(ctx, entry.Path)
				if result.Status == enums.SnapshotStatusIssue {
					issue(retrievalIssueForFileIssue(result.Issue))
					continue
				}
				if result.Status != enums.SnapshotStatusSnapshot || result.Snapshot == nil {
					continue
				}
				if report != nil {
					report(relative, result.Snapshot)
				}
				preview := previewFile(*result.Snapshot)
				state.Previews[relative] = preview
				state.Donors[relative] = []Donor{{Path: relative, ContentHash: result.Snapshot.ContentHash}}
				item := requests.NavigationItem{Path: relative, Kind: enums.NavigationKindFile, FilePreview: preview}
				if len(result.Snapshot.Source) > SourceInspectionLimit {
					reportLimit()
					items = append(items, item)
					continue
				}
				if requestSize(requests.NavigationRequest(query, []requests.NavigationItem{item}, anchor)) > NavigationBatchBytes {
					for _, chunk := range splitText(result.Snapshot.Source, 12_000) {
						items = append(items, requests.NavigationItem{
							Path: relative, Kind: enums.NavigationKindFile,
							FilePreview: &requests.FilePreview{
								SizeBytes: preview.SizeBytes, Extension: preview.Extension,
								Text: chunk.Text, PreviewBytes: len(chunk.Text),
								Truncated: true, Range: "sampled source ranges",
							},
						})
					}
					continue
				}
				items = append(items, item)
			}
		}
		ahead.stop()
		classified := []scoredItem{}
		if fatal != nil && fatal() {
			return nil
		}
		if len(items) > 0 {
			finalChoices := func() []string { return candidatePaths(state) }
			if err := navigator.score(items, anchor, finalChoices, func(item requests.NavigationItem, score float64) {
				classified = append(classified, scoredItem{item: item, score: score})
			}); err != nil {
				return err
			}
		}
		if fatal != nil && fatal() {
			return nil
		}
		admitted, suppressWeak := navigationAdmissions(classified, navigator.strong)
		for _, entry := range admitted {
			if entry.item.Kind == enums.NavigationKindDirectory {
				directories = append(directories, entry.item.Path)
				continue
			}
			prior, ok := state.Candidates[entry.item.Path]
			if !ok || entry.score > prior.Score {
				state.Candidates[entry.item.Path] = candidate{
					Path: entry.item.Path, ContentHash: donorHash(state.Donors[entry.item.Path]), Score: entry.score,
				}
			}
		}
		for _, entry := range classified {
			if entry.item.Kind != enums.NavigationKindDirectory || anchor != nil {
				continue
			}
			if entry.score <= RelevanceBoundary || (suppressWeak && entry.score < WeakScoreCeiling) {
				state.Pruned[entry.item.Path] = entry.item
			}
		}
	}
	if len(directories) > 0 && !stopped() {
		reportLimit()
	}
	return nil
}

func donorHash(donors []Donor) string {
	if len(donors) == 0 {
		return ""
	}
	return donors[0].ContentHash
}

func candidatePaths(state *discoveryState) []string {
	ordered := sortedCandidates(state)
	out := make([]string, len(ordered))
	for index, candidate := range ordered {
		out[index] = candidate.Path
	}
	return out
}

func sortedCandidates(state *discoveryState) []candidate {
	out := make([]candidate, 0, len(state.Candidates))
	for _, candidate := range state.Candidates {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Path < out[j].Path
	})
	return out
}
