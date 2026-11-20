package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSchemaUsesExactEndpoint(t *testing.T) {
	endpoint := "https://generativelanguage.googleapis.com/v1beta/decisions"
	path := filepath.Join(t.TempDir(), "credentials.json")
	body := `{"schema":"gemini-decisions","baseURL":"` + endpoint + `","model":"gemini-decision-1","apiKey":"key"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := Resolve(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Endpoint != endpoint || profile.Model != "gemini-decision-1" {
		t.Fatalf("profile: %+v", profile)
	}
}

func TestLoadMigratesLegacyProviderToSystemOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{"provider":"vercel","apiKey":"legacy-key"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	credentials, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"schema":"system-one"`) || strings.Contains(string(encoded), `"provider"`) ||
		credentials.Model != "typesafe-ai/jev" || credentials.Endpoint != "https://ai-gateway.vercel.sh/typesafe/v1/systemone" ||
		credentials.APIKey != "legacy-key" {
		t.Fatalf("credentials: %+v", credentials)
	}
}

func TestSavePersistsSchemaNotProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	body := `{"schema":"openai-decisions","baseURL":"https://api.openai.com/v1/decisions","model":"gpt-6-luna","apiKey":"openai-key"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, credentials); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), `"schema":"openai-decisions"`) || strings.Contains(string(stored), `"provider"`) {
		t.Fatalf("credentials JSON: %s", stored)
	}
}
