//go:build windows

package cli

import (
	"syscall"
	"testing"
)

func TestBrokenPipeWindowsErrors(t *testing.T) {
	if !brokenPipe(syscall.ERROR_BROKEN_PIPE) {
		t.Fatal("ERROR_BROKEN_PIPE not detected")
	}
	if !brokenPipe(syscall.Errno(232)) {
		t.Fatal("ERROR_NO_DATA not detected")
	}
	if brokenPipe(nil) {
		t.Fatal("nil detected as broken pipe")
	}
}
