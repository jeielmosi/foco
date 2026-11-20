package provider

import (
	"encoding/json"
	"net/http"
)

type geminiDecisionsSchema struct{}

type geminiQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

type geminiRequest struct {
	Model     string                    `json:"model"`
	Input     string                    `json:"input"`
	Questions map[string]geminiQuestion `json:"questions"`
}

func (geminiDecisionsSchema) marshalRequest(model string, request Request) ([]byte, error) {
	input, err := decisionInput(request.State)
	if err != nil {
		return nil, err
	}
	questions := make(map[string]geminiQuestion, len(request.Questions))
	for _, question := range request.Questions {
		questions[question.ID] = geminiQuestion{Type: "noul", Instructions: question.Instructions}
	}
	return json.Marshal(geminiRequest{Model: model, Input: input, Questions: questions})
}

func (geminiDecisionsSchema) setAuthentication(request *http.Request, apiKey string) {
	request.Header.Set("x-goog-api-key", apiKey)
}

func (geminiDecisionsSchema) decodeResponse(payload []byte, request Request) (map[string]float64, *Failure) {
	var response noulResponse
	if err := json.Unmarshal(payload, &response); err != nil || response.Answers == nil {
		return nil, invalidResponse()
	}
	scores := make(map[string]float64, len(request.Questions))
	for _, question := range request.Questions {
		answer, ok := response.Answers[question.ID]
		if !ok || answer.Type != "noul" || answer.Noul == nil || !validGeminiProbability(*answer.Noul) {
			return nil, invalidResponse()
		}
		scores[question.ID] = *answer.Noul
	}
	return scores, nil
}
