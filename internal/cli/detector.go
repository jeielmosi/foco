package cli

import (
	"runtime"

	"foco/internal/detect"
	"foco/internal/detect/catalog"
)

func newDetector() (detect.Engine, error) {
	catalogValue, err := catalog.Load()
	if err != nil {
		return nil, err
	}
	titus, err := detect.NewTitus()
	if err != nil {
		return nil, err
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > 8 {
		workers = 8
	}
	better, err := detect.NewBetterleaks(workers)
	if err != nil {
		_ = titus.Close()
		return nil, err
	}
	return detect.NewUnion(titus, better, catalogValue), nil
}
