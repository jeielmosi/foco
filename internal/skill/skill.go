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

	skillasset "foco/skill"
)

var focoSkill = skillasset.Content

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
	source, err := stageSkill()
	if err != nil {
		return 1, ErrInstallerFailed
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

// skillCacheDir is overridden in tests.
var skillCacheDir = os.UserCacheDir

// stageSkill writes the skill shipped with this binary to a content-addressed
// cache directory for the skills installer. The installed skill then matches the
// running foco version instead of an unrelated upstream repository, and the
// project's skills-lock.json keeps a source path that still exists. The
// directory is recreated so a stale layout from an older version is never sent
// to the installer.
func stageSkill() (string, error) {
	base, err := skillCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(focoSkill))
	dir := filepath.Join(base, "foco", "skill", hex.EncodeToString(sum[:8]), skillName)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(focoSkill), 0o644); err != nil {
		return "", err
	}
	return dir, nil
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
