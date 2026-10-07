package application_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

func assertRunTraceOrder(t *testing.T, trace []string, events ...string) {
	t.Helper()
	position := -1
	for _, event := range events {
		next := slices.Index(trace[position+1:], event)
		if next < 0 {
			t.Fatalf("trace %v missing ordered %q after %d", trace, event, position)
		}
		position += next + 1
	}
}

func assertRunContractStop(t *testing.T, err error, reason policy.Reason) {
	t.Helper()
	stop, ok := policy.BareStop(err)
	if !ok || stop.Reason != reason {
		t.Fatalf("error=%v want bare stop=%s", err, reason)
	}
}

func assertRunContractFailure(t *testing.T, err error, code status.Code) {
	t.Helper()
	failure, ok := errors.AsType[status.Failure](err)
	if !ok || failure.Code != code {
		t.Fatalf("error=%v want failure=%s", err, code)
	}
}

func TestRunContractCompletedLifetime(t *testing.T) {
	for _, reservation := range []bool{false, true} {
		for _, failed := range []bool{false, true} {
			t.Run(admissionCaseName(reservation, false, failed), func(t *testing.T) { checkRunContractCompletedLifetime(t, reservation, failed) })
		}
	}
}

func TestRunContractInheritedLifetime(t *testing.T) {
	for _, stage := range []string{"completed", "cancelled", "stop-error", "unconfirmed", "launch-error", "launch-owned-error"} {
		t.Run(stage, func(t *testing.T) { checkRunContractInheritedLifetime(t, stage) })
	}
}

func TestRunContractCancelledOwnedLifetime(t *testing.T) {
	for _, stopError := range []error{nil, errRunContract, application.ErrChildRetirementUnconfirmed} {
		t.Run(admissionCaseName(false, stopError != nil, false), func(t *testing.T) { checkRunContractCancelledOwnedLifetime(t, stopError) })
	}
}

func TestRunContractAdmissionFailures(t *testing.T) {
	for _, stage := range []string{"collector", "append", "cancel-before-collect", "cancel-during-collect", "cancel-wait", "cancel-receipt-refused", "capacity", "capacity-receipt-refused", "storage", "resolution-cleanup", "replan", "launch", "launch-owned"} {
		t.Run(stage, func(t *testing.T) { checkRunContractAdmissionFailures(t, stage) })
	}
}

func TestRunContractPrelaunchPortAndEvidenceFailures(t *testing.T) {
	for _, stage := range []string{"cleanup", "join", "port-refused", "port-error", "port-contention", "open", "snapshot-contention", "snapshot-error", "allocation"} {
		t.Run(stage, func(t *testing.T) { checkRunContractPrelaunchPortAndEvidenceFailures(t, stage) })
	}
}

func TestRunContractCoordinationDeferrals(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure error
		reason  policy.Reason
	}{
		{"replan", coordination.ErrReservationReplan, policy.ReasonReplanRequired},
		{"protocol", coordination.ErrCoordinationProtocolMismatch, policy.ReasonProtocolMismatch},
		{"reservation", coordination.ErrReservationDeferred, policy.ReasonCapacityDeferred},
		{"contention", coordination.ErrCoordinationDeferred, policy.ReasonCapacityDeferred},
		{"compatibility-exhaustion", nil, policy.ReasonCapacityDeferred},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRunContractFixture()
			config := fixture.settings()
			config.Policy.LeaseWait = 0
			fixture.begin = func(context.Context, application.RunConfig) (application.Admission, error) {
				return application.Admission{Attempt: func(context.Context, time.Duration, time.Time) (application.AdmissionResult, error) {
					return application.AdmissionResult{Waiting: true}, nil
				}, Close: func() {}}, test.failure
			}
			code, err := fixture.services().Run(context.Background(), config)
			if code != 0 || fixture.starts != 0 || fixture.finalizations != 0 || fixture.releases != 0 {
				t.Fatalf("result=%d %v trace=%v", code, err, fixture.trace)
			}
			assertRunContractStop(t, err, test.reason)
		})
	}
}

func TestRunContractFinalizationAndReleaseFailures(t *testing.T) {
	for _, stage := range []string{"finalize", "cleanup-final", "release", "port-release", "cleanup-deferred", "peak", "peak-contention", "existing-failure"} {
		t.Run(stage, func(t *testing.T) { checkRunContractFinalizationAndReleaseFailures(t, stage) })
	}
}

func checkRunContractCompletedLifetime(t *testing.T, reservation, failed bool) {
	t.Helper()
	fixture := newRunContractFixture()
	config := fixture.settings()
	config.ReservationPolicy.Enabled = reservation
	config.LeasePort = 4321
	config.TaskClass = ""
	config.Now = nil
	config.WorkingDirectory = "workspace"
	fixture.completion = application.ChildCompletion{ExitCode: 7, Failed: failed}
	observed := -1
	config.ObserveChildStatus = func(code int) { observed = code }
	code, err := fixture.services().Run(context.Background(), config)
	outcome := evidence.OutcomePassed
	if failed {
		outcome = evidence.OutcomeTaskFailed
	}
	if err != nil || code != 7 || observed != 7 || fixture.outcome != outcome || fixture.starts != 1 || fixture.stops != 0 || fixture.finalizations != 1 || fixture.cleanups != 2 || fixture.releases != 1 || fixture.portReleases != 1 || fixture.abandoned != 0 || fixture.config.TaskClass != policy.TaskEphemeral || fixture.config.DiskPath != "workspace" {
		t.Fatalf("code=%d err=%v observed=%d fixture=%+v", code, err, observed, fixture)
	}
	assertRunTraceOrder(t, fixture.trace, "validate", "normalize", "cleanup", "begin-admission", "close-admission", "begin-port", "open", "identity", "context", "collect", "append", "start", "ticker", "stop-ticker", "finalize", "cleanup", "release-port", "release")
	if reservation {
		assertRunTraceOrder(t, fixture.trace, "snapshot", "reservation-context", "start", "activate", "peak", "finalize")
		if fixture.config.Resolution.Concurrency != 2 || fixture.observedOwners != 1 {
			t.Fatalf("reservation configuration=%+v owners=%d", fixture.config.Resolution, fixture.observedOwners)
		}
	}
}

func checkRunContractInheritedLifetime(t *testing.T, stage string) {
	t.Helper()
	fixture := newRunContractFixture()
	fixture.inherited = true
	config := fixture.settings()
	config.LeasePort = 4321
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupRunContractInheritedLifetime(fixture, cancel, stage)
	code, err := fixture.services().Run(ctx, config)
	if fixture.starts != 1 || fixture.finalizations != 0 || fixture.collectCalls != 0 || fixture.cleanups != 1 {
		t.Fatalf("inherited trace=%v", fixture.trace)
	}
	switch stage {
	case "completed":
		if code != 0 || err != nil || fixture.stops != 0 {
			t.Fatalf("completed=%d %v", code, err)
		}
	case "cancelled":
		if code != 137 || err != nil || fixture.stops != 1 {
			t.Fatalf("cancelled=%d %v", code, err)
		}
	default:
		if code != 1 || err == nil {
			t.Fatalf("failed=%d %v", code, err)
		}
	}
	retained := stage == "unconfirmed" || stage == "launch-owned-error"
	if retained {
		if fixture.abandoned != 1 || fixture.portAbandoned != 1 || fixture.releases != 0 || fixture.portReleases != 0 {
			t.Fatalf("retention trace=%v", fixture.trace)
		}
	} else if fixture.releases != 1 || fixture.portReleases != 1 {
		t.Fatalf("retirement trace=%v", fixture.trace)
	}
}

func checkRunContractCancelledOwnedLifetime(t *testing.T, stopError error) {
	t.Helper()
	fixture := newRunContractFixture()
	fixture.initialDone = false
	fixture.stopError = stopError
	config := fixture.settings()
	config.LeasePort = 4321
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fixture.startHook = cancel
	observed := -1
	config.ObserveChildStatus = func(code int) { observed = code }
	code, err := fixture.services().Run(ctx, config)
	if fixture.starts != 1 || fixture.stops != 1 || fixture.finalizations != 1 || fixture.outcome != evidence.OutcomeTaskFailed {
		t.Fatalf("trace=%v outcome=%v", fixture.trace, fixture.outcome)
	}
	if stopError == nil {
		if code != 137 || err != nil || observed != 137 {
			t.Fatalf("result=%d %v observed=%d", code, err, observed)
		}
	} else if code != 1 || !errors.Is(err, stopError) || observed != -1 {
		t.Fatalf("failed stop=%d %v observed=%d", code, err, observed)
	}
	if errors.Is(stopError, application.ErrChildRetirementUnconfirmed) {
		if fixture.abandoned != 1 || fixture.portAbandoned != 1 || fixture.releases != 0 || fixture.portReleases != 0 {
			t.Fatalf("ownership not retained: %v", fixture.trace)
		}
	} else if fixture.releases != 1 || fixture.portReleases != 1 {
		t.Fatalf("ownership not retired: %v", fixture.trace)
	}
}

func checkRunContractAdmissionFailures(t *testing.T, stage string) {
	t.Helper()
	fixture := newRunContractFixture()
	config := fixture.settings()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wantOutcome, wantCode := setupRunContractAdmissionFailure(fixture, &config, cancel, stage)
	code, err := fixture.services().Run(ctx, config)
	if code != wantCode || err == nil || fixture.finalizations != 1 || fixture.outcome != wantOutcome || fixture.stops != 0 {
		t.Fatalf("code=%d err=%v outcome=%s want=%s trace=%v", code, err, fixture.outcome, wantOutcome, fixture.trace)
	}
	starts := 0
	if strings.HasPrefix(stage, "launch") {
		starts = 1
	}
	if fixture.starts != starts {
		t.Fatalf("starts=%d want=%d", fixture.starts, starts)
	}
	switch stage {
	case "capacity":
		assertRunContractStop(t, err, policy.ReasonCapacityDeferred)
	case "storage", "resolution-cleanup":
		assertRunContractStop(t, err, policy.ReasonStorageBlocked)
	case "replan":
		assertRunContractStop(t, err, policy.ReasonReplanRequired)
	case "append", "capacity-receipt-refused":
		assertRunContractFailure(t, err, status.CodeEvidenceUnwritable)
	case "cancel-before-collect", "cancel-during-collect", "cancel-wait", "cancel-receipt-refused":
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
	default:
		if !errors.Is(err, errRunContract) {
			t.Fatalf("failure lost: %v", err)
		}
	}
	if strings.HasPrefix(stage, "cancel") || strings.HasPrefix(stage, "capacity") {
		if fixture.receipts != 1 || fixture.receiptStates[0] != "never-started" {
			t.Fatalf("receipt=%v/%v", fixture.receiptStates, fixture.receiptReasons)
		}
	}
	if stage == "launch-owned" {
		if fixture.abandoned != 1 || fixture.releases != 0 {
			t.Fatalf("partial launcher ownership released: %v", fixture.trace)
		}
	} else if fixture.releases != 1 {
		t.Fatalf("unstarted ownership not released: %v", fixture.trace)
	}
}

func checkRunContractPrelaunchPortAndEvidenceFailures(t *testing.T, stage string) {
	t.Helper()
	fixture := newRunContractFixture()
	config := fixture.settings()
	wantCode := 1
	switch stage {
	case "cleanup":
		fixture.cleanupFirstError = errRunContract
		fixture.refused = true
	case "join":
		fixture.beginError = errRunContract
		fixture.refused = true
	case "port-refused", "port-error", "port-contention":
		config.LeasePort = 4321
		fixture.portBeginError = errRunContract
		if stage == "port-refused" {
			fixture.refused = true
		}
		if stage == "port-contention" {
			fixture.portBeginError = coordination.ErrCoordinationDeferred
			wantCode = 0
		}
	case "open":
		fixture.openError = errRunContract
		fixture.refused = true
	case "snapshot-contention", "snapshot-error":
		config.ReservationPolicy.Enabled = true
		fixture.snapshotError = errRunContract
		if stage == "snapshot-contention" {
			fixture.snapshotError = coordination.ErrCoordinationDeferred
			wantCode = 0
		}
	case "allocation":
		config.ReservationPolicy.Enabled = true
		fixture.lease.Allocation.CPU = 0
		wantCode = 0
	}
	code, err := fixture.services().Run(context.Background(), config)
	if code != wantCode || err == nil || fixture.starts != 0 || fixture.finalizations != 0 {
		t.Fatalf("result=%d %v trace=%v", code, err, fixture.trace)
	}
	switch stage {
	case "cleanup", "join", "open":
		assertRunContractFailure(t, err, status.CodeEvidenceUnwritable)
	case "port-refused":
		assertRunContractFailure(t, err, status.CodeLeaseUnwritable)
	case "port-contention", "snapshot-contention":
		assertRunContractStop(t, err, policy.ReasonCapacityDeferred)
	case "allocation":
		if !strings.Contains(err.Error(), "floor") {
			t.Fatalf("allocation error=%v", err)
		}
	default:
		if !errors.Is(err, errRunContract) {
			t.Fatalf("lost error: %v", err)
		}
	}
	if stage != "cleanup" && stage != "join" && fixture.releases != 1 {
		t.Fatalf("rollback releases=%d trace=%v", fixture.releases, fixture.trace)
	}
}

func checkRunContractFinalizationAndReleaseFailures(t *testing.T, stage string) {
	t.Helper()
	fixture := newRunContractFixture()
	config := fixture.settings()
	switch stage {
	case "finalize":
		fixture.finalizeError = errRunContract
	case "cleanup-final":
		fixture.cleanupLastError = errRunContract
	case "release":
		fixture.releaseError = errRunContract
	case "port-release":
		config.LeasePort = 4321
		fixture.portReleaseError = errRunContract
	case "cleanup-deferred":
		fixture.releaseError = coordination.ErrCoordinationCleanupDeferred
	case "peak":
		config.ReservationPolicy.Enabled = true
		fixture.peakError = errRunContract
	case "peak-contention":
		config.ReservationPolicy.Enabled = true
		fixture.peakError = coordination.ErrCoordinationDeferred
	case "existing-failure":
		fixture.startError = context.Canceled
		fixture.finalizeError = errRunContract
		fixture.releaseError = errRunContract
	}
	code, err := fixture.services().Run(context.Background(), config)
	if stage == "cleanup-deferred" || stage == "peak-contention" {
		if code != 0 || err != nil {
			t.Fatalf("contention changed successful outcome: %d %v", code, err)
		}
	} else if code != 1 || err == nil {
		t.Fatalf("failure=%d %v", code, err)
	}
	if stage == "existing-failure" {
		if !errors.Is(err, context.Canceled) || errors.Is(err, errRunContract) {
			t.Fatalf("own error replaced: %v", err)
		}
	} else if stage != "cleanup-deferred" && stage != "peak-contention" && !errors.Is(err, errRunContract) {
		t.Fatalf("original failure lost: %v", err)
	}
	count := 1
	if stage == "peak" {
		count = 0
	}
	if fixture.finalizations != count || fixture.releases != 1 {
		t.Fatalf("finalize=%d releases=%d trace=%v", fixture.finalizations, fixture.releases, fixture.trace)
	}
}

func setupRunContractInheritedLifetime(fixture *runContractFixture, cancel context.CancelFunc, stage string) {
	switch stage {
	case "cancelled", "stop-error", "unconfirmed":
		fixture.initialDone = false
		fixture.startHook = cancel
	case "launch-error", "launch-owned-error":
		fixture.startError = errRunContract
	}
	if stage == "stop-error" {
		fixture.stopError = errRunContract
	}
	if stage == "unconfirmed" {
		fixture.stopError = application.ErrChildRetirementUnconfirmed
	}
	if stage == "launch-owned-error" {
		fixture.startHasWorkload = true
	}
}

func setupRunContractAdmissionFailure(fixture *runContractFixture, config *application.RunConfig, cancel context.CancelFunc, stage string) (evidence.Outcome, int) {
	wantOutcome := evidence.OutcomeAdmissionFailed
	wantCode := 1
	switch stage {
	case "collector":
		fixture.collectErrorAt = 1
		fixture.collectError = errRunContract
	case "append":
		fixture.appendErrorAt = 1
		fixture.appendError = errRunContract
		fixture.refused = true
	case "cancel-before-collect":
		fixture.begin = func(context.Context, application.RunConfig) (application.Admission, error) {
			cancel()
			return application.Admission{Admitted: &fixture.lease, Close: func() {}}, nil
		}
		wantOutcome = evidence.OutcomeAdmissionCancelled
	case "cancel-during-collect":
		fixture.collectErrorAt = 1
		fixture.collectError = errRunContract
		fixture.collectHook = func(int, context.Context, *policy.Reading) { cancel() }
		wantOutcome = evidence.OutcomeAdmissionCancelled
	case "cancel-wait", "cancel-receipt-refused":
		fixture.collectHook = func(_ int, _ context.Context, reading *policy.Reading) {
			reading.Sample.CPUUtilizationPercent = new(99.0)
		}
		fixture.waitError = context.Canceled
		wantOutcome = evidence.OutcomeAdmissionCancelled
		if stage == "cancel-receipt-refused" {
			fixture.receiptError = errRunContract
			fixture.refused = true
			wantOutcome = evidence.OutcomeAdmissionFailed
		}
	case "capacity", "capacity-receipt-refused":
		config.Policy.AdmissionWindow = 0
		fixture.collectHook = func(_ int, _ context.Context, reading *policy.Reading) {
			reading.Sample.CPUUtilizationPercent = new(99.0)
		}
		wantOutcome = evidence.OutcomeCapacityDeferred
		wantCode = 0
		if stage == "capacity-receipt-refused" {
			fixture.receiptError = errRunContract
			fixture.refused = true
			wantOutcome = evidence.OutcomeAdmissionFailed
			wantCode = 1
		}
	case "storage":
		fixture.collectHook = func(_ int, _ context.Context, reading *policy.Reading) {
			reading.Sample.DiskFreeBytes = new(10 * policy.GiB)
		}
		wantOutcome = evidence.OutcomeStorageBlocked
		wantCode = 0
	case "resolution-cleanup":
		config.Resolution.Decision = policy.DecisionCleanup
		config.Resolution.Reason = policy.ReasonStorageBlocked
		wantOutcome = evidence.OutcomeStorageBlocked
		wantCode = 0
	case "replan":
		config.Resolution.Decision = policy.DecisionReplan
		wantCode = 0
	case "launch", "launch-owned":
		fixture.startError = errRunContract
		fixture.startHasWorkload = stage == "launch-owned"
	}
	return wantOutcome, wantCode
}

func TestRunContractDegradedAdmissionKeepsReservationAllocation(t *testing.T) {
	for _, reservation := range []bool{false, true} {
		t.Run(admissionCaseName(reservation, false, false), func(t *testing.T) {
			fixture := newRunContractFixture()
			config := fixture.settings()
			config.ReservationPolicy.Enabled = reservation
			config.ConcurrencyEnvironment = []string{"JOBS"}
			config.Environment = []string{"JOBS=2"}
			config.Policy.TrendWindow = 2 * time.Second
			fixture.collectHook = func(_ int, _ context.Context, reading *policy.Reading) { reading.Sample.MemoryPressureLevel = new(2) }
			code, err := fixture.services().Run(context.Background(), config)
			want := 1
			if reservation {
				want = fixture.lease.Allocation.CPU
			}
			if code != 0 || err != nil || fixture.starts != 1 || fixture.waits != 2 || fixture.finalizations != 1 || fixture.outcome != evidence.OutcomePassed || fixture.config.Resolution.Concurrency != want || application.EnvironmentValue(fixture.config.Environment, "JOBS") != string(rune('0'+want)) {
				t.Fatalf("degraded=%d %v resolution=%+v env=%v trace=%v", code, err, fixture.config.Resolution, fixture.config.Environment, fixture.trace)
			}
			assertRunTraceOrder(t, fixture.trace, "collect", "wait", "collect", "wait", "collect", "context", "start")
		})
	}
}
