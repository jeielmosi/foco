package detect

import "context"

type Detector interface {
	DetectPath(ctx context.Context, path string) (excluded bool, err error)
}

// ContentDetector scans bytes the caller already read from path, so the
// verdict covers exactly the content the caller will use and the file is not
// read a second time. path still drives path-based rules.
type ContentDetector interface {
	DetectContent(ctx context.Context, path string, content []byte) (excluded bool, err error)
}

type Engine interface {
	Detector
	Close() error
}
