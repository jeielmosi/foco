package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"foco/internal/config"
	"foco/internal/enums"
)

type providerBody struct {
	State     json.RawMessage            `json:"state"`
	Questions map[string]json.RawMessage `json:"questions"`
}

type observedRequest struct {
	method        string
	path          string
	authorization string
	contentType   string
	body          string
}

type requestRecorder struct {
	mu       sync.Mutex
	requests []observedRequest
}

func (recorder *requestRecorder) record(request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	request.Body = io.NopCloser(bytes.NewReader(body))
	recorder.mu.Lock()
	recorder.requests = append(recorder.requests, observedRequest{
		method:        request.Method,
		path:          request.URL.Path,
		authorization: request.Header.Get("Authorization"),
		contentType:   request.Header.Get("Content-Type"),
		body:          string(body),
	})
	recorder.mu.Unlock()
}

func (recorder *requestRecorder) snapshot() []observedRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	out := make([]observedRequest, len(recorder.requests))
	copy(out, recorder.requests)
	return out
}

func recordRequests(recorder *requestRecorder, handler http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		recorder.record(request)
		handler(writer, request)
	}
}

func routedProvider(t *testing.T, recorder *requestRecorder) *httptest.Server {
	t.Helper()
	return httptest.NewServer(recordRequests(recorder, func(writer http.ResponseWriter, request *http.Request) {
		var body providerBody
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		answers := map[string]map[string]any{}
		var state struct {
			Items        []map[string]any `json:"items"`
			Preview      map[string]any   `json:"preview"`
			Declarations []map[string]any `json:"declarations"`
		}
		_ = json.Unmarshal(body.State, &state)
		for id := range body.Questions {
			value := 0.9
			switch {
			case state.Items != nil:
				value = 0.1
				index := 0
				if _, err := fmt.Sscanf(id, "q%d", &index); err == nil && index < len(state.Items) {
					if path, _ := state.Items[index]["path"].(string); strings.HasSuffix(path, ".ts") {
						value = 0.9
					}
				}
			case state.Preview != nil:
				if id == "implementation" || id == "priority" {
					value = 0.9
				} else {
					value = 0.1
				}
			case state.Declarations != nil:
				value = 0.9
			}
			answers[id] = map[string]any{"type": "noul", "noul": value}
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"answers": answers})
	}))
}

func TestSearchIntegrationRendersHeaderSourceAndContext(t *testing.T) {
	recorder := &requestRecorder{}
	server := routedProvider(t, recorder)
	defer server.Close()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	credentialsPath := filepath.Join(configHome, "foco", "credentials.json")
	if err := config.Save(credentialsPath, config.Credentials{
		Schema: enums.SchemaSystemOne, Endpoint: server.URL + "/typesafe/v1", Model: "gateway/jev-2", APIKey: "custom-key",
	}); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../testdata/search/tree")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), []string{"event recording", root}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Foco: 2 relevant files.",
		`- "main.ts" — implementation; source below`,
		`"src/events.ts"`,
		`Source block "main.ts" lines 1-3:`,
		`AGENTS.md lookup (root and returned-file ancestors): "AGENTS.md".`,
		"\n\nEnd context.\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in:\n%s", want, output)
		}
	}
	if strings.Contains(output, "DO_NOT_UPLOAD") {
		t.Fatalf("excluded content leaked:\n%s", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("progress must be disabled for buffers: %q", stderr.String())
	}
	requests := recorder.snapshot()
	if len(requests) == 0 {
		t.Fatal("expected provider requests")
	}
	for _, request := range requests {
		if request.path != "/typesafe/v1" {
			t.Fatalf("custom endpoint path = %q, want /typesafe/v1", request.path)
		}
		if request.body == "" {
			t.Fatal("empty provider request body")
		}
		if !strings.Contains(request.body, `"model":"gateway/jev-2"`) {
			t.Fatalf("body missing model: %s", request.body)
		}
		if !strings.Contains(request.body, `"type":"noul"`) {
			t.Fatalf("body missing noul question: %s", request.body)
		}
	}
}

func TestSearchIntegrationUsesExactConfiguredEndpoint(t *testing.T) {
	recorder := &requestRecorder{}
	server := httptest.NewServer(recordRequests(recorder, func(writer http.ResponseWriter, request *http.Request) {
		var body providerBody
		_ = json.NewDecoder(request.Body).Decode(&body)
		answers := map[string]map[string]any{}
		for id := range body.Questions {
			answers[id] = map[string]any{"type": "noul", "noul": 0.9}
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"answers": answers})
	}))
	defer server.Close()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if err := config.Save(filepath.Join(configHome, "foco", "credentials.json"), config.Credentials{
		Schema: enums.SchemaSystemOne, Endpoint: server.URL + "/typesafe/v1", Model: "gateway/jev-2", APIKey: "custom-key",
	}); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../testdata/search/tree")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), []string{"event", root}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	requests := recorder.snapshot()
	if len(requests) == 0 {
		t.Fatal("expected at least one provider request")
	}
	for _, request := range requests {
		if request.method != http.MethodPost {
			t.Fatalf("method = %q, want POST", request.method)
		}
		if request.path != "/typesafe/v1" {
			t.Fatalf("custom endpoint path = %q, want /typesafe/v1", request.path)
		}
	}
	first := requests[0]
	if first.authorization != "Bearer custom-key" {
		t.Fatalf("authorization = %q, want Bearer custom-key", first.authorization)
	}
	if !strings.HasPrefix(first.contentType, "application/json") {
		t.Fatalf("content type = %q, want application/json", first.contentType)
	}
}
