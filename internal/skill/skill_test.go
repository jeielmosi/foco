package skill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	version    string
	versionErr error
	code       int
	runErr     error
	runs       int
	name       string
	args       []string
	process    Process
	onRun      func([]string)
}

func (f *fakeRunner) NodeVersion(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return f.version, f.versionErr
}

func (f *fakeRunner) Run(_ context.Context, name string, args []string, process Process) (int, error) {
	f.runs++
	f.name = name
	f.args = append([]string(nil), args...)
	f.process = process
	if f.onRun != nil {
		f.onRun(f.args)
	}
	return f.code, f.runErr
}

func TestInstallRejectsOldNodeVersions(t *testing.T) {
	for _, version := range []string{"20.20.0", "22.0.0", "22.19.9"} {
		runner := &fakeRunner{version: version}
		code, err := Install(context.Background(), Options{}, runner, Process{})
		if code != 1 {
			t.Fatalf("%s: exit %d", version, code)
		}
		if runner.runs != 0 {
			t.Fatalf("%s: installer was launched", version)
		}
		if err == nil || err.Error() != "foco skill requires Node 22.20 or newer (skills@1.7.0)." {
			t.Fatalf("%s: %v", version, err)
		}
	}
}

func TestInstallAcceptsSupportedNodeVersions(t *testing.T) {
	for _, version := range []string{"22.20.0", "22.23.3", "23.0.0", "24.0.0"} {
		runner := &fakeRunner{version: version}
		code, err := Install(context.Background(), Options{}, runner, Process{})
		if code != 0 || err != nil || runner.runs != 1 {
			t.Fatalf("%s: code=%d err=%v runs=%d", version, code, err, runner.runs)
		}
	}
}

func TestInstallRequiresNpxWhenNodeIsMissing(t *testing.T) {
	runner := &fakeRunner{versionErr: ErrExecutableNotFound}
	code, err := Install(context.Background(), Options{}, runner, Process{})
	if code != 1 || runner.runs != 0 {
		t.Fatalf("code=%d runs=%d", code, runner.runs)
	}
	if err == nil || err.Error() != "Skill installation requires npx. Install npm, then run foco skill again." {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallRequiresNpxWhenRunReturnsNotFound(t *testing.T) {
	runner := &fakeRunner{version: "22.20.0", runErr: ErrExecutableNotFound}
	code, err := Install(context.Background(), Options{}, runner, Process{})
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if err == nil || err.Error() != "Skill installation requires npx. Install npm, then run foco skill again." {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallMapsUnexpectedRunError(t *testing.T) {
	runner := &fakeRunner{version: "22.20.0", runErr: errors.New("boom")}
	code, err := Install(context.Background(), Options{}, runner, Process{})
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !errors.Is(err, ErrInstallerFailed) {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallPropagatesChildExitCode(t *testing.T) {
	for _, want := range []int{0, 1, 2, 130} {
		runner := &fakeRunner{version: "22.20.0", code: want}
		code, err := Install(context.Background(), Options{}, runner, Process{})
		if err != nil || code != want {
			t.Fatalf("want %d: code=%d err=%v", want, code, err)
		}
	}
}

func TestInstallAbortsBeforeSpawnWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &fakeRunner{version: "22.20.0"}
	code, err := Install(ctx, Options{}, runner, Process{})
	if code != 130 || runner.runs != 0 {
		t.Fatalf("code=%d runs=%d", code, runner.runs)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallStagesShippedFocoSkill(t *testing.T) {
	cache := t.TempDir()
	previous := skillCacheDir
	skillCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { skillCacheDir = previous })

	options := Options{
		Agents: []string{"claude-code", "cursor"},
		Global: true,
		Yes:    true,
	}
	var source, content string
	runner := &fakeRunner{version: "22.20.0", onRun: func(args []string) {
		source = args[4]
		data, err := os.ReadFile(filepath.Join(source, "SKILL.md"))
		if err != nil {
			t.Fatalf("read staged skill: %v", err)
		}
		content = string(data)
	}}
	code, err := Install(context.Background(), options, runner, Process{})
	if code != 0 || err != nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if content != focoSkill {
		t.Fatal("installer did not receive the embedded foco skill")
	}
	if runner.name != "npx" {
		t.Fatalf("name = %q", runner.name)
	}
	want := []string{
		"--yes", "--ignore-scripts", "skills@1.7.0", "add", source,
		"--skill", "foco", "--copy",
		"--agent", "claude-code", "--agent", "cursor", "--global", "--yes",
	}
	if len(runner.args) != len(want) {
		t.Fatalf("args = %v, want %v", runner.args, want)
	}
	for index := range want {
		if runner.args[index] != want[index] {
			t.Fatalf("args = %v, want %v", runner.args, want)
		}
	}
	if _, err := os.Stat(filepath.Join(source, "SKILL.md")); err != nil {
		t.Fatalf("staged skill source must remain valid for skills-lock.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(source, "v0")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged skill source must contain only SKILL.md: %v", err)
	}
}

func TestInstallClearsStaleVersionedLayout(t *testing.T) {
	cache := t.TempDir()
	previous := skillCacheDir
	skillCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { skillCacheDir = previous })

	var source string
	runner := &fakeRunner{version: "22.20.0", onRun: func(args []string) { source = args[4] }}
	if code, err := Install(context.Background(), Options{}, runner, Process{}); code != 0 || err != nil {
		t.Fatalf("first install: code=%d err=%v", code, err)
	}
	stale := filepath.Join(source, "v0")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "SKILL.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner = &fakeRunner{version: "22.20.0", onRun: func(args []string) { source = args[4] }}
	if code, err := Install(context.Background(), Options{}, runner, Process{}); code != 0 || err != nil {
		t.Fatalf("second install: code=%d err=%v", code, err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale versioned layout was sent to the installer: %v", err)
	}
	if _, err := os.Stat(filepath.Join(source, "SKILL.md")); err != nil {
		t.Fatalf("staged skill missing: %v", err)
	}
}

func TestShippedFocoSkillDescribesFoco(t *testing.T) {
	for _, required := range []string{
		"name: foco",
		"`foco`",
		"foco auth",
		"system-one",
		"gemini-decisions",
		"openai-decisions",
		"End context.",
	} {
		if !strings.Contains(focoSkill, required) {
			t.Fatalf("shipped skill is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"`jg`",
		"jg files",
		"jg doctor",
		"jg auth",
		"--provider",
		"jevgrep",
		"npm install --global",
	} {
		if strings.Contains(focoSkill, forbidden) {
			t.Fatalf("shipped skill still references %q", forbidden)
		}
	}
}

func TestInstallPassesProcessStreamsThrough(t *testing.T) {
	var stdout, stderr strings.Builder
	process := Process{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	runner := &fakeRunner{version: "22.20.0"}
	if _, err := Install(context.Background(), Options{}, runner, process); err != nil {
		t.Fatal(err)
	}
	if runner.process.Stdout != &stdout || runner.process.Stderr != &stderr {
		t.Fatal("process streams were not forwarded")
	}
}

func TestParseNodeVersion(t *testing.T) {
	cases := []struct {
		raw          string
		major, minor int
		wantErr      bool
	}{
		{"v22.20.0", 22, 20, false},
		{"22.19.9", 22, 19, false},
		{"v24.1.0", 24, 1, false},
		{"v22", 0, 0, true},
		{"garbage", 0, 0, true},
	}
	for _, tc := range cases {
		major, minor, err := parseNodeVersion(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%q: expected error", tc.raw)
			}
			continue
		}
		if err != nil || major != tc.major || minor != tc.minor {
			t.Fatalf("%q: %d.%d, %v", tc.raw, major, minor, err)
		}
	}
}
