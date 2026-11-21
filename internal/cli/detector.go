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
	workers := runtime.GOMAXPROCS(0)
	if workers > 8 {
		workers = 8
	}
	// Rule compilation for the two engines is independent; overlap it.
	type built struct {
		engine *detect.Betterleaks
		err    error
	}
	pending := make(chan built, 1)
	go func() {
		engine, err := detect.NewBetterleaks(workers)
		pending <- built{engine, err}
	}()
	titus, titusErr := detect.NewTitus()
	better := <-pending
	if titusErr != nil {
		return nil, titusErr
	}
	if better.err != nil {
		_ = titus.Close()
		return nil, better.err
	}
	return detect.NewUnion(titus, better.engine, catalogValue), nil
}
