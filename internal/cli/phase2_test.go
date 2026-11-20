package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"foco/internal/config"
	"foco/internal/enums"
	"foco/internal/prompt"
)

func setConfigHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	return filepath.Join(home, "foco", "credentials.json")
}

func TestRunAuthStdin(t *testing.T) {
	path := setConfigHome(t)
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(),
		[]string{"auth", "--schema", "system-one", "--endpoint", "https://api.example.com/systemone", "--model", "jev-latest", "--stdin"},
		strings.NewReader("test-gateway-secret\n"), &output, &errorOutput)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, output.String())
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr: %q", errorOutput.String())
	}
	if !strings.Contains(output.String(), "System One (Jev/Clef-compatible) endpoint (api.example.com) key saved") {
		t.Fatalf("output: %q", output.String())
	}
	if strings.Contains(output.String(), "test-gateway-secret") {
		t.Fatal("key echoed")
	}
	credentials, err := config.Load(path)
	if err != nil || credentials.APIKey != "test-gateway-secret" {
		t.Fatalf("credentials: %+v, %v", credentials, err)
	}
}

func TestRunAuthRequiresSchemaWhenPiped(t *testing.T) {
	setConfigHome(t)
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"auth"},
		strings.NewReader("key\n"), &output, &errorOutput)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(output.String(), "--schema") {
		t.Fatalf("output: %q", output.String())
	}
}

func TestRunDoctorVerifiesCustomEndpoint(t *testing.T) {
	path := setConfigHome(t)
	var requestPath, authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authorization = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"answers":{"relevant":{"type":"noul","noul":0.9}}}`)
	}))
	defer server.Close()
	endpoint := server.URL + "/typesafe/v1/systemone"
	if err := config.Save(path, config.Credentials{
		Schema: enums.SchemaSystemOne, Endpoint: endpoint, Model: "gateway/jev-2", APIKey: "custom-key",
	}); err != nil {
		t.Fatal(err)
	}
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"doctor"}, strings.NewReader(""), &output, &errorOutput)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, output.String())
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr: %q", errorOutput.String())
	}
	label := "System One (Jev/Clef-compatible) endpoint (" + strings.TrimPrefix(server.URL, "http://") + ")"
	if !strings.Contains(output.String(), "API connection verified through "+label+".") {
		t.Fatalf("output: %q", output.String())
	}
	if strings.Contains(output.String(), "custom-key") {
		t.Fatal("key echoed")
	}
	if requestPath != "/typesafe/v1/systemone" || authorization != "Bearer custom-key" {
		t.Fatalf("path=%q auth=%q", requestPath, authorization)
	}
}

func TestRunDoctorMissingCredentials(t *testing.T) {
	setConfigHome(t)
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"doctor"}, strings.NewReader(""), &output, &errorOutput)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(output.String(), "Run foco auth or use foco auth --schema NAME --endpoint URL --model ID --stdin.") {
		t.Fatalf("output: %q", output.String())
	}
}

func TestRunDoctorInvalidCredentials(t *testing.T) {
	path := setConfigHome(t)
	if err := config.Save(path, config.Credentials{
		Schema: enums.SchemaSystemOne, Endpoint: "https://gateway.example.com/v1/systemone",
		Model: "jev-latest", APIKey: "stored-key",
	}); err != nil {
		t.Fatal(err)
	}
	body := `{"schema":"system-one","endpoint":"http://gateway.example.com/v1/systemone","model":"jev-latest","apiKey":"stored-key"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"doctor"}, strings.NewReader(""), &output, &errorOutput)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(output.String(), "Could not read valid credentials. Run foco auth again.") {
		t.Fatalf("output: %q", output.String())
	}
	if strings.Contains(output.String(), "stored-key") || strings.Contains(output.String(), "gateway.example.com") {
		t.Fatalf("leaked: %q", output.String())
	}
}

func TestRunDoctorFormatsAPIFailure(t *testing.T) {
	path := setConfigHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"message":"Free tier users do not have access."}}`)
	}))
	defer server.Close()
	if err := config.Save(path, config.Credentials{
		Schema: enums.SchemaSystemOne, Endpoint: server.URL + "/systemone",
		Model: "jev-latest", APIKey: "custom-key",
	}); err != nil {
		t.Fatal(err)
	}
	var output, errorOutput bytes.Buffer
	code := RunContext(context.Background(), []string{"doctor"}, strings.NewReader(""), &output, &errorOutput)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	message := output.String()
	if !strings.Contains(message, "HTTP 403") || !strings.Contains(message, "API: Free tier users do not have access.") {
		t.Fatalf("output: %q", message)
	}
	if strings.Contains(message, "custom-key") {
		t.Fatalf("key leaked: %q", message)
	}
}

func TestFinishMapsPromptCancellation(t *testing.T) {
	var output bytes.Buffer
	if code := finish(&output, prompt.ErrCancelled); code != 130 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(output.String()) != "Interrupted." {
		t.Fatalf("output: %q", output.String())
	}
}
