package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

type Kind int

const (
	Config Kind = iota
	Cache
)

var (
	ErrMissing       = errors.New("Run foco auth or use foco auth --schema NAME --endpoint URL --model ID --stdin.")
	ErrInvalid       = errors.New("Could not read valid credentials. Run foco auth again.")
	ErrInvalidSchema = errors.New("Invalid API schema. Run foco auth again.")
)

func Directory(kind Kind) (string, error) {
	if runtime.GOOS == "windows" {
		name := "Roaming"
		variable := "APPDATA"
		if kind == Cache {
			name = "Local"
			variable = "LOCALAPPDATA"
		}
		if base := os.Getenv(variable); base != "" {
			return filepath.Join(base, "foco"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "AppData", name, "foco"), nil
	}
	variable := "XDG_CONFIG_HOME"
	name := ".config"
	if kind == Cache {
		variable = "XDG_CACHE_HOME"
		name = ".cache"
	}
	if base := os.Getenv(variable); base != "" && filepath.IsAbs(base) {
		return filepath.Join(base, "foco"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, name, "foco"), nil
}

func CredentialsPath() (string, error) {
	directory, err := Directory(Config)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "credentials.json"), nil
}

func Load(path string) (Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credentials{}, ErrMissing
		}
		return Credentials{}, ErrInvalid
	}
	var credentials Credentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return Credentials{}, ErrInvalid
	}
	normalized, err := normalizeCredentials(credentials)
	if err != nil {
		if errors.Is(err, ErrInvalidSchema) {
			return Credentials{}, ErrInvalidSchema
		}
		return Credentials{}, ErrInvalid
	}
	return normalized, nil
}

func Save(path string, credentials Credentials) error {
	normalized, err := normalizeCredentials(credentials)
	if err != nil {
		return err
	}
	credentials = normalized
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return err
	}
	body, err := json.Marshal(credentials)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".credentials-*.json")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(body, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
