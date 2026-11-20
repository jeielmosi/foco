//go:build !windows

package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"foco/internal/enums"
	"foco/internal/retrieve"
	"foco/internal/skill"
)

type failAfterWriter struct {
	limit   int
	written bytes.Buffer
}

func (w *failAfterWriter) Write(data []byte) (int, error) {
	if w.limit >= len(data) {
		w.limit -= len(data)
		return w.written.Write(data)
	}
	n, _ := w.written.Write(data[:w.limit])
	w.limit = 0
	return n, syscall.EPIPE
}

func TestOutputWriterSwallowsBrokenPipe(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	defer write.Close()

	called := false
	output := newOutputWriter(write)
	output.setOnClose(func() { called = true })
	n, err := output.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if !output.Closed() {
		t.Fatal("writer did not mark the closed pipe")
	}
	if !called {
		t.Fatal("onClose was not called")
	}
	if output.Failure() != nil {
		t.Fatalf("failure = %v", output.Failure())
	}
}

func TestRunHelpAndVersionWithClosedStdoutExitZero(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--version"}} {
		read, write, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := read.Close(); err != nil {
			t.Fatal(err)
		}
		var errorOutput bytes.Buffer
		code := Run(args, write, &errorOutput)
		_ = write.Close()
		if code != 0 {
			t.Fatalf("%v: exit %d", args, code)
		}
		if errorOutput.Len() != 0 {
			t.Fatalf("%v: stderr %q", args, errorOutput.String())
		}
	}
}

func TestFinishClosedStdoutReturnsZero(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	output := newOutputWriter(write)
	if code := finish(output, errors.New("command failed")); code != 0 {
		t.Fatalf("exit %d", code)
	}
	_ = write.Close()
	if !output.Closed() {
		t.Fatal("closed pipe was not detected")
	}
}

func TestFinishSearchClosedStdoutSkipsRender(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	output := newOutputWriter(write)
	code := finishSearch(
		retrieve.RetrievalResult{Status: enums.RetrievalStatusComplete},
		nil,
		SearchOptions{},
		output,
		newProgress(false, output),
	)
	_ = write.Close()
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !output.Closed() {
		t.Fatal("closed pipe was not detected")
	}
	if output.Failure() != nil {
		t.Fatalf("failure %v", output.Failure())
	}
}

type epipeSkillRunner struct{}

func (epipeSkillRunner) NodeVersion(context.Context) (string, error) {
	return "22.20.0", nil
}

func (epipeSkillRunner) Run(_ context.Context, _ string, _ []string, process skill.Process) (int, error) {
	_, _ = io.WriteString(process.Stdout, "installer-output\n")
	return 1, skill.ErrInstallerFailed
}

func TestRunSkillClosedStdoutExitsZeroQuietly(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	withSkillRunner(t, epipeSkillRunner{})
	var errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"skill"}, strings.NewReader(""), write, &errorOutput)
	_ = write.Close()
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr %q", errorOutput.String())
	}
}

func TestSubprocessSkillClosedStdoutExitsZeroWithRealRunner(t *testing.T) {
	binary := buildFoco(t)
	binDir := t.TempDir()
	fakes := map[string]string{
		"node": "#!/bin/sh\nprintf 'v22.20.0\\n'\n",
		"npx":  "#!/bin/sh\nprintf 'installer-output\\n'\nexit 1\n",
	}
	for name, body := range fakes {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "skill")
	command.Env = subprocessEnv("PATH=" + binDir)
	command.Stdout = write
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		_ = write.Close()
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	var waitErr error
	select {
	case waitErr = <-waited:
	case <-time.After(10 * time.Second):
		_ = killAndWait(t, command, waited)
		_ = write.Close()
		t.Fatal("foco skill did not exit after its stdout pipe closed")
	}
	_ = write.Close()
	if waitErr != nil {
		t.Fatalf("%v (stderr %q)", waitErr, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestFinishSearchMidRenderEPIPEReturnsZero(t *testing.T) {
	writer := &failAfterWriter{limit: 8}
	output := newOutputWriter(writer)
	code := finishSearch(
		retrieve.RetrievalResult{Status: enums.RetrievalStatusComplete},
		nil,
		SearchOptions{},
		output,
		newProgress(false, output),
	)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !output.Closed() {
		t.Fatal("mid-render closed pipe was not detected")
	}
	if output.Failure() != nil {
		t.Fatalf("failure %v", output.Failure())
	}
	if writer.written.Len() != 8 {
		t.Fatalf("rendered %d bytes before the pipe broke: %q", writer.written.Len(), writer.written.String())
	}
}

func TestFinishSearchInterruptedMidRenderEPIPEReturnsZero(t *testing.T) {
	writer := &failAfterWriter{limit: 8}
	output := newOutputWriter(writer)
	code := finishSearch(
		retrieve.RetrievalResult{Status: enums.RetrievalStatusIncomplete},
		context.Canceled,
		SearchOptions{},
		output,
		newProgress(false, output),
	)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !output.Closed() {
		t.Fatal("mid-render closed pipe was not detected")
	}
	if output.Failure() != nil {
		t.Fatalf("failure %v", output.Failure())
	}
	if writer.written.Len() != 8 {
		t.Fatalf("rendered %d bytes before the pipe broke: %q", writer.written.Len(), writer.written.String())
	}
}
