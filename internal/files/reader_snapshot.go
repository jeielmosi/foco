package files

import (
	"context"
	"errors"
	"io"
	"os"
	"unicode/utf8"

	"foco/internal/enums"
)

func (r *Reader) ReadSnapshot(ctx context.Context, path string) SnapshotResult {
	return r.readSnapshot(ctx, path, false)
}

func (r *Reader) ReadSnapshotReuse(ctx context.Context, path string) SnapshotResult {
	return r.readSnapshot(ctx, path, true)
}

func (r *Reader) readSnapshot(ctx context.Context, path string, reuse bool) SnapshotResult {
	started := r.now().UnixNano()
	if ctx.Err() != nil {
		return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueInterrupted}
	}
	decision, err := r.decide(ctx, path, true, false)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueInterrupted}
		}
		return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueUnreadable}
	}
	if !decision.Eligible {
		r.forgetReusable(decision.Path)
		return SnapshotResult{Status: enums.SnapshotStatusExcluded, Reason: decision.Reason}
	}
	if decision.Info == nil || !decision.Info.Mode().IsRegular() {
		r.forgetReusable(decision.Path)
		return SnapshotResult{Status: enums.SnapshotStatusExcluded, Reason: enums.FileReasonNotFile}
	}
	if reuse {
		if snapshot, ok := r.reusableSnapshot(decision.Path); ok {
			return SnapshotResult{Status: enums.SnapshotStatusSnapshot, Snapshot: &snapshot}
		}
	}
	if r.policy.MaxFileBytes > 0 && decision.Info.Size() > r.policy.MaxFileBytes {
		return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueResourceLimit}
	}
	data, issue := readFileHardened(decision.Path, decision.Info, r.policy.MaxFileBytes)
	if issue != enums.UnknownFileIssueKind {
		return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: issue}
	}
	for _, b := range data {
		if b <= 0x08 || b == 0x0b || (b >= 0x0e && b <= 0x1f) || b == 0x7f {
			return SnapshotResult{Status: enums.SnapshotStatusExcluded, Reason: enums.FileReasonBinary}
		}
	}
	if !utf8.Valid(data) {
		return SnapshotResult{Status: enums.SnapshotStatusExcluded, Reason: enums.FileReasonInvalidUTF8}
	}
	if !r.policy.IncludeSensitive && r.detector != nil {
		excluded, err := r.detector.DetectPath(ctx, decision.Path)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueInterrupted}
			}
			return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueDetector}
		}
		if excluded {
			return SnapshotResult{Status: enums.SnapshotStatusExcluded, Reason: enums.FileReasonSensitiveContent}
		}
	}
	if !sameFileInfoAt(decision.Path, decision.Info) {
		return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueChanged}
	}
	recheck, err := r.decide(ctx, decision.Path, true, false)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueInterrupted}
		}
		return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueChanged}
	}
	if !recheck.Eligible || !sameFileInfo(decision.Info, recheck.Info) {
		return SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueChanged}
	}
	snapshot := Snapshot{Path: decision.Path, Source: string(data), ContentHash: hashBytes(data)}
	identity := identityFor(decision.Info)
	if canReuseIdentity(identity, started) {
		r.retainReusable(decision.Path, snapshot, identity, int64(len(data)))
	}
	return SnapshotResult{Status: enums.SnapshotStatusSnapshot, Snapshot: &snapshot}
}

func readFileHardened(path string, expected os.FileInfo, max int64) ([]byte, enums.FileIssueKind) {
	file, err := openReadonly(path)
	if err != nil {
		return nil, enums.FileIssueUnreadable
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return nil, enums.FileIssueUnreadable
	}
	if !before.Mode().IsRegular() {
		return nil, enums.FileIssueChanged
	}
	if expected != nil && (!os.SameFile(expected, before) || before.Size() != expected.Size() || !before.ModTime().Equal(expected.ModTime())) {
		return nil, enums.FileIssueChanged
	}
	if max > 0 && before.Size() > max {
		return nil, enums.FileIssueResourceLimit
	}
	var data []byte
	if max > 0 {
		data, err = io.ReadAll(io.LimitReader(file, max+1))
	} else {
		data, err = io.ReadAll(file)
	}
	if err != nil {
		return nil, enums.FileIssueUnreadable
	}
	if max > 0 && int64(len(data)) > max {
		return nil, enums.FileIssueResourceLimit
	}
	after, err := file.Stat()
	if err != nil {
		return nil, enums.FileIssueUnreadable
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return nil, enums.FileIssueChanged
	}
	return data, enums.UnknownFileIssueKind
}

func sameFileInfoAt(path string, info os.FileInfo) bool {
	after, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return sameFileInfo(info, after)
}

func sameFileInfo(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return false
	}
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
