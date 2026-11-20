package detect

import "context"

type Detector interface {
	DetectPath(ctx context.Context, path string) (excluded bool, err error)
}

type Engine interface {
	Detector
	Close() error
}
