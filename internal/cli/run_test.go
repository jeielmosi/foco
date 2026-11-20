package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelpAndVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(nil, &out, &errOut); code != 0 {
		t.Fatalf("help exit %d", code)
	}
	if !strings.Contains(out.String(), `Usage: foco "question" [root]`) {
		t.Fatalf("help missing usage: %s", out.String())
	}
	out.Reset()
	if code := Run([]string{"--version"}, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) == "" {
		t.Fatalf("version: code=%d out=%q", code, out.String())
	}
}

func TestRunUnknownOption(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"--nope"}, &out, &errOut); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "Unknown option") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}
