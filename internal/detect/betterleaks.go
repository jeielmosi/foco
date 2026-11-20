package detect

import (
	"context"
	"os"

	"github.com/betterleaks/betterleaks/config"
	"github.com/betterleaks/betterleaks/detect"
	"github.com/betterleaks/betterleaks/sources"
)

type Betterleaks struct {
	pool chan *detect.Detector
}

func NewBetterleaks(workers int) (*Betterleaks, error) {
	if workers < 1 {
		workers = 1
	}
	cfg, err := config.Default()
	if err != nil {
		return nil, err
	}
	pool := make(chan *detect.Detector, workers)
	for i := 0; i < workers; i++ {
		detector := detect.NewDetectorContext(context.Background(), cfg, detect.ValidationOptions{})
		detector.SkipFindingAppend = true
		pool <- detector
	}
	return &Betterleaks{pool: pool}, nil
}

func (b *Betterleaks) DetectPath(ctx context.Context, path string) (bool, error) {
	var detector *detect.Detector
	select {
	case detector = <-b.pool:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	defer func() { b.pool <- detector }()

	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	var found bool
	var firstErr error
	for result := range detector.Run(ctx, &sources.File{Path: path, Content: file}) {
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
