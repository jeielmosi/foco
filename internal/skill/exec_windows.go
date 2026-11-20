//go:build windows

package skill

import (
	"os/exec"
	"strings"
	"syscall"
)

func commandLineFor(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, syscall.EscapeArg(name))
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

func configureCommand(command *exec.Cmd, line string) {
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
}
