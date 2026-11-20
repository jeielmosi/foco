package detect

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"foco/internal/detect/catalog"
)

type groundTruthCase struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	Generate string `json:"generate"`
}

type groundTruthManifest struct {
	Version  int               `json:"version"`
	Positive []groundTruthCase `json:"positive"`
	Benign   []groundTruthCase `json:"benign"`
}

func loadGroundTruth(t *testing.T) (groundTruthManifest, string) {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "groundtruth")
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest groundTruthManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 {
		t.Fatalf("manifest version = %d, want 1", manifest.Version)
	}
	if len(manifest.Positive) != 5 {
		t.Fatalf("positive cases = %d, want 5", len(manifest.Positive))
	}
	if len(manifest.Benign) == 0 {
		t.Fatal("manifest has no benign cases")
	}
	return manifest, root
}

func materializeGroundTruthCase(t *testing.T, sourceRoot, targetRoot string, tc groundTruthCase) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sourceRoot, tc.Path))
	if err != nil {
		t.Fatal(err)
	}
	switch tc.Generate {
	case "":
	case "rsa2048":
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		data = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	default:
		t.Fatalf("%s: unsupported generate %q", tc.Type, tc.Generate)
	}
	target := filepath.Join(targetRoot, tc.Path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestGroundTruthRecallAndFalsePositives(t *testing.T) {
	manifest, sourceRoot := loadGroundTruth(t)
	targetRoot := t.TempDir()
	titus, err := NewTitus()
	if err != nil {
		t.Fatal(err)
	}
	better, err := NewBetterleaks(1)
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
	union := NewUnion(titus, better, c)
	defer union.Close()
	ctx := context.Background()

	missed := []string{}
	for _, tc := range manifest.Positive {
		path := materializeGroundTruthCase(t, sourceRoot, targetRoot, tc)
		excluded, err := union.DetectPath(ctx, path)
		if err != nil {
			t.Fatalf("%s: %v", tc.Type, err)
		}
		if !excluded {
			missed = append(missed, tc.Type)
		}
	}

	falsePositives := []string{}
	for _, tc := range manifest.Benign {
		path := materializeGroundTruthCase(t, sourceRoot, targetRoot, tc)
		excluded, err := union.DetectPath(ctx, path)
		if err != nil {
			t.Fatalf("%s: %v", tc.Type, err)
		}
		if excluded {
			falsePositives = append(falsePositives, tc.Type)
		}
	}

	total := len(manifest.Positive)
	hits := total - len(missed)
	recall := float64(hits) / float64(total)
	t.Logf("ground truth recall=%.3f (%d/%d), false positives=%d/%d",
		recall, hits, total, len(falsePositives), len(manifest.Benign))
	if len(missed) > 0 {
		t.Fatalf("planted secrets passed the gate: %v", missed)
	}
	if len(falsePositives) > 0 {
		t.Fatalf("benign fixtures were excluded: %v", falsePositives)
	}
	if recall != 1 {
		t.Fatalf("recall = %.3f, want 1.000", recall)
	}
}
