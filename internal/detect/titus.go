package detect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"

	titus "github.com/praetorian-inc/titus"
	"github.com/praetorian-inc/titus/pkg/rule"
)

var _ Engine = (*Titus)(nil)
var _ ContentDetector = (*Titus)(nil)

// titusParallelThreshold mirrors the Titus portable matcher: content at or
// above this size is matched by a worker per CPU, below it on one goroutine.
const titusParallelThreshold = 10000

// titusExclusiveThreshold is the size from which a scan gets every CPU slot.
// Between the two thresholds a scan takes a quarter of the slots, so up to
// four mid-sized files overlap: Titus spreads one file's rules across all
// CPUs, but a few slow rules leave most of them idle near the end of each
// file. The overlap at most quadruples per-rule latency, which for content
// under this size stays milliseconds against Titus's 5s per-match timeout.
const titusExclusiveThreshold = 256 << 10

// Titus is an Engine that detects sensitive content with the Titus scanner.
//
// The scanner is safe for concurrent use. Scans are admitted through a
// CPU-weighted semaphore (see scanWeight) so concurrent scans keep the CPUs
// busy without starving any regex toward Titus's per-match timeout.
type Titus struct {
	mu      sync.RWMutex
	scanner *titus.Scanner
	closed  bool
	cpu     *cpuSlots
	slots   int
}

// cpuSlots is a weighted semaphore over CPU slots. Acquisitions of more than
// one slot are serialized so two wide scans can never each hold part of the
// pool and wait on the other.
type cpuSlots struct {
	tokens chan struct{}
	wide   sync.Mutex
}

func newCPUSlots(n int) *cpuSlots {
	return &cpuSlots{tokens: make(chan struct{}, n)}
}

func (c *cpuSlots) acquire(ctx context.Context, n int) error {
	if n > 1 {
		c.wide.Lock()
		defer c.wide.Unlock()
	}
	for taken := 0; taken < n; taken++ {
		select {
		case c.tokens <- struct{}{}:
		case <-ctx.Done():
			c.release(taken)
			return ctx.Err()
		}
	}
	return nil
}

func (c *cpuSlots) release(n int) {
	for range n {
		<-c.tokens
	}
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
	slots := max(1, runtime.GOMAXPROCS(0))
	return &Titus{scanner: scanner, cpu: newCPUSlots(slots), slots: slots}, nil
}

// DetectPath reports whether path contains content the scanner treats as sensitive.
func (t *Titus) DetectPath(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if t.scanner == nil {
		return false, fmt.Errorf("detect: titus scanner is not initialized")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading file: %w", err)
	}
	return t.DetectContent(ctx, path, data)
}

// DetectContent reports whether content contains anything the scanner treats as sensitive.
func (t *Titus) DetectContent(ctx context.Context, _ string, content []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if t.scanner == nil {
		return false, fmt.Errorf("detect: titus scanner is not initialized")
	}
	weight := scanWeight(len(content), t.slots)
	if err := t.cpu.acquire(ctx, weight); err != nil {
		return false, err
	}
	defer t.cpu.release(weight)
	t.mu.RLock()
	defer t.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if t.closed {
		return false, errors.New("detect: engine closed")
	}
	findings, err := t.scanner.ScanBytesWithFindings(ctx, content)
	if err != nil {
		return false, err
	}
	return len(findings) > 0, nil
}

func scanWeight(size, slots int) int {
	switch {
	case size < titusParallelThreshold:
		return 1
	case size < titusExclusiveThreshold:
		return max(1, (slots+3)/4)
	default:
		return slots
	}
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
