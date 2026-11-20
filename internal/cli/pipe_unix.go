//go:build !windows

package cli

import (
	"errors"
	"syscall"
)

func brokenPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}
