package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"foco/internal/config"
	"foco/internal/enums"
	"foco/internal/prompt"
)

type fakePrompter struct {
	selection      string
	selectMessage  string
	selectOptions  []prompt.Option
	initial        int
	inputs         []string
	inputMessages  []string
	placeholders   []string
	password       string
	passwordPrompt string
	err            error
}

func (f *fakePrompter) Select(message string, options []prompt.Option, initial int) (string, error) {
	f.selectMessage = message
	f.selectOptions = options
	f.initial = initial
	if f.err != nil {
		return "", f.err
	}
	return f.selection, nil
}

func (f *fakePrompter) Input(message, placeholder string, validate func(string) error) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	value := f.inputs[0]
	f.inputs = f.inputs[1:]
	f.inputMessages = append(f.inputMessages, message)
	f.placeholders = append(f.placeholders, placeholder)
	if validate != nil {
		if err := validate(value); err != nil {
			return "", err
		}
	}
	return value, nil
}

func (f *fakePrompter) Password(message string, validate func(string) error) (string, error) {
	f.passwordPrompt = message
	if f.err != nil {
		return "", f.err
	}
	if validate != nil {
		if err := validate(f.password); err != nil {
			return "", err
		}
	}
	return f.password, nil
}

func setConfigHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	return filepath.Join(home, "foco", "credentials.json")
}

func TestAuthenticateStdinSchema(t *testing.T) {
	path := setConfigHome(t)
	var output bytes.Buffer
	err := Authenticate(context.Background(), Options{
		Schema: enums.SchemaGemini, Endpoint: "https://generativelanguage.googleapis.com/v1beta/decisions",
		Model: "gemini-decision-1", Stdin: true,
	},
		strings.NewReader("test-gateway-secret\n"), &output, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Gemini Decisions endpoint (generativelanguage.googleapis.com) key saved. Run foco doctor to verify access.") {
		t.Fatalf("output: %q", output.String())
	}
	if strings.Contains(output.String(), "test-gateway-secret") {
		t.Fatal("key echoed")
	}
	credentials, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Schema != enums.SchemaGemini || credentials.Model != "gemini-decision-1" ||
		credentials.Endpoint != "https://generativelanguage.googleapis.com/v1beta/decisions" || credentials.APIKey != "test-gateway-secret" {
		t.Fatalf("credentials: %+v", credentials)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %o", info.Mode().Perm())
	}
}

func TestAuthenticateRejectsWhitespaceWithoutReplacing(t *testing.T) {
	path := setConfigHome(t)
	if err := config.Save(path, config.Credentials{Provider: config.LegacyProviderVercel, APIKey: "existing-key"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := Authenticate(context.Background(), Options{
		Schema: enums.SchemaSystemOne, Endpoint: "https://api.example.com/systemone", Model: "jev-latest", Stdin: true,
	},
		strings.NewReader("two secrets\n"), &output, nil)
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(output.String(), "two secrets") {
		t.Fatal("key echoed")
	}
	credentials, _ := config.Load(path)
	if credentials.APIKey != "existing-key" {
		t.Fatalf("credentials replaced: %+v", credentials)
	}
}

func TestAuthenticateBoundsInput(t *testing.T) {
	path := setConfigHome(t)
	var output bytes.Buffer
	err := Authenticate(context.Background(), Options{
		Schema: enums.SchemaSystemOne, Endpoint: "https://api.example.com/systemone", Model: "jev-latest", Stdin: true,
	},
		strings.NewReader(strings.Repeat("x", 8193)), &output, nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("got %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("credentials must not be written")
	}
}

func TestAuthenticateSystemOneEndpoint(t *testing.T) {
	path := setConfigHome(t)
	var output bytes.Buffer
	err := Authenticate(context.Background(), Options{
		Schema: enums.SchemaSystemOne, Endpoint: "http://127.0.0.1:9090/typesafe/v1/systemone",
		Model: "gateway/jev-2", Stdin: true,
	}, strings.NewReader("custom-key\n"), &output, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "System One (Jev/Clef-compatible) endpoint (127.0.0.1:9090) key saved") {
		t.Fatalf("output: %q", output.String())
	}
	credentials, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Schema != enums.SchemaSystemOne || credentials.Endpoint != "http://127.0.0.1:9090/typesafe/v1/systemone" ||
		credentials.Model != "gateway/jev-2" || credentials.APIKey != "custom-key" {
		t.Fatalf("credentials: %+v", credentials)
	}
}

func TestAuthenticateSchemaRejectsInsecureEndpoint(t *testing.T) {
	setConfigHome(t)
	var output bytes.Buffer
	err := Authenticate(context.Background(), Options{
		Schema: enums.SchemaOpenAI, Endpoint: "http://gateway.example.com/v1/decisions",
		Model: "gateway/jev-2", Stdin: true,
	}, strings.NewReader("custom-key\n"), &output, nil)
	if err == nil || !strings.Contains(err.Error(), "https://") {
		t.Fatalf("got %v", err)
	}
}

func TestAuthenticateRequiresSchemaWhenNotInteractive(t *testing.T) {
	setConfigHome(t)
	var output bytes.Buffer
	err := Authenticate(context.Background(), Options{}, strings.NewReader("key\n"), &output, nil)
	if err == nil || !strings.Contains(err.Error(), "--schema") || !strings.Contains(err.Error(), "system-one") {
		t.Fatalf("got %v", err)
	}
}

func TestAuthenticateInteractiveSchema(t *testing.T) {
	path := setConfigHome(t)
	var output bytes.Buffer
	ui := &fakePrompter{
		selection: enums.SchemaOpenAI.String(),
		inputs:    []string{"https://api.openai.com/v1/decisions", "gpt-6-luna"},
		password:  "interactive-key",
	}
	err := Authenticate(context.Background(), Options{}, strings.NewReader(""), &output, ui)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Schema != enums.SchemaOpenAI || credentials.Endpoint != "https://api.openai.com/v1/decisions" ||
		credentials.Model != "gpt-6-luna" || credentials.APIKey != "interactive-key" {
		t.Fatalf("credentials: %+v", credentials)
	}
	if ui.selectMessage != "Choose an API schema" || ui.initial != 0 {
		t.Fatalf("select call: %q initial=%d", ui.selectMessage, ui.initial)
	}
	if len(ui.selectOptions) != 3 || ui.selectOptions[0].Value != enums.SchemaSystemOne.String() ||
		ui.selectOptions[2].Value != enums.SchemaOpenAI.String() || ui.selectOptions[2].Label != "OpenAI Decisions" {
		t.Fatalf("options: %+v", ui.selectOptions)
	}
	if ui.passwordPrompt != "Paste the API key for OpenAI Decisions" {
		t.Fatalf("password prompt: %q", ui.passwordPrompt)
	}
}

func TestAuthenticateInteractiveEndpoint(t *testing.T) {
	path := setConfigHome(t)
	var output bytes.Buffer
	ui := &fakePrompter{
		selection: enums.SchemaGemini.String(),
		inputs:    []string{"https://generativelanguage.googleapis.com/v1beta/decisions", "gemini-decision-1"},
		password:  "interactive-key",
	}
	err := Authenticate(context.Background(), Options{}, strings.NewReader(""), &output, ui)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Schema != enums.SchemaGemini || credentials.Endpoint != "https://generativelanguage.googleapis.com/v1beta/decisions" ||
		credentials.Model != "gemini-decision-1" {
		t.Fatalf("credentials: %+v", credentials)
	}
	if len(ui.inputMessages) != 2 || ui.inputMessages[0] != "Exact API endpoint URL" ||
		ui.inputMessages[1] != "Model ID" {
		t.Fatalf("input messages: %+v", ui.inputMessages)
	}
	if ui.passwordPrompt != "Paste the API key for Gemini Decisions" {
		t.Fatalf("password prompt: %q", ui.passwordPrompt)
	}
}

func TestAuthenticateInteractiveCancel(t *testing.T) {
	setConfigHome(t)
	var output bytes.Buffer
	ui := &fakePrompter{err: prompt.ErrCancelled}
	err := Authenticate(context.Background(), Options{}, strings.NewReader(""), &output, ui)
	if !errors.Is(err, prompt.ErrCancelled) {
		t.Fatalf("got %v", err)
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(data []byte) (int, error) { return f(data) }

func TestReadKeyAbortsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release := make(chan struct{})
	reader := readerFunc(func([]byte) (int, error) {
		<-release
		return 0, io.EOF
	})
	_, err := readKey(ctx, reader)
	close(release)
	if !errors.Is(err, prompt.ErrCancelled) {
		t.Fatalf("got %v", err)
	}
}

func TestAuthenticateCanceledContextDoesNotWrite(t *testing.T) {
	path := setConfigHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	err := Authenticate(ctx, Options{
		Schema: enums.SchemaSystemOne, Endpoint: "https://api.example.com/systemone", Model: "jev-latest", Stdin: true,
	},
		strings.NewReader("key\n"), &output, nil)
	if !errors.Is(err, prompt.ErrCancelled) {
		t.Fatalf("got %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("credentials must not be written")
	}
}

type cancelingPrompter struct {
	cancel   context.CancelFunc
	password string
}

func (p cancelingPrompter) Select(string, []prompt.Option, int) (string, error) {
	return enums.SchemaSystemOne.String(), nil
}

func (p cancelingPrompter) Input(message, _ string, _ func(string) error) (string, error) {
	if message == "Exact API endpoint URL" {
		return "https://api.example.com/systemone", nil
	}
	return "jev-latest", nil
}

func (p cancelingPrompter) Password(string, func(string) error) (string, error) {
	p.cancel()
	return p.password, nil
}

func TestAuthenticateDoesNotSaveWhenContextCancelsDuringPrompt(t *testing.T) {
	path := setConfigHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	var output bytes.Buffer
	ui := cancelingPrompter{cancel: cancel, password: "interactive-key"}
	err := Authenticate(ctx, Options{}, strings.NewReader(""), &output, ui)
	if !errors.Is(err, prompt.ErrCancelled) {
		t.Fatalf("got %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("credentials must not be written")
	}
}
