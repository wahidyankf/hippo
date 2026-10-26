package support

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

const (
	annotationPublicationLine  = "HIPPO advisory: display label publication failed; monitoring may show an unavailable label.\n"
	annotationRemovalLine      = "HIPPO advisory: display label cleanup failed; later reconciliation may be needed.\n"
	annotationOperationWrite   = "write"
	annotationOperationBoth    = "both"
	annotationOperationRemoval = "removal"
	annotationOperationMissing = "missing"
	annotationOwnerLabel       = "private-owner-label"
)

type annotationRunState struct {
	operation string
	memory    *annotationMemoryRun
	native    *annotationNativeRun
}

func (driver *Driver) annotationFailureBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a labeled guarded run whose annotation (write|removal) fails$`, driver.prepareAnnotationFailure),
		step(`^that guarded run completes$`, driver.completeAnnotationRun),
		step(`^its admission resources child status and release outcome remain unchanged$`, driver.requireAnnotationRunOutcome),
		step(`^the annotation failure is reported without private paths$`, driver.requireAnnotationRunReport),
	}
}

func (driver *Driver) prepareAnnotationFailure(operation string) error {
	if operation != annotationOperationWrite && operation != annotationOperationRemoval {
		return fmt.Errorf("unknown annotation operation %q", operation)
	}
	state := &annotationRunState{operation: operation}
	if driver.mode == contract.Unit {
		state.memory = newAnnotationMemoryRun(operation)
	} else {
		native, err := driver.prepareAnnotationNative(operation, true, 37, false)
		if err != nil {
			return err
		}
		state.native = native
	}
	driver.annotationRun = state
	return nil
}

func (driver *Driver) completeAnnotationRun(ctx context.Context) error {
	state := driver.annotationRun
	if state == nil {
		return errors.New("annotation run was not prepared")
	}
	if driver.mode == contract.Unit {
		if state.memory == nil {
			return errors.New("unit annotation run has no in-memory application fixture")
		}
		state.memory.run(ctx, true)
		return nil
	}
	if state.native == nil {
		return errors.New("native annotation run has no real child fixture")
	}
	return driver.executeAnnotationNative(ctx, state.native)
}

func requireAnnotationMemoryOutcome(fixture *annotationMemoryRun, reservation bool) error {
	wantFinalized, wantOutcome := 1, evidence.OutcomeTaskFailed
	if fixture.inherited {
		wantFinalized, wantOutcome = 0, evidence.OutcomeUnset
		if !slices.Equal(fixture.trace, []string{"begin", "close", startPhase, taskClassRelease}) {
			return fmt.Errorf("inherited annotation run changed its non-owning lifecycle: %v", fixture.trace)
		}
	}
	if fixture.exit != 37 || fixture.err != nil || fixture.observed != 37 || fixture.starts != 1 || fixture.releases != 1 || fixture.finalized != wantFinalized || fixture.closed != 1 || fixture.outcome != wantOutcome {
		return annotationMemoryLifecycleError(fixture)
	}
	if !fixture.inherited && fixture.owned {
		return errors.New("annotation failure retained the retired in-memory owner")
	}
	if reservation && (application.EnvironmentValue(fixture.started.Environment, "HIPPO_CONCURRENCY") != "2" || application.EnvironmentValue(fixture.started.Environment, "HIPPO_RESERVED_MEMORY_BYTES") != strconv.FormatInt(policy.GiB, 10)) {
		return fmt.Errorf("actual application did not pass the unchanged allocation to Start: %v", fixture.started.Environment)
	}
	if !fixture.inherited {
		position := -1
		for _, event := range []string{"begin", annotationPublishEvent, "close", startPhase, "finalize", taskClassRelease, "remove"} {
			next := slices.Index(fixture.trace[position+1:], event)
			if next < 0 {
				return fmt.Errorf("actual lifecycle lost ordered %s: %v", event, fixture.trace)
			}
			position += next + 1
		}
	}
	return nil
}

func (driver *Driver) requireAnnotationRunOutcome() error {
	state := driver.annotationRun
	if state == nil {
		return errors.New("annotation outcome has no prepared subject")
	}
	if driver.mode == contract.Unit {
		if state.memory == nil {
			return errors.New("annotation outcome has no Unit subject")
		}
		if err := requireAnnotationMemoryOutcome(state.memory, true); err != nil {
			return err
		}
		if state.memory.writes != 1 || state.memory.removals != 1 || state.memory.failures != 1 {
			return fmt.Errorf("selected store fault did not actually occur once: writes=%d removals=%d failures=%d", state.memory.writes, state.memory.removals, state.memory.failures)
		}
		return nil
	}
	if state.native == nil {
		return errors.New("annotation outcome has no native subject")
	}
	return state.native.requireOutcome()
}

func requireAnnotationDiagnostics(output, diagnostics, operation string) error {
	want := annotationPublicationLine
	if operation == annotationOperationRemoval {
		want = annotationRemovalLine
	}
	if operation == annotationOperationBoth {
		want += annotationRemovalLine
	}
	if diagnostics != want {
		return fmt.Errorf("annotation mutation report=%q want fixed once-only %q", diagnostics, want)
	}
	for _, private := range []string{"/private/", annotationOwnerLabel, "private-argument", "raw-error", "\x1b", "hippo:", `"schemaVersion"`} {
		if strings.Contains(output+diagnostics, private) {
			return fmt.Errorf("annotation report exposed private/classified data %q: stdout=%q stderr=%q", private, output, diagnostics)
		}
	}
	return nil
}

func (driver *Driver) requireAnnotationRunReport() error {
	state := driver.annotationRun
	if state == nil {
		return errors.New("annotation report has no prepared subject")
	}
	if driver.mode == contract.Unit {
		if state.memory == nil {
			return errors.New("annotation report has no Unit subject")
		}
		return requireAnnotationDiagnostics("", state.memory.stderr.String(), state.operation)
	}
	if state.native == nil {
		return errors.New("annotation report has no native subject")
	}
	if strings.Contains(state.native.output.String()+state.native.diagnostics.String(), state.native.root) {
		return errors.New("annotation reporting exposed its private root")
	}
	return requireAnnotationDiagnostics(state.native.output.String(), state.native.diagnostics.String(), state.operation)
}

func annotationMemoryLifecycleError(fixture *annotationMemoryRun) error {
	if fixture.err != nil {
		return fmt.Errorf("annotation changed actual Run lifecycle: exit=%d error=%w observed=%d starts=%d releases=%d finalized=%d closed=%d outcome=%v trace=%v", fixture.exit, fixture.err, fixture.observed, fixture.starts, fixture.releases, fixture.finalized, fixture.closed, fixture.outcome, fixture.trace)
	}
	return fmt.Errorf("annotation changed actual Run lifecycle: exit=%d error=%v observed=%d starts=%d releases=%d finalized=%d closed=%d outcome=%v trace=%v", fixture.exit, nil, fixture.observed, fixture.starts, fixture.releases, fixture.finalized, fixture.closed, fixture.outcome, fixture.trace)
}
