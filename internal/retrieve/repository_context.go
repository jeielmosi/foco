package retrieve

import (
	"context"
	"path"

	"foco/internal/enums"
)

func gatherRepositoryContext(ctx context.Context, reader Reader, entries []FileEvidence) (RepositoryContext, error) {
	seen := map[string]bool{".": true}
	directories := []string{"."}
	for _, entry := range entries {
		for directory := path.Dir(entry.Path); directory != "." && directory != "/"; directory = path.Dir(directory) {
			if !seen[directory] {
				seen[directory] = true
				directories = append(directories, directory)
			}
			parent := path.Dir(directory)
			if parent == directory {
				break
			}
		}
	}
	instructionFiles := []string{}
	incomplete := false
	for _, directory := range directories {
		relative := "AGENTS.md"
		if directory != "." {
			relative = directory + "/AGENTS.md"
		}
		result, err := reader.LookupFile(ctx, absPath(reader.Root(), relative))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return RepositoryContext{InstructionFiles: instructionFiles, InstructionLookupIncomplete: true}, ctxErr
			}
			incomplete = true
			continue
		}
		if result.Status == enums.LookupStatusFile {
			instructionFiles = append(instructionFiles, relative)
			continue
		}
		if result.Status != enums.LookupStatusExcluded || result.Reason != enums.FileReasonMissing {
			incomplete = true
		}
	}
	return RepositoryContext{InstructionFiles: instructionFiles, InstructionLookupIncomplete: incomplete}, nil
}
