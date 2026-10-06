package policy

import (
	"encoding/json"
	"fmt"
	"math"
)

// Reason is why one of HIPPO's own layers stopped work. It is the closed set of
// causes the guard, the policy, and the command handlers hand each other, and
// the command-line boundary alone turns one into a status code and an exit
// status. The zero value is ReasonNone: nothing stopped.
//
// v0.8.4 carried these as the integers 73, 74, 75, 76, and 78 among the layers.
// Those integers never leave the process except where the public contract
// publishes them, which are status JSON's profile.exitCode and the reservation
// ledger's shedding code; Reason encodes itself as its integer for the first.
type Reason uint8

const (
	// ReasonNone is the zero value: no stop.
	ReasonNone Reason = iota
	// ReasonStorageBlocked is work the disk floor stopped: cleanup is required
	// before retrying.
	ReasonStorageBlocked
	// ReasonCapacityDeferred is work that never started because capacity was not
	// safe in time: transient pressure that a later retry may clear.
	ReasonCapacityDeferred
	// ReasonPressureShed is a started child shed under host pressure other than
	// storage, which is never mistaken for a deferral that started nothing.
	ReasonPressureShed
	// ReasonProtocolMismatch is live peer coordination state this client cannot
	// safely join.
	ReasonProtocolMismatch
	// ReasonReplanRequired is strict capacity or configuration no profile admits.
	ReasonReplanRequired
)

// String names the reason in words. It carries no integer, so a message built
// from it never prints one of the integers v0.8.4 passed among its layers.
func (reason Reason) String() string {
	switch reason {
	case ReasonNone:
		return "no reason"
	case ReasonStorageBlocked:
		return "storage blocked"
	case ReasonCapacityDeferred:
		return "capacity deferred"
	case ReasonPressureShed:
		return "pressure shed"
	case ReasonProtocolMismatch:
		return "protocol mismatch"
	case ReasonReplanRequired:
		return "replan required"
	}

	return "an unknown reason"
}

// legacyExitCode is the integer v0.8.4 carried for the reason. This switch is
// the only table of those integers, read both ways by the JSON codec.
func (reason Reason) legacyExitCode() (int, error) {
	switch reason {
	case ReasonNone:
		return 0, nil
	case ReasonStorageBlocked:
		return 73, nil
	case ReasonPressureShed:
		return 74, nil
	case ReasonCapacityDeferred:
		return 75, nil
	case ReasonProtocolMismatch:
		return 76, nil
	case ReasonReplanRequired:
		return 78, nil
	}

	return 0, fmt.Errorf("reason %d is not one hippo carries", uint8(reason))
}

// MarshalJSON writes the integer v0.8.4 published for the reason, and refuses a
// value that is no member.
func (reason Reason) MarshalJSON() ([]byte, error) {
	code, err := reason.legacyExitCode()
	if err != nil {
		return nil, err
	}

	return json.Marshal(code)
}

// UnmarshalJSON reads one of the integers MarshalJSON writes and refuses every
// other value, including integers outside the set, so a number is never read as
// a reason it does not name. It asks every possible reason for its integer, so a
// member added to the constants is read back without a second table.
func (reason *Reason) UnmarshalJSON(data []byte) error {
	var code int
	if err := json.Unmarshal(data, &code); err != nil {
		return fmt.Errorf("a reason is an integer: %w", err)
	}
	for raw := uint8(0); ; raw++ {
		if legacy, err := Reason(raw).legacyExitCode(); err == nil && legacy == code {
			*reason = Reason(raw)

			return nil
		}
		if raw == math.MaxUint8 {
			break
		}
	}

	return fmt.Errorf("%d is not an integer hippo carries for a reason", code)
}

// Stop is an error that says a layer stopped for a Reason, with the cause it
// has to report beside it, if any. A handler that stops work returns a Stop
// instead of a reserved integer, so what stopped is a value the compiler checks
// and the status the caller sees is decided once, at the command-line boundary.
//
//nolint:errname // "Stop" is the noun the command-line boundary switches on; StopError would call an outcome a fault.
type Stop struct {
	Reason Reason
	Cause  error
}

// Stopped returns the stop for a reason. The cause may be nil: being shed or
// deferred against a limit is an outcome rather than a fault, and says nothing
// beyond its reason.
func Stopped(reason Reason, cause error) *Stop {
	return &Stop{Reason: reason, Cause: cause}
}

// Error is the cause's text, or the reason in words when the stop has no cause.
func (stop *Stop) Error() string {
	if stop.Cause != nil {
		return stop.Cause.Error()
	}

	return "stopped: " + stop.Reason.String()
}

// Unwrap returns the cause.
func (stop *Stop) Unwrap() error {
	return stop.Cause
}

// BareStop returns err as a stop that carries no cause: a layer that stopped
// work and had nothing else to report. Only a stop that is the whole error
// counts. One a wrapper or errors.Join holds says more than its reason, so it is
// not bare, however plain the stop inside it is.
func BareStop(err error) (*Stop, bool) {
	// The top-level value is the point; errors.As would find a buried stop.
	stop, isStop := err.(*Stop) //nolint:errorlint // Only a stop that is the whole error is bare.
	if isStop && stop != nil && stop.Cause == nil {
		return stop, true
	}

	return nil, false
}

// CarriesNoError reports whether a layer's result says nothing beyond its status:
// no error, or a bare stop. Callers that once read a nil error beside a status as
// "nothing went wrong beyond this status" read a bare stop the same, so a failure
// that surfaces while a run unwinds replaces it, and an in-process caller sees no
// error for an outcome that is not a fault.
func CarriesNoError(err error) bool {
	if err == nil {
		return true
	}
	_, bare := BareStop(err)

	return bare
}
