package provider

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type systemOneSchema struct{}

type systemOneQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

type noulAnswer struct {
	Type string   `json:"type"`
	Noul *float64 `json:"noul"`
}

type noulResponse struct {
	Answers map[string]noulAnswer `json:"answers"`
}

func (systemOneSchema) marshalRequest(model string, request Request) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString(`{"model":`)
	encoded, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	buffer.Write(encoded)
	buffer.WriteString(`,"state":`)
	encoded, err = json.Marshal(request.State)
	if err != nil {
		return nil, err
	}
	buffer.Write(encoded)
	buffer.WriteString(`,"questions":{`)
	for index, question := range request.Questions {
		if index > 0 {
			buffer.WriteByte(',')
		}
		id, err := json.Marshal(question.ID)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(systemOneQuestion{Type: "noul", Instructions: question.Instructions})
		if err != nil {
			return nil, err
		}
		buffer.Write(id)
		buffer.WriteByte(':')
		buffer.Write(value)
	}
	buffer.WriteString("}}")
	return buffer.Bytes(), nil
}

func (systemOneSchema) setAuthentication(request *http.Request, apiKey string) {
	request.Header.Set("Authorization", "Bearer "+apiKey)
}

func (systemOneSchema) decodeResponse(payload []byte, request Request) (map[string]float64, *Failure) {
	var response noulResponse
	if err := json.Unmarshal(payload, &response); err != nil || response.Answers == nil {
		return nil, invalidResponse()
	}
	scores := make(map[string]float64, len(request.Questions))
	for _, question := range request.Questions {
		answer, ok := response.Answers[question.ID]
		if !ok || answer.Type != "noul" || answer.Noul == nil || !validProbability(*answer.Noul) {
			return nil, invalidResponse()
		}
		scores[question.ID] = *answer.Noul
	}
	return scores, nil
}
