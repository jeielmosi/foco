//go:build !unix

package main

import "os"

func usage(state *os.ProcessState) (float64, int64) {
	return float64(state.UserTime().Microseconds()) / 1000, 0
}
