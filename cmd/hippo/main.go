package main

import (
	"context"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/bootstrap"
)

// process is the host the entry point runs against. A test replaces it with an in-memory host.
var process = system.Production

// run is the one place operating-system signals become cancellation. The
// cancellation carries the signal itself, so the application can end an
// interrupted invocation with the 128+N a shell reports for it after a queued
// waiter has left its receipt or a guard has stopped its child.
func run(host system.System) int {
	ctx, stop := cli.SignalContext(context.Background(), host.Signals)
	defer stop()

	return bootstrap.Execute(ctx, host, host.Environment.Args[1:])
}

func main() {
	process.Environment.Exit(run(process))
}
