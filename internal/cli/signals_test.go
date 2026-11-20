package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestFinishMapsContextCancellation(t *testing.T) {
	var output bytes.Buffer
	if code := finish(&output, context.Canceled); code != 130 {
		t.Fatalf("exit %d", code)
	}
	if output.String() != "Interrupted.\n" {
		t.Fatalf("output %q", output.String())
	}
}

func TestRunDoctorCanceledContext(t *testing.T) {
	setConfigHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output, errorOutput bytes.Buffer
	code := RunContext(ctx, []string{"doctor"}, strings.NewReader(""), &output, &errorOutput)
	if code != 130 {
		t.Fatalf("exit %d: %q", code, output.String())
	}
	if output.String() != "Interrupted.\n" {
		t.Fatalf("output %q", output.String())
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("stderr %q", errorOutput.String())
	}
}
