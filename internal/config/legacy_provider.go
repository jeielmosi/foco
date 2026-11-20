package config

type LegacyProvider string

const (
	UnknownLegacyProvider    LegacyProvider = ""
	LegacyProviderVercel     LegacyProvider = "vercel"
	LegacyProviderTypeSafe   LegacyProvider = "typesafe"
	LegacyProviderOpenRouter LegacyProvider = "openrouter"
	LegacyProviderOpenCode   LegacyProvider = "opencode"
	LegacyProviderCustom     LegacyProvider = "custom"
)

func legacyProviderIsOneOf(value LegacyProvider, valid ...LegacyProvider) bool {
	for _, candidate := range valid {
		if value == candidate {
			return true
		}
	}
	return false
}

func (value LegacyProvider) IsValid() bool {
	return legacyProviderIsOneOf(value, LegacyProviderVercel, LegacyProviderTypeSafe, LegacyProviderOpenRouter, LegacyProviderOpenCode, LegacyProviderCustom)
}

func (value LegacyProvider) String() string {
	if !value.IsValid() {
		return string(UnknownLegacyProvider)
	}
	return string(value)
}

func NewLegacyProvider(input string) (LegacyProvider, bool) {
	value := LegacyProvider(input)
	if value.IsValid() {
		return value, true
	}
	return UnknownLegacyProvider, false
}
