package provider

import (
	"encoding/json"
	"math"
	"net/http"

	"foco/internal/enums"
)

type decisionSchema interface {
	marshalRequest(model string, request Request) ([]byte, error)
	setAuthentication(request *http.Request, apiKey string)
	decodeResponse(payload []byte, request Request) (map[string]float64, *Failure)
}

func schemaAdapterFor(schema enums.Schema) decisionSchema {
	switch schema {
	case enums.SchemaSystemOne:
		return systemOneSchema{}
	case enums.SchemaGemini:
		return geminiDecisionsSchema{}
	case enums.SchemaOpenAI:
		return openAIDecisionsSchema{}
	default:
		return nil
	}
}

func decisionInput(state any) (string, error) {
	if text, ok := state.(string); ok {
		return text, nil
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func validProbability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func validGeminiProbability(value float64) bool {
	return validProbability(value) && value >= 0.01 && value <= 0.99
}
