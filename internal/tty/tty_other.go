//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package tty

import "errors"

func isTerminal(uintptr) bool { return false }

func disableEcho(uintptr) (func() error, error) {
	return nil, errors.New("terminal password input is unsupported on this platform")
}
