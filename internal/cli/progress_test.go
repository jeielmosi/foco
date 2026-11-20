package cli

import (
	"bytes"
	"os"
	"testing"
)

func TestTerminalDetection(t *testing.T) {
	var buffer bytes.Buffer
	if terminalWriter(&buffer) {
		t.Fatal("bytes.Buffer reported as terminal writer")
	}
	if isInteractive(&buffer) {
		t.Fatal("bytes.Buffer reported as interactive")
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	if terminalWriter(devNull) {
		t.Fatalf("%s reported as terminal writer", os.DevNull)
	}
	if isInteractive(devNull) {
		t.Fatalf("%s reported as interactive", os.DevNull)
	}
}

func TestProgressLineExactFormat(t *testing.T) {
	if got := progressLine(0, 1, 1536); got != "- Searching — 1 file · 1.5 KiB" {
		t.Fatalf("line = %q", got)
	}
	if got := progressLine(2, 3, 1024); got != "| Searching — 3 files · 1.0 KiB" {
		t.Fatalf("line = %q", got)
	}
	if got := progressLine(1, 2, 1023); got != "\\ Searching — 2 files · 1023 B" {
		t.Fatalf("line = %q", got)
	}
}
