package support

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/tests/contract"
)

// NewGuardedTUIJob holds an actual application Run invocation at its outward
// workload boundary. The probe proves use-case survival; physical child and
// coordination ownership are separate Integration/public candidate obligations.
func (driver *Driver) NewGuardedTUIJob(ctx context.Context) (func() error, func() error, error) {
	if driver.mode != contract.Unit {
		return driver.newNativeGuardedTUIJob(ctx)
	}
	fixture := newAnnotationMemoryRun("healthy")
	fixture.heldDone = make(chan application.ChildCompletion, 1)
	fixture.heldStarted = make(chan struct{})
	// The observed application belongs to the fixture, beyond one scenario.
	// Its explicit completion and bounded join retain responsibility for cleanup.
	owned, cancel := context.WithCancel(context.WithoutCancel(ctx))
	returned := make(chan struct{})
	go func() { defer close(returned); fixture.run(owned, true) }()
	readyBound := time.NewTimer(5 * time.Second)
	defer readyBound.Stop()
	select {
	case <-fixture.heldStarted:
	case <-returned:
		cancel()
		return nil, nil, fmt.Errorf("guarded application exited before activation: %d/%w", fixture.exit, fixture.err)
	case <-ctx.Done():
		cancel()
		<-returned
		return nil, nil, ctx.Err()
	case <-readyBound.C:
		cancel()
		<-returned
		return nil, nil, errors.New("guarded application did not activate within its fixture bound")
	}
	probe := func() error {
		select {
		case <-returned:
			return errors.New("TUI retired the independent guarded application")
		default:
		}
		if fixture.heldStops.Load() != 0 || fixture.heldReleases.Load() != 0 || fixture.heldFinalized.Load() != 0 {
			return errors.New("TUI stopped, released or finalized the independent guarded application")
		}
		return nil
	}
	var once sync.Once
	var closeError error
	finish := func() error {
		once.Do(func() {
			fixture.heldDone <- application.ChildCompletion{ExitCode: 37, Failed: true}
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case <-returned:
			case <-timer.C:
				cancel()
				<-returned
				closeError = errors.New("guarded application failed to join after its own completion")
			}
			cancel()
			if closeError == nil && (fixture.exit != 37 || fixture.heldStops.Load() != 0 || fixture.heldReleases.Load() != 1 || fixture.heldFinalized.Load() != 1) {
				closeError = fmt.Errorf("independent completion changed: exit=%d stops=%d releases=%d finalized=%d", fixture.exit, fixture.heldStops.Load(), fixture.heldReleases.Load(), fixture.heldFinalized.Load())
			}
		})
		return closeError
	}
	driver.stops = append(driver.stops, guardedTUIJobCleanup(finish))
	return probe, finish, nil
}
