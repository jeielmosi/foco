package doctor

import (
	"context"
	"errors"
	"fmt"

	"foco/internal/enums"
	"foco/internal/provider"
)

const doctorSource = "export function recordEvent(event) { events.push(event); }"

func Verify(ctx context.Context, client *provider.Client, label string) error {
	answers, err := client.Evaluate(ctx, provider.Request{
		State: map[string]string{"source": doctorSource},
		Questions: []provider.Question{{
			ID:           "relevant",
			Instructions: "Does this source implement recording an event?",
		}},
	}, provider.EvalOptions{})
	if err != nil {
		var failure *provider.Failure
		if errors.As(err, &failure) {
			if failure.Kind == enums.FailureAuthentication || failure.Diagnostic != nil {
				return diagnosticError(label, failure)
			}
		}
		return fmt.Errorf("API connection check failed through %s. Check the endpoint, key, model, and network.", label)
	}
	if answers["relevant"] <= 0.5 {
		return fmt.Errorf("%s returned an unexpected answer to the connection check.", label)
	}
	return nil
}

func diagnosticError(label string, failure *provider.Failure) error {
	if failure.Diagnostic == nil {
		return fmt.Errorf("API connection check failed through %s. Check the endpoint, key, model, and network.", label)
	}
	detail := "Check the endpoint, key, model access, and account billing."
	if failure.Diagnostic.Message != "" {
		detail = "API: " + failure.Diagnostic.Message
	}
	return fmt.Errorf("API connection check failed through %s (HTTP %d).\n%s",
		label, failure.Diagnostic.StatusCode, detail)
}
