package cli

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestOutputWriterRecordsNonPipeFailure(t *testing.T) {
	failure := errors.New("disk full")
	output := newOutputWriter(failingWriter{err: failure})
	if _, err := output.Write([]byte("hello")); !errors.Is(err, failure) {
		t.Fatalf("Write error = %v", err)
	}
	if output.Closed() {
		t.Fatal("non-pipe failure must not mark the pipe closed")
	}
	if !errors.Is(output.Failure(), failure) {
		t.Fatalf("failure = %v", output.Failure())
	}
}

func TestOutputWriterForwardsFileDescriptor(t *testing.T) {
	output := newOutputWriter(os.Stdout)
	if output.Fd() != os.Stdout.Fd() {
		t.Fatalf("Fd = %d, want %d", output.Fd(), os.Stdout.Fd())
	}
	buffer := newOutputWriter(&bytes.Buffer{})
	if buffer.Fd() == os.Stdout.Fd() {
		t.Fatal("buffer must not report stdout's descriptor")
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
