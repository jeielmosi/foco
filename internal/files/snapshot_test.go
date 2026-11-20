package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"foco/internal/enums"
)

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type fakeDetector struct {
	excluded bool
	err      error
}

func (f fakeDetector) DetectPath(context.Context, string) (bool, error) {
	return f.excluded, f.err
}

type countingDetector struct {
	excluded bool
	err      error
	calls    int
}

func (d *countingDetector) DetectPath(context.Context, string) (bool, error) {
	d.calls++
	return d.excluded, d.err
}

func newTestReader(t *testing.T, root string, policy Policy, detector ContentChecker) *Reader {
	t.Helper()
	reader, err := New(root, policy, detector, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	return reader
}

func TestReadSnapshotStatuses(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(name string, data []byte) {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("ok.txt", []byte("hello\n"))
	mustWrite("binary.bin", []byte{0x00, 0x01})
	mustWrite("bad.txt", []byte{0xff, 0xfe})
	mustWrite("secret.txt", []byte("token=abc\n"))

	reader := newTestReader(t, root, DefaultPolicy(), fakeDetector{excluded: true})
	cases := map[string]SnapshotResult{
		"missing.txt": {Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueUnreadable},
		"binary.bin":  {Status: enums.SnapshotStatusExcluded, Reason: enums.FileReasonBinary},
		"bad.txt":     {Status: enums.SnapshotStatusExcluded, Reason: enums.FileReasonInvalidUTF8},
		"secret.txt":  {Status: enums.SnapshotStatusExcluded, Reason: enums.FileReasonSensitiveContent},
	}
	for name, want := range cases {
		got := reader.ReadSnapshot(context.Background(), filepath.Join(root, name))
		if got.Status != want.Status || got.Reason != want.Reason || got.Issue != want.Issue {
			t.Errorf("%s: got %+v, want %+v", name, got, want)
		}
	}
	reader2 := newTestReader(t, root, DefaultPolicy(), fakeDetector{})
	ok := reader2.ReadSnapshot(context.Background(), filepath.Join(root, "ok.txt"))
	if ok.Status != enums.SnapshotStatusSnapshot || ok.Snapshot.ContentHash == "" || ok.Snapshot.Source != "hello\n" {
		t.Fatalf("ok snapshot: %+v", ok)
	}
}

func TestReadSnapshotIncludeSensitiveSkipsDetector(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plain.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy := DefaultPolicy()
	policy.IncludeSensitive = true
	detector := &countingDetector{excluded: true}
	reader := newTestReader(t, root, policy, detector)

	got := reader.ReadSnapshot(context.Background(), filepath.Join(root, "plain.txt"))
	if got.Status != enums.SnapshotStatusSnapshot {
		t.Fatalf("got %+v, want snapshot", got)
	}
	if got.Snapshot == nil || got.Snapshot.Source != "hello\n" {
		t.Fatalf("snapshot source: %+v", got.Snapshot)
	}
	if detector.calls != 0 {
		t.Fatalf("detector calls = %d, want 0", detector.calls)
	}
}

func TestReadSnapshotDetectorError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plain.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, root, DefaultPolicy(), fakeDetector{err: errors.New("boom")})

	got := reader.ReadSnapshot(context.Background(), filepath.Join(root, "plain.txt"))
	want := SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueDetector}
	if got.Status != want.Status || got.Issue != want.Issue {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadSnapshotDetectorCancelled(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plain.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, root, DefaultPolicy(), fakeDetector{err: context.Canceled})

	got := reader.ReadSnapshot(context.Background(), filepath.Join(root, "plain.txt"))
	want := SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueInterrupted}
	if got.Status != want.Status || got.Issue != want.Issue {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadSnapshotDirectoryIsNotFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, root, DefaultPolicy(), nil)

	got := reader.ReadSnapshot(context.Background(), filepath.Join(root, "dir"))
	want := SnapshotResult{Status: enums.SnapshotStatusExcluded, Reason: enums.FileReasonNotFile}
	if got.Status != want.Status || got.Reason != want.Reason {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadSnapshotSizeBoundary(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "four.txt"), []byte("abcd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "five.txt"), []byte("abcde"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, root, Policy{MaxFileBytes: 4}, nil)

	atLimit := reader.ReadSnapshot(context.Background(), filepath.Join(root, "four.txt"))
	if atLimit.Status != enums.SnapshotStatusSnapshot {
		t.Fatalf("four bytes: got %+v, want snapshot", atLimit)
	}
	overLimit := reader.ReadSnapshot(context.Background(), filepath.Join(root, "five.txt"))
	want := SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueResourceLimit}
	if overLimit.Status != want.Status || overLimit.Issue != want.Issue {
		t.Fatalf("five bytes: got %+v, want %+v", overLimit, want)
	}
}

func TestReadSnapshotEmptyFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, root, DefaultPolicy(), nil)

	got := reader.ReadSnapshot(context.Background(), filepath.Join(root, "empty.txt"))
	if got.Status != enums.SnapshotStatusSnapshot {
		t.Fatalf("got %+v, want snapshot", got)
	}
	if got.Snapshot == nil || got.Snapshot.Source != "" || got.Snapshot.ContentHash != emptySHA256 {
		t.Fatalf("empty snapshot: %+v", got.Snapshot)
	}
}

type rewritingDetector struct{}

func (rewritingDetector) DetectPath(_ context.Context, path string) (bool, error) {
	if err := os.WriteFile(path, []byte("rewritten content that is longer\n"), 0o644); err != nil {
		return false, err
	}
	return false, nil
}

func TestReadSnapshotDetectorRewriteIsChanged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plain.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, root, DefaultPolicy(), rewritingDetector{})

	got := reader.ReadSnapshot(context.Background(), path)
	want := SnapshotResult{Status: enums.SnapshotStatusIssue, Issue: enums.FileIssueChanged}
	if got.Status != want.Status || got.Issue != want.Issue {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// The include-sensitive path skips the detector, so a deterministic TOCTOU
// rewrite cannot be injected through it; the post-read revalidation is covered
// directly through its identity helpers instead.
func TestSameFileInfoDetectsIdentityAndMetadataChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plain.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !sameFileInfo(before, after) {
		t.Fatal("same file info should match")
	}
	if !sameFileInfoAt(path, before) {
		t.Fatal("sameFileInfoAt should match the current file")
	}
	if sameFileInfo(before, nil) || sameFileInfo(nil, after) {
		t.Fatal("nil file info must not match")
	}
	if err := os.WriteFile(path, []byte("rewritten and longer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if sameFileInfoAt(path, before) {
		t.Fatal("sameFileInfoAt should detect the rewritten file")
	}
}
