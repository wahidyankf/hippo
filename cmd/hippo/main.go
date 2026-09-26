package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/status"
)

// run is the one place operating-system signals become cancellation. The
// cancellation carries the signal itself, so the application can end an
// interrupted invocation with the 128+N a shell reports for it after a queued
// waiter has left its receipt or a guard has stopped its child.
func run() int {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	go func() {
		select {
		case received := <-signals:
			if number, ok := received.(syscall.Signal); ok {
				cancel(status.Interruption{Signal: number})
			}
		case <-ctx.Done():
		}
	}()

	return cli.Execute(ctx, os.Args[1:])
}

func main() {
	os.Exit(run())
}
