package prompt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func newTestPrompter(input, output *bytes.Buffer) Prompter {
	return New(context.Background(), input, output)
}

func TestSelectDefaultsOnEmptyLine(t *testing.T) {
	input := bytes.NewBufferString("\n")
	var output bytes.Buffer
	value, err := newTestPrompter(input, &output).Select("Choose an API schema", []Option{
		{Value: "system-one", Label: "System One"},
		{Value: "gemini-decisions", Label: "Gemini Decisions"},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if value != "gemini-decisions" {
		t.Fatalf("value %q", value)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("output contains ANSI styling: %q", output.String())
	}
}

func TestSelectAcceptsNumber(t *testing.T) {
	input := bytes.NewBufferString("1\n")
	var output bytes.Buffer
	value, err := newTestPrompter(input, &output).Select("Choose an API schema", []Option{
		{Value: "system-one", Label: "System One"},
		{Value: "gemini-decisions", Label: "Gemini Decisions"},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if value != "system-one" {
		t.Fatalf("value %q", value)
	}
	if !strings.Contains(output.String(), "1) System One") {
		t.Fatalf("options not listed: %q", output.String())
	}
}

func TestSelectRepromptsOnInvalidChoice(t *testing.T) {
	input := bytes.NewBufferString("9\n2\n")
	var output bytes.Buffer
	value, err := newTestPrompter(input, &output).Select("Choose an API schema", []Option{
		{Value: "system-one", Label: "System One"},
		{Value: "gemini-decisions", Label: "Gemini Decisions"},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if value != "gemini-decisions" {
		t.Fatalf("value %q", value)
	}
	if !strings.Contains(output.String(), "Invalid choice") {
		t.Fatalf("missing invalid choice message: %q", output.String())
	}
}

func TestInputReturnsValue(t *testing.T) {
	input := bytes.NewBufferString("https://example.test/v1\n")
	var output bytes.Buffer
	value, err := newTestPrompter(input, &output).Input("Base URL", "https://example.test/v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if value != "https://example.test/v1" {
		t.Fatalf("value %q", value)
	}
	if !strings.Contains(output.String(), "(e.g. https://example.test/v1)") {
		t.Fatalf("placeholder missing: %q", output.String())
	}
}

func TestInputRepromptsUntilValid(t *testing.T) {
	input := bytes.NewBufferString("bad\ngood\n")
	var output bytes.Buffer
	validate := func(value string) error {
		if value != "good" {
			return errors.New("invalid value")
		}
		return nil
	}
	value, err := newTestPrompter(input, &output).Input("Model", "", validate)
	if err != nil {
		t.Fatal(err)
	}
	if value != "good" {
		t.Fatalf("value %q", value)
	}
	if !strings.Contains(output.String(), "invalid value") {
		t.Fatalf("validation error missing: %q", output.String())
	}
}

func TestPasswordReadsLineWhenNotTerminal(t *testing.T) {
	input := bytes.NewBufferString("s3cret\n")
	var output bytes.Buffer
	value, err := newTestPrompter(input, &output).Password("Paste your API key", nil)
	if err != nil {
		t.Fatal(err)
	}
	if value != "s3cret" {
		t.Fatalf("value %q", value)
	}
}

func TestReadLineReturnsCancelledOnContext(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	var output bytes.Buffer
	prompter := New(ctx, reader, &output)
	cancel()
	if _, err := prompter.Input("Model", "", nil); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err %v", err)
	}
}
