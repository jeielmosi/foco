package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// build is one foco binary under test.
type build struct {
	Label  string `json:"label"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
	Dir    string `json:"-"`
	Binary string `json:"-"`
}

func git(dir string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Stderr = os.Stderr
	out, err := command.Output()
	return strings.TrimSpace(string(out)), err
}

// prepareBuild checks ref out into a cached worktree and builds ./cmd/cli.
// The ref "." builds the current working tree, uncommitted changes included.
func prepareBuild(label, ref, repo, cache, tags string, logf func(string, ...any)) (build, error) {
	out := build{Label: label, Ref: ref}
	key := sha256.Sum256([]byte(tags))
	if ref == "." {
		commit, err := git(repo, "rev-parse", "HEAD")
		if err != nil {
			return out, err
		}
		out.Commit, out.Dir = commit, repo
		out.Binary = filepath.Join(cache, "bin", "worktree-"+hex.EncodeToString(key[:4]))
		return out, compile(out.Dir, out.Binary, tags, logf)
	}
	commit, err := git(repo, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return out, fmt.Errorf("resolve %s: %w", ref, err)
	}
	out.Commit = commit
	out.Dir = filepath.Join(cache, "worktrees", commit[:12])
	if _, err := os.Stat(out.Dir); err != nil {
		logf("checking out %s (%s)", ref, commit[:12])
		if _, err := git(repo, "worktree", "add", "-q", "--detach", out.Dir, commit); err != nil {
			return out, err
		}
	}
	out.Binary = filepath.Join(cache, "bin", commit[:12]+"-"+hex.EncodeToString(key[:4]))
	if _, err := os.Stat(out.Binary); err == nil {
		return out, nil
	}
	return out, compile(out.Dir, out.Binary, tags, logf)
}

func compile(dir, binary, tags string, logf func(string, ...any)) error {
	logf("building %s", dir)
	args := []string{"build", "-trimpath", "-o", binary}
	env := append(os.Environ(), "CGO_ENABLED=0")
	if tags != "" {
		args = append(args, "-tags", tags)
		env = append(os.Environ(), "CGO_ENABLED=1")
	}
	command := exec.Command("go", append(args, "./cmd/cli")...)
	command.Dir = dir
	command.Env = env
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	return command.Run()
}
