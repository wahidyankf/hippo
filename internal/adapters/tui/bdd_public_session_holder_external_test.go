package tui_test

import (
	"context"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// These are genuine selected-binary fixture controls, separate from the original
// canonical viewport examples and their unchanged public-handler RED record.
func TestTUIBDDPublicSessionHolderSignals(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.E2E {
		t.Fatal("holder signal controls require explicit E2E mode")
	}
	for _, control := range []publicHolderSignalControl{
		{name: "raw-control-c", key: []byte{3}, expected: 130},
		{name: "candidate-sigint", signal: syscall.SIGINT, expected: 130},
		{name: "candidate-sigterm", signal: syscall.SIGTERM, expected: 143},
		{name: "cooked-watch-kernel-control-c", key: []byte{3}, expected: 130, cooked: true},
	} {
		if !t.Run(control.name, func(t *testing.T) {
			runPublicHolderSignalControl(t, control)
		}) {
			return
		}
	}
	// Ordinary readonly CLI regressions share this already-routed E2E entry.
	t.Run("readonly-output", runPublicReadOnlyOutput)
}

type publicHolderSignalControl struct {
	name     string
	key      []byte
	signal   syscall.Signal
	expected int
	cooked   bool
}

func runPublicHolderSignalControl(t *testing.T, control publicHolderSignalControl) {
	t.Helper()
	adapter, err := contract.AdapterByName(contract.E2E)
	if err != nil {
		t.Fatal(err)
	}
	driver := support.NewDriver(adapter)
	driver.Reset()
	t.Cleanup(driver.Close)
	root, directory, err := driver.PreparePublicTUIRoot()
	if err != nil {
		t.Fatal(err)
	}
	probe, err := driver.NewPublicTUIHostProbe(t.Context(), directory, support.PublicTUIHostReply{CPUPercent: 20})
	if err != nil {
		t.Fatal(err)
	}
	reading, err := probe.HoldNextReading()
	if err != nil {
		t.Fatal(err)
	}
	environment := probe.Environment([]string{"HIPPO_ROOT=" + root, "TERM=xterm-ghostty", "NO_COLOR=1", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"})
	screen := tui.NewBDDNativeTransportScreen(39, 15)
	journey, stopJourney := context.WithTimeout(t.Context(), 20*time.Second)
	defer stopJourney()
	options := support.PublicTUIStart{Driver: driver, Mode: contract.E2E, Executable: os.Getenv("HIPPO_BIN"), Root: root, Directory: directory, Arguments: []string{"tui"}, Environment: environment, Width: 39, Height: 15, Feed: screen.Feed, ResizeScreen: screen.Resize, Finish: screen.Finish}
	var transport *support.PublicTUITransport
	if control.cooked {
		transport, err = support.StartPublicTUIWatchSignalControl(journey, options)
	} else {
		transport, err = support.StartPublicTUI(journey, options)
	}
	if err != nil {
		t.Fatal(err)
	}
	readingReleased := false
	t.Cleanup(func() { cleanupPublicHolderSignal(t, transport, probe, reading, &readingReleased) })
	before := requirePublicHolderSignalReady(journey, t, control, transport, probe, reading, screen)
	action, stop := context.WithTimeout(journey, 500*time.Millisecond)
	if control.signal != 0 {
		err = transport.Signal(before.Identity.PID, control.signal)
	} else {
		err = transport.SendKey(action, control.key)
	}
	stop()
	if err != nil {
		t.Fatal(err)
	}
	// Ordinary Linux FIFO os.ReadFile is not cancellable. Release its actual
	// acknowledged reading after the signal; record completion rather than a
	// fabricated Darwin cancellation/EOF event.
	if runtime.GOOS == "linux" {
		if err = probe.ReleaseReading(reading); err != nil {
			t.Fatal(err)
		}
		readingReleased = true
	}
	drain, stop := context.WithTimeout(journey, 3*time.Second)
	defer stop()
	result, trace := requirePublicHolderSignalDrained(drain, t, control, transport, probe, reading, before)
	t.Logf("actual signal control=%s candidate=%+v holder=%+v wait=%+v holderExit=%d liveAtPost=%t ack=%t before=%+v ready=%+v after=%+v collector=%+v stdout=%q stderr=%q", control.name, result.Identity, result.Holder, result.CandidateWait, result.HolderExitCode, result.HolderAliveAtPostChild, result.PostChildAcknowledged, result.TerminalBefore, result.TerminalReady, result.TerminalAfter, trace, result.Stdout, result.Stderr)
}

func cleanupPublicHolderSignal(t *testing.T, transport *support.PublicTUITransport, probe *support.PublicTUIHostProbe, reading int, readingReleased *bool) {
	t.Helper()
	cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	first := transport.Result()
	if t.Failed() {
		t.Logf("first holder signal failure candidate=%+v holder=%+v wait=%+v stdout=%q stderr=%q reports=%q", first.Identity, first.Holder, first.CandidateWait, first.Stdout, first.Stderr, first.Reports)
	}
	var releaseError error
	if runtime.GOOS == "linux" && !*readingReleased {
		for _, event := range probe.Trace().Events {
			if event.Reading == reading && event.Phase == "start" {
				releaseError = probe.ReleaseReading(reading)
				*readingReleased = releaseError == nil
				break
			}
		}
	}
	closeError := transport.Close(cleanup)
	probeError := probe.Close(cleanup)
	result := transport.Result()
	t.Logf("holder signal cleanup joined=%t forced=%t closeError=%v probeError=%v releaseError=%v", result.Joined(), result.Forced, closeError, probeError, releaseError)
	if closeError != nil || probeError != nil || releaseError != nil || !result.Joined() {
		t.Errorf("owned holder/probe cleanup: close=%v probe=%v release=%v result=%+v", closeError, probeError, releaseError, result)
	}
}

func requirePublicHolderSignalReady(journey context.Context, t *testing.T, control publicHolderSignalControl, transport *support.PublicTUITransport, probe *support.PublicTUIHostProbe, reading int, screen *tui.BDDNativeTransportScreen) support.PublicTUIResult {
	t.Helper()
	var err error
	ready, stop := context.WithTimeout(journey, 3*time.Second)
	if err = probe.WaitForReading(ready, reading); err == nil {
		err = transport.AwaitReady(ready, func() (bool, error) {
			if control.cooked {
				return true, nil
			} // Actual collector ACK above plus native cooked ownership below.
			frame := screen.Snapshot()
			return frame.Alternate && frame.OverflowWrites == 0 && strings.Contains(strings.Join(frame.Rows, "\n"), "Terminal too small: need 40x16"), nil
		})
	}
	stop()
	if err != nil {
		t.Fatal(err)
	}
	before := transport.Result()
	if before.Identity.PID == before.Holder.PID || before.Identity.Session != before.Holder.PID || before.Identity.Group != before.Holder.PID || !before.TerminalReadyAvailable || before.TerminalReady.Foreground != before.Holder.PID {
		t.Fatalf("signal readiness lacked distinct stable native owners: %+v", before)
	}
	if control.cooked {
		if before.TerminalReady.Mode.Lflag&syscall.ISIG == 0 || before.TerminalReady.RawInput {
			t.Fatal("kernel signal control was not genuinely cooked ISIG-on")
		}
	} else if !before.TerminalReady.RawInput || before.TerminalReady.Mode.Lflag&syscall.ISIG != 0 {
		t.Fatal("raw signal control lacked observed ISIG-off")
	}
	return before
}

func requirePublicHolderSignalDrained(drain context.Context, t *testing.T, control publicHolderSignalControl, transport *support.PublicTUITransport, probe *support.PublicTUIHostProbe, reading int, before support.PublicTUIResult) (support.PublicTUIResult, support.PublicTUIHostProbeTrace) {
	t.Helper()
	result, err := transport.AwaitCandidateExit(drain)
	if err != nil {
		t.Fatal(err)
	}
	if !result.NaturalExit() || result.ExitCode != control.expected || result.CandidateWait.PID != before.Identity.PID {
		t.Fatalf("signal did not produce actual natural candidate%d: %+v", control.expected, result)
	}
	// Real candidate reap and real probe close/actor joins precede ACK of the
	// retained native witness. The same drain deadline covers every stage.
	if err = probe.Close(drain); err != nil {
		t.Fatal(err)
	}
	// A cohort can still be landing its in-flight reading when Close
	// returns on a loaded host. Wait bounded for it to join, then require
	// the exact settled cohort: a genuine leak still fails, as does an
	// extra reading.
	joined := time.NewTicker(5 * time.Millisecond)
	defer joined.Stop()
	for probe.Trace().Active != 0 {
		select {
		case <-drain.Done():
			t.Fatalf("collector cohort survived: %+v", probe.Trace())
		case <-joined.C:
		}
	}
	trace := probe.Trace()
	if !trace.Closed || trace.Active != 0 || trace.Readings != reading {
		t.Fatalf("collector cohort survived or acquired an extra reading: %+v", trace)
	}
	if runtime.GOOS == "darwin" {
		requirePublicHolderCanceledProbe(t, trace, reading)
	}
	result, err = transport.WaitAndDrain(drain)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Joined() || !result.NaturalExit() || result.ExitCode != control.expected || result.HolderExitCode != 0 || !result.HolderAliveAtPostChild || !result.PostChildAcknowledged || !result.TerminalAfterAvailable || result.TerminalAfter.Mode != result.TerminalBefore.Mode || result.TerminalAfter.Foreground != result.TerminalBefore.Foreground {
		t.Fatalf("signal lacked full native restoration/actual joins: %+v", result)
	}
	if control.cooked {
		caught := false
		for _, signal := range result.HolderSignals {
			caught = caught || signal == os.Interrupt.String()
		}
		if !caught {
			t.Fatalf("kernel03 did not retain an actual caught holder SIGINT: %+v", result.HolderSignals)
		}
	} else if len(result.HolderSignals) != 0 {
		t.Fatalf("raw/candidate-only signal was duplicated to holder: %+v", result.HolderSignals)
	}
	return result, trace
}

func requirePublicHolderCanceledProbe(t *testing.T, trace support.PublicTUIHostProbeTrace, reading int) {
	t.Helper()
	canceled := false
	for _, event := range trace.Events {
		canceled = canceled || event.Reading == reading && event.Phase == "canceled-eof"
	}
	if !canceled {
		t.Fatalf("Darwin canceled true probe has no real IPC EOF: %+v", trace)
	}
}
