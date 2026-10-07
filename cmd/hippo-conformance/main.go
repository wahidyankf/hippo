// Command hippo-conformance executes a strict runtime-supplied consumer manifest.
package main

import (
	"context"
	"os"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/bootstrap"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := cli.SignalContext(context.Background())
	defer stop()

	return cli.ConformanceMain(ctx, os.Args[1:], os.Stdout, os.Stderr, bootstrap.ConformanceService())
}
