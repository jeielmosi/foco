//go:build unix

package files

import (
	"os"
	"syscall"
)

func openReadonly(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
