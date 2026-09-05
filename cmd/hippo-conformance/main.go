// Command hippo-conformance executes a strict runtime-supplied consumer manifest.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/wahidyankf/hippo/internal/conformance"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) != 2 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: hippo-conformance <manifest.json>")

		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := conformance.Run(ctx, os.Args[1], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		return 1
	}

	return 0
}
