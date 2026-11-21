package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

type sample struct {
	WallMS     float64       `json:"wallMs"`
	UserMS     float64       `json:"userMs"`
	MaxRSSKB   int64         `json:"maxRssKb"`
	ExitCode   int           `json:"exitCode"`
	OutputHash string        `json:"outputHash"`
	FirstLine  string        `json:"firstLine"`
	Provider   providerStats `json:"provider"`
}

// searchEnv writes an isolated config pointing foco at the fake provider.
func searchEnv(scratch string, provider *fakeProvider) ([]string, error) {
	configHome := filepath.Join(scratch, "config")
	dir := filepath.Join(configHome, "foco")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	credentials, _ := json.Marshal(map[string]string{
		"schema": "system-one", "endpoint": provider.endpoint(), "model": "foco-bench", "apiKey": "foco-bench",
	})
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), append(credentials, '\n'), 0o600); err != nil {
		return nil, err
	}
	env := append(os.Environ(), "XDG_CONFIG_HOME="+configHome, "CI=1")
	if provider.caFile != "" {
		env = append(env, "SSL_CERT_FILE="+provider.caFile)
	}
	return env, nil
}

func runSearch(binary string, env []string, args []string, provider *fakeProvider, forbid []string) (sample, error) {
	provider.reset(forbid)
	var stdout bytes.Buffer
	command := exec.Command(binary, args...)
	command.Env = env
	command.Stdout = &stdout
	started := time.Now()
	err := command.Run()
	elapsed := time.Since(started)
	out := sample{WallMS: float64(elapsed.Microseconds()) / 1000, Provider: provider.snapshot()}
	if command.ProcessState == nil {
		return out, err
	}
	out.ExitCode = command.ProcessState.ExitCode()
	out.UserMS, out.MaxRSSKB = usage(command.ProcessState)
	sum := sha256.Sum256(stdout.Bytes())
	out.OutputHash = hex.EncodeToString(sum[:8])
	if line, _, found := bytes.Cut(stdout.Bytes(), []byte("\n")); found || len(line) > 0 {
		out.FirstLine = string(line)
	}
	return out, nil
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}
