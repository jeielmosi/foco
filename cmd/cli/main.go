package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"foco/internal/cli"
	_ "foco/internal/detect"
)

func main() {
	ignoreSIGPIPE()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if os.Getenv("FOCO_TEST_SIGNAL_READY") == "1" {
		fmt.Fprintln(os.Stderr, "ready")
	}
	os.Exit(cli.RunContext(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
