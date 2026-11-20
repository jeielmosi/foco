package skill

import (
	"testing"
)

func TestInstallerArgsExactOrder(t *testing.T) {
	_, args := commandFor("linux", "/tmp/foco-skill/foco", Options{}, "")
	want := []string{
		"--yes", "--ignore-scripts", "skills@1.7.0", "add", "/tmp/foco-skill/foco",
		"--skill", "foco", "--copy",
	}
	if len(args) != len(want) {
		t.Fatalf("args = %v", args)
	}
	for index := range want {
		if args[index] != want[index] {
			t.Fatalf("args = %v, want %v", args, want)
		}
	}
}

func TestInstallerArgsAppendAgentsAndFlags(t *testing.T) {
	_, args := commandFor("linux", "/tmp/foco-skill/foco", Options{
		Agents: []string{"claude-code", "cursor"},
		Global: true,
		Yes:    true,
	}, "")
	want := []string{
		"--yes", "--ignore-scripts", "skills@1.7.0", "add", "/tmp/foco-skill/foco",
		"--skill", "foco", "--copy",
		"--agent", "claude-code", "--agent", "cursor", "--global", "--yes",
	}
	if len(args) != len(want) {
		t.Fatalf("args = %v", args)
	}
	for index := range want {
		if args[index] != want[index] {
			t.Fatalf("args = %v, want %v", args, want)
		}
	}
}

func TestCommandForUsesNpxOnPosix(t *testing.T) {
	name, _ := commandFor("linux", "/tmp/foco-skill/foco", Options{}, "")
	if name != "npx" {
		t.Fatalf("name = %q", name)
	}
}

func TestCommandForUsesComSpecOnWindows(t *testing.T) {
	name, _ := commandFor("windows", `C:\tmp\foco-skill\foco`, Options{}, "")
	if name != "cmd.exe" {
		t.Fatalf("name = %q", name)
	}
	name, _ = commandFor("windows", `C:\tmp\foco-skill\foco`, Options{}, `C:\Windows\System32\cmd.exe`)
	if name != `C:\Windows\System32\cmd.exe` {
		t.Fatalf("name = %q", name)
	}
}

func TestWindowsInstallerCommand(t *testing.T) {
	_, args := commandFor("windows", `C:\tmp\foco-skill\foco`, Options{
		Agents: []string{"claude-code"},
		Global: true,
		Yes:    true,
	}, "cmd.exe")
	want := []string{
		"/d", "/s", "/c",
		`"npx ^"--yes^" ^"--ignore-scripts^" ^"skills@1.7.0^" ^"add^" ^"C:\tmp\foco-skill\foco^" ^"--skill^" ^"foco^" ^"--copy^" ^"--agent^" ^"claude-code^" ^"--global^" ^"--yes^""`,
	}
	if len(args) != len(want) {
		t.Fatalf("args = %v", args)
	}
	for index := range want {
		if args[index] != want[index] {
			t.Fatalf("args = %v, want %v", args, want)
		}
	}
}
