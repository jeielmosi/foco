package detect

import (
	"context"
	"errors"
	"fmt"
	"sync"

	titus "github.com/praetorian-inc/titus"
	"github.com/praetorian-inc/titus/pkg/rule"
)

var _ Engine = (*Titus)(nil)

// Titus is an Engine that detects sensitive content with the Titus scanner.
type Titus struct {
	mu      sync.Mutex
	scanner *titus.Scanner
	closed  bool
}

// NewTitus returns a Titus engine backed by a new Titus scanner.
// Noisy rules are excluded, matching the default behaviour of `titus scan`.
func NewTitus() (*Titus, error) {
	rules, err := rule.NewLoader().LoadBuiltinRules()
	if err != nil {
		return nil, fmt.Errorf("detect: loading titus rules: %w", err)
	}
	scanner, err := titus.NewScanner(titus.WithRules(rule.FilterNoisy(rules, false)))
	if err != nil {
		return nil, fmt.Errorf("detect: creating titus scanner: %w", err)
	}
	return &Titus{scanner: scanner}, nil
}

// DetectPath reports whether path contains content the scanner treats as sensitive.
func (t *Titus) DetectPath(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if t.scanner == nil {
		return false, fmt.Errorf("detect: titus scanner is not initialized")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if t.closed {
		return false, errors.New("detect: engine closed")
	}
	findings, err := t.scanner.ScanFileWithFindings(ctx, path)
	if err != nil {
		return false, err
	}
	return len(findings) > 0, nil
}

// Close releases the underlying scanner. Additional calls are no-ops.
func (t *Titus) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.scanner == nil {
		return nil
	}
	t.closed = true
	return t.scanner.Close()
}
