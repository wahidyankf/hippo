package tui_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/tests/support"
)

func (fixture *publicBDDObservation) lifecycleHandler(pattern string) any {
	switch pattern {
	case `^TUI is displaying (dashboard|job detail|capacity detail|help|minimum-size notice) in an isolated terminal$`:
		return fixture.preparePublicLifecycle
	case `^the user presses q$`:
		return fixture.quitPublicLifecycle
	case `^TUI exits 0 after cancelling and joining its observation work$`:
		return fixture.requirePublicQuit
	case `^the original terminal modes and foreground ownership are restored$`:
		return fixture.requirePublicRestored
	case `^no guarded job is cancelled or retired$`:
		return fixture.requirePublicGuarded
	case `^TUI is observing in an isolated terminal$`:
		return fixture.preparePublicObserving
	case `^TUI receives (raw Ctrl-C|SIGINT|SIGTERM)$`:
		return fixture.interruptPublicLifecycle
	case `^TUI exits (\d+) without a HIPPO failure diagnostic$`:
		return fixture.requirePublicInterrupted
	case `^observation is cancelled and joined before the runner returns$`:
		return fixture.requirePublicRunnerJoined
	default:
		return nil // Fault rows wait for the genuine native debugger qualification.
	}
}

func (fixture *publicBDDObservation) preparePublicLifecycle(ctx context.Context, view string) (context.Context, error) {
	ctx, err := fixture.prepareCase(ctx, "known", "")
	if err != nil {
		return ctx, err
	}
	fixture.family = &publicBDDInteractive{view: "dashboard", exitCode: 0}
	probe, finish, err := fixture.driver.NewGuardedTUIJob(ctx)
	if err != nil {
		return ctx, err
	}
	fixture.family.guardedProbe, fixture.family.guardedFinish = probe, finish
	if err := fixture.startInteractive(ctx, 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false); err != nil {
		return ctx, err
	}
	if err := fixture.live(ctx); err != nil {
		return ctx, err
	}
	if view == "minimum-size notice" {
		view = "minimum-size"
	}
	return ctx, fixture.openPublicView(ctx, view)
}

func (fixture *publicBDDObservation) preparePublicObserving(ctx context.Context) (context.Context, error) {
	ctx, err := fixture.preparePublicLifecycle(ctx, "dashboard")
	if err != nil {
		return ctx, err
	}
	return ctx, fixture.holdRefresh(ctx, support.PublicTUIHostReply{CPUPercent: 20})
}

func (fixture *publicBDDObservation) quitPublicLifecycle(ctx context.Context) error {
	if err := fixture.finish(ctx); err != nil {
		return err
	}
	fixture.family.terminal = fixture.transport.Result()
	return nil
}

func (fixture *publicBDDObservation) requirePublicQuit(_ context.Context) error {
	result := fixture.family.terminal
	if !result.Joined() || !result.NaturalExit() || result.ExitCode != 0 {
		return fmt.Errorf("public q did not naturally exit/join: %+v", result)
	}
	return nil
}

func (fixture *publicBDDObservation) requirePublicRestored(_ context.Context) error {
	result := fixture.family.terminal
	if !result.TerminalReadyAvailable || !result.TerminalAfterAvailable || result.TerminalBefore.Mode != result.TerminalAfter.Mode || result.TerminalBefore.Foreground != result.TerminalAfter.Foreground || result.HolderExitCode != 0 || !result.HolderAliveAtPostChild || !result.PostChildAcknowledged {
		return fmt.Errorf("public lifecycle lacks full same-slave restoration witness: %+v", result)
	}
	return nil
}

func (fixture *publicBDDObservation) requirePublicGuarded(_ context.Context) error {
	if fixture.family.guardedProbe == nil {
		return errors.New("lifecycle omitted genuine independent guarded command")
	}
	if err := fixture.family.guardedProbe(); err != nil {
		return err
	}
	return fixture.family.guardedFinish()
}

func (fixture *publicBDDObservation) interruptPublicLifecycle(ctx context.Context, kind string) error {
	before := fixture.transport.Result()
	fixture.family.exitCode = 130
	switch kind {
	case "raw Ctrl-C":
		action, stop := context.WithTimeout(ctx, 500*time.Millisecond)
		err := fixture.transport.SendKey(action, []byte{3})
		stop()
		if err != nil {
			return err
		}
	case "SIGINT":
		if err := fixture.transport.Signal(before.Identity.PID, syscall.SIGINT); err != nil {
			return err
		}
	case "SIGTERM":
		fixture.family.exitCode = 143
		if err := fixture.transport.Signal(before.Identity.PID, syscall.SIGTERM); err != nil {
			return err
		}
	default:
		return errors.New("unsupported original public interruption")
	}
	drain, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	if _, err := fixture.transport.AwaitCandidateExit(drain); err != nil {
		return err
	}
	if err := fixture.probe.Close(drain); err != nil {
		return err
	}
	result, err := fixture.transport.WaitAndDrain(drain)
	fixture.family.terminal = result
	return err
}

func (fixture *publicBDDObservation) requirePublicInterrupted(_ context.Context, exit int) error {
	result := fixture.family.terminal
	if exit != fixture.family.exitCode || !result.Joined() || !result.NaturalExit() || result.ExitCode != exit || len(result.Stderr) != 0 {
		return fmt.Errorf("actual public interruption changed original %d or diagnostic contract: %+v", exit, result)
	}
	return nil
}

func (fixture *publicBDDObservation) requirePublicRunnerJoined(_ context.Context) error {
	trace := fixture.probe.Trace()
	if !fixture.family.terminal.Joined() || trace.Active != 0 || !trace.Closed {
		return errors.New("public lifecycle observation/EOF/Wait not joined")
	}
	if runtime.GOOS == "darwin" && fixture.family.gate > 0 {
		canceled := false
		for _, event := range trace.Events {
			canceled = canceled || event.Reading == fixture.family.gate && event.Phase == "canceled-eof"
		}
		if !canceled {
			return errors.New("interruption lacks actual held-reading cancellation EOF")
		}
	}
	return nil
}
