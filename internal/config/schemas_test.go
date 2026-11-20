package config

import (
	"strings"
	"testing"

	"foco/internal/enums"
)

func TestSchemaIDsAreSupportedDecisionFormats(t *testing.T) {
	want := []enums.Schema{enums.SchemaSystemOne, enums.SchemaGemini, enums.SchemaOpenAI}
	got := SchemaIDs()
	if len(got) != len(want) {
		t.Fatalf("schemas: %v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("schemas: %v, want %v", got, want)
		}
	}
}

func TestLegacyProvidersAreValidPersistedValues(t *testing.T) {
	cases := []struct {
		provider  LegacyProvider
		persisted string
	}{
		{LegacyProviderVercel, "vercel"},
		{LegacyProviderTypeSafe, "typesafe"},
		{LegacyProviderOpenRouter, "openrouter"},
		{LegacyProviderOpenCode, "opencode"},
		{LegacyProviderCustom, "custom"},
	}
	for _, tc := range cases {
		t.Run(tc.persisted, func(t *testing.T) {
			if !tc.provider.IsValid() {
				t.Fatal("provider must be valid")
			}
			if got := tc.provider.String(); got != tc.persisted {
				t.Fatalf("String() = %q, want %q", got, tc.persisted)
			}
			if got, ok := NewLegacyProvider(tc.persisted); !ok || got != tc.provider {
				t.Fatalf("NewLegacyProvider() = %q, %v; want %q, true", got, ok, tc.provider)
			}
		})
	}

	for _, provider := range []LegacyProvider{UnknownLegacyProvider, LegacyProvider("arbitrary")} {
		if provider.IsValid() {
			t.Errorf("%q must be invalid", provider)
		}
		if got := provider.String(); got != "" {
			t.Errorf("%q.String() = %q, want empty", provider, got)
		}
		if got, ok := NewLegacyProvider(string(provider)); ok || got != UnknownLegacyProvider {
			t.Errorf("NewLegacyProvider(%q) = %q, %v; want empty, false", provider, got, ok)
		}
	}
}

func TestResolveSystemOneUsesExactEndpoint(t *testing.T) {
	endpoint := "https://gateway.example.com/v1/systemone"
	got, err := Resolve(Credentials{
		Schema: enums.SchemaSystemOne, Endpoint: endpoint, Model: "jev-latest", APIKey: "key",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Profile{
		Schema: enums.SchemaSystemOne, Label: "System One (Jev/Clef-compatible) endpoint (gateway.example.com)",
		Endpoint: endpoint, Model: "jev-latest",
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestResolveRejectsUnknownSchema(t *testing.T) {
	if _, err := Resolve(Credentials{Schema: enums.Schema("unknown"), Endpoint: "https://example.com", Model: "m", APIKey: "key"}); err == nil {
		t.Fatal("unknown schema must fail")
	}
}

func TestValidateEndpoint(t *testing.T) {
	cases := map[string]string{
		" https://gateway.example.com/v1 ": "https://gateway.example.com/v1",
		"http://localhost:8080/v1":         "http://localhost:8080/v1",
		"http://127.0.0.1:8080/v1":         "http://127.0.0.1:8080/v1",
	}
	for value, want := range cases {
		got, err := ValidateEndpoint(value)
		if err != nil {
			t.Errorf("%q: %v", value, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", value, got, want)
		}
	}
	for _, value := range []string{
		"http://gateway.example.com/v1",
		"http://192.168.1.10:8080/v1",
		"ftp://gateway.example.com/v1",
		"https://gateway.example.com/v1?api-version=1",
		"https://gateway.example.com/v1#fragment",
		"https://gateway.example.com/v1?",
		"https://gateway.example.com/v1#",
		"https://user:password@gateway.example.com/v1",
		"gateway.example.com/v1",
		"",
		"   ",
	} {
		if _, err := ValidateEndpoint(value); err == nil {
			t.Errorf("%q should fail", value)
		}
	}
}

func TestValidateEndpointErrors(t *testing.T) {
	cases := map[string]string{
		"":                                   "Provide an endpoint URL",
		"ftp://gateway.example.com/v1":       "absolute http(s) URL",
		"http://gateway.example.com/v1":      "https://",
		"https://gateway.example.com/v1?x=1": "query string",
	}
	for value, want := range cases {
		_, err := ValidateEndpoint(value)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %q", value, err, want)
		}
	}
}

func TestValidateModel(t *testing.T) {
	if got, err := ValidateModel(" gateway/jev-2 "); err != nil || got != "gateway/jev-2" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, value := range []string{"", "  ", "two words"} {
		if _, err := ValidateModel(value); err == nil {
			t.Errorf("%q should fail", value)
		}
	}
}

func TestValidateKey(t *testing.T) {
	if got, err := ValidateKey(" \tfake-latin-é-secret\r\n"); err != nil || got != "fake-latin-é-secret" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, value := range []string{
		"",
		"two secrets",
		"invalid-fake\x00secret",
		"invalid-fake\x01secret",
		"invalid-fake\x7fsecret",
		"invalid-fake☃secret",
		strings.Repeat("x", 8193),
	} {
		if _, err := ValidateKey(value); err == nil {
			t.Errorf("%q should fail", value)
		}
	}
}
