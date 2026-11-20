package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func buildFoco(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "foco")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	command := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/cli")
	command.Dir = "../.."
	command.Env = subprocessEnv("GOTOOLCHAIN=auto", "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build foco: %v\n%s", err, output)
	}
	return binary
}

func subprocessEnv(extra ...string) []string {
	overridden := make(map[string]bool, len(extra))
	for _, entry := range extra {
		name, _, _ := strings.Cut(entry, "=")
		overridden[name] = true
	}
	env := make([]string, 0, len(os.Environ())+len(extra))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if overridden[name] {
			continue
		}
		env = append(env, entry)
	}
	return append(env, extra...)
}

func killAndWait(t *testing.T, command *exec.Cmd, waited <-chan error) error {
	t.Helper()
	if command.Process != nil {
		_ = command.Process.Kill()
	}
	select {
	case err := <-waited:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("child did not exit after SIGKILL")
		return nil
	}
}

func TestSubprocessClosedStdoutExitsZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX EPIPE semantics")
	}
	binary := buildFoco(t)
	var help bytes.Buffer
	probe := exec.Command(binary, "--help")
	probe.Stdout = &help
	if err := probe.Start(); err != nil {
		t.Fatalf("--help probe: %v", err)
	}
	probed := make(chan error, 1)
	go func() { probed <- probe.Wait() }()
	select {
	case err := <-probed:
		if err != nil {
			t.Fatalf("--help probe: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = killAndWait(t, probe, probed)
		t.Fatal("--help probe did not exit")
	}
	if help.Len() == 0 {
		t.Fatal("--help wrote nothing; closed-stdout case would not attempt a write")
	}
	for _, args := range [][]string{{"--help"}, {"--version"}} {
		read, write, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := read.Close(); err != nil {
			t.Fatal(err)
		}
		command := exec.Command(binary, args...)
		command.Stdout = write
		var stderr bytes.Buffer
		command.Stderr = &stderr
		if err := command.Start(); err != nil {
			_ = write.Close()
			t.Fatalf("%v: start: %v", args, err)
		}
		waited := make(chan error, 1)
		go func() { waited <- command.Wait() }()
		var waitErr error
		select {
		case waitErr = <-waited:
		case <-time.After(10 * time.Second):
			_ = killAndWait(t, command, waited)
			_ = write.Close()
			t.Fatalf("%v: child did not exit after closed stdout", args)
		}
		_ = write.Close()
		if waitErr != nil {
			t.Fatalf("%v: %v (stderr %q)", args, waitErr, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("%v: stderr %q", args, stderr.String())
		}
	}
}

func TestSubprocessSIGINTStdinAuthExits130(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signal delivery")
	}
	binary := buildFoco(t)
	home := t.TempDir()
	command := exec.Command(binary, "auth", "--schema", "system-one", "--endpoint", "https://api.example.com/systemone", "--model", "jev-latest", "--stdin")
	command.Env = subprocessEnv("XDG_CONFIG_HOME="+home, "FOCO_TEST_SIGNAL_READY=1")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	var stdout bytes.Buffer
	command.Stdout = &stdout
	stderr, err := command.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	ready := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(stderr).ReadString('\n')
		if err != nil {
			ready <- err
			return
		}
		if line != "ready\n" {
			ready <- fmt.Errorf("readiness line %q", line)
			return
		}
		ready <- nil
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = killAndWait(t, command, waited)
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		_ = killAndWait(t, command, waited)
		t.Fatal("signal handler not ready")
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		_ = killAndWait(t, command, waited)
		t.Fatal(err)
	}
	var waitErr error
	select {
	case waitErr = <-waited:
	case <-time.After(10 * time.Second):
		_ = killAndWait(t, command, waited)
		t.Fatal("child did not exit after SIGINT")
	}
	code := 0
	if waitErr != nil {
		var exit *exec.ExitError
		if !errors.As(waitErr, &exit) {
			t.Fatalf("wait: %v", waitErr)
		}
		code = exit.ExitCode()
	}
	if code != 130 {
		t.Fatalf("exit %d; stdout=%q", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Interrupted.") {
		t.Fatalf("stdout %q", stdout.String())
	}
}
