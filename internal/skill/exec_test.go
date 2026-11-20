//go:build !windows

package skill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const execRunnerWaitDelay = 2 * time.Second

func TestExecRunnerRunPropagatesExitCode(t *testing.T) {
	runner := NewExecRunner()
	code, err := runner.Run(context.Background(), "sh", []string{"-c", "exit 7"}, Process{})
	if code != 7 || err != nil {
		t.Fatalf("code=%d err=%v, want 7, nil", code, err)
	}
}

func TestExecRunnerMissingExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	runner := NewExecRunner()
	code, err := runner.Run(context.Background(), "foco-missing-executable", nil, Process{})
	if code != 1 || !errors.Is(err, ErrExecutableNotFound) {
		t.Fatalf("Run: code=%d err=%v, want 1, ErrExecutableNotFound", code, err)
	}
	if _, err := runner.NodeVersion(context.Background()); !errors.Is(err, ErrExecutableNotFound) {
		t.Fatalf("NodeVersion: err=%v, want ErrExecutableNotFound", err)
	}
}

func TestExecRunnerRunCancellationStopsProcess(t *testing.T) {
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "ready")
	pidPath := filepath.Join(dir, "pid")
	script := "echo $$ > " + shellQuote(pidPath) +
		"; echo ready > " + shellQuote(readyPath) +
		"; exec sleep 5"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(func() { killPIDFromFile(pidPath) })

	type result struct {
		code int
		err  error
	}
	results := make(chan result, 1)
	go func() {
		code, err := NewExecRunner().Run(ctx, "sh", []string{"-c", script}, Process{})
		results <- result{code: code, err: err}
	}()

	waitForPath(t, readyPath, 5*time.Second)
	cancel()

	var got result
	select {
	case got = <-results:
	case <-time.After(execRunnerWaitDelay + 2*time.Second):
		t.Fatalf("Run did not return within WaitDelay+2s of cancellation")
	}
	if got.code != 130 || got.err != nil {
		t.Fatalf("code=%d err=%v, want 130, nil", got.code, got.err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for pidAlive(pidPath) {
		if time.Now().After(deadline) {
			t.Fatalf("spawned process %d survived cancellation", mustReadPID(t, pidPath))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForPath(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat %s: %v", path, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s was not created within %s", path, timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func readPID(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func mustReadPID(t *testing.T, path string) int {
	t.Helper()
	pid, ok := readPID(path)
	if !ok {
		t.Fatalf("could not read pid from %s", path)
	}
	return pid
}

func pidAlive(path string) bool {
	pid, ok := readPID(path)
	if !ok {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func killPIDFromFile(path string) {
	pid, ok := readPID(path)
	if !ok {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
