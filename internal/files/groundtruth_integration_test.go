package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"foco/internal/detect"
	"foco/internal/detect/catalog"
	"foco/internal/enums"
)

func TestGroundTruthReaderRoutesRealHitToSensitiveContent(t *testing.T) {
	sourceRoot := filepath.Join("..", "..", "testdata", "groundtruth")
	positive, err := os.ReadFile(filepath.Join(sourceRoot, "positive", "github_pat.json"))
	if err != nil {
		t.Fatal(err)
	}
	benign, err := os.ReadFile(filepath.Join(sourceRoot, "benign", "aws_docs.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	positivePath := filepath.Join(root, "token.json")
	benignPath := filepath.Join(root, "docs.yaml")
	if err := os.WriteFile(positivePath, positive, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(benignPath, benign, 0o644); err != nil {
		t.Fatal(err)
	}

	titus, err := detect.NewTitus()
	if err != nil {
		t.Fatal(err)
	}
	better, err := detect.NewBetterleaks(1)
	if err != nil {
		_ = titus.Close()
		t.Fatal(err)
	}
	c, err := catalog.Load()
	if err != nil {
		_ = titus.Close()
		_ = better.Close()
		t.Fatal(err)
	}
	union := detect.NewUnion(titus, better, c)
	defer union.Close()
	reader, err := New(root, DefaultPolicy(), union, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	hit := reader.ReadSnapshot(context.Background(), positivePath)
	if hit.Status != enums.SnapshotStatusExcluded || hit.Reason != enums.FileReasonSensitiveContent {
		t.Fatalf("positive snapshot = %+v, want excluded/%s", hit, enums.FileReasonSensitiveContent)
	}
	clean := reader.ReadSnapshot(context.Background(), benignPath)
	if clean.Status != enums.SnapshotStatusSnapshot {
		t.Fatalf("benign snapshot = %+v, want snapshot", clean)
	}
}
