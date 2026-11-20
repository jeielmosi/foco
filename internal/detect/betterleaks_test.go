package detect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestBetterleaksPooledDetectorReuse(t *testing.T) {
	b, err := NewBetterleaks(1)
	if err != nil {
		t.Fatalf("NewBetterleaks: %v", err)
	}
	defer b.Close()

	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("ordinary prose with no credentials here\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 2; i++ {
		found, err := b.DetectPath(context.Background(), path)
		if err != nil {
			t.Fatalf("scan %d: %v", i, err)
		}
		if found {
			t.Fatalf("scan %d: stale finding on clean file", i)
		}
	}
}

func TestBetterleaksDetectsKnownSecret(t *testing.T) {
	b, err := NewBetterleaks(1)
	if err != nil {
		t.Fatalf("NewBetterleaks: %v", err)
	}
	defer b.Close()

	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.txt")
	if err := os.WriteFile(clean, []byte("ordinary prose with no credentials here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Verified fixture: betterleaks' generic API-key rule detects this
	// high-entropy password assignment while Titus does not.
	secret := filepath.Join(dir, "secret.env")
	if err := os.WriteFile(secret, []byte("db_password=SuperSecret123!@#\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	found, err := b.DetectPath(context.Background(), clean)
	if err != nil || found {
		t.Fatalf("clean: found=%v err=%v", found, err)
	}
	found, err = b.DetectPath(context.Background(), secret)
	if err != nil || !found {
		t.Fatalf("secret: found=%v err=%v", found, err)
	}
}

func TestBetterleaksConcurrentScans(t *testing.T) {
	b, err := NewBetterleaks(2)
	if err != nil {
		t.Fatalf("NewBetterleaks: %v", err)
	}
	defer b.Close()

	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.txt")
	if err := os.WriteFile(clean, []byte("ordinary prose with no credentials here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "secret.env")
	if err := os.WriteFile(secret, []byte("db_password=SuperSecret123!@#\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n*2)
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			found, err := b.DetectPath(context.Background(), clean)
			if err != nil {
				errs <- err
				return
			}
			if found {
				errs <- errors.New("clean: got found=true")
			}
		}()
		go func() {
			defer wg.Done()
			found, err := b.DetectPath(context.Background(), secret)
			if err != nil {
				errs <- err
				return
			}
			if !found {
				errs <- errors.New("secret: got found=false")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
