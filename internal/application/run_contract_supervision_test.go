package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

func tickRunContract(fixture *runContractFixture) {
	fixture.initialDone = false
	fixture.startHook = func() { fixture.ticks <- fixture.now }
}

func criticalRunContractReading(reading *policy.Reading) {
	reading.Sample.AvailableMemoryBytes = new(policy.GiB)
	reading.Sample.MemoryPressureLevel = new(4)
}

func postlaunchContractReading(fixture *runContractFixture, modify func(*policy.Reading)) {
	fixture.collectHook = func(call int, _ context.Context, reading *policy.Reading) {
		if call > 1 {
			modify(reading)
		}
	}
}

func TestRunContractSelectedOwnerStopsBeforeNewSample(t *testing.T) {
	for _, test := range []struct {
		name    string
		class   policy.TaskClass
		cause   coordination.ShedCause
		outcome evidence.Outcome
	}{
		{"pressure", policy.TaskEphemeral, coordination.ShedCausePressure, evidence.OutcomePressureShed},
		{"storage", policy.TaskService, coordination.ShedCauseStorage, evidence.OutcomeStorageShed},
		{"transactional", policy.TaskTransactional, coordination.ShedCausePressure, evidence.OutcomeEmergencySafetyStop},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRunContractFixture()
			tickRunContract(fixture)
			fixture.selected = true
			fixture.shedCause = test.cause
			config := fixture.settings()
			config.ReservationPolicy.Enabled = true
			config.TaskClass = test.class
			code, err := fixture.services().Run(context.Background(), config)
			assertRunContractStop(t, err, test.cause.Reason())
			if code != 0 || fixture.starts != 1 || fixture.stops != 1 || fixture.collectCalls != 1 || fixture.outcome != test.outcome || fixture.finalizations != 1 || fixture.releases != 1 {
				t.Fatalf("code=%d outcome=%s trace=%v", code, fixture.outcome, fixture.trace)
			}
			assertRunTraceOrder(t, fixture.trace, "start", "activate", "ticker", "selection", "stop", "stop-ticker", "finalize", "release")
			if test.class == policy.TaskTransactional && (fixture.receipts != 1 || fixture.receiptStates[0] != "started-safety-stop" || fixture.receiptReasons[0] != "emergency-pressure") {
				t.Fatalf("receipt=%v/%v", fixture.receiptStates, fixture.receiptReasons)
			}
		})
	}
}

func TestRunContractActivationFailureNeverClaimsDeferral(t *testing.T) {
	for _, contention := range []bool{false, true} {
		fixture := newRunContractFixture()
		fixture.initialDone = false
		config := fixture.settings()
		config.ReservationPolicy.Enabled = true
		fixture.activationError = errRunContract
		reason := "coordination-activation-failure"
		if contention {
			fixture.activationError = coordination.ErrCoordinationDeferred
			reason = "coordination-contention"
		}
		fixture.stopError = errRunContract
		fixture.receiptError = errRunContract
		code, err := fixture.services().Run(context.Background(), config)
		assertRunContractFailure(t, err, status.CodeSupervisionFailed)
		if code != 125 || fixture.starts != 1 || fixture.stops != 1 || fixture.outcome != evidence.OutcomeTaskFailed || fixture.receipts != 1 || fixture.receiptStates[0] != "started-activation-failure" || fixture.receiptReasons[0] != reason || fixture.finalizations != 1 {
			t.Fatalf("code=%d err=%v trace=%v", code, err, fixture.trace)
		}
	}
}

func TestRunContractPostlaunchReadAndWriteFailures(t *testing.T) {
	for _, stage := range []string{"selection", "collect", "typed-collect", "append", "peak", "collect-cancelled", "collect-cancelled-stop-error"} {
		t.Run(stage, func(t *testing.T) { checkRunContractPostlaunchReadAndWriteFailures(t, stage) })
	}
}

func TestRunContractSampleRetentionAndContendedMetadata(t *testing.T) {
	for _, mode := range []string{"compatibility", "reservation-contention", "reservation-cancelled-peak", "transactional"} {
		t.Run(mode, func(t *testing.T) { checkRunContractSampleRetentionAndContendedMetadata(t, mode) })
	}
}

func TestRunContractPressureAndStorageStops(t *testing.T) {
	for _, mode := range []string{"compatibility-critical", "service-warning", "compatibility-storage", "reservation-self", "reservation-emergency", "reservation-fallback-emergency", "victim-error", "remote-error", "no-victim", "victim-contention", "remote-retired"} {
		t.Run(mode, func(t *testing.T) { checkRunContractPressureAndStorageStops(t, mode) })
	}
}

type runContractContextKey struct{}

func TestRunContractVictimCleanupPreservesValuesAndBounds(t *testing.T) {
	for _, stage := range []string{"retired", "observe-error", "wait-error", "two-observations"} {
		t.Run(stage, func(t *testing.T) { checkRunContractVictimCleanupPreservesValuesAndBounds(t, stage) })
	}
}

func checkRunContractPostlaunchReadAndWriteFailures(t *testing.T, stage string) {
	t.Helper()
	fixture := newRunContractFixture()
	tickRunContract(fixture)
	config := fixture.settings()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupRunContractPostlaunchFailures(fixture, &config, cancel, stage)
	code, err := fixture.services().Run(ctx, config)
	expected := evidence.OutcomeSupervisionFailed
	if strings.HasPrefix(stage, "collect-cancelled") {
		expected = evidence.OutcomeTaskFailed
	}
	finalized := 1
	if stage == "peak" {
		finalized = 0
	}
	if fixture.starts != 1 || fixture.stops != 1 || fixture.outcome != expected && finalized != 0 || fixture.finalizations != finalized || fixture.releases != 1 || fixture.receipts != 0 {
		t.Fatalf("outcome=%s trace=%v", fixture.outcome, fixture.trace)
	}
	switch stage {
	case "collect-cancelled":
		if code != 137 || err != nil {
			t.Fatalf("cancelled=%d %v", code, err)
		}
	case "typed-collect":
		if code != 1 || err == nil || strings.Contains(err.Error(), string(status.CodeEvidenceUnwritable)) {
			t.Fatalf("postlaunch error falsely named prelaunch refusal: %d %v", code, err)
		}
	default:
		if code != 1 || !errors.Is(err, errRunContract) {
			t.Fatalf("original fault=%d %v", code, err)
		}
	}
}

func checkRunContractSampleRetentionAndContendedMetadata(t *testing.T, mode string) {
	t.Helper()
	fixture := newRunContractFixture()
	fixture.initialDone = false
	config := fixture.settings()
	config.Policy.TrendWindow = 0
	config.ReservationPolicy.Enabled = strings.HasPrefix(mode, "reservation")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fixture.startHook = func() {
		for range 4 {
			fixture.ticks <- fixture.now
		}
	}
	fixture.collectHook = func(call int, _ context.Context, reading *policy.Reading) {
		if mode == "transactional" && call > 1 {
			criticalRunContractReading(reading)
		}
		// Cancellation must not compete with an unrelated synthetic healthy completion.
		if call == 5 && mode != "reservation-cancelled-peak" {
			fixture.done <- application.ChildCompletion{}
		}
	}
	if mode == "transactional" {
		config.TaskClass = policy.TaskTransactional
	}
	if mode == "reservation-contention" {
		fixture.selectionError = coordination.ErrCoordinationDeferred
		fixture.peakError = coordination.ErrCoordinationDeferred
	}
	if mode == "reservation-cancelled-peak" {
		observed := false
		fixture.peakHook = func(ctx context.Context) (int, error) {
			if !observed && fixture.collectCalls == 2 && ctx.Err() == nil {
				observed = true
				cancel()
				return 0, errRunContract
			}
			return 2, nil
		}
	}
	code, err := fixture.services().Run(ctx, config)
	if err != nil || fixture.starts != 1 || fixture.finalizations != 1 || fixture.releases != 1 {
		t.Fatalf("code=%d err=%v trace=%v", code, err, fixture.trace)
	}
	if mode == "reservation-cancelled-peak" {
		if code != 137 || fixture.stops != 1 {
			t.Fatalf("cancelled peak=%d stops=%d", code, fixture.stops)
		}
	} else if code != 0 || fixture.stops != 0 || fixture.appends != 5 || fixture.outcome != evidence.OutcomePassed {
		t.Fatalf("healthy/transactional=%d outcome=%s trace=%v", code, fixture.outcome, fixture.trace)
	}
}

func checkRunContractPressureAndStorageStops(t *testing.T, mode string) {
	t.Helper()
	fixture := newRunContractFixture()
	tickRunContract(fixture)
	config := fixture.settings()
	config.Policy.TrendWindow = 0
	config.Policy.ServiceWarningGrace = 0
	config.ReservationPolicy.Enabled = !strings.HasPrefix(mode, "compatibility") && mode != "service-warning"
	postlaunchContractReading(fixture, criticalRunContractReading)
	fixture.victimSelected = true
	fixture.victim = coordination.ReservationOwner{Token: fixture.lease.Token, Class: policy.TaskEphemeral}

	setupRunContractPressureStop(fixture, &config, mode)
	code, err := fixture.services().Run(context.Background(), config)
	if fixture.starts != 1 || fixture.finalizations != 1 || fixture.releases != 1 {
		t.Fatalf("lifetime trace=%v", fixture.trace)
	}
	switch mode {
	case "victim-error", "remote-error":
		if code != 1 || !errors.Is(err, errRunContract) || fixture.stops != 1 {
			t.Fatalf("fault=%d %v trace=%v", code, err, fixture.trace)
		}
	case "no-victim", "victim-contention", "remote-retired":
		if code != 0 || err != nil || fixture.stops != 0 || fixture.outcome != evidence.OutcomePassed {
			t.Fatalf("peer/contended election stopped local work: %d %v trace=%v", code, err, fixture.trace)
		}
	default:
		reason := policy.ReasonPressureShed
		outcome := evidence.OutcomePressureShed
		if mode == "compatibility-storage" {
			reason = policy.ReasonStorageBlocked
			outcome = evidence.OutcomeStorageShed
		}
		if strings.HasSuffix(mode, "emergency") {
			outcome = evidence.OutcomeEmergencySafetyStop
			if fixture.receipts != 1 || !strings.Contains(strings.Join(fixture.trace, " "), "emergency-victim") {
				t.Fatalf("emergency receipt/selection trace=%v", fixture.trace)
			}
		}
		assertRunContractStop(t, err, reason)
		if code != 0 || fixture.stops != 1 || fixture.outcome != outcome {
			t.Fatalf("shed=%d %v outcome=%s want=%s trace=%v", code, err, fixture.outcome, outcome, fixture.trace)
		}
	}
}

func checkRunContractVictimCleanupPreservesValuesAndBounds(t *testing.T, stage string) {
	t.Helper()
	fixture := newRunContractFixture()
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), runContractContextKey{}, "identity"))
	cancel()
	seen := 0
	fixture.presentHook = func(ctx context.Context) (bool, error) {
		seen++
		if ctx.Err() != nil || ctx.Value(runContractContextKey{}) != "identity" {
			t.Fatalf("fresh cleanup context lost parent value: %v", ctx)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Fatalf("unbounded cleanup deadline=%v", deadline)
		}
		switch stage {
		case "observe-error":
			return false, errRunContract
		case "retired":
			return false, nil
		case "two-observations":
			return seen < 2, nil
		default:
			return true, nil
		}
	}
	if stage == "wait-error" {
		fixture.waitError = context.DeadlineExceeded
	}
	err := fixture.services().WaitVictimRelease(parent, "root", coordination.ReservationOwner{Token: "peer"}, 200*time.Millisecond)
	switch stage {
	case "observe-error":
		if !errors.Is(err, errRunContract) {
			t.Fatal(err)
		}
	case "wait-error":
		if err == nil || !strings.Contains(err.Error(), "bounded observation") {
			t.Fatal(err)
		}
	default:
		if err != nil {
			t.Fatal(err)
		}
	}
	want := 0
	if stage == "wait-error" || stage == "two-observations" {
		want = 1
	}
	if fixture.waits != want {
		t.Fatalf("waits=%d want=%d", fixture.waits, want)
	}
}

func setupRunContractPostlaunchFailures(fixture *runContractFixture, config *application.RunConfig, cancel context.CancelFunc, stage string) {
	switch stage {
	case "selection":
		config.ReservationPolicy.Enabled = true
		fixture.selectionError = errRunContract
	case "collect", "typed-collect", "collect-cancelled", "collect-cancelled-stop-error":
		fixture.collectErrorAt = 2
		fixture.collectError = errRunContract
		if stage == "typed-collect" {
			fixture.collectError = status.Fail(status.CodeEvidenceUnwritable, "host evidence became unreadable")
		}
		if strings.HasPrefix(stage, "collect-cancelled") {
			postlaunchContractReading(fixture, func(*policy.Reading) { cancel() })
		}
		if stage == "collect-cancelled-stop-error" {
			fixture.stopError = errRunContract
		}
	case "append":
		fixture.appendErrorAt = 2
		fixture.appendError = errRunContract
	case "peak":
		config.ReservationPolicy.Enabled = true
		fixture.peakError = errRunContract
	}
}

func setupRunContractPressureStop(fixture *runContractFixture, config *application.RunConfig, mode string) {
	switch mode {
	case "service-warning":
		config.TaskClass = policy.TaskService
		postlaunchContractReading(fixture, func(reading *policy.Reading) {
			reading.Sample.AvailableMemoryBytes = new(8 * policy.GiB)
			reading.Sample.MemoryPressureLevel = new(2)
		})
	case "compatibility-storage":
		postlaunchContractReading(fixture, func(reading *policy.Reading) { reading.Sample.DiskFreeBytes = new(10 * policy.GiB) })
	case "reservation-emergency", "reservation-fallback-emergency":
		config.TaskClass = policy.TaskTransactional
		fixture.victim.Class = policy.TaskTransactional
		config.EmergencyAvailableMemoryBytes = 2 * policy.GiB
		if mode == "reservation-fallback-emergency" {
			postlaunchContractReading(fixture, func(reading *policy.Reading) {
				criticalRunContractReading(reading)
				reading.Sample.AvailableMemoryBytes = nil
				reading.Sample.AvailableNonCompressedEstimateBytes = new(policy.GiB)
			})
		}
	case "victim-error":
		fixture.victimError = errRunContract
	case "remote-error":
		fixture.victim.Token = "peer"
		fixture.presentError = errRunContract
	case "no-victim", "victim-contention":
		fixture.victimHook = func() (coordination.ReservationOwner, bool, error) {
			fixture.done <- application.ChildCompletion{}
			if mode == "victim-contention" {
				return coordination.ReservationOwner{}, false, coordination.ErrCoordinationDeferred
			}
			return coordination.ReservationOwner{}, false, nil
		}
	case "remote-retired":
		fixture.victim.Token = "peer"
		fixture.presentHook = func(context.Context) (bool, error) { fixture.done <- application.ChildCompletion{}; return false, nil }
	}
}
