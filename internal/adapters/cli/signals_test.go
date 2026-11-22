package cli //nolint:testpackage // SignalContext is bound through the system signal port it takes.

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/status"
)

// notAnOperatingSystemSignal is an os.Signal that is not a syscall.Signal. The entry must not read it as an
// interruption.
type notAnOperatingSystemSignal struct{}

func (notAnOperatingSystemSignal) String() string { return "not-an-operating-system-signal" }
func (notAnOperatingSystemSignal) Signal()        {}

func TestSignalContextCancelsWithTheSignalThatArrives(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	ctx, stop := SignalContext(context.Background(), host.Port().Signals)
	defer stop()

	host.Signals.Deliver(syscall.SIGTERM)
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("a delivered SIGTERM did not cancel the context")
	}

	interruption, interrupted := status.Interrupted(ctx)
	if !interrupted || interruption.Signal != int(syscall.SIGTERM) || interruption.Name != syscall.SIGTERM.String() {
		t.Fatalf("cause = %+v interrupted=%v, want the SIGTERM interruption", interruption, interrupted)
	}
	if got, want := interruption.Status(), 128+int(syscall.SIGTERM); got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestSignalContextStopUnregistersItsChannelAndCancelsWithoutAnInterruption(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	ctx, stop := SignalContext(context.Background(), host.Port().Signals)

	stop()

	if host.Signals.Stopped != 1 {
		t.Fatalf("signal registrations stopped = %d, want 1", host.Signals.Stopped)
	}
	if !errors.Is(context.Cause(ctx), context.Canceled) {
		t.Fatalf("cause after stop = %v, want context.Canceled", context.Cause(ctx))
	}
	if _, interrupted := status.Interrupted(ctx); interrupted {
		t.Fatal("stopping the entry reported a signal interruption")
	}
}

func TestSignalContextIgnoresASignalThatIsNotAnOperatingSystemSignal(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	ctx, stop := SignalContext(context.Background(), host.Port().Signals)

	// The channel has room, so the delivery does not block whichever side of the select takes the value.
	host.Signals.Deliver(notAnOperatingSystemSignal{})
	stop()

	if !errors.Is(context.Cause(ctx), context.Canceled) {
		t.Fatalf("cause = %v, want only stop's context.Canceled", context.Cause(ctx))
	}
	if _, interrupted := status.Interrupted(ctx); interrupted {
		t.Fatal("a signal that is not an operating-system signal became an interruption")
	}
}
