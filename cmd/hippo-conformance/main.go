// Command hippo-conformance executes a strict runtime-supplied consumer manifest.
package main

import (
	"context"
	"os"

	"github.com/wahidyankf/hippo/internal/conformance"
	"github.com/wahidyankf/hippo/internal/status"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := status.SignalContext(context.Background())
	defer stop()

	return conformance.Main(ctx, os.Args[1:], os.Stdout, os.Stderr)
}
