package files

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"foco/internal/enums"
	"foco/internal/filetypes"
)

func fileTypeReason(category enums.FileCategory) enums.FileReason {
	if !category.IsValid() {
		return enums.UnknownFileReason
	}
	reason, ok := enums.NewFileReason("file_type:" + category.String())
	if !ok {
		return enums.UnknownFileReason
	}
	return reason
}

func (r *Reader) decide(ctx context.Context, path string, final bool, allowMissing bool) (Decision, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Decision{}, err
	}
	absolute = resolveAncestors(filepath.Clean(absolute))
	if !inside(r.root, absolute) {
		real, err := filepath.EvalSymlinks(absolute)
		if err != nil || real != r.root {
			return Decision{Path: absolute, Reason: enums.FileReasonOutsideRoot}, nil
		}
		absolute = real
	}
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	rel, err := filepath.Rel(r.root, absolute)
	if err != nil {
		return Decision{Path: absolute, Reason: enums.FileReasonOutsideRoot}, nil
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == ".git" {
			return Decision{Path: absolute, Reason: enums.FileReasonProtected}, nil
		}
	}
	for _, p := range r.protected {
		if inside(p, absolute) {
			return Decision{Path: absolute, Reason: enums.FileReasonProtected}, nil
		}
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		if allowMissing && errors.Is(err, fs.ErrNotExist) {
			return Decision{Path: absolute, Reason: enums.FileReasonMissing}, nil
		}
		return Decision{Path: absolute, Reason: enums.UnknownFileReason}, err
	}
	name := filepath.Base(absolute)
	sensitivePath := !r.policy.IncludeSensitive && r.policy.SensitivePaths != nil && r.policy.SensitivePaths.Match(filepath.ToSlash(rel))
	for _, part := range parts {
		if part == "." || part == ".." {
			continue
		}
		if !r.policy.Hidden && strings.HasPrefix(part, ".") {
			return Decision{Path: absolute, Reason: enums.FileReasonHidden, Info: info}, nil
		}
		if !r.policy.IncludeSensitive && (IsSensitiveName(part) || sensitivePath) {
			return Decision{Path: absolute, Reason: enums.FileReasonSensitiveName, Info: info}, nil
		}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Decision{Path: absolute, Reason: enums.FileReasonSymlink, Info: info}, nil
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return Decision{Path: absolute, Reason: enums.FileReasonSpecialFile, Info: info}, nil
	}
	if !r.policy.IncludeDependencies {
		for _, part := range parts {
			if IsDependencyName(part) {
				return Decision{Path: absolute, Reason: enums.FileReasonDependency, Info: info}, nil
			}
		}
	}
	if r.ignore.Ignored(rel, info.IsDir()) {
		if ExcludedByPattern(r.policy.Exclude, rel, info.IsDir()) {
			return Decision{Path: absolute, Reason: enums.FileReasonExcludePattern, Info: info}, nil
		}
		return Decision{Path: absolute, Reason: enums.FileReasonIgnored, Info: info}, nil
	}
	if final && info.Mode().IsRegular() && (len(r.policy.Only) > 0 || len(r.policy.ExcludeTypes) > 0) {
		var content []byte
		if ext := filepath.Ext(name); ext == "" {
			file, err := os.Open(absolute)
			if err == nil {
				buffer := make([]byte, 1024)
				n, _ := file.Read(buffer)
				content = buffer[:n]
				file.Close()
			}
		}
		category, _ := filetypes.Classify(name, content)
		if !CategoryAllowed(r.policy, category) {
			return Decision{Path: absolute, Reason: fileTypeReason(category), Info: info}, nil
		}
	}
	if !info.IsDir() {
		for i := range parts {
			if i == len(parts)-1 {
				break
			}
			parent := filepath.Join(r.root, filepath.Join(parts[:i+1]...))
			parentInfo, err := os.Lstat(parent)
			if err != nil || !parentInfo.IsDir() {
				return Decision{Path: absolute, Reason: enums.FileReasonNotDirectory, Info: info}, nil
			}
		}
	}
	return Decision{Path: absolute, Eligible: true, Info: info}, nil
}

func resolveAncestors(path string) string {
	dir, base := filepath.Split(path)
	for {
		if real, err := filepath.EvalSymlinks(filepath.Clean(dir)); err == nil {
			return filepath.Join(real, base)
		}
		parent := filepath.Dir(filepath.Clean(dir))
		if parent == filepath.Clean(dir) {
			return path
		}
		base = filepath.Join(filepath.Base(filepath.Clean(dir)), base)
		dir = parent
	}
}
