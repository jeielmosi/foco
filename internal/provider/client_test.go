package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"foco/internal/config"
	"foco/internal/enums"
)

func systemOneProfile(server *httptest.Server) config.Profile {
	return config.Profile{
		Schema: enums.SchemaSystemOne, Label: "System One",
		Model: "jev-latest", Endpoint: server.URL + "/systemone",
	}
}

func systemOneEndpointProfile(server *httptest.Server) config.Profile {
	return config.Profile{
		Schema: enums.SchemaSystemOne, Label: "System One endpoint",
		Model:    "gateway/jev-2",
		Endpoint: server.URL + "/typesafe/v1",
	}
}

func testRequest() Request {
	return Request{
		State: map[string]string{"source": "export function recordEvent() {}"},
		Questions: []Question{
			{ID: "q1", Instructions: "Does it record?"},
			{ID: "q0", Instructions: "Does it delete?"},
		},
	}
}

func answersBody() string {
	return `{"model":"typesafe-ai/jev","answers":{"q1":{"type":"noul","noul":0.83},"q0":{"type":"noul","noul":0.12}},"usage":{"input_tokens":12,"output_tokens":2}}`
}

func TestEvaluateSendsProtocolAndMapsAnswers(t *testing.T) {
	var mu sync.Mutex
	var observedPath, observedAuth, observedType, observedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		observedPath = r.URL.Path
		observedAuth = r.Header.Get("Authorization")
		observedType = r.Header.Get("Content-Type")
		observedBody = string(body)
		mu.Unlock()
		fmt.Fprint(w, answersBody())
	}))
	defer server.Close()

	client := New(systemOneProfile(server), "synthetic-key", Options{})
	scores, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if scores["q1"] != 0.83 || scores["q0"] != 0.12 {
		t.Fatalf("scores: %+v", scores)
	}
	if client.Requests() != 1 {
		t.Fatalf("requests: %d", client.Requests())
	}
	if observedPath != "/systemone" || observedAuth != "Bearer synthetic-key" {
		t.Fatalf("path=%q auth=%q", observedPath, observedAuth)
	}
	if !strings.HasPrefix(observedType, "application/json") {
		t.Fatalf("content type: %q", observedType)
	}
	if !strings.Contains(observedBody, `"model":"jev-latest"`) ||
		!strings.Contains(observedBody, `"type":"noul"`) ||
		!strings.Contains(observedBody, `"source":"export function recordEvent() {}"`) {
		t.Fatalf("body: %s", observedBody)
	}
	if strings.Index(observedBody, `"q1"`) > strings.Index(observedBody, `"q0"`) {
		t.Fatalf("question order lost: %s", observedBody)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(observedBody), &body); err != nil {
		t.Fatal(err)
	}
	questions, ok := body["questions"].(map[string]any)
	if !ok || len(questions) != 2 {
		t.Fatalf("questions: %#v", body["questions"])
	}
}

func TestSystemOneUsesExactConfiguredEndpoint(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		fmt.Fprint(w, answersBody())
	}))
	defer server.Close()

	client := New(systemOneEndpointProfile(server), "key", Options{})
	request := Request{
		State:     map[string]string{"source": "x"},
		Questions: []Question{{ID: "q1", Instructions: "a"}, {ID: "q0", Instructions: "b"}},
	}
	if _, err := client.Evaluate(context.Background(), request, EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if path != "/typesafe/v1" {
		t.Fatalf("path: %q", path)
	}
}

func TestAuthenticationFailureStopsFurtherRequests(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"message":"Free tier users do not have access."}}`)
	}))
	defer server.Close()

	client := New(systemOneProfile(server), "key", Options{})
	_, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{})
	var failure *Failure
	if !errors.As(err, &failure) || failure.Kind != enums.FailureAuthentication {
		t.Fatalf("first: %v", err)
	}
	if failure.Diagnostic == nil || failure.Diagnostic.StatusCode != 403 || failure.Diagnostic.Message != "Free tier users do not have access." {
		t.Fatalf("diagnostic: %+v", failure.Diagnostic)
	}
	_, err = client.Evaluate(context.Background(), testRequest(), EvalOptions{})
	if !errors.As(err, &failure) || failure.Kind != enums.FailureAuthentication {
		t.Fatalf("second: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls: %d", calls)
	}
}

func TestRetriesTransientThenSucceeds(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, answersBody())
	}))
	defer server.Close()

	client := New(systemOneProfile(server), "key", Options{})
	if _, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls: %d", calls)
	}
}

func TestRequestLimitStopsBeforeNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, answersBody())
	}))
	defer server.Close()

	client := New(systemOneProfile(server), "key", Options{RequestLimit: 1})
	if _, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	_, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{})
	var failure *Failure
	if !errors.As(err, &failure) || failure.Kind != enums.FailureRequestLimit {
		t.Fatalf("got %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls: %d", calls)
	}
}

func TestNavigationRetries429Once(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, answersBody())
	}))
	defer server.Close()

	client := New(systemOneProfile(server), "key", Options{})
	if _, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{Navigation: true}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls: %d", calls)
	}
}

func TestNavigationServerErrorDoesNotRetryAndSplits(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := New(systemOneProfile(server), "key", Options{})
	_, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{Navigation: true})
	var failure *Failure
	if !errors.As(err, &failure) || failure.Kind != enums.FailureProvider || !failure.SplitEligible {
		t.Fatalf("got %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls: %d", calls)
	}
}

func TestDiagnosticSanitizesAndRedactsKey(t *testing.T) {
	const key = "super-secret-key-value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error":{"message":"Rejected %s\nwith \u001b[31m%s\u001b[0m and PRIVATE"}}`, key, key)
	}))
	defer server.Close()

	client := New(systemOneProfile(server), key, Options{})
	_, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{})
	var failure *Failure
	if !errors.As(err, &failure) || failure.Diagnostic == nil {
		t.Fatalf("got %v", err)
	}
	message := failure.Diagnostic.Message
	if strings.Contains(message, key) || strings.ContainsRune(message, '\x1b') {
		t.Fatalf("unsafe diagnostic: %q", message)
	}
	if !strings.Contains(message, "[redacted]") {
		t.Fatalf("message: %q", message)
	}
}

func TestDiagnosticOmittedWhenKeyIsSplit(t *testing.T) {
	const key = "abcdefghijklmnop"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"message":"Rejected token %s %s"}`, key[:6], key[6:])
	}))
	defer server.Close()

	client := New(systemOneProfile(server), key, Options{})
	_, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{})
	var failure *Failure
	if !errors.As(err, &failure) || failure.Diagnostic == nil || failure.Diagnostic.Message != "" {
		t.Fatalf("got %+v", failure)
	}
}

func TestInvalidAnswersAreRejected(t *testing.T) {
	cases := map[string]string{
		"missing":   `{"answers":{"q1":{"type":"noul","noul":0.8}}}`,
		"range":     `{"answers":{"q1":{"type":"noul","noul":0.8},"q0":{"type":"noul","noul":2}}}`,
		"type":      `{"answers":{"q1":{"type":"noul","noul":0.8},"q0":{"type":"boolean","noul":0.2}}}`,
		"null":      `{"answers":{"q1":{"type":"noul","noul":0.8},"q0":{"type":"noul","noul":null}}}`,
		"malformed": `{broken`,
	}
	for name, body := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		}))
		client := New(systemOneProfile(server), "key", Options{})
		_, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{})
		var failure *Failure
		if !errors.As(err, &failure) || failure.Kind != enums.FailureProvider {
			t.Errorf("%s: got %v", name, err)
		}
		server.Close()
	}
}

func TestEvaluateTimeoutReportsExactMessage(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		fmt.Fprint(w, answersBody())
	}))
	defer server.Close()
	defer close(release)

	client := New(systemOneEndpointProfile(server), "key", Options{Timeout: 50 * time.Millisecond})
	_, err := client.Evaluate(context.Background(), testRequest(), EvalOptions{})
	failure, ok := err.(*Failure)
	if !ok {
		t.Fatalf("err = %T %v, want *Failure", err, err)
	}
	if failure.Kind != enums.FailureProvider {
		t.Fatalf("kind = %q, want %q", failure.Kind, enums.FailureProvider)
	}
	const want = "Request timed out after 50 ms (max concurrent requests: 32)"
	if failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

func TestCancelledContextDoesNotCallProvider(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, answersBody())
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := New(systemOneProfile(server), "key", Options{})
	_, err := client.Evaluate(ctx, testRequest(), EvalOptions{})
	var failure *Failure
	if !errors.As(err, &failure) || failure.Kind != enums.FailureCancelled {
		t.Fatalf("got %v", err)
	}
	if calls != 0 {
		t.Fatalf("calls: %d", calls)
	}
}
