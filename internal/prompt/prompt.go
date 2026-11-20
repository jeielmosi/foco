package prompt

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"foco/internal/tty"
)

type Option struct {
	Value string
	Label string
}

type Prompter interface {
	Select(message string, options []Option, initial int) (string, error)
	Input(message, placeholder string, validate func(string) error) (string, error)
	Password(message string, validate func(string) error) (string, error)
}

var ErrCancelled = errors.New("prompt cancelled")

type terminal struct {
	ctx    context.Context
	in     io.Reader
	out    io.Writer
	reader *bufio.Reader
}

func New(ctx context.Context, in io.Reader, out io.Writer) Prompter {
	if ctx == nil {
		ctx = context.Background()
	}
	return &terminal{ctx: ctx, in: in, out: out, reader: bufio.NewReader(in)}
}

func (t *terminal) Select(message string, options []Option, initial int) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no options to choose from")
	}
	if initial < 0 || initial >= len(options) {
		initial = 0
	}
	fmt.Fprintln(t.out, message)
	for index, option := range options {
		fmt.Fprintf(t.out, "  %d) %s\n", index+1, option.Label)
	}
	for {
		fmt.Fprintf(t.out, "Enter a number [%d]: ", initial+1)
		line, err := t.readLine()
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return options[initial].Value, nil
		}
		choice, err := strconv.Atoi(line)
		if err == nil && choice >= 1 && choice <= len(options) {
			return options[choice-1].Value, nil
		}
		fmt.Fprintf(t.out, "Invalid choice. Enter a number from 1 to %d.\n", len(options))
	}
}

func (t *terminal) Input(message, placeholder string, validate func(string) error) (string, error) {
	for {
		if placeholder != "" {
			fmt.Fprintf(t.out, "%s (e.g. %s): ", message, placeholder)
		} else {
			fmt.Fprintf(t.out, "%s: ", message)
		}
		line, err := t.readLine()
		if err != nil {
			return "", err
		}
		if validate != nil {
			if err := validate(line); err != nil {
				fmt.Fprintln(t.out, err.Error())
				continue
			}
		}
		return line, nil
	}
}

func (t *terminal) Password(message string, validate func(string) error) (string, error) {
	for {
		fmt.Fprintf(t.out, "%s: ", message)
		line, echoed, err := t.readSecret()
		if err != nil {
			return "", err
		}
		if !echoed {
			fmt.Fprintln(t.out)
		}
		if validate != nil {
			if err := validate(line); err != nil {
				fmt.Fprintln(t.out, err.Error())
				continue
			}
		}
		return line, nil
	}
}

func (t *terminal) readSecret() (string, bool, error) {
	type descriptor interface {
		Fd() uintptr
	}
	if file, ok := t.in.(descriptor); ok && t.reader.Buffered() == 0 && tty.IsTerminal(t.in) {
		data, err := tty.ReadPassword(file.Fd(), t.reader)
		if err != nil {
			return "", true, err
		}
		return string(data), false, nil
	}
	line, err := t.readLine()
	return line, true, err
}

func (t *terminal) readLine() (string, error) {
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := t.reader.ReadString('\n')
		done <- result{line: line, err: err}
	}()
	select {
	case <-t.ctx.Done():
		if closer, ok := t.in.(io.Closer); ok {
			_ = closer.Close()
		}
		return "", ErrCancelled
	case outcome := <-done:
		if t.ctx.Err() != nil {
			return "", ErrCancelled
		}
		if outcome.err != nil {
			if errors.Is(outcome.err, io.EOF) && outcome.line != "" {
				return strings.TrimRight(outcome.line, "\r\n"), nil
			}
			if errors.Is(outcome.err, io.EOF) {
				return "", ErrCancelled
			}
			return "", outcome.err
		}
		return strings.TrimRight(outcome.line, "\r\n"), nil
	}
}
