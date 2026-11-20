package tty

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestIsTerminalForBuffer(t *testing.T) {
	if IsTerminal(&bytes.Buffer{}) {
		t.Fatal("bytes.Buffer must not be a terminal")
	}
}

func TestReadPasswordRejectsNonTerminal(t *testing.T) {
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if _, err := ReadPassword(file.Fd(), bufio.NewReader(strings.NewReader("secret\n"))); err == nil {
		t.Fatal("ReadPassword accepted a non-terminal descriptor")
	}
}
