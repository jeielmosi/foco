//go:build !windows

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the binary")
	}
	binary := filepath.Join(t.TempDir(), "foco")
	command := exec.Command("go", "build", "-o", binary, ".")
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "GOTOOLCHAIN=") || strings.HasPrefix(entry, "CGO_ENABLED=") {
			continue
		}
		env = append(env, entry)
	}
	command.Env = append(env, "GOTOOLCHAIN=auto", "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	return binary
}

func TestClosedStdoutPipeDoesNotDieOnSIGPIPE(t *testing.T) {
	binary := buildBinary(t)
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	defer write.Close()

	var errorOutput bytes.Buffer
	command := exec.Command(binary, "--help")
	command.Stdout = write
	command.Stderr = &errorOutput
	err = command.Run()
	_ = write.Close()

	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			t.Fatalf("process died from %v (fd 1 SIGPIPE not ignored); stderr %q",
				status.Signal(), errorOutput.String())
		}
		t.Fatalf("exit %d; stderr %q", exitError.ExitCode(), errorOutput.String())
	}
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr %q", errorOutput.String())
	}
}
