//go:build windows

package cli

import (
	"errors"
	"syscall"
)

// errorNoData is ERROR_NO_DATA, which Windows reports when a console pipe
// handle has been closed. The syscall package defines ERROR_BROKEN_PIPE but
// not this one.
const errorNoData syscall.Errno = 232

func brokenPipe(err error) bool {
	return errors.Is(err, syscall.ERROR_BROKEN_PIPE) || errors.Is(err, errorNoData)
}
