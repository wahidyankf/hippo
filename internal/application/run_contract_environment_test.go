package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

func TestRunContractRejectsInvalidInputsBeforeEffects(t *testing.T) {
	for _, stage := range []string{"cancelled", "command", "executable", "class", "sample", "termination", "admission", "lease", "collector", "environment", "reservation-environment"} {
		t.Run(stage, func(t *testing.T) { checkRunContractRejectsInvalidInputsBeforeEffects(t, stage) })
	}
}

func TestRunContractDefaultPolicyAndDiskPath(t *testing.T) {
	fixture := newRunContractFixture()
	config := fixture.settings()
	config.Policy = policy.Policy{}
	config.Resolution = policy.Resolution{}
	config.Now = nil
	code, err := fixture.services().Run(context.Background(), config)
	if code != 0 || err != nil || fixture.config.DiskPath != "." || fixture.config.Policy.SampleInterval != policy.DefaultPolicy().SampleInterval || fixture.waits != 2 {
		t.Fatalf("result=%d %v config=%+v waits=%d", code, err, fixture.config, fixture.waits)
	}
}

func TestRunContractEnvironmentNamesAndDuplicateKeys(t *testing.T) {
	for _, name := range []string{"", "9COUNT", "COUNT-", "COUNTé", "HIPPO_CONCURRENCY"} {
		t.Run(name, func(t *testing.T) {
			_, err := application.NormalizeConcurrencyEnvironment([]string{name})
			assertRunContractFailure(t, err, status.CodeArgsInvalid)
		})
	}
	actual, err := application.NormalizeConcurrencyEnvironment([]string{"_jobs9", "Jobs", "_jobs9"})
	if err != nil || !reflect.DeepEqual(actual, []string{"_jobs9", "Jobs"}) {
		t.Fatalf("names=%v err=%v", actual, err)
	}
	original := []string{"OTHER=a", "JOBS=8", "JOBS_EXTRA=7", "JOBS=2"}
	if application.EnvironmentValue(original, "JOBS") != "2" {
		t.Fatal("child's last-value semantics lost")
	}
	replaced := application.WithEnvironment(original, "JOBS", "3")
	if !reflect.DeepEqual(replaced, []string{"OTHER=a", "JOBS_EXTRA=7", "JOBS=3"}) || original[1] != "JOBS=8" {
		t.Fatalf("replacement=%v original=%v", replaced, original)
	}
	if !reflect.DeepEqual(application.WithEnvironmentIfMissing(original, "JOBS", "4"), original) || application.EnvironmentValue(application.WithEnvironmentIfMissing([]string{"JOBS="}, "JOBS", "4"), "JOBS") != "4" {
		t.Fatal("missing default semantics changed")
	}
}

func TestRunContractResolvedAndReservationEnvironment(t *testing.T) {
	resolution := policy.Resolution{ResolvedProfile: "balanced", Concurrency: 4}
	for _, force := range []bool{false, true} {
		environment := application.ResolvedEnvironment([]string{"JOBS=2"}, resolution, force, []string{"JOBS", "WORKERS"})
		jobs := "2"
		if force {
			jobs = "4"
		}
		if application.EnvironmentValue(environment, "JOBS") != jobs || application.EnvironmentValue(environment, "WORKERS") != "4" || application.EnvironmentValue(environment, "HIPPO_PROFILE") != "balanced" || application.EnvironmentValue(environment, "HIPPO_CONCURRENCY") != "4" {
			t.Fatalf("resolved=%v", environment)
		}
	}
	for _, resolution := range []policy.Resolution{{Concurrency: 4}, {ResolvedProfile: "balanced"}} {
		if actual := application.ResolvedEnvironment([]string{"OTHER=a"}, resolution, true, nil); !reflect.DeepEqual(actual, []string{"OTHER=a"}) {
			t.Fatalf("empty resolution mutated env=%v", actual)
		}
	}
	allocation := coordination.ReservationVector{CPU: 3, MemoryBytes: policy.GiB}
	for _, value := range []string{"", "1", "3", "9", "0", "-1", "all"} {
		t.Run(value, func(t *testing.T) {
			environment, err := application.ReservationEnvironment([]string{"JOBS=" + value}, resolution, allocation, []string{"JOBS", "WORKERS"})
			invalid := value == "0" || value == "-1" || value == "all"
			if invalid {
				if err == nil || !strings.Contains(err.Error(), "positive integer") {
					t.Fatalf("invalid=%v", err)
				}
				return
			}
			want := value
			if value == "" || value == "9" {
				want = "3"
			}
			if err != nil || application.EnvironmentValue(environment, "JOBS") != want || application.EnvironmentValue(environment, "WORKERS") != "3" || application.EnvironmentValue(environment, "HIPPO_CONCURRENCY") != "3" || application.EnvironmentValue(environment, "HIPPO_RESERVED_MEMORY_BYTES") != "1073741824" {
				t.Fatalf("allocation env=%v err=%v", environment, err)
			}
		})
	}
	for _, allocation := range []coordination.ReservationVector{{CPU: 0, MemoryBytes: policy.GiB}, {CPU: 1, MemoryBytes: 1}} {
		if _, err := application.ReservationEnvironment(nil, resolution, allocation, nil); err == nil {
			t.Fatal("allocation below floor accepted")
		}
	}
}

func TestRunContractOutcomeAndFailurePromotion(t *testing.T) {
	for _, outcome := range []evidence.Outcome{evidence.OutcomeUnset, evidence.OutcomePassed, evidence.OutcomeTaskFailed, evidence.OutcomeCapacityDeferred} {
		actual, err := application.FinalOutcome(outcome)
		if outcome == evidence.OutcomeUnset {
			if actual != evidence.OutcomeSupervisionFailed {
				t.Fatal(actual)
			}
			assertRunContractFailure(t, err, status.CodeSupervisionFailed)
		} else if actual != outcome || err != nil {
			t.Fatalf("outcome=%v err=%v", actual, err)
		}
	}
}

func TestRunContractReleaseFailurePromotion(t *testing.T) {
	for _, reason := range []policy.Reason{policy.ReasonNone, policy.ReasonStorageBlocked, policy.ReasonCapacityDeferred, policy.ReasonReplanRequired, policy.ReasonPressureShed} {
		var own error
		if reason != policy.ReasonNone {
			own = policy.Stopped(reason, nil)
		}
		code, err := application.PromoteRelease(0, own, errRunContract)
		if reason == policy.ReasonStorageBlocked || reason == policy.ReasonCapacityDeferred {
			stop, ok := errors.AsType[*policy.Stop](err)
			if code != 0 || !ok || stop.Reason != reason || !errors.Is(err, errRunContract) {
				t.Fatalf("preserved=%d %v", code, err)
			}
		} else if code != 1 || !errors.Is(err, errRunContract) {
			t.Fatalf("promoted=%d %v", code, err)
		}
	}
	for _, release := range []error{nil, errRunContract} {
		code, err := application.PromoteRelease(7, context.Canceled, release)
		if code != 7 || !errors.Is(err, context.Canceled) {
			t.Fatalf("own failure=%d %v", code, err)
		}
	}
}

func TestRunContractFinalizeFailurePromotion(t *testing.T) {
	fixture := newRunContractFixture()
	fixture.refused = true
	for _, launched := range []bool{false, true} {
		code, err := fixture.services().PromoteFinalize(0, nil, errRunContract, launched)
		if code != 1 {
			t.Fatal(code)
		}
		if launched {
			if !errors.Is(err, errRunContract) {
				t.Fatal(err)
			}
		} else {
			assertRunContractFailure(t, err, status.CodeEvidenceUnwritable)
		}
	}
	code, err := fixture.services().PromoteFinalize(7, context.Canceled, errRunContract, false)
	if code != 7 || !errors.Is(err, context.Canceled) {
		t.Fatalf("own failure=%d %v", code, err)
	}
	code, err = fixture.services().PromoteFinalize(7, nil, nil, true)
	if code != 7 || err != nil {
		t.Fatalf("no failure=%d %v", code, err)
	}
}

func checkRunContractRejectsInvalidInputsBeforeEffects(t *testing.T, stage string) {
	t.Helper()
	fixture := newRunContractFixture()
	config := fixture.settings()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wantCode := 1
	switch stage {
	case "cancelled":
		cancel()
	case "command":
		config.Command = ""
	case "executable":
		failure := status.Fail(status.CodeArgsInvalid, "invalid command")
		fixture.validateFailure = &failure
		wantCode = failure.Status()
	case "class":
		config.TaskClass = "invalid"
	case "sample":
		config.Policy.SampleInterval = -1
	case "termination":
		config.Policy.TerminationGrace = -1
	case "admission":
		config.Policy.AdmissionWindow = -1
	case "lease":
		config.Policy.LeaseWait = -1
	case "collector":
		config.Collector = nil
	case "environment":
		config.ConcurrencyEnvironment = []string{"bad-name"}
	case "reservation-environment":
		config.ConcurrencyEnvironment = []string{"HIPPO_PRIVATE"}
		config.ReservationPolicy.Enabled = true
		wantCode = 0
	}
	code, err := fixture.services().Run(ctx, config)
	if code != wantCode || err == nil || fixture.starts != 0 || fixture.cleanups != 0 || fixture.closed != 0 {
		t.Fatalf("result=%d %v trace=%v", code, err, fixture.trace)
	}
	if stage == "reservation-environment" {
		stop, ok := errors.AsType[*policy.Stop](err)
		if !ok || stop.Reason != policy.ReasonReplanRequired {
			t.Fatalf("stop=%v", err)
		}
	}
}
