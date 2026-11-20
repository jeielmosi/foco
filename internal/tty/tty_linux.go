//go:build linux

package tty

import (
	"runtime"
	"syscall"
	"unsafe"
)

func isTerminal(fd uintptr) bool {
	var state syscall.Termios
	return ioctlTermios(fd, syscall.TCGETS, &state) == nil
}

func disableEcho(fd uintptr) (func() error, error) {
	var original syscall.Termios
	if err := ioctlTermios(fd, syscall.TCGETS, &original); err != nil {
		return nil, err
	}

	hidden := original
	hidden.Lflag &^= syscall.ECHO | syscall.ECHONL
	if err := ioctlTermios(fd, syscall.TCSETS, &hidden); err != nil {
		return nil, err
	}
	return func() error { return ioctlTermios(fd, syscall.TCSETS, &original) }, nil
}

func ioctlTermios(fd, request uintptr, state *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(unsafe.Pointer(state)))
	runtime.KeepAlive(state)
	if errno != 0 {
		return errno
	}
	return nil
}
