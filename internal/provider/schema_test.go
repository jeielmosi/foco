package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"foco/internal/config"
	"foco/internal/enums"
)

func TestEvaluateGeminiDecisionsSchema(t *testing.T) {
	var observedAuth string
	var observed map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedAuth = r.Header.Get("x-goog-api-key")
		if err := json.NewDecoder(r.Body).Decode(&observed); err != nil {
			t.Error(err)
		}
		fmt.Fprint(w, `{"answers":{"q1":{"type":"noul","noul":0.83,"confidence":0.91},"q0":{"type":"noul","noul":0.12}}}`)
	}))
	defer server.Close()

	client := New(config.Profile{
		Schema: enums.SchemaGemini, Endpoint: server.URL, Model: "gemini-decision-1",
	}, "gemini-key", Options{})
	scores, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if observedAuth != "gemini-key" {
		t.Fatalf("x-goog-api-key: %q", observedAuth)
	}
	if observed["model"] != "gemini-decision-1" || observed["input"] != `{"source":"export function recordEvent() {}"}` {
		t.Fatalf("request: %#v", observed)
	}
	questions, ok := observed["questions"].(map[string]any)
	question, questionOK := questions["q1"].(map[string]any)
	if !ok || !questionOK || question["type"] != "noul" {
		t.Fatalf("questions: %#v", observed["questions"])
	}
	if scores["q1"] != 0.83 || scores["q0"] != 0.12 {
		t.Fatalf("scores: %+v", scores)
	}
}

func TestEvaluateOpenAIDecisionsSchema(t *testing.T) {
	var observedAuth string
	var observed map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&observed); err != nil {
			t.Error(err)
		}
		fmt.Fprint(w, `{"answers":[{"type":"predicate","name":"q1","probability":0.83},{"type":"predicate","name":"q0","probability":0.12}]}`)
	}))
	defer server.Close()

	client := New(config.Profile{
		Schema: enums.SchemaOpenAI, Endpoint: server.URL, Model: "gpt-6-luna",
	}, "openai-key", Options{})
	scores, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if observedAuth != "Bearer openai-key" {
		t.Fatalf("Authorization: %q", observedAuth)
	}
	if observed["model"] != "gpt-6-luna" || observed["input"] != `{"source":"export function recordEvent() {}"}` {
		t.Fatalf("request: %#v", observed)
	}
	questions, ok := observed["questions"].([]any)
	if !ok || len(questions) != 2 {
		t.Fatalf("questions: %#v", observed["questions"])
	}
	first, firstOK := questions[0].(map[string]any)
	if !firstOK || first["type"] != "predicate" || first["name"] != "q1" {
		t.Fatalf("first question: %#v", first)
	}
	if scores["q1"] != 0.83 || scores["q0"] != 0.12 {
		t.Fatalf("scores: %+v", scores)
	}
}

func TestEvaluateRejectsMissingSchema(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"answers":{"q1":{"type":"noul","noul":0.9}}}`)
	}))
	defer server.Close()

	client := New(config.Profile{Endpoint: server.URL, Model: "model"}, "key", Options{})
	if _, err := client.Evaluate(context.Background(), Request{
		State: "state", Questions: []Question{{ID: "q1", Instructions: "question"}},
	}, EvalOptions{}); err == nil {
		t.Fatal("missing API schema must fail")
	}
	if calls != 0 {
		t.Fatalf("requests sent with missing schema: %d", calls)
	}
}

func TestGeminiDecisionsRejectsOutOfRangeNoul(t *testing.T) {
	request := Request{Questions: []Question{{ID: "q", Instructions: "question"}}}
	for _, value := range []float64{0.01, 0.99} {
		payload := fmt.Sprintf(`{"answers":{"q":{"type":"noul","noul":%v}}}`, value)
		if _, failure := (geminiDecisionsSchema{}).decodeResponse([]byte(payload), request); failure != nil {
			t.Errorf("rejected Gemini noul boundary %v: %v", value, failure)
		}
	}
	for _, value := range []float64{0, 1, -0.1, 1.1} {
		payload := fmt.Sprintf(`{"answers":{"q":{"type":"noul","noul":%v}}}`, value)
		if _, failure := (geminiDecisionsSchema{}).decodeResponse([]byte(payload), request); failure == nil {
			t.Errorf("accepted Gemini noul probability %v", value)
		}
	}
}

func TestOpenAIDecisionsRejectsUnmatchedAnswers(t *testing.T) {
	request := Request{Questions: []Question{
		{ID: "q0", Instructions: "first"}, {ID: "q1", Instructions: "second"},
	}}
	for _, payload := range []string{
		`{"answers":[{"type":"predicate","name":"q0","probability":0.8}]}`,
		`{"answers":[{"type":"predicate","name":"q0","probability":0.8},{"type":"predicate","name":"q0","probability":0.2}]}`,
		`{"answers":[{"type":"predicate","name":"unknown","probability":0.8},{"type":"predicate","name":"q1","probability":0.2}]}`,
		`{"answers":[{"type":"predicate","name":"q0","probability":1.1},{"type":"predicate","name":"q1","probability":0.2}]}`,
	} {
		if _, failure := (openAIDecisionsSchema{}).decodeResponse([]byte(payload), request); failure == nil {
			t.Errorf("accepted invalid response: %s", payload)
		}
	}
}
