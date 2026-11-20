package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"foco/internal/enums"
)

func TestDirectoryHonorsAbsoluteXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix layout")
	}
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")
	t.Setenv("XDG_CACHE_HOME", "/xdg/cache")
	config, err := Directory(Config)
	if err != nil || config != filepath.Join("/xdg/config", "foco") {
		t.Fatalf("config: %q, %v", config, err)
	}
	cache, err := Directory(Cache)
	if err != nil || cache != filepath.Join("/xdg/cache", "foco") {
		t.Fatalf("cache: %q, %v", cache, err)
	}
}

func TestDirectoryIgnoresRelativeXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix layout")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	t.Setenv("XDG_CACHE_HOME", "../cache")
	config, err := Directory(Config)
	if err != nil || config != filepath.Join(home, ".config", "foco") {
		t.Fatalf("config: %q, %v", config, err)
	}
	cache, err := Directory(Cache)
	if err != nil || cache != filepath.Join(home, ".cache", "foco") {
		t.Fatalf("cache: %q, %v", cache, err)
	}
}

func TestCredentialsPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix layout")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := CredentialsPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "credentials.json" || filepath.Base(filepath.Dir(path)) != "foco" {
		t.Fatalf("path: %s", path)
	}
}

func TestCredentialsSchemaJSONValue(t *testing.T) {
	body, err := json.Marshal(Credentials{Schema: enums.SchemaSystemOne})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"schema":"system-one"`) {
		t.Fatalf("credentials JSON: %s", body)
	}
}

func TestSaveAndLoadMigratedLegacyProvider(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "foco", "credentials.json")
	if err := Save(path, Credentials{Provider: LegacyProviderVercel, APIKey: "test-key"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode: %o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode: %o", dirInfo.Mode().Perm())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"schema":"system-one"`) || !strings.Contains(string(body), `"apiKey":"test-key"`) ||
		!strings.Contains(string(body), `"endpoint":"https://ai-gateway.vercel.sh/typesafe/v1/systemone"`) {
		t.Fatalf("body: %s", body)
	}
	if strings.Contains(string(body), `"provider"`) || strings.Contains(string(body), `"baseURL"`) {
		t.Fatalf("legacy provider fields must not be persisted: %s", body)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != enums.SchemaSystemOne || got.APIKey != "test-key" {
		t.Fatalf("loaded: %+v", got)
	}
}

func TestSaveAndLoadLegacyCustomAsSystemOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	legacyCredentials := Credentials{
		Provider: LegacyProviderCustom, BaseURL: "https://gateway.example.com/v1",
		Model: "gateway/jev-2", APIKey: "custom-key",
	}
	if err := Save(path, legacyCredentials); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != enums.SchemaSystemOne || got.Endpoint != legacyCredentials.BaseURL || got.Model != legacyCredentials.Model || got.APIKey != legacyCredentials.APIKey {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadDefaultsLegacyCredentialsToSystemOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"apiKey":"key"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != enums.SchemaSystemOne || got.Endpoint != "https://ai-gateway.vercel.sh/typesafe/v1/systemone" {
		t.Fatalf("credentials: %+v", got)
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		body string
		want error
	}{
		{"missing", "", ErrMissing},
		{"malformed", "{broken", ErrInvalid},
		{"empty key", `{"provider":"vercel","apiKey":"  "}`, ErrInvalid},
		{"unknown legacy provider", `{"provider":"unknown","apiKey":"key"}`, ErrInvalidSchema},
		{"custom insecure", `{"provider":"custom","baseURL":"http://gateway.example.com/v1","model":"jev","apiKey":"key"}`, ErrInvalid},
	}
	for _, tc := range cases {
		path := filepath.Join(dir, tc.name+".json")
		if tc.body != "" {
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		_, err := Load(path)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}
