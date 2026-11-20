package skill

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

type ExecRunner struct{}

func NewExecRunner() Runner {
	return ExecRunner{}
}

func (ExecRunner) NodeVersion(ctx context.Context) (string, error) {
	command := exec.CommandContext(ctx, "node", "--version")
	configureCommand(command, "node --version")
	command.WaitDelay = 2 * time.Second
	output, err := command.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return "", ErrExecutableNotFound
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (ExecRunner) Run(ctx context.Context, name string, args []string, process Process) (int, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = process.Stdin
	command.Stdout = process.Stdout
	command.Stderr = process.Stderr
	configureCommand(command, commandLineFor(name, args))
	command.WaitDelay = 2 * time.Second
	err := command.Run()
	if ctx.Err() != nil {
		return 130, nil
	}
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if code := exit.ExitCode(); code >= 0 {
			return code, nil
		}
		return 1, nil
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return 1, ErrExecutableNotFound
	}
	return 1, err
}
