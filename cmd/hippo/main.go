package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/wahidyankf/hippo/internal/cli"
)

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return cli.Execute(ctx, os.Args[1:])
}

func main() {
	os.Exit(run())
}
