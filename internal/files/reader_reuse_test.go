package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"foco/internal/enums"
)

func TestReadSnapshotReuseSkipsDetectorForStableIdentity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "stable.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	detector := &countingDetector{}
	reader, err := New(root, DefaultPolicy(), detector, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	// Move the read clock beyond the 3s recent-change margin so the first read is reusable.
	reader.now = func() time.Time { return time.Now().Add(10 * time.Second) }

	first := reader.ReadSnapshotReuse(context.Background(), path)
	if first.Status != enums.SnapshotStatusSnapshot || detector.calls != 1 {
		t.Fatalf("first = %+v calls=%d", first, detector.calls)
	}
	second := reader.ReadSnapshotReuse(context.Background(), path)
	if second.Status != enums.SnapshotStatusSnapshot || detector.calls != 1 {
		t.Fatalf("second = %+v calls=%d, want detector skipped", second, detector.calls)
	}
	if second.Snapshot.ContentHash != first.Snapshot.ContentHash {
		t.Fatal("reused snapshot hash changed")
	}
	if err := os.WriteFile(path, []byte("world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Rewriting the same size may not change mtime on coarse filesystems; force a distinct stamp.
	stamp := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	third := reader.ReadSnapshotReuse(context.Background(), path)
	if third.Status != enums.SnapshotStatusSnapshot || detector.calls != 2 {
		t.Fatalf("third = %+v calls=%d, want reread", third, detector.calls)
	}
	if third.Snapshot.Source != "world\n" {
		t.Fatalf("third source = %q", third.Snapshot.Source)
	}
}

func TestReusableLRUHonorsByteBudget(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	budget := reusableEntryOverhead + 16
	reader, err := NewWithLimits(root, DefaultPolicy(), nil, nil, Limits{
		MaxOpenDirectories: 64, MaxReusableBytes: budget, MaxIgnoreBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	reader.now = func() time.Time { return time.Now().Add(10 * time.Second) }
	reader.ReadSnapshotReuse(context.Background(), filepath.Join(root, "a.txt"))
	reader.ReadSnapshotReuse(context.Background(), filepath.Join(root, "b.txt"))
	if len(reader.reusable) != 1 || reader.reusableBytes > budget {
		t.Fatalf("reusable=%d bytes=%d, want one entry <= %d", len(reader.reusable), reader.reusableBytes, budget)
	}
}

func TestReusableLRUBoundsEmptySnapshots(t *testing.T) {
	root := t.TempDir()
	budget := 3 * reusableEntryOverhead
	reader, err := NewWithLimits(root, DefaultPolicy(), nil, nil, Limits{
		MaxOpenDirectories: 64, MaxReusableBytes: budget, MaxIgnoreBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	reader.now = func() time.Time { return time.Now().Add(10 * time.Second) }
	state := func() (int, int64) {
		reader.reuseMu.Lock()
		defer reader.reuseMu.Unlock()
		return len(reader.reusable), reader.reusableBytes
	}
	for index := 0; index < 40; index++ {
		path := filepath.Join(root, fmt.Sprintf("empty-%02d.txt", index))
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		got := reader.ReadSnapshotReuse(context.Background(), path)
		if got.Status != enums.SnapshotStatusSnapshot || got.Snapshot == nil {
			t.Fatalf("read %d = %+v", index, got)
		}
		entries, accounted := state()
		if entries > 3 {
			t.Fatalf("after %d reads: entries=%d, want <= 3", index+1, entries)
		}
		if accounted > budget {
			t.Fatalf("after %d reads: bytes=%d, want <= %d", index+1, accounted, budget)
		}
		if accounted != int64(entries)*reusableEntryOverhead {
			t.Fatalf("after %d reads: accounted=%d entries=%d, want %d per entry", index+1, accounted, entries, reusableEntryOverhead)
		}
	}
}

func TestReadSnapshotReuseConcurrent(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, 4)
	for index := range paths {
		paths[index] = filepath.Join(root, fmt.Sprintf("file-%d.txt", index))
		if err := os.WriteFile(paths[index], []byte(fmt.Sprintf("content-%d\n", index)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := NewWithLimits(root, DefaultPolicy(), nil, nil, Limits{
		MaxOpenDirectories: 2, MaxReusableBytes: 2*reusableEntryOverhead + 32, MaxIgnoreBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	reader.now = func() time.Time { return time.Now().Add(10 * time.Second) }

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for round := 0; round < 50; round++ {
				path := paths[(worker+round)%len(paths)]
				got := reader.ReadSnapshotReuse(context.Background(), path)
				if got.Status != enums.SnapshotStatusSnapshot || got.Snapshot == nil {
					t.Errorf("worker %d round %d: %+v", worker, round, got)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
}
