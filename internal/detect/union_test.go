package detect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"foco/internal/detect/catalog"
)

type fakeEngine struct {
	name     string
	called   int
	result   bool
	err      error
	closed   int
	closeErr error
	order    *[]string
}

func (f *fakeEngine) DetectPath(context.Context, string) (bool, error) {
	f.called++
	if f.order != nil {
		*f.order = append(*f.order, f.name)
	}
	return f.result, f.err
}

func (f *fakeEngine) Close() error {
	f.closed++
	return f.closeErr
}

func loadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	return c
}

func TestUnionPrimaryHitStops(t *testing.T) {
	titus := &fakeEngine{name: "titus", result: true}
	better := &fakeEngine{name: "betterleaks", result: true}
	u := NewUnion(titus, better, loadCatalog(t))
	excluded, err := u.DetectPath(context.Background(), "main.go")
	if err != nil || !excluded || better.called != 0 {
		t.Fatalf("excluded=%v err=%v betterCalls=%d", excluded, err, better.called)
	}
}

func TestUnionSecondPassOnConfigPath(t *testing.T) {
	titus := &fakeEngine{name: "titus", result: false}
	better := &fakeEngine{name: "betterleaks", result: true}
	u := NewUnion(titus, better, loadCatalog(t))
	excluded, err := u.DetectPath(context.Background(), "config/app.yaml")
	if err != nil || !excluded || better.called != 1 {
		t.Fatalf("excluded=%v err=%v betterCalls=%d", excluded, err, better.called)
	}
}

func TestUnionSkipsSecondPass(t *testing.T) {
	titus := &fakeEngine{name: "titus", result: false}
	better := &fakeEngine{name: "betterleaks", result: true}
	u := NewUnion(titus, better, loadCatalog(t))
	excluded, err := u.DetectPath(context.Background(), "main.go")
	if err != nil || excluded || better.called != 0 {
		t.Fatalf("excluded=%v err=%v betterCalls=%d", excluded, err, better.called)
	}
}

func TestUnionErrorSuppressesSecondPass(t *testing.T) {
	boom := errors.New("titus failed")
	titus := &fakeEngine{name: "titus", err: boom}
	better := &fakeEngine{name: "betterleaks", result: true}
	u := NewUnion(titus, better, loadCatalog(t))
	excluded, err := u.DetectPath(context.Background(), "config/app.yaml")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if excluded {
		t.Fatalf("excluded = true, want false")
	}
	if better.called != 0 {
		t.Fatalf("betterCalls = %d, want 0", better.called)
	}
}

func TestUnionCatalogOnlyPathRunsSecondPass(t *testing.T) {
	titus := &fakeEngine{name: "titus", result: false}
	better := &fakeEngine{name: "betterleaks", result: true}
	u := NewUnion(titus, better, loadCatalog(t))
	excluded, err := u.DetectPath(context.Background(), ".npmrc")
	if err != nil || !excluded {
		t.Fatalf("excluded=%v err=%v", excluded, err)
	}
	if better.called != 1 {
		t.Fatalf("betterCalls = %d, want 1", better.called)
	}
}

func TestUnionCallOrder(t *testing.T) {
	var order []string
	titus := &fakeEngine{name: "titus", order: &order}
	better := &fakeEngine{name: "betterleaks", order: &order}
	u := NewUnion(titus, better, loadCatalog(t))
	if _, err := u.DetectPath(context.Background(), "config/app.yaml"); err != nil {
		t.Fatalf("DetectPath: %v", err)
	}
	want := []string{"titus", "betterleaks"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestUnionAllRunsBothEnginesAndORs(t *testing.T) {
	cases := []struct {
		name          string
		titus, better bool
		want          bool
	}{
		{name: "titus hit", titus: true, better: false, want: true},
		{name: "betterleaks hit", titus: false, better: true, want: true},
		{name: "both miss", titus: false, better: false, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			titus := &fakeEngine{name: "titus", result: tc.titus}
			better := &fakeEngine{name: "betterleaks", result: tc.better}
			u := NewUnionAll(titus, better, loadCatalog(t))
			excluded, err := u.DetectPath(context.Background(), "main.go")
			if err != nil || excluded != tc.want {
				t.Fatalf("excluded=%v err=%v want=%v", excluded, err, tc.want)
			}
			if titus.called != 1 || better.called != 1 {
				t.Fatalf("titus calls=%d betterleaks calls=%d, want 1 each", titus.called, better.called)
			}
		})
	}
}

func TestUnionAllRunsSecondEngineAfterPrimaryError(t *testing.T) {
	boom := errors.New("boom")
	titus := &fakeEngine{name: "titus", err: boom}
	better := &fakeEngine{name: "betterleaks", result: true}
	u := NewUnionAll(titus, better, loadCatalog(t))
	if _, err := u.DetectPath(context.Background(), "main.go"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if titus.called != 1 || better.called != 1 {
		t.Fatalf("titus calls=%d betterleaks calls=%d, want 1 each", titus.called, better.called)
	}
}

func TestUnionAllReportsBothEngineFailures(t *testing.T) {
	titus := &fakeEngine{name: "titus", err: errors.New("titus failed")}
	better := &fakeEngine{name: "betterleaks", err: errors.New("betterleaks failed")}
	u := NewUnionAll(titus, better, loadCatalog(t))
	if _, err := u.DetectPath(context.Background(), "main.go"); err == nil {
		t.Fatal("expected an error when both engines fail")
	}
	if titus.called != 1 || better.called != 1 {
		t.Fatalf("titus calls=%d betterleaks calls=%d, want 1 each", titus.called, better.called)
	}
}

func TestUnionCloseClosesBoth(t *testing.T) {
	titus := &fakeEngine{name: "titus"}
	better := &fakeEngine{name: "betterleaks"}
	u := NewUnion(titus, better, loadCatalog(t))
	if err := u.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if titus.closed != 1 || better.closed != 1 {
		t.Fatalf("titusClosed=%d betterClosed=%d, want 1 each", titus.closed, better.closed)
	}
}

type spyEngine struct {
	Engine
	called int
}

func (s *spyEngine) DetectPath(ctx context.Context, path string) (bool, error) {
	s.called++
	return s.Engine.DetectPath(ctx, path)
}

func TestUnionRealSecondPassOnEnv(t *testing.T) {
	titus, err := NewTitus()
	if err != nil {
		t.Fatal(err)
	}
	better, err := NewBetterleaks(1)
	if err != nil {
		t.Fatal(err)
	}
	spy := &spyEngine{Engine: better}
	union := NewUnion(titus, spy, loadCatalog(t))
	defer union.Close()

	path := filepath.Join(t.TempDir(), ".env")
	// Verified fixture: Titus misses this high-entropy password assignment,
	// while betterleaks' generic API-key rule detects it.
	if err := os.WriteFile(path, []byte("db_password=SuperSecret123!@#\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	primary, err := titus.DetectPath(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if primary {
		t.Fatal("fixture must be missed by Titus for the second pass to be exercised")
	}
	secondary, err := better.DetectPath(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !secondary {
		t.Fatal("fixture must be detected by betterleaks")
	}

	excluded, err := union.DetectPath(context.Background(), path)
	if err != nil || !excluded {
		t.Fatalf("excluded=%v err=%v", excluded, err)
	}
	if spy.called != 1 {
		t.Fatalf("betterleaks calls = %d, want 1", spy.called)
	}
}
