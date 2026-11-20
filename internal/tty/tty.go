package tty

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

type descriptor interface {
	Fd() uintptr
}

// IsTerminal reports whether stream is backed by a terminal file descriptor.
// It accepts any value so callers can pass io.Reader, io.Writer or *os.File.
func IsTerminal(stream any) bool {
	file, ok := stream.(descriptor)
	return ok && isTerminal(file.Fd())
}

// ReadPassword reads one line with terminal echo disabled until it returns.
func ReadPassword(fd uintptr, reader *bufio.Reader) (password []byte, err error) {
	restore, err := disableEcho(fd)
	if err != nil {
		return nil, err
	}
	defer func() {
		if restoreErr := restore(); err == nil && restoreErr != nil {
			err = restoreErr
			password = nil
		}
	}()

	line, err := reader.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || line == "") {
		return nil, err
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return []byte(line), nil
}
