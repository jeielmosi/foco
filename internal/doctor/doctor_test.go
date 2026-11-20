package doctor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"foco/internal/config"
	"foco/internal/enums"
	"foco/internal/provider"
)

func serverWith(handler http.HandlerFunc) (*httptest.Server, *string, *string) {
	var path, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		handler(w, r)
	}))
	return server, &path, &body
}

func systemOneProfile(server *httptest.Server) config.Profile {
	return config.Profile{
		Schema: enums.SchemaSystemOne, Label: "System One endpoint", Model: "gateway/jev-2",
		Endpoint: server.URL + "/typesafe/v1",
	}
}

func TestVerifySendsSyntheticCheck(t *testing.T) {
	server, path, body := serverWith(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer custom-key" {
			t.Errorf("authorization: %q", got)
		}
		fmt.Fprint(w, `{"answers":{"relevant":{"type":"noul","noul":0.9}}}`)
	})
	defer server.Close()
	client := provider.New(systemOneProfile(server), "custom-key", provider.Options{})
	if err := Verify(context.Background(), client, "System One endpoint"); err != nil {
		t.Fatal(err)
	}
	if *path != "/typesafe/v1" {
		t.Fatalf("path: %q", *path)
	}
	if !strings.Contains(*body, `"model":"gateway/jev-2"`) ||
		!strings.Contains(*body, `"relevant"`) ||
		!strings.Contains(*body, `"type":"noul"`) ||
		!strings.Contains(*body, "recordEvent") {
		t.Fatalf("body: %s", *body)
	}
}

func TestVerifyRejectsUnexpectedAnswer(t *testing.T) {
	server, _, _ := serverWith(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"answers":{"relevant":{"type":"noul","noul":0.2}}}`)
	})
	defer server.Close()
	client := provider.New(systemOneProfile(server), "key", provider.Options{})
	err := Verify(context.Background(), client, "System One endpoint")
	if err == nil || !strings.Contains(err.Error(), "unexpected answer") {
		t.Fatalf("got %v", err)
	}
}

func TestVerifyFormatsAPIDiagnostic(t *testing.T) {
	const key = "super-secret-key"
	server, _, _ := serverWith(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"error":{"message":"Free tier users do not have access. Key %s"}}`, key)
	})
	defer server.Close()
	client := provider.New(systemOneProfile(server), key, provider.Options{})
	err := Verify(context.Background(), client, "System One endpoint")
	if err == nil {
		t.Fatal("expected failure")
	}
	message := err.Error()
	if !strings.Contains(message, "System One endpoint") || !strings.Contains(message, "HTTP 403") ||
		!strings.Contains(message, "API: Free tier users do not have access.") {
		t.Fatalf("message: %q", message)
	}
	if strings.Contains(message, key) {
		t.Fatalf("key leaked: %q", message)
	}
}

func TestVerifyFallsBackWithoutMessage(t *testing.T) {
	server, _, _ := serverWith(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `<html>PRIVATE</html>`)
	})
	defer server.Close()
	client := provider.New(systemOneProfile(server), "key", provider.Options{})
	err := Verify(context.Background(), client, "System One endpoint")
	if err == nil || !strings.Contains(err.Error(), "Check the endpoint, key, model access, and account billing.") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "PRIVATE") {
		t.Fatalf("body leaked: %q", err)
	}
}

func TestVerifyNetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	profile := systemOneProfile(server)
	server.Close()
	client := provider.New(profile, "key", provider.Options{})
	err := Verify(context.Background(), client, "System One endpoint")
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("got %v", err)
	}
}
