package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"foco/internal/enums"
	"foco/internal/retrieve"
)

const (
	DefaultMaxSourceBytes int64 = 0
	DefaultMaxOutputBytes int64 = 256_000
)

func quote(value string) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return `""`
	}
	escaped := strings.TrimSuffix(buffer.String(), "\n")
	var out strings.Builder
	for _, r := range escaped {
		switch {
		case r >= 0x7f && r <= 0x9f,
			r == 0x061c, r == 0x200e, r == 0x200f,
			r >= 0x2028 && r <= 0x202e,
			r >= 0x2066 && r <= 0x2069:
			fmt.Fprintf(&out, "\\u%04x", r)
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func fenceFor(source string) string {
	longest, current := 0, 0
	for _, r := range source {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}
	length := 3
	if longest+1 > length {
		length = longest + 1
	}
	return strings.Repeat("`", length)
}

func isFenceLine(line string) bool {
	if len(line) < 3 {
		return false
	}
	for _, r := range line {
		if r != '`' {
			return false
		}
	}
	return true
}

type renderedFile struct {
	file     retrieve.FileEvidence
	excerpts []retrieve.Excerpt
	omitted  bool
}

func RenderResult(result retrieve.RetrievalResult, maxSourceBytes, maxOutputBytes int64, noLocations bool) string {
	_ = noLocations
	remaining := int64(math.MaxInt64)
	if maxSourceBytes > 0 {
		remaining = maxSourceBytes
	}
	files := make([]renderedFile, len(result.Files))
	for index, file := range result.Files {
		files[index] = renderedFile{file: file, omitted: file.SourceOmitted}
	}
	sort.SliceStable(files, func(i, j int) bool {
		left, right := files[i].file, files[j].file
		leftPriority, rightPriority := left.Score, right.Score
		if left.Priority != nil {
			leftPriority = *left.Priority
		}
		if right.Priority != nil {
			rightPriority = *right.Priority
		}
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		return left.Path < right.Path
	})
	for index := range files {
		source := files[index].file.Excerpts
		if files[index].file.PresentationExcerpts != nil {
			source = files[index].file.PresentationExcerpts
		}
		excerpts := make([]retrieve.Excerpt, 0, len(source))
		for _, excerpt := range source {
			size := int64(len(excerpt.Source))
			if size > remaining {
				files[index].omitted = true
				continue
			}
			remaining -= size
			excerpts = append(excerpts, excerpt)
		}
		files[index].excerpts = excerpts
	}
	omittedCount := 0
	for _, file := range files {
		if file.omitted {
			omittedCount++
		}
	}
	lines := []string{}
	suffix := ""
	if result.Status != enums.RetrievalStatusComplete {
		suffix = "; discovery incomplete"
	}
	lines = append(lines, fmt.Sprintf("Foco: %d relevant files%s.", len(files), suffix))
	if len(files) == 0 && result.Status == enums.RetrievalStatusComplete {
		lines = append(lines, "No confident match found under this root.")
	}
	lowConfidence := false
	for _, issue := range result.Issues {
		if issue.Kind == enums.IssueCodeLowConfidenceBudget {
			lowConfidence = true
		}
	}
	if lowConfidence {
		lines = append(lines, fmt.Sprintf(
			"No confident match within the navigation budget (%d serialized bytes); narrow the root to continue.",
			retrieve.NavigationByteBudget,
		))
	}
	lines = append(lines, "Roles are estimates. Declaration locations omitted.")
	instructions := "none found"
	if len(result.RepositoryContext.InstructionFiles) > 0 {
		quoted := make([]string, 0, len(result.RepositoryContext.InstructionFiles))
		for _, path := range result.RepositoryContext.InstructionFiles {
			quoted = append(quoted, quote(path))
		}
		instructions = strings.Join(quoted, ", ")
	}
	lookup := ""
	if result.RepositoryContext.InstructionLookupIncomplete {
		lookup = "; lookup incomplete"
	}
	lines = append(lines, fmt.Sprintf("AGENTS.md lookup (root and returned-file ancestors): %s%s.", instructions, lookup))
	if result.Status == enums.RetrievalStatusInterrupted {
		lines = append(lines, "Interrupted.")
	}
	if omittedCount > 0 {
		lines = append(lines, fmt.Sprintf("Source omitted: %d file(s).", omittedCount))
	}
	for _, issue := range result.Issues {
		lines = append(lines, fmt.Sprintf("Issue: %s: %d", quote(issue.Kind.String()), issue.Count))
	}
	if result.ProviderFailure != "" {
		lines = append(lines, "API error: "+quote(result.ProviderFailure))
	}
	for _, file := range files {
		roles := "relevant; role uncertain"
		if len(file.file.Roles) > 0 {
			roleNames := make([]string, len(file.file.Roles))
			for index, role := range file.file.Roles {
				roleNames[index] = role.String()
			}
			roles = strings.Join(roleNames, ", ")
		}
		state := "path only"
		if len(file.excerpts) > 0 {
			state = "source below"
		} else if file.omitted {
			state = "source omitted"
		}
		lines = append(lines, fmt.Sprintf("- %s — %s; %s", quote(file.file.Path), roles, state))
	}
	lines = append(lines, "End file list.")
	for _, file := range files {
		for _, excerpt := range file.excerpts {
			bytesText := ""
			if excerpt.SourceByteStart != nil && excerpt.SourceByteEnd != nil {
				bytesText = fmt.Sprintf("; UTF-8 bytes [%d, %d)", *excerpt.SourceByteStart, *excerpt.SourceByteEnd)
			}
			annotation := ""
			if excerpt.Partial || excerpt.SourceByteStart != nil {
				annotation = " (partial excerpt" + bytesText + ")"
			}
			sourceLines := strings.Split(excerpt.Source, "\n")
			if (excerpt.Partial || excerpt.SourceByteStart != nil) &&
				len(sourceLines) > excerpt.Range.EndLine-excerpt.Range.StartLine+1 &&
				sourceLines[len(sourceLines)-1] == "" {
				sourceLines = sourceLines[:len(sourceLines)-1]
			}
			lines = append(lines, "", fmt.Sprintf(
				"Source block %s lines %d-%d%s:",
				quote(file.file.Path), excerpt.Range.StartLine, excerpt.Range.EndLine, annotation,
			))
			fence := fenceFor(excerpt.Source)
			lines = append(lines, fence)
			lines = append(lines, sourceLines...)
			lines = append(lines, fence)
		}
	}
	output := strings.Join(lines, "\n") + "\n\nEnd context.\n"
	if maxOutputBytes == 0 || int64(len(output)) <= maxOutputBytes {
		return output
	}
	suffixFor := func(omitted int64) string {
		return fmt.Sprintf(
			"\nOutput truncated at the byte limit (%d bytes); %d original bytes omitted.\n\nEnd context.\n",
			maxOutputBytes, omitted,
		)
	}
	suffixBytes := int64(len(suffixFor(int64(len(output)))))
	capacity := maxOutputBytes
	if int64(len(output)) < capacity {
		capacity = int64(len(output))
	}
	var prefix strings.Builder
	if capacity > 0 {
		prefix.Grow(int(capacity))
	}
	prefixBytes := int64(0)
	openFence := ""
	for _, line := range lines {
		nextFence := ""
		if openFence != "" {
			if line == openFence {
				nextFence = ""
			} else {
				nextFence = openFence
			}
		} else if isFenceLine(line) {
			nextFence = line
		}
		lineBytes := int64(len(line) + 1)
		closingBytes := int64(0)
		if nextFence != "" {
			closingBytes = int64(len(nextFence) + 1)
		}
		if prefixBytes+lineBytes+closingBytes+suffixBytes > maxOutputBytes {
			break
		}
		prefix.WriteString(line)
		prefix.WriteByte('\n')
		prefixBytes += lineBytes
		openFence = nextFence
	}
	if openFence != "" {
		prefix.WriteString(openFence)
		prefix.WriteByte('\n')
	}
	return prefix.String() + suffixFor(int64(len(output))-prefixBytes)
}
