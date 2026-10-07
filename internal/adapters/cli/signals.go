package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/wahidyankf/hippo/internal/status"
)

// SignalContext is the one place a process entry turns SIGINT and SIGTERM into
// cancellation. The cancellation carries the signal itself as an Interruption,
// so the command can finish cleanly and still end with the 128+N a shell
// reports for it. Call stop once the command has returned.
func SignalContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case received := <-signals:
			if number, ok := received.(syscall.Signal); ok {
				cancel(status.Interruption{Signal: int(number), Name: number.String()})
			}
		case <-ctx.Done():
		}
	}()

	return ctx, func() {
		signal.Stop(signals)
		cancel(nil)
	}
}
