//go:build windows

package tty

import "syscall"

const enableEchoInput = 0x0004

var setConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")

func isTerminal(fd uintptr) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(fd), &mode) == nil
}

func disableEcho(fd uintptr) (func() error, error) {
	var original uint32
	if err := syscall.GetConsoleMode(syscall.Handle(fd), &original); err != nil {
		return nil, err
	}
	if err := setConsoleModeValue(fd, original&^enableEchoInput); err != nil {
		return nil, err
	}
	return func() error { return setConsoleModeValue(fd, original) }, nil
}

func setConsoleModeValue(fd uintptr, mode uint32) error {
	result, _, err := setConsoleMode.Call(fd, uintptr(mode))
	if result == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
