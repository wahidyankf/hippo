package status_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/wahidyankf/hippo/internal/status"
)

func TestStatusContractPublishedCodesHaveDeclaredStatusesAndRetryAdvice(t *testing.T) {
	cases := []struct {
		code      status.Code
		exit      int
		retryable bool
	}{
		{status.CodeArgsInvalid, status.CallerError, false},
		{status.CodeConfigUnreadable, status.GuardFailed, false},
		{status.CodeConfigUnresolvable, status.GuardFailed, false},
		{status.CodeLimitCapacityDeferred, status.LimitShed, true},
		{status.CodeLimitStorageBlocked, status.LimitShed, false},
		{status.CodeLimitPressureShed, status.LimitShed, true},
		{status.CodeLimitReleaseEnvelopeExceeded, status.LimitShed, false},
		{status.CodeIdentityInvalid, status.GuardFailed, false},
		{status.CodePolicyReplanRequired, status.GuardFailed, false},
		{status.CodeCoordinationProtocolMismatch, status.GuardFailed, false},
		{status.CodeChildNotFound, status.ChildNotFound, false},
		{status.CodeChildNotExecutable, status.ChildNotExecutable, false},
		{status.CodeHostUnreadable, status.GuardFailed, false},
		{status.CodeEvidenceUnwritable, status.GuardFailed, false},
		{status.CodeEvidenceUnreadable, status.GuardFailed, false},
		{status.CodeLeaseUnwritable, status.GuardFailed, false},
		{status.CodeSupervisionFailed, status.GuardFailed, false},
		{status.CodeInternalFailure, status.CallerError, false},
	}
	if len(cases) != len(status.All) {
		t.Fatalf("published vocabulary=%d, documented cases=%d", len(status.All), len(cases))
	}
	seen := map[status.Code]bool{}
	for _, test := range cases {
		t.Run(string(test.code), func(t *testing.T) {
			if !status.Known(test.code) || status.Status(test.code) != test.exit || status.Retryable(test.code) != test.retryable {
				t.Fatalf("known=%t exit=%d retryable=%t", status.Known(test.code), status.Status(test.code), status.Retryable(test.code))
			}
			seen[test.code] = true
		})
	}
	for _, code := range status.All {
		if !seen[code] {
			t.Fatalf("published code missing semantic expectation: %s", code)
		}
	}
	for _, code := range []status.Code{"", "hippo.future.reason"} {
		if status.Known(code) || status.Status(code) != status.CallerError || status.Retryable(code) {
			t.Fatalf("unknown code=%q known=%t exit=%d retryable=%t", code, status.Known(code), status.Status(code), status.Retryable(code))
		}
	}
}

func TestStatusContractFailurePreservesStructuredFieldsAndHumanDescription(t *testing.T) {
	failure := status.Fail(status.CodeLimitCapacityDeferred, "capacity needs %d more CPU units", 2)
	failure.Field = "--reserve-cpu"
	want := "hippo: [hippo.limit.capacity-deferred] capacity needs 2 more CPU units"
	if failure.Error() != want || failure.Status() != status.LimitShed || !failure.Retryable() {
		t.Fatalf("description=%q status=%d retryable=%t", failure.Error(), failure.Status(), failure.Retryable())
	}
	wrapped := fmt.Errorf("request refused: %w", failure)
	recovered, ok := errors.AsType[status.Failure](wrapped)
	if !ok || recovered != failure || recovered.Field != "--reserve-cpu" {
		t.Fatalf("structured failure lost: %+v found=%t", recovered, ok)
	}
	permanent := status.Fail(status.CodeLimitStorageBlocked, "storage reserve unavailable")
	if permanent.Retryable() || permanent.Field != "" || permanent.Message != "storage reserve unavailable" {
		t.Fatalf("permanent failure=%+v retryable=%t", permanent, permanent.Retryable())
	}
}

func TestStatusContractInterruptionNamesAndShellStatuses(t *testing.T) {
	cases := []struct {
		name         string
		interruption status.Interruption
		description  string
		exit         int
	}{
		{"interrupt", status.Interruption{Signal: 2}, "interrupt signal received", 130},
		{"termination", status.Interruption{Signal: 15}, "terminated signal received", 143},
		{"unnamed other signal", status.Interruption{Signal: 9}, "signal 9 signal received", 137},
		{"provided name", status.Interruption{Signal: 2, Name: "operator stop"}, "operator stop signal received", 130},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.interruption.Error() != test.description || test.interruption.Status() != test.exit {
				t.Fatalf("description=%q status=%d", test.interruption.Error(), test.interruption.Status())
			}
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			cancel(fmt.Errorf("process boundary: %w", test.interruption))
			actual, interrupted := status.Interrupted(ctx)
			if !interrupted || actual != test.interruption {
				t.Fatalf("context interruption=%+v found=%t", actual, interrupted)
			}
		})
	}
}

func TestStatusContractOrdinaryContextCancellationIsNotASignal(t *testing.T) {
	if actual, ok := status.Interrupted(context.Background()); ok || actual != (status.Interruption{}) {
		t.Fatalf("active context interruption=%+v found=%t", actual, ok)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if actual, ok := status.Interrupted(ctx); ok || actual != (status.Interruption{}) {
		t.Fatalf("ordinary cancellation=%+v found=%t", actual, ok)
	}
	ctx, cancelCause := context.WithCancelCause(context.Background())
	defer cancelCause(nil)
	cancelCause(status.Fail(status.CodeSupervisionFailed, "probe failed"))
	if actual, ok := status.Interrupted(ctx); ok || actual != (status.Interruption{}) {
		t.Fatalf("classified failure misread as signal=%+v found=%t", actual, ok)
	}
}
