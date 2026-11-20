package provider

import (
	"encoding/json"
	"net/http"
)

type openAIDecisionsSchema struct{}

type openAIQuestion struct {
	Type         string `json:"type"`
	Name         string `json:"name"`
	Instructions string `json:"instructions"`
}

type openAIRequest struct {
	Model     string           `json:"model"`
	Input     string           `json:"input"`
	Questions []openAIQuestion `json:"questions"`
}

type openAIAnswer struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Probability *float64 `json:"probability"`
}

type openAIResponse struct {
	Answers []openAIAnswer `json:"answers"`
}

func (openAIDecisionsSchema) marshalRequest(model string, request Request) ([]byte, error) {
	input, err := decisionInput(request.State)
	if err != nil {
		return nil, err
	}
	questions := make([]openAIQuestion, len(request.Questions))
	for index, question := range request.Questions {
		questions[index] = openAIQuestion{Type: "predicate", Name: question.ID, Instructions: question.Instructions}
	}
	return json.Marshal(openAIRequest{Model: model, Input: input, Questions: questions})
}

func (openAIDecisionsSchema) setAuthentication(request *http.Request, apiKey string) {
	request.Header.Set("Authorization", "Bearer "+apiKey)
}

func (openAIDecisionsSchema) decodeResponse(payload []byte, request Request) (map[string]float64, *Failure) {
	var response openAIResponse
	if err := json.Unmarshal(payload, &response); err != nil || len(response.Answers) != len(request.Questions) {
		return nil, invalidResponse()
	}
	expected := make(map[string]struct{}, len(request.Questions))
	for _, question := range request.Questions {
		expected[question.ID] = struct{}{}
	}
	scores := make(map[string]float64, len(request.Questions))
	for _, answer := range response.Answers {
		if answer.Type != "predicate" || answer.Probability == nil || !validProbability(*answer.Probability) {
			return nil, invalidResponse()
		}
		if _, ok := expected[answer.Name]; !ok {
			return nil, invalidResponse()
		}
		if _, duplicate := scores[answer.Name]; duplicate {
			return nil, invalidResponse()
		}
		scores[answer.Name] = *answer.Probability
	}
	if len(scores) != len(expected) {
		return nil, invalidResponse()
	}
	return scores, nil
}
