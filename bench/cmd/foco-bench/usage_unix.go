//go:build unix

package main

import (
	"os"
	"runtime"
	"syscall"
)

// usage returns a finished child's user CPU in milliseconds and its peak RSS
// in KB (getrusage reports bytes on macOS and KB elsewhere).
func usage(state *os.ProcessState) (float64, int64) {
	rusage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok {
		return float64(state.UserTime().Microseconds()) / 1000, 0
	}
	rss := int64(rusage.Maxrss)
	if runtime.GOOS == "darwin" {
		rss /= 1024
	}
	return float64(state.UserTime().Microseconds()) / 1000, rss
}
