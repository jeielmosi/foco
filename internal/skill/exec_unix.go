//go:build !windows

package skill

import (
	"os"
	"os/exec"
	"strings"
)

func commandLineFor(name string, args []string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

func configureCommand(command *exec.Cmd, _ string) {
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return command.Process.Signal(os.Interrupt)
	}
}
