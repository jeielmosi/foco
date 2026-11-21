package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanReportsSecretSpans(t *testing.T) {
	root := t.TempDir()
	// Split so this source file is not itself flagged by secret scanners.
	secret := "hunter2!" + "Prod2026"
	content := "db_password = \"" + secret + "\"\n"
	if err := os.WriteFile(filepath.Join(root, "config.py"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "clean.go"), []byte("package clean\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), []string{"scan", "--json", root}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	var finding scanFinding
	if err := json.Unmarshal(bytes.SplitN(stdout.Bytes(), []byte("\n"), 2)[0], &finding); err != nil {
		t.Fatalf("output %q: %v", stdout.String(), err)
	}
	if !strings.HasSuffix(finding.Path, "config.py") || content[finding.Start:finding.End] != secret || finding.Line != 1 {
		t.Fatalf("finding = %+v", finding)
	}
}

func TestScanCleanTreeExitsZero(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := RunContext(context.Background(), []string{"scan", root}, strings.NewReader(""), &stdout, &stderr); code != 0 || stdout.Len() != 0 {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}
