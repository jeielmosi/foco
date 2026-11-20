package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"foco/internal/config"
	"foco/internal/enums"
)

type Question struct {
	ID           string
	Instructions string
}

type Request struct {
	State     any
	Questions []Question
}

type EvalOptions struct {
	Navigation bool
}

type Diagnostic struct {
	StatusCode int
	Message    string
}

type Failure struct {
	Kind          enums.ProviderFailureKind
	SplitEligible bool
	Diagnostic    *Diagnostic
	Message       string
}

func (f *Failure) Error() string {
	if f.Message != "" {
		return f.Message
	}
	return "API evaluation failed: " + f.Kind.String()
}

type Options struct {
	Concurrency  int
	RequestLimit int
	Timeout      time.Duration
	HTTPClient   *http.Client
	Now          func() time.Time
	Sleep        func(context.Context, time.Duration) error
}

type Client struct {
	profile      config.Profile
	schema       decisionSchema
	apiKey       string
	httpClient   *http.Client
	timeout      time.Duration
	requestLimit int
	now          func() time.Time
	sleep        func(context.Context, time.Duration) error
	slots        chan struct{}
	authDone     chan struct{}

	mu         sync.Mutex
	requests   int
	authFailed bool
	cooldown   time.Time
}

func New(profile config.Profile, apiKey string, options Options) *Client {
	concurrency := options.Concurrency
	if concurrency < 1 {
		concurrency = 32
	}
	requestLimit := options.RequestLimit
	if requestLimit < 1 {
		requestLimit = 50000
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	client := &Client{
		profile: profile, schema: schemaAdapterFor(profile.Schema), apiKey: apiKey,
		httpClient: httpClient, timeout: timeout,
		requestLimit: requestLimit, now: now, sleep: options.Sleep,
		slots: make(chan struct{}, concurrency), authDone: make(chan struct{}),
	}
	return client
}

func (c *Client) Requests() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests
}

func (c *Client) Evaluate(ctx context.Context, request Request, options EvalOptions) (map[string]float64, error) {
	if c.schema == nil {
		return nil, &Failure{Kind: enums.FailureProvider, Message: "Unsupported API schema."}
	}
	body, err := c.schema.marshalRequest(c.profile.Model, request)
	if err != nil {
		return nil, &Failure{Kind: enums.FailureProvider, Message: "Invalid evaluation request."}
	}
	multiple := len(request.Questions) > 1
	attemptLimit := 2
	if options.Navigation && multiple {
		attemptLimit = 1
	}
	var last *Failure
	for attempt := 0; attempt < attemptLimit; attempt++ {
		if err := c.assertActive(ctx); err != nil {
			return nil, err
		}
		if err := c.acquire(ctx); err != nil {
			if failure := c.interruption(ctx); failure != nil {
				return nil, failure
			}
			return nil, &Failure{Kind: enums.FailureCancelled, Message: "API evaluation cancelled."}
		}
		scores, failure, transient, status := c.attempt(ctx, body, request)
		c.release()
		if failure == nil {
			return scores, nil
		}
		switch failure.Kind {
		case enums.FailureAuthentication, enums.FailureRequestLimit, enums.FailureCancelled:
			return nil, failure
		}
		last = failure
		if options.Navigation && status == http.StatusTooManyRequests && attemptLimit < 2 {
			attemptLimit = 2
		}
		if (options.Navigation && !transient) || attempt+1 == attemptLimit {
			failure.SplitEligible = options.Navigation && multiple && transient && status != http.StatusTooManyRequests
			return nil, failure
		}
	}
	if last != nil {
		return nil, last
	}
	return nil, &Failure{Kind: enums.FailureProvider, Message: "API evaluation failed."}
}

func (c *Client) attempt(ctx context.Context, body []byte, request Request) (map[string]float64, *Failure, bool, int) {
	if failure := c.waitForCapacity(ctx); failure != nil {
		return nil, failure, false, 0
	}
	if !c.takeRequest() {
		return nil, &Failure{Kind: enums.FailureRequestLimit, Message: "The request limit for this search has been reached."}, false, 0
	}
	requestContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPost, c.profile.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &Failure{Kind: enums.FailureProvider, Message: "Invalid API endpoint."}, false, 0
	}
	c.schema.setAuthentication(httpRequest, c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		if failure := c.interruption(ctx); failure != nil {
			return nil, failure, false, 0
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &Failure{
				Kind:    enums.FailureProvider,
				Message: fmt.Sprintf("Request timed out after %d ms (max concurrent requests: %d)", c.timeout.Milliseconds(), cap(c.slots)),
			}, true, 0
		}
		return nil, &Failure{
			Kind:    enums.FailureProvider,
			Message: fmt.Sprintf("Network request failed (connection unavailable or reset) (max concurrent requests: %d)", cap(c.slots)),
		}, true, 0
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode == http.StatusTooManyRequests {
		c.noteRetryAfter(response.Header.Get("Retry-After"))
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		diagnostic := buildDiagnostic(response.StatusCode, payload, c.apiKey)
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			c.markAuthenticationFailed()
			return nil, &Failure{
				Kind: enums.FailureAuthentication, Diagnostic: diagnostic,
				Message: describe(diagnostic, cap(c.slots)),
			}, false, response.StatusCode
		}
		transient := response.StatusCode == http.StatusRequestTimeout ||
			response.StatusCode == http.StatusTooManyRequests ||
			(response.StatusCode >= 500 && response.StatusCode <= 599)
		return nil, &Failure{
			Kind: enums.FailureProvider, Diagnostic: diagnostic,
			Message: describe(diagnostic, cap(c.slots)),
		}, transient, response.StatusCode
	}
	scores, failure := c.schema.decodeResponse(payload, request)
	if failure != nil {
		return nil, failure, false, response.StatusCode
	}
	return scores, nil, false, response.StatusCode
}

func (c *Client) waitForCapacity(ctx context.Context) *Failure {
	for {
		if failure := c.assertActive(ctx); failure != nil {
			return failure
		}
		wait := c.cooldownRemaining()
		if wait > 0 {
			if wait > 60*time.Second {
				wait = 60 * time.Second
			}
			if err := c.waitSleep(ctx, wait); err != nil {
				if failure := c.interruption(ctx); failure != nil {
					return failure
				}
				return &Failure{Kind: enums.FailureCancelled, Message: "API evaluation cancelled."}
			}
			continue
		}
		if failure := c.assertActive(ctx); failure != nil {
			return failure
		}
		if c.cooldownRemaining() > 0 {
			continue
		}
		return nil
	}
}

func (c *Client) acquire(ctx context.Context) error {
	select {
	case c.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.authDone:
		return errors.New("authentication failed")
	}
}

func (c *Client) waitSleep(ctx context.Context, wait time.Duration) error {
	if c.sleep != nil {
		return c.sleep(ctx, wait)
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.authDone:
		return errors.New("authentication failed")
	}
}

func (c *Client) release() { <-c.slots }

func (c *Client) takeRequest() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.requests >= c.requestLimit {
		return false
	}
	c.requests++
	return true
}

func (c *Client) markAuthenticationFailed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.authFailed {
		return
	}
	c.authFailed = true
	close(c.authDone)
}

func (c *Client) assertActive(ctx context.Context) *Failure {
	if ctx.Err() != nil {
		return &Failure{Kind: enums.FailureCancelled, Message: "API evaluation cancelled."}
	}
	c.mu.Lock()
	failed := c.authFailed
	c.mu.Unlock()
	if failed {
		return &Failure{Kind: enums.FailureAuthentication, Message: "API authentication failed."}
	}
	return nil
}

func (c *Client) interruption(ctx context.Context) *Failure {
	return c.assertActive(ctx)
}

func (c *Client) cooldownRemaining() time.Duration {
	c.mu.Lock()
	until := c.cooldown
	c.mu.Unlock()
	wait := until.Sub(c.now())
	if wait < 0 {
		return 0
	}
	return wait
}

func (c *Client) noteRetryAfter(value string) {
	wait := time.Second
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds >= 0 {
		wait = time.Duration(seconds * float64(time.Second))
	} else if when, err := http.ParseTime(value); err == nil {
		wait = when.Sub(c.now())
		if wait < 0 {
			wait = 0
		}
	}
	until := c.now().Add(wait)
	c.mu.Lock()
	if until.After(c.cooldown) {
		c.cooldown = until
	}
	c.mu.Unlock()
}

func invalidResponse() *Failure {
	return &Failure{Kind: enums.FailureProvider, Message: "Invalid or incomplete API response."}
}

var ansiPattern = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\)|[@-Z\\-_])`)

func buildDiagnostic(status int, payload []byte, apiKey string) *Diagnostic {
	diagnostic := &Diagnostic{StatusCode: status}
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return diagnostic
	}
	message, found := lookupMessage(data)
	if !found {
		return diagnostic
	}
	safe := sanitize(message, apiKey)
	diagnostic.Message = safe
	return diagnostic
}

func lookupMessage(data map[string]any) (string, bool) {
	if value, ok := data["message"].(string); ok {
		return value, true
	}
	if nested, ok := data["error"]; ok {
		if value, ok := nested.(string); ok {
			return value, true
		}
		if object, ok := nested.(map[string]any); ok {
			if value, ok := object["message"].(string); ok {
				return value, true
			}
		}
	}
	if value, ok := data["detail"].(string); ok {
		return value, true
	}
	return "", false
}

func sanitize(message, apiKey string) string {
	safe := ansiPattern.ReplaceAllString(message, "")
	var builder strings.Builder
	previousControl := false
	for _, r := range safe {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			if !previousControl {
				builder.WriteByte(' ')
			}
			previousControl = true
			continue
		}
		previousControl = false
		builder.WriteRune(r)
	}
	redacted := strings.TrimSpace(builder.String())
	if apiKey != "" {
		redacted = strings.ReplaceAll(redacted, apiKey, "[redacted]")
		withoutSpace := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, redacted)
		if strings.Contains(withoutSpace, apiKey) {
			return ""
		}
	}
	runes := []rune(redacted)
	if len(runes) > 500 {
		runes = runes[:500]
	}
	return strings.TrimSpace(string(runes))
}

func describe(diagnostic *Diagnostic, concurrency int) string {
	description := "Invalid or incomplete API response"
	if diagnostic != nil {
		description = fmt.Sprintf("HTTP %d", diagnostic.StatusCode)
		if diagnostic.Message != "" {
			description += ": " + diagnostic.Message
		}
	}
	return fmt.Sprintf("%s (max concurrent requests: %d)", description, concurrency)
}
