package cli

import (
	"io"
	"os"
	"sync"
)

type outputWriter struct {
	mu      sync.Mutex
	writer  io.Writer
	closed  bool
	failure error
	onClose func()
}

func newOutputWriter(writer io.Writer) *outputWriter {
	return &outputWriter{writer: writer}
}

func (w *outputWriter) setOnClose(callback func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onClose = callback
}

func (w *outputWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return len(data), nil
	}
	written, err := w.writer.Write(data)
	if err != nil {
		if brokenPipe(err) {
			w.closed = true
			if w.onClose != nil {
				w.onClose()
			}
			return len(data), nil
		}
		w.failure = err
	}
	return written, err
}

func (w *outputWriter) Closed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed
}

func (w *outputWriter) Failure() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.failure
}

// Fd forwards the underlying descriptor so terminal detection still works.
// The sentinel value is not a valid descriptor, so buffers and other non-file
// writers are not reported as terminals.
func (w *outputWriter) Fd() uintptr {
	if file, ok := w.writer.(interface{ Fd() uintptr }); ok {
		return file.Fd()
	}
	return ^uintptr(0)
}

// File returns the underlying terminal file when stdout is an *os.File.
// It returns nil for buffers and other injected writers so callers can fall
// back to the wrapped writer.
func (w *outputWriter) File() *os.File {
	file, ok := w.writer.(*os.File)
	if !ok {
		return nil
	}
	return file
}

func pipeClosed(writer io.Writer) bool {
	closed, ok := writer.(interface{ Closed() bool })
	return ok && closed.Closed()
}
