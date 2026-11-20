//go:build linux

package tty

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestReadPasswordDoesNotEchoAndRestoresTerminal(t *testing.T) {
	masterFD, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("open pseudo-terminal: %v", err)
	}
	master := os.NewFile(uintptr(masterFD), "/dev/ptmx")
	t.Cleanup(func() { _ = master.Close() })

	unlocked := int32(0)
	if err := ttyTestIoctl(uintptr(masterFD), syscall.TIOCSPTLCK, unsafe.Pointer(&unlocked)); err != nil {
		t.Skipf("unlock pseudo-terminal: %v", err)
	}
	var number uint32
	if err := ttyTestIoctl(uintptr(masterFD), syscall.TIOCGPTN, unsafe.Pointer(&number)); err != nil {
		t.Skipf("get pseudo-terminal number: %v", err)
	}

	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("open pseudo-terminal slave: %v", err)
	}
	t.Cleanup(func() { _ = slave.Close() })

	var before syscall.Termios
	if err := ttyTestIoctl(slave.Fd(), syscall.TCGETS, unsafe.Pointer(&before)); err != nil {
		t.Fatal(err)
	}
	before.Lflag |= syscall.ECHO
	before.Lflag &^= syscall.ECHONL
	if err := ttyTestIoctl(slave.Fd(), syscall.TCSETS, unsafe.Pointer(&before)); err != nil {
		t.Fatal(err)
	}

	type result struct {
		password []byte
		err      error
	}
	read := make(chan result, 1)
	go func() {
		password, err := ReadPassword(slave.Fd(), bufio.NewReader(slave))
		read <- result{password: password, err: err}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		var state syscall.Termios
		if err := ttyTestIoctl(slave.Fd(), syscall.TCGETS, unsafe.Pointer(&state)); err != nil {
			t.Fatal(err)
		}
		if state.Lflag&syscall.ECHO == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("password reader did not disable terminal echo")
		}
		time.Sleep(time.Millisecond)
	}

	if _, err := master.Write([]byte("secret\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-read:
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if string(outcome.password) != "secret" {
			t.Fatalf("password = %q, want secret", outcome.password)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("password read did not finish")
	}

	if err := syscall.SetNonblock(masterFD, true); err != nil {
		t.Fatal(err)
	}
	var echoed [32]byte
	if n, err := master.Read(echoed[:]); n > 0 || (err != nil && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK)) {
		t.Fatalf("master read = (%d, %v), expected no echoed input", n, err)
	}

	var after syscall.Termios
	if err := ttyTestIoctl(slave.Fd(), syscall.TCGETS, unsafe.Pointer(&after)); err != nil {
		t.Fatal(err)
	}
	if after.Lflag&syscall.ECHO == 0 {
		t.Fatal("terminal echo was not restored")
	}
}

func ttyTestIoctl(fd uintptr, request uintptr, value unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(value))
	if errno != 0 {
		return errno
	}
	return nil
}
