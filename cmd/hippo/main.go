package main

import (
	"context"
	"os"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/bootstrap"
)

// run is the one place operating-system signals become cancellation. The
// cancellation carries the signal itself, so the application can end an
// interrupted invocation with the 128+N a shell reports for it after a queued
// waiter has left its receipt or a guard has stopped its child.
func run() int {
	ctx, stop := cli.SignalContext(context.Background())
	defer stop()

	return bootstrap.Execute(ctx, os.Args[1:])
}

func main() {
	os.Exit(run())
}
