package tui_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/tests/support"
)

func (fixture *publicBDDObservation) refreshHandler(pattern string) any {
	switch pattern {
	case `^a TUI whose previous observation has just completed$`:
		return fixture.preparePublicCompleted
	case `^five seconds elapse after that completion$`:
		return fixture.waitPublicDeadline
	case `^exactly one new observation starts and no observation starts before that deadline$`:
		return fixture.requirePublicDeadline
	case `^one blocked TUI observation$`:
		return fixture.preparePublicBlocked
	case `^refresh is requested three times before that observation completes$`:
		return fixture.requestPublicThree
	case `^no second observation overlaps it and exactly one immediate follow-up starts after completion$`:
		return fixture.requirePublicCoalescing
	case `^one blocked TUI observation and a selected running job$`:
		return fixture.preparePublicBusySelection
	case `^the operator switches section and resizes to 40 by 16 cells$`:
		return fixture.navigatePublicBusy
	case `^the section and compact layout change before the observation is released$`:
		return fixture.requirePublicBusyNavigation
	case `^a TUI without a successful observation$`:
		return fixture.preparePublicInitial
	case `^its first observation fails$`:
		return fixture.failPublicInitial
	case `^the display shows unavailable data and a safe error summary with refresh still accessible$`:
		return fixture.requirePublicInitialFailure
	case `^a successful TUI snapshot with one owner and known allocated resources$`:
		return fixture.preparePublicOwner
	case `^the next observation fails$`:
		return fixture.failPublicNext
	case `^the same owner and allocation remain visible with STALE sample age and a safe error summary$`:
		return fixture.requirePublicStale
	case `^a stale TUI snapshot after an observation failure$`:
		return fixture.preparePublicStale
	case `^a later observation succeeds$`:
		return fixture.recoverPublicObservation
	case `^the new snapshot replaces the old data and STALE and ERROR markers disappear$`:
		return fixture.requirePublicRecovery
	case `^a successful observation whose second host reading was measured ten seconds ago$`:
		return fixture.preparePublicAged
	case `^one lightweight age tick elapses before the next refresh deadline$`:
		return fixture.publicAgeTick
	case `^the displayed sample age advances and the host collector is not called again$`:
		return fixture.requirePublicAge
	case `^the operator quits and the collector receives cancellation$`:
		return fixture.quitPublicBusy
	case `^the TUI exits after its observation and timing work finish and no late result updates its display$`:
		return fixture.requirePublicBusyJoined
	default:
		return nil
	}
}

func (fixture *publicBDDObservation) preparePublicCompleted(ctx context.Context) (context.Context, error) {
	// Omit --interval: this row verifies the unchanged public default five seconds.
	ctx, err := fixture.prepareInteractive(ctx, "known", 120, 32, "", support.PublicTUIHostReply{CPUPercent: 20}, false)
	if err != nil {
		return ctx, err
	}
	fixture.family.beforeTrace = fixture.probe.Trace()
	fixture.family.completedUpper = fixture.family.lastLive
	// Last ACK is only a lower bound on completion; never call it completedAt.
	var last int64
	for _, event := range fixture.family.beforeTrace.Events {
		if event.Reading == 2 && event.Phase == "output-ack" {
			last = max(last, event.ElapsedNanoseconds)
		}
	}
	if last == 0 {
		return ctx, errors.New("default refresh lacks the real final reading ACK lower bound")
	}
	fixture.family.completedLower = fixture.family.probeBefore.Add(time.Duration(last))
	return ctx, nil
}

func (fixture *publicBDDObservation) waitPublicDeadline(ctx context.Context) error {
	until := fixture.family.completedLower.Add(5 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(until) {
		if fixture.probe.Trace().Readings != 2 {
			return errors.New("public default refresh started before the completion lower-bound deadline")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	bound, stop := context.WithDeadline(ctx, publicRefreshStartDeadline(fixture.family.completedUpper))
	defer stop()
	return fixture.probe.WaitForReading(bound, 3)
}

func (fixture *publicBDDObservation) requirePublicDeadline(ctx context.Context) error {
	trace := fixture.probe.Trace()
	var first int64
	for _, event := range trace.Events {
		if event.Reading == 3 && event.Phase == "start" {
			first = event.ElapsedNanoseconds
			break
		}
	}
	lower := fixture.family.probeBefore.Add(time.Duration(first))
	upper := fixture.family.probeAfter.Add(time.Duration(first))
	if !publicRefreshStartWithinCompletionBracket(fixture.family, first, trace.MaximumActive) {
		return fmt.Errorf("default refresh outside externally observed completion bracket: origin=%s..%s completion=%s..%s start=%s..%s allowed=%s..%s elapsedStart=%d maximumActive=%d; trace=%+v", fixture.family.probeBefore.Format(time.RFC3339Nano), fixture.family.probeAfter.Format(time.RFC3339Nano), fixture.family.completedLower.Format(time.RFC3339Nano), fixture.family.completedUpper.Format(time.RFC3339Nano), lower.Format(time.RFC3339Nano), upper.Format(time.RFC3339Nano), fixture.family.completedLower.Add(5*time.Second).Format(time.RFC3339Nano), publicRefreshStartDeadline(fixture.family.completedUpper).Format(time.RFC3339Nano), first, trace.MaximumActive, trace)
	}
	if err := fixture.live(ctx); err != nil {
		return err
	}
	if fixture.probe.Trace().Readings != 4 {
		return errors.New("default refresh did not produce exactly one new real observation")
	}
	return fixture.finish(ctx)
}

// The external helper request follows scheduler dispatch and real process startup.
// Both wait and assertion retain the existing bounded dispatch allowance.
func publicRefreshStartDeadline(completedUpper time.Time) time.Time {
	return completedUpper.Add(5*time.Second + 500*time.Millisecond)
}

func publicRefreshStartWithinCompletionBracket(family *publicBDDInteractive, first int64, maximumActive int) bool {
	if family == nil || first == 0 || maximumActive != 1 {
		return false
	}
	lower := family.probeBefore.Add(time.Duration(first))
	upper := family.probeAfter.Add(time.Duration(first))
	return !upper.Before(family.completedLower.Add(5*time.Second)) && !lower.After(publicRefreshStartDeadline(family.completedUpper))
}

func (fixture *publicBDDObservation) preparePublicBlocked(ctx context.Context) (context.Context, error) {
	ctx, err := fixture.prepareInteractive(ctx, "known", 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20}, true)
	if err != nil {
		return ctx, err
	}
	// These two original busy bindings need input while the genuine first
	// reading is held. Collector start alone never authenticates native input.
	err = fixture.waitInteractive(ctx, []string{"No observation", "refreshing"}, nil)
	if err != nil {
		return ctx, err
	}
	trace := fixture.probe.Trace()
	if trace.Readings != 1 || trace.Active != 1 || trace.MaximumActive != 1 {
		return ctx, fmt.Errorf("busy readiness lost the genuinely held first reading: %+v", trace)
	}
	return ctx, nil
}

func (fixture *publicBDDObservation) requestPublicThree(ctx context.Context) error {
	for range 3 {
		if err := fixture.sendInteractive(ctx, "r"); err != nil {
			return err
		}
	}
	// Help is a real subsequent input barrier while the collector remains blocked.
	if err := fixture.sendInteractive(ctx, "?"); err != nil {
		return err
	}
	_, err := fixture.await(ctx, []string{"HIPPO | Help"}, nil)
	return err
}

func (fixture *publicBDDObservation) requirePublicCoalescing(ctx context.Context) error {
	before := fixture.probe.Trace()
	if before.Readings != 1 || before.MaximumActive != 1 {
		return errors.New("refresh requests overlapped the actual blocked reading")
	}
	if err := fixture.probe.ReleaseReading(fixture.family.gate); err != nil {
		return err
	}
	bound, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	if err := fixture.probe.WaitForReading(bound, 3); err != nil {
		return err
	}
	if err := fixture.live(ctx); err != nil {
		return err
	}
	after := fixture.probe.Trace()
	if after.Readings != 4 || after.MaximumActive != 1 {
		return fmt.Errorf("coalescing did not produce one follow-up: %+v", after)
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicBusySelection(ctx context.Context) (context.Context, error) {
	ctx, err := fixture.prepareInteractive(ctx, "three", 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false)
	if err != nil {
		return ctx, err
	}
	if err := fixture.selectPublicMiddle(ctx); err != nil {
		return ctx, err
	}
	return ctx, fixture.holdRefresh(ctx, support.PublicTUIHostReply{CPUPercent: 20})
}

func (fixture *publicBDDObservation) navigatePublicBusy(ctx context.Context) error {
	if err := fixture.sendInteractive(ctx, "n"); err != nil {
		return err
	}
	return fixture.resizeInteractive(ctx, 40, 16)
}

func (fixture *publicBDDObservation) requirePublicBusyNavigation(ctx context.Context) error {
	_, err := fixture.await(ctx, []string{"[Queue]", "refreshing"}, nil)
	if err != nil {
		return err
	}
	trace := fixture.probe.Trace()
	if trace.Readings != fixture.family.gate || trace.Active != 1 {
		return errors.New("busy navigation was not witnessed before gate release")
	}
	if err := fixture.releaseRefresh(ctx); err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicInitial(ctx context.Context) (context.Context, error) {
	return fixture.prepareInteractive(ctx, "one", 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20, InvalidMemory: true}, true)
}

func (fixture *publicBDDObservation) failPublicInitial(ctx context.Context) error {
	if err := fixture.probe.ReleaseReading(fixture.family.gate); err != nil {
		return err
	}
	err := fixture.waitInteractive(ctx, []string{"ERROR", "unavailable"}, []string{"refreshing"})
	return err
}

func (fixture *publicBDDObservation) requirePublicInitialFailure(ctx context.Context) error {
	text := strings.Join(fixture.screen.Snapshot().Rows, "\n")
	if strings.Contains(text, "Running 0/0") || strings.Contains(text, "Queue 0/0") || strings.Contains(text, "unchecked-payload") {
		return errors.New("first public failure invented empty accounting or leaked unsafe content")
	}
	if err := fixture.holdRefresh(ctx, support.PublicTUIHostReply{CPUPercent: 20}); err != nil {
		return err
	}
	if err := fixture.releaseRefresh(ctx); err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicOwner(ctx context.Context) (context.Context, error) {
	return fixture.prepareInteractive(ctx, "one", 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false)
}

func (fixture *publicBDDObservation) failPublicNext(ctx context.Context) error {
	fixture.family.before = fixture.screen.Snapshot()
	if err := fixture.holdRefresh(ctx, support.PublicTUIHostReply{CPUPercent: 20, InvalidMemory: true}); err != nil {
		return err
	}
	if err := fixture.probe.ReleaseReading(fixture.family.gate); err != nil {
		return err
	}
	err := fixture.waitInteractive(ctx, []string{"STALE", "ERROR"}, []string{"refreshing"})
	return err
}

func (fixture *publicBDDObservation) requirePublicStale(ctx context.Context) error {
	if err := fixture.sendInteractive(ctx, "Enter"); err != nil {
		return err
	}
	row := fixture.ledger.Expected().Owners[0]
	_, err := fixture.await(ctx, []string{row.RunID, "2CPU", "1.0GiB", "STALE", "ERROR"}, nil)
	if err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicStale(ctx context.Context) (context.Context, error) {
	ctx, err := fixture.preparePublicOwner(ctx)
	if err != nil {
		return ctx, err
	}
	return ctx, fixture.failPublicNext(ctx)
}

func (fixture *publicBDDObservation) recoverPublicObservation(ctx context.Context) error {
	if err := fixture.holdRefresh(ctx, support.PublicTUIHostReply{CPUPercent: 40}); err != nil {
		return err
	}
	return fixture.releaseRefresh(ctx)
}

func (fixture *publicBDDObservation) requirePublicRecovery(ctx context.Context) error {
	if err := fixture.sendInteractive(ctx, "c"); err != nil {
		return err
	}
	_, err := fixture.await(ctx, []string{"Host CPU utilization: 40%"}, []string{"STALE", "ERROR"})
	if err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicAged(ctx context.Context) (context.Context, error) {
	ctx, err := fixture.preparePublicOwner(ctx)
	if err != nil {
		return ctx, err
	}
	bound, stop := context.WithTimeout(ctx, 11*time.Second)
	defer stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		text := strings.Join(fixture.screen.Snapshot().Rows, "\n")
		if strings.Contains(text, "10s") {
			fixture.family.ageBefore = text
			fixture.family.beforeTrace = fixture.probe.Trace()
			return ctx, nil
		}
		select {
		case <-bound.Done():
			return ctx, bound.Err()
		case <-ticker.C:
		}
	}
}

func (fixture *publicBDDObservation) publicAgeTick(ctx context.Context) error {
	bound, stop := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if strings.Contains(strings.Join(fixture.screen.Snapshot().Rows, "\n"), "11s") {
			return nil
		}
		select {
		case <-bound.Done():
			return bound.Err()
		case <-ticker.C:
		}
	}
}

func (fixture *publicBDDObservation) requirePublicAge(ctx context.Context) error {
	after := fixture.probe.Trace()
	if after.Readings != fixture.family.beforeTrace.Readings || len(after.Events) != len(fixture.family.beforeTrace.Events) {
		return errors.New("sample-age tick invoked real collector again")
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) quitPublicBusy(ctx context.Context) error {
	fixture.family.before = fixture.screen.Snapshot()
	if err := fixture.finish(ctx); err != nil {
		return err
	}
	fixture.family.terminal = fixture.transport.Result()
	return nil
}

func (fixture *publicBDDObservation) requirePublicBusyJoined(ctx context.Context) error {
	trace := fixture.probe.Trace()
	canceled := false
	for _, event := range trace.Events {
		canceled = canceled || event.Phase == "canceled-eof"
	}
	if !canceled || trace.Active != 0 || !trace.Closed || !fixture.family.terminal.Joined() {
		return errors.New("busy public quit lacks actual cancellation/EOF/join")
	}
	// Joined input/output loops cannot subsequently apply any late observation.
	return fixture.ledger.VerifyLiveness(ctx)
}
