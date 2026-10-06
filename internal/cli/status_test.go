package cli //nolint:testpackage // classify and reasonCode are the command-line boundary's own seams.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// reasonReports is what the boundary says for each internal reason: the code,
// the exit status, and the sentence a stop with no error of its own carries.
// They are the v0.8.4 statuses, codes, and messages, which docs/reference/
// exit-codes.md publishes and the contract scenarios hold.
var reasonReports = []struct {
	reason  policy.Reason
	code    status.Code
	status  int
	message string
}{
	{
		policy.ReasonStorageBlocked, status.CodeLimitStorageBlocked, status.LimitShed,
		"the disk floor stopped this work; free space before retrying",
	},
	{
		policy.ReasonCapacityDeferred, status.CodeLimitCapacityDeferred, status.LimitShed,
		"capacity deferred this work; retry when the host is quieter",
	},
	{
		policy.ReasonPressureShed, status.CodeLimitPressureShed, status.LimitShed,
		"host pressure shed this work after the payload ran; recover it before repeating anything",
	},
	{
		policy.ReasonProtocolMismatch, status.CodeCoordinationProtocolMismatch, status.GuardFailed,
		"live peer coordination state this client cannot safely join",
	},
	{
		policy.ReasonReplanRequired, status.CodePolicyReplanRequired, status.GuardFailed,
		"no profile admits this request as asked for",
	},
}

func TestEveryReasonKeepsItsCodeStatusAndMessage(t *testing.T) {
	for _, row := range reasonReports {
		if code := reasonCode(row.reason); code != row.code {
			t.Errorf("reasonCode(%d) = %s, want %s", row.reason, code, row.code)
		}
		if exit := status.Status(reasonCode(row.reason)); exit != row.status {
			t.Errorf("reason %d exits %d, want %d", row.reason, exit, row.status)
		}
		if message := reasonMessage(row.reason); message != row.message {
			t.Errorf("reasonMessage(%d) = %q, want %q", row.reason, message, row.message)
		}

		failure := classify(&commandExecution{}, policy.Stopped(row.reason, nil))
		if failure == nil || failure.Code != row.code || failure.Status() != row.status || failure.Message != row.message {
			t.Errorf("a bare stop for reason %d reports %+v, want %s, %d, and %q", row.reason, failure, row.code, row.status, row.message)
		}
	}
}

func TestAStopReportsItsCauseWhenItHasOne(t *testing.T) {
	cause := errors.New("verify schema-3 activation: legacy owners remain")
	failure := classify(&commandExecution{}, policy.Stopped(policy.ReasonProtocolMismatch, cause))
	if failure == nil || failure.Code != status.CodeCoordinationProtocolMismatch || failure.Message != cause.Error() {
		t.Errorf("a stop with a cause reports %+v, want the protocol mismatch carrying %q", failure, cause)
	}

	wrapped := fmt.Errorf("outer: %w", policy.Stopped(policy.ReasonPressureShed, cause))
	failure = classify(&commandExecution{}, wrapped)
	if failure == nil || failure.Code != status.CodeLimitPressureShed || failure.Message != wrapped.Error() {
		t.Errorf("a stop inside a wrapping error reports %+v, want the pressure shed carrying %q", failure, wrapped)
	}
}

func TestAStopInsideAWrapperReportsTheWrappersText(t *testing.T) {
	// A wrapper says more than the stop it wraps, so the stop is not bare: the
	// failure carries the wrapper's text, and the result is not read as no error.
	wrapped := fmt.Errorf("release: %w", policy.Stopped(policy.ReasonPressureShed, nil))
	failure := classify(&commandExecution{}, wrapped)
	if failure == nil || failure.Code != status.CodeLimitPressureShed || failure.Message != wrapped.Error() {
		t.Errorf("a wrapped bare stop reports %+v, want the pressure shed carrying %q", failure, wrapped)
	}
	if endedByInterruption(wrapped) {
		t.Error("a wrapped bare stop was read as an interruption")
	}
	if !endedByInterruption(policy.Stopped(policy.ReasonPressureShed, nil)) {
		t.Error("a bare stop was not read as no error")
	}
}

func TestReasonNoneInsideAStopReportsASupervisionFailure(t *testing.T) {
	// A stop that names no reason is hippo's own fault, never a limit: the
	// zero reason and a value that is no member both report it.
	for _, reason := range []policy.Reason{policy.ReasonNone, policy.Reason(200)} {
		if code := reasonCode(reason); code != status.CodeSupervisionFailed {
			t.Errorf("reasonCode(%d) = %s, want %s", reason, code, status.CodeSupervisionFailed)
		}
		failure := classify(&commandExecution{}, policy.Stopped(reason, nil))
		if failure == nil || failure.Code != status.CodeSupervisionFailed || failure.Status() != status.GuardFailed ||
			failure.Message != "hippo could not complete this work" {
			t.Errorf("a stop for reason %d reports %+v, want a supervision failure", reason, failure)
		}
	}
}

func TestAClassifiedFailureOutranksTheStopThatCarriesIt(t *testing.T) {
	// A handler that hands the boundary its own classified failure keeps it,
	// as it did when the status travelled beside the error.
	cause := status.Fail(status.CodeConfigUnresolvable, "no configuration resolves")
	failure := classify(&commandExecution{}, policy.Stopped(policy.ReasonReplanRequired, cause))
	if failure == nil || failure.Code != status.CodeConfigUnresolvable {
		t.Errorf("a stop carrying a classified failure reports %+v, want %s", failure, status.CodeConfigUnresolvable)
	}
}

func TestAStartedChildsStatusIsNeverExplainedByAStop(t *testing.T) {
	childStatus := 3
	execution := &commandExecution{childStatus: &childStatus}
	if failure := classify(execution, policy.Stopped(policy.ReasonPressureShed, nil)); failure != nil {
		t.Errorf("a child's status was explained as %+v", failure)
	}
}
