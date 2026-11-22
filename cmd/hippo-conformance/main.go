// Command hippo-conformance executes a strict runtime-supplied consumer manifest.
package main

import (
	"context"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/bootstrap"
)

// process is the host the entry point runs against. A test replaces it with an in-memory host.
var process = system.Production

func main() {
	process.Environment.Exit(run(process))
}

func run(host system.System) int {
	ctx, stop := cli.SignalContext(context.Background(), host.Signals)
	defer stop()

	return cli.ConformanceMain(
		ctx,
		host.Environment.Args[1:],
		host.Environment.Stdout,
		host.Environment.Stderr,
		bootstrap.ConformanceService(host.Clock),
	)
}
