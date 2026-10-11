package tui_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// Interactive state records input actions and independently observed frames.
// It never supplies a snapshot, renderer frame, hardware value or sample time.
type publicBDDInteractive struct {
	view, previousView, key        string
	selected                       string
	before                         tui.BDDNativeTransportFrame
	beforeTrace                    support.PublicTUIHostProbeTrace
	gate                           int
	completedLower, completedUpper time.Time
	lastLive                       time.Time
	probeBefore, probeAfter        time.Time
	ageBefore                      string
	textKind                       string
	guardedProbe                   func() error
	guardedFinish                  func() error
	terminal                       support.PublicTUIResult
	exitCode                       int
	settling                       bool
	resizeEvidence                 []string
}

func (fixture *publicBDDObservation) startInteractive(ctx context.Context, width, height int, interval string, reply support.PublicTUIHostReply, held bool) error {
	if fixture.transport != nil || fixture.ledger == nil {
		return errors.New("public interactive launch requires one unstarted owned ledger")
	}
	var err error
	fixture.width, fixture.height = width, height
	if fixture.family == nil {
		fixture.family = &publicBDDInteractive{view: "dashboard", previousView: "dashboard"}
	}
	fixture.family.probeBefore = time.Now()
	fixture.probe, err = fixture.driver.NewPublicTUIHostProbeWithHelper(ctx, fixture.directory, reply, fixture.helper)
	fixture.family.probeAfter = time.Now()
	if err != nil {
		return err
	}
	if fixture.family == nil {
		fixture.family = &publicBDDInteractive{view: "dashboard", previousView: "dashboard"}
	}
	if held {
		fixture.family.gate, err = fixture.probe.HoldNextReading()
		if err != nil {
			return err
		}
	}
	fixture.screen = tui.NewBDDNativeTransportScreen(width, height)
	arguments := []string{"tui", "--config", fixture.ledger.Configuration(), "--disk-path", fixture.directory}
	if interval != "" {
		arguments = append(arguments, "--interval", interval)
	}
	arguments = append(arguments, fixture.filters...)
	fixture.transport, err = support.StartPublicTUI(ctx, support.PublicTUIStart{Driver: fixture.driver, Mode: contract.E2E, Executable: os.Getenv("HIPPO_BIN"), Root: fixture.root, Directory: fixture.directory, Arguments: arguments, Environment: fixture.environment(fixture.probe), Width: width, Height: height, Feed: fixture.screen.Feed, ResizeScreen: fixture.screen.Resize, Finish: fixture.screen.Finish})
	return err
}

func (fixture *publicBDDObservation) waitInteractive(ctx context.Context, present, absent []string) error {
	waitStarted := time.Now()
	ready, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	selection := flag.Lookup("test.run")
	targeted := selection != nil && (selection.Value.String() == "^TestE2EBehaviours$/^Unrelated_keys_do_not_capture_host_navigation#05$" || selection.Value.String() == "^TestE2EBehaviours$/^Detail_and_help_scroll_and_return_without_a_focus_trap(#.*)?$")
	deadline, _ := ready.Deadline()
	var err error
	defer func() {
		if err == nil && !targeted {
			return
		}
		waitFinished := time.Now()
		trace := fixture.probe.Trace()
		trace.Events = trace.Events[:min(32, len(trace.Events))]
		fmt.Fprintf(os.Stderr, "public readiness timing probeBefore=%s probeAfter=%s waitStarted=%s deadline=%s waitFinished=%s result=%v trace=%+v\n", fixture.family.probeBefore.Format(time.RFC3339Nano), fixture.family.probeAfter.Format(time.RFC3339Nano), waitStarted.Format(time.RFC3339Nano), deadline.Format(time.RFC3339Nano), waitFinished.Format(time.RFC3339Nano), err, trace)
	}()
	var observed tui.BDDNativeTransportFrame
	err = fixture.transport.AwaitReady(ready, func() (bool, error) {
		observed = fixture.screen.Snapshot()
		if observed.OverflowWrites != 0 {
			return false, errors.New("public frame wrote beyond native viewport")
		}
		text := strings.Join(observed.Rows, "\n")
		match := observed.Alternate && observed.Width == fixture.width && observed.Height == fixture.height
		if fixture.family.settling {
			trace := fixture.probe.Trace()
			match = match && trace.Active == 0 && trace.MaximumActive == 1 && trace.Readings >= max(2, fixture.family.gate+1)
		}
		for _, part := range present {
			match = match && strings.Contains(text, part)
		}
		for _, part := range absent {
			match = match && !strings.Contains(text, part)
		}
		return match, nil
	})
	if err == nil {
		fixture.frames = append(fixture.frames, observed)
	}
	return err
}

func (fixture *publicBDDObservation) live(ctx context.Context) error {
	fixture.family.settling = true
	defer func() { fixture.family.settling = false }()
	present, absent := []string{"LIVE"}, []string{"refreshing", "ERROR", "STALE"}
	if fixture.width < 40 || fixture.height < 16 {
		present, absent = nil, nil
	}
	err := fixture.waitInteractive(ctx, present, absent)
	if err != nil {
		frame := fixture.screen.Snapshot()
		trace := fixture.probe.Trace()
		return fmt.Errorf("public readiness stage=initial-live viewport=%dx%d frame=%q readings=%d active=%d maximumActive=%d native=%s: %w", fixture.width, fixture.height, frame.Rows, trace.Readings, trace.Active, trace.MaximumActive, fixture.transport.ReadinessEvidence(), fmt.Errorf("%w; collectorEvents=%+v", err, trace.Events[:min(32, len(trace.Events))]))
	}
	trace := fixture.probe.Trace()
	if trace.Active != 0 || trace.MaximumActive != 1 || trace.Readings < 2 {
		return fmt.Errorf("LIVE lacks settled real two-reading cohorts: %+v", trace)
	}
	fixture.family.lastLive = time.Now()
	return nil
}

func (fixture *publicBDDObservation) prepareInteractive(ctx context.Context, variant string, width, height int, interval string, reply support.PublicTUIHostReply, held bool) (context.Context, error) {
	ctx, err := fixture.prepareCase(ctx, variant, "")
	if err != nil {
		return ctx, err
	}
	fixture.family = &publicBDDInteractive{view: "dashboard", previousView: "dashboard"}
	if err := fixture.startInteractive(ctx, width, height, interval, reply, held); err != nil {
		return ctx, err
	}
	if held {
		ready, stop := context.WithTimeout(ctx, 3*time.Second)
		defer stop()
		return ctx, fixture.probe.WaitForReading(ready, fixture.family.gate)
	}
	return ctx, fixture.live(ctx)
}

func (fixture *publicBDDObservation) sendInteractive(ctx context.Context, key string) error {
	encoded, err := publicInteractiveKey(key)
	if err != nil {
		return err
	}
	action, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stop()
	if err := fixture.transport.SendKey(action, encoded); err != nil {
		return err
	}
	fixture.family.key = key
	return nil
}

func publicInteractiveKey(key string) ([]byte, error) {
	switch key {
	case "Tab":
		return []byte("\t"), nil
	case "Shift-Tab":
		return []byte("\x1b[Z"), nil
	case "Down":
		return []byte("\x1b[B"), nil
	case "Up":
		return []byte("\x1b[A"), nil
	case "Enter":
		return []byte("\r"), nil
	case "Ctrl-Space":
		return []byte{0}, nil
	case "Ctrl-h":
		return []byte{8}, nil
	case "Ctrl-j":
		return []byte{10}, nil
	case "Ctrl-k":
		return []byte{11}, nil
	case "Ctrl-l":
		return []byte{12}, nil
	case "Ctrl-backslash":
		return []byte{28}, nil
	case "Escape":
		return []byte("\x1b[27u"), nil
	case "n", "p", "j", "k", "c", "?", "b", "r", "q", "x":
		return []byte(key), nil
	default:
		return nil, fmt.Errorf("unknown original public key %q", key)
	}
}

func (fixture *publicBDDObservation) resizeInteractive(ctx context.Context, width, height int) error {
	action, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stop()
	before := fixture.screen.Snapshot()
	if err := fixture.transport.Resize(action, width, height); err != nil {
		return err
	}
	fixture.width, fixture.height = width, height
	after := fixture.screen.Snapshot()
	// Native Resize serializes its decoder callback with the output pump. The
	// snapshots bracket that actual call; overflow records retain write-time size.
	if len(fixture.family.resizeEvidence) < 16 {
		fixture.family.resizeEvidence = append(fixture.family.resizeEvidence, fmt.Sprintf("resize %dx%d -> %dx%d beforeOverflow=%s afterOverflow=%s at=%s", before.Width, before.Height, after.Width, after.Height, before.OverflowEvidence, after.OverflowEvidence, time.Now().Format(time.RFC3339Nano)))
	}
	_, err := fixture.await(action, nil, nil)
	return err
}

func (fixture *publicBDDObservation) armRefresh(reply support.PublicTUIHostReply) error {
	if err := fixture.probe.SetReply(reply); err != nil {
		return err
	}
	gate, err := fixture.probe.HoldNextReading()
	if err != nil {
		return err
	}
	fixture.family.gate = gate
	return nil
}

func (fixture *publicBDDObservation) holdRefresh(ctx context.Context, reply support.PublicTUIHostReply) error {
	if err := fixture.armRefresh(reply); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "r"); err != nil {
		return err
	}
	bound, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	return fixture.probe.WaitForReading(bound, fixture.family.gate)
}

func (fixture *publicBDDObservation) releaseRefresh(ctx context.Context) error {
	if err := fixture.probe.ReleaseReading(fixture.family.gate); err != nil {
		return err
	}
	return fixture.live(ctx)
}

func (fixture *publicBDDObservation) interactiveHandler(pattern string) any {
	if handler := fixture.refreshHandler(pattern); handler != nil {
		return handler
	}
	if handler := fixture.keyboardHandler(pattern); handler != nil {
		return handler
	}
	if handler := fixture.viewportHandler(pattern); handler != nil {
		return handler
	}
	return fixture.lifecycleHandler(pattern)
}
