package skill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	minimumNodeMajor = 22
	minimumNodeMinor = 20
)

// ErrExecutableNotFound is returned by a Runner when node or npx is absent.
var ErrExecutableNotFound = errors.New("executable not found")

// ErrInstallerFailed marks an unexpected installer failure. The CLI replaces it
// with the upstream generic failure message instead of printing raw OS errors.
var ErrInstallerFailed = errors.New("installer failed")

type Options struct {
	Agents []string
	Global bool
	Yes    bool
}

type Process struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type Runner interface {
	NodeVersion(ctx context.Context) (string, error)
	Run(ctx context.Context, name string, args []string, process Process) (int, error)
}

func Install(ctx context.Context, options Options, runner Runner, process Process) (int, error) {
	version, err := runner.NodeVersion(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return 130, context.Canceled
		}
		return 1, installRequirementError()
	}
	major, minor, err := parseNodeVersion(version)
	if err != nil {
		return 1, installRequirementError()
	}
	if major < minimumNodeMajor || (major == minimumNodeMajor && minor < minimumNodeMinor) {
		return 1, fmt.Errorf("foco skill requires Node 22.20 or newer (skills@1.7.0).")
	}
	if err := ctx.Err(); err != nil {
		return 130, err
	}
	source, err := skillSourceDir()
	if err != nil {
		return 1, err
	}
	name, args := commandFor(runtime.GOOS, source, options, os.Getenv("ComSpec"))
	code, err := runner.Run(ctx, name, args, process)
	if ctx.Err() != nil {
		return 130, nil
	}
	if err != nil {
		if errors.Is(err, ErrExecutableNotFound) {
			return 1, installRequirementError()
		}
		return 1, ErrInstallerFailed
	}
	return code, nil
}

func installRequirementError() error {
	return errors.New("Skill installation requires npx. Install npm, then run foco skill again.")
}

func skillSourceDir() (string, error) {
	configRoot := os.Getenv("XDG_CONFIG_HOME")
	if configRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve Foco config directory: %w", err)
		}
		configRoot = filepath.Join(home, ".config")
	}

	source := filepath.Join(configRoot, skillName)
	skillPath := filepath.Join(source, "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return "", fmt.Errorf("Foco skill file unavailable at %s; run make install: %w", skillPath, err)
	}

	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve Foco skill cache directory: %w", err)
	}
	sum := sha256.Sum256(content)
	stagedSource := filepath.Join(cacheRoot, "foco", "skill", hex.EncodeToString(sum[:8]), skillName)
	if err := os.RemoveAll(stagedSource); err != nil {
		return "", fmt.Errorf("prepare Foco skill source directory: %w", err)
	}
	if err := os.MkdirAll(stagedSource, 0o755); err != nil {
		return "", fmt.Errorf("create Foco skill source directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stagedSource, "SKILL.md"), content, 0o644); err != nil {
		return "", fmt.Errorf("stage Foco skill file: %w", err)
	}
	return stagedSource, nil
}

func parseNodeVersion(raw string) (int, int, error) {
	value := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid node version %q", raw)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return 0, 0, fmt.Errorf("invalid node version %q", raw)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return 0, 0, fmt.Errorf("invalid node version %q", raw)
	}
	return major, minor, nil
}
