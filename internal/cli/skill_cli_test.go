package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"foco/internal/skill"
)

type fakeCLISkillRunner struct {
	version string
	code    int
	runErr  error
	runs    int
	name    string
	args    []string
	process skill.Process
}

func (f *fakeCLISkillRunner) NodeVersion(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return f.version, nil
}

func (f *fakeCLISkillRunner) Run(_ context.Context, name string, args []string, process skill.Process) (int, error) {
	f.runs++
	f.name = name
	f.args = append([]string(nil), args...)
	f.process = process
	return f.code, f.runErr
}

func withSkillRunner(t *testing.T, runner skill.Runner) {
	t.Helper()
	configRoot := t.TempDir()
	skillDir := filepath.Join(configRoot, "foco")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Foco skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("LOCALAPPDATA", cacheRoot)
	t.Setenv("HOME", cacheRoot)

	previous := skillRunnerFactory
	skillRunnerFactory = func() skill.Runner { return runner }
	t.Cleanup(func() { skillRunnerFactory = previous })
}

func TestRunSkillPropagatesExitCodeAndForwardsArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cmd.exe argument construction is covered by internal/skill tests")
	}
	runner := &fakeCLISkillRunner{version: "22.20.0", code: 2}
	withSkillRunner(t, runner)
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"skill", "--agent", "claude-code", "--global", "--yes"},
		strings.NewReader(""), &output, &errorOutput)
	if code != 2 {
		t.Fatalf("exit %d", code)
	}
	if output.Len() != 0 || errorOutput.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", output.String(), errorOutput.String())
	}
	if len(runner.args) < 5 {
		t.Fatalf("name=%q args=%v", runner.name, runner.args)
	}
	want := []string{
		"--yes", "--ignore-scripts", "skills@1.7.0", "add", runner.args[4],
		"--skill", "foco", "--copy", "--agent", "claude-code", "--global", "--yes",
	}
	if runner.name != "npx" || len(runner.args) != len(want) {
		t.Fatalf("name=%q args=%v", runner.name, runner.args)
	}
	for index := range want {
		if runner.args[index] != want[index] {
			t.Fatalf("args=%v want=%v", runner.args, want)
		}
	}
	if runner.process.Stdout == nil || runner.process.Stdout != runner.process.Stderr {
		t.Fatal("installer stdout/stderr must both go to CLI stdout")
	}
	fmt.Fprint(runner.process.Stdout, "installer-output\n")
	if !strings.Contains(output.String(), "installer-output") {
		t.Fatalf("installer output did not reach CLI stdout: %q", output.String())
	}
}

func TestRunSkillKeepsWrapperForNonTerminalFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "skill-stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	runner := &fakeCLISkillRunner{version: "22.20.0"}
	withSkillRunner(t, runner)
	var errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"skill"}, strings.NewReader(""), file, &errorOutput)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if runner.process.Stdout == file {
		t.Fatal("non-terminal stdout must stay wrapped so os/exec observes EPIPE")
	}
	if _, ok := runner.process.Stdout.(*outputWriter); !ok {
		t.Fatalf("installer stdout is %T, want *outputWriter", runner.process.Stdout)
	}
	if runner.process.Stdout != runner.process.Stderr {
		t.Fatalf("installer stdout is %T, stderr is %T, want the same wrapper",
			runner.process.Stdout, runner.process.Stderr)
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr %q", errorOutput.String())
	}
}

func TestRunSkillPassesRawFileForTerminalStdout(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "skill-stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	previous := skillTTY
	skillTTY = func(any) bool { return true }
	t.Cleanup(func() { skillTTY = previous })
	runner := &fakeCLISkillRunner{version: "22.20.0"}
	withSkillRunner(t, runner)
	var errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"skill"}, strings.NewReader(""), file, &errorOutput)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if runner.process.Stdout != file {
		t.Fatalf("installer stdout is %T, want the terminal *os.File", runner.process.Stdout)
	}
	if runner.process.Stderr != file {
		t.Fatalf("installer stderr is %T, want the terminal *os.File", runner.process.Stderr)
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr %q", errorOutput.String())
	}
}

func TestRunSkillNodeTooOld(t *testing.T) {
	runner := &fakeCLISkillRunner{version: "22.19.9"}
	withSkillRunner(t, runner)
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"skill"}, strings.NewReader(""), &output, &errorOutput)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if output.String() != "foco skill requires Node 22.20 or newer (skills@1.7.0).\n" {
		t.Fatalf("stdout %q", output.String())
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr %q", errorOutput.String())
	}
	if runner.runs != 0 {
		t.Fatal("installer launched for unsupported Node")
	}
}

func TestRunSkillMissingNpx(t *testing.T) {
	runner := &fakeCLISkillRunner{version: "22.20.0", runErr: skill.ErrExecutableNotFound}
	withSkillRunner(t, runner)
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"skill"}, strings.NewReader(""), &output, &errorOutput)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if output.String() != "Skill installation requires npx. Install npm, then run foco skill again.\n" {
		t.Fatalf("stdout %q", output.String())
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr %q", errorOutput.String())
	}
}

func TestRunSkillInterruptedMidInstallExits130WithoutMessage(t *testing.T) {
	runner := &fakeCLISkillRunner{version: "22.20.0", code: 130}
	withSkillRunner(t, runner)
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"skill"}, strings.NewReader(""), &output, &errorOutput)
	if code != 130 {
		t.Fatalf("exit %d", code)
	}
	if output.Len() != 0 || errorOutput.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", output.String(), errorOutput.String())
	}
}

func TestRunSkillInterruptedBeforeSpawnPrintsMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &fakeCLISkillRunner{version: "22.20.0"}
	withSkillRunner(t, runner)
	var output, errorOutput bytes.Buffer
	code := RunContext(ctx, []string{"skill"}, strings.NewReader(""), &output, &errorOutput)
	if code != 130 {
		t.Fatalf("exit %d", code)
	}
	if output.String() != "Interrupted.\n" {
		t.Fatalf("stdout %q", output.String())
	}
	if runner.runs != 0 {
		t.Fatal("installer launched after cancellation")
	}
}

func TestRunSkillUnrelatedRunnerErrorExitsOne(t *testing.T) {
	runner := &fakeCLISkillRunner{version: "22.20.0", runErr: errors.New("installer exploded")}
	withSkillRunner(t, runner)
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"skill"}, strings.NewReader(""), &output, &errorOutput)
	want := "Command failed. Check the root, permissions, credentials, and network.\n"
	if code != 1 || output.String() != want || errorOutput.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, output.String(), errorOutput.String())
	}
	if strings.Contains(output.String(), "installer exploded") {
		t.Fatal("raw installer error leaked")
	}
}
