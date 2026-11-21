package detect

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/betterleaks/betterleaks/config"
	"github.com/betterleaks/betterleaks/detect"
	"github.com/betterleaks/betterleaks/sources"
)

// Betterleaks scans with a pool of betterleaks detectors. Building a detector
// compiles its rules (~10ms each) and mutates the shared config, so the pool
// is built sequentially in the background, before any scan can use it; the
// search meanwhile starts listing and Titus-scanning files.
type Betterleaks struct {
	pool  chan *detect.Detector
	ready chan struct{}
}

func NewBetterleaks(workers int) (*Betterleaks, error) {
	if workers < 1 {
		workers = 1
	}
	cfg, err := config.Default()
	if err != nil {
		return nil, err
	}
	b := &Betterleaks{pool: make(chan *detect.Detector, workers), ready: make(chan struct{})}
	go func() {
		defer close(b.ready)
		for i := 0; i < workers; i++ {
			detector := detect.NewDetectorContext(context.Background(), cfg, detect.ValidationOptions{})
			detector.SkipFindingAppend = true
			b.pool <- detector
		}
	}()
	return b, nil
}

func (b *Betterleaks) acquire(ctx context.Context) (*detect.Detector, error) {
	select {
	case <-b.ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case detector := <-b.pool:
		return detector, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *Betterleaks) DetectPath(ctx context.Context, path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	return b.scan(ctx, path, file)
}

func (b *Betterleaks) DetectContent(ctx context.Context, path string, content []byte) (bool, error) {
	return b.scan(ctx, path, bytes.NewReader(content))
}

func (b *Betterleaks) scan(ctx context.Context, path string, content io.Reader) (bool, error) {
	detector, err := b.acquire(ctx)
	if err != nil {
		return false, err
	}
	defer func() { b.pool <- detector }()

	var found bool
	var firstErr error
	for result := range detector.Run(ctx, &sources.File{Path: path, Content: content}) {
		if result.Err != nil {
			if firstErr == nil {
				firstErr = result.Err
			}
			continue
		}
		if result.Finding.RuleID != "" {
			found = true
		}
	}
	return found, firstErr
}

func (b *Betterleaks) Close() error { return nil }

var _ Engine = (*Betterleaks)(nil)
var _ ContentDetector = (*Betterleaks)(nil)
