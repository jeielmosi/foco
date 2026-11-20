package config

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"foco/internal/enums"
)

type Profile struct {
	Schema   enums.Schema
	Label    string
	Endpoint string
	Model    string
}

type Credentials struct {
	Schema   enums.Schema   `json:"schema,omitempty"`
	Endpoint string         `json:"endpoint,omitempty"`
	Model    string         `json:"model,omitempty"`
	APIKey   string         `json:"apiKey"`
	Provider LegacyProvider `json:"provider,omitempty"` // Legacy field used only to migrate saved credentials.
	BaseURL  string         `json:"baseURL,omitempty"`  // Legacy field used only to migrate saved credentials.
}

var legacySystemOneProfiles = map[LegacyProvider]Profile{
	LegacyProviderVercel: {
		Schema: enums.SchemaSystemOne, Label: "System One",
		Endpoint: "https://ai-gateway.vercel.sh/typesafe/v1/systemone", Model: "typesafe-ai/jev",
	},
	LegacyProviderTypeSafe: {
		Schema: enums.SchemaSystemOne, Label: "System One",
		Endpoint: "https://api.typesafe.ai/v1/systemone", Model: "jev-1.13.0",
	},
	LegacyProviderOpenRouter: {
		Schema: enums.SchemaSystemOne, Label: "System One",
		Endpoint: "https://openrouter.ai/api/v1/systemone", Model: "typesafe/jev-1.13",
	},
	LegacyProviderOpenCode: {
		Schema: enums.SchemaSystemOne, Label: "System One",
		Endpoint: "https://opencode.ai/zen/v1/systemone", Model: "jev-1.13",
	},
}

func SchemaIDs() []enums.Schema {
	return []enums.Schema{enums.SchemaSystemOne, enums.SchemaGemini, enums.SchemaOpenAI}
}

func SchemaLabel(schema enums.Schema) string {
	switch schema {
	case enums.SchemaSystemOne:
		return "System One (Jev/Clef-compatible)"
	case enums.SchemaGemini:
		return "Gemini Decisions"
	case enums.SchemaOpenAI:
		return "OpenAI Decisions"
	default:
		return schema.String()
	}
}

func normalizeCredentials(credentials Credentials) (Credentials, error) {
	if credentials.Schema == enums.UnknownSchema {
		provider := credentials.Provider
		if provider == UnknownLegacyProvider {
			provider = LegacyProviderVercel
		}
		if provider == LegacyProviderCustom {
			credentials.Schema = enums.SchemaSystemOne
			credentials.Endpoint = credentials.BaseURL
		} else if profile, ok := legacySystemOneProfiles[provider]; ok {
			credentials.Schema = profile.Schema
			credentials.Endpoint = profile.Endpoint
			credentials.Model = profile.Model
		} else {
			return Credentials{}, ErrInvalidSchema
		}
	}
	if !credentials.Schema.IsValid() {
		return Credentials{}, ErrInvalidSchema
	}
	if credentials.Endpoint == "" {
		credentials.Endpoint = credentials.BaseURL
	}
	endpoint, err := ValidateEndpoint(credentials.Endpoint)
	if err != nil {
		return Credentials{}, err
	}
	model, err := ValidateModel(credentials.Model)
	if err != nil {
		return Credentials{}, err
	}
	key, err := ValidateKey(credentials.APIKey)
	if err != nil {
		return Credentials{}, err
	}
	credentials.Endpoint = endpoint
	credentials.Model = model
	credentials.APIKey = key
	credentials.Provider = UnknownLegacyProvider
	credentials.BaseURL = ""
	return credentials, nil
}

func Resolve(credentials Credentials) (Profile, error) {
	normalized, err := normalizeCredentials(credentials)
	if err != nil {
		return Profile{}, err
	}
	parsed, err := url.Parse(normalized.Endpoint)
	if err != nil {
		return Profile{}, errors.New("Endpoint must be an absolute http(s) URL.")
	}
	label := SchemaLabel(normalized.Schema) + " endpoint (" + parsed.Host + ")"
	return Profile{
		Schema: normalized.Schema, Label: label, Endpoint: normalized.Endpoint, Model: normalized.Model,
	}, nil
}

func ValidateEndpoint(value string) (string, error) {
	endpoint := strings.TrimSpace(value)
	if endpoint == "" {
		return "", errors.New("Provide an endpoint URL.")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("Endpoint must be an absolute http(s) URL.")
	}
	if parsed.Scheme == "http" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
		return "", errors.New("Endpoint must use https:// (http:// is allowed only for localhost or 127.0.0.1).")
	}
	if strings.ContainsAny(endpoint, "?#") || parsed.User != nil {
		return "", errors.New("Endpoint must not contain credentials, a query string, or a fragment.")
	}
	return endpoint, nil
}

func ValidateModel(value string) (string, error) {
	model := strings.TrimSpace(value)
	if model == "" {
		return "", errors.New("Provide one non-empty model ID without whitespace.")
	}
	for _, r := range model {
		if unicode.IsSpace(r) {
			return "", errors.New("Provide one non-empty model ID without whitespace.")
		}
	}
	return model, nil
}

func ValidateKey(value string) (string, error) {
	key := strings.TrimSpace(value)
	if key == "" || len(key) > 8192 {
		return "", errors.New("Provide one non-empty API key without whitespace (maximum 8 KiB).")
	}
	for _, r := range key {
		if unicode.IsSpace(r) {
			return "", errors.New("Provide one non-empty API key without whitespace (maximum 8 KiB).")
		}
	}
	if !utf8.ValidString(key) {
		return "", errors.New("Provide an API key containing only valid HTTP header characters.")
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f || r > 0xff {
			return "", errors.New("Provide an API key containing only valid HTTP header characters.")
		}
	}
	return key, nil
}
