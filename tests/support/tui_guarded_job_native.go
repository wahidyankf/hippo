package support

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/tests/contract"
)

func (driver *Driver) newNativeGuardedTUIJob(ctx context.Context) (func() error, func() error, error) {
	state, err := driver.prepareAnnotationNative("healthy", true, 37, false)
	if err != nil {
		return nil, nil, err
	}
	owned, cancel := context.WithTimeout(ctx, 40*time.Second)
	environment := []string{hippoRootEnvironment + "=" + state.root, "ANNOTATION_READY=" + state.ready, "ANNOTATION_RELEASE=" + state.release, "ANNOTATION_CHILD_EXIT=" + strconv.Itoa(state.childExit), isolatedPath}
	arguments := []string{runCommandName, configFlag, state.configPath, annotationCwdFlag, state.scratch, diskPathFlag, state.scratch, tuiNativeProfileFlag, profileConstrained, tuiNativeLabelFlag, annotationOwnerLabel, reserveCPUFlag, "2", reserveMemoryFlag, "1024", "--", shellPath, "-c", annotationChildScript}
	returned := make(chan struct{})
	var launchError error
	go func() {
		defer close(returned)
		if driver.mode == contract.E2E {
			launchError = driver.runCompiledAnnotation(owned, state, arguments, environment)
		} else {
			driver.runEmbeddedAnnotation(owned, state, arguments, environment)
		}
	}()
	readyTimer := time.NewTimer(30 * time.Second)
	defer readyTimer.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	if err = state.awaitAnnotationLaunch(owned, readyTimer.C, ticker.C); err != nil {
		cancel()
		<-returned
		return nil, nil, err
	}
	sidecar := filepath.Join(state.root, "run-display", state.token+".json")
	if err = state.verifyOwnedAnnotation(sidecar); err != nil {
		cancel()
		<-returned
		return nil, nil, err
	}
	probe := func() error {
		select {
		case <-returned:
			return errors.New("TUI retired the independent guarded child")
		default:
		}
		totals, err := runtimeadapter.NewEngine().Snapshot(ctx, state.root, reservationMode)
		if err != nil {
			return err
		}
		if totals.ActiveOwners != 1 || len(totals.Owners) != 1 || totals.Owners[0].RunID != state.token {
			return errors.New("TUI changed the guarded child's actual ownership")
		}
		data, err := os.ReadFile(sidecar)
		if err != nil {
			return err
		}
		if string(data) != string(state.retainedAnnotation) {
			return errors.New("TUI changed the guarded child's display annotation")
		}
		return nil
	}
	var once sync.Once
	var closeError error
	finish := func() error {
		once.Do(func() {
			closeError = os.WriteFile(state.release, []byte("complete independently"), 0o600)
			if closeError != nil {
				cancel()
			}
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case <-returned:
			case <-timer.C:
				cancel()
				<-returned
				closeError = errors.Join(closeError, errors.New("guarded child did not complete within fixture bound"))
			}
			cancel()
			if launchError != nil {
				closeError = errors.Join(closeError, launchError)
			}
			if state.exit != 37 {
				closeError = errors.Join(closeError, fmt.Errorf("independent child exit changed to %d", state.exit))
			}
			closeError = errors.Join(closeError, state.requireGuardedTUICleanup(ctx))
		})
		return closeError
	}
	driver.stops = append(driver.stops, guardedTUIJobCleanup(finish))
	return probe, finish, nil
}

func (state *annotationNativeRun) requireGuardedTUICleanup(ctx context.Context) error {
	var closeError error
	totals, err := runtimeadapter.NewEngine().Snapshot(ctx, state.root, reservationMode)
	closeError = errors.Join(closeError, err)
	if totals.ActiveOwners != 0 || totals.WaitingOwners != 0 {
		closeError = errors.Join(closeError, errors.New("independent completion retained actual guarded ownership"))
	}
	closeError = errors.Join(closeError, requireAnnotationNativeOwnerAbsent(filepath.Join(state.root, "run-display", state.token+".json")))
	return closeError
}

func guardedTUIJobCleanup(finish func() error) func() {
	return func() {
		if err := finish(); err != nil {
			panic(fmt.Errorf("clean up guarded TUI job: %w", err))
		}
	}
}
