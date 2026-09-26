// Package status is HIPPO's two-layer failure contract: a closed set of exit
// statuses a shell can branch on, and a closed set of namespaced codes that
// say precisely what happened.
//
// The statuses are deliberately few. Before this package hippo exited 73, 75,
// 76, and 78 — four numbers that meant cleanup, retry, drain a peer, and
// replan, and that no consumer anywhere branched on: the ./hippo bootstrap
// shared across nine repositories reaches `exit 78` from a single refuse()
// helper in sixteen call sites and never asks which reason produced it. A
// vocabulary nobody reads is not information, it is a number to get wrong.
// The guard and policy layers still pass those numbers among themselves, but
// none leaves the process: the command-line boundary maps each to one of the
// statuses below and names its reason.
//
// What replaces it is the one convention that already exists for this problem.
// `timeout` returns 124 when the limit stopped the work and 125 when timeout
// itself failed; every POSIX shell returns 126 for a command found but not
// executable and 127 for one not found. A caller meeting 127 from hippo reads
// it correctly without ever having heard of hippo.
//
// The precise reason does not disappear; it moves to a layer built to carry
// it. Every failure names a Code, which reaches the caller twice: as a message
// on stderr, so a shell script needs no parser, and as error.code in the
// machine-readable body, for a caller that wants to branch on it.
package status

import "fmt"

// The closed exit vocabulary. Nothing else is ever returned, except a status
// the operating system produced: a started child's own status passes through
// verbatim, and a death by signal N surfaces as 128+N.
const (
	// Success is the run happening and the answer being affirmative.
	Success = 0
	// NegativeResult is the run happening and the answer being negative. It is
	// separate from Success because a caller that cannot tell an empty answer
	// from a satisfied one has to parse the payload to find out.
	NegativeResult = 1
	// CallerError is the invocation itself being unusable: an unknown flag, a
	// missing argument, a value outside its choices.
	CallerError = 2
	// LimitShed is a limit stopping the work — a bounded wait that elapsed, a
	// deferral against capacity, a child shed under pressure. It is the status
	// `timeout` returns for the same situation, and it is retryable in the
	// sense that the limit can lift.
	LimitShed = 124
	// GuardFailed is hippo failing to start or supervise the work: before a
	// child starts, or after one started and hippo lost it. Retrying it
	// unchanged will fail the same way.
	GuardFailed = 125
	// ChildNotExecutable is a child that exists and cannot be run.
	ChildNotExecutable = 126
	// ChildNotFound is a child that is not there at all.
	ChildNotFound = 127
)

// Code is one closed reason, namespaced `hippo.area.reason`. The area says
// which part of hippo decided, and the reason says what it decided.
type Code string

// The closed code vocabulary. Every failure hippo reports names exactly one of
// these, and docs/reference/exit-codes.md publishes the same list; a unit
// case asserts the two agree in both directions, so neither can drift.
const (
	// CodeArgsInvalid is an invocation hippo could not parse or accept.
	CodeArgsInvalid Code = "hippo.args.invalid"
	// CodeConfigUnreadable is a resource configuration that could not be read.
	CodeConfigUnreadable Code = "hippo.config.unreadable"
	// CodeConfigUnresolvable is a configuration read but not usable: a profile
	// that does not resolve, a value outside what this host can represent.
	CodeConfigUnresolvable Code = "hippo.config.unresolvable"
	// CodeLimitCapacityDeferred is admission deferred against capacity, or a
	// bounded wait that elapsed before a slot came free.
	CodeLimitCapacityDeferred Code = "hippo.limit.capacity-deferred"
	// CodeLimitStorageBlocked is the hard disk floor stopping the work. The
	// remedy is cleanup rather than a retry, which is why it is its own code
	// and not a flavour of the one above.
	CodeLimitStorageBlocked Code = "hippo.limit.storage-blocked"
	// CodeLimitPressureShed is a started child shed because the host crossed a
	// pressure threshold while it ran.
	CodeLimitPressureShed Code = "hippo.limit.pressure-shed"
	// CodePolicyReplanRequired is strict capacity or configuration that cannot
	// admit this work as asked for. A different request may succeed.
	CodePolicyReplanRequired Code = "hippo.policy.replan-required"
	// CodeCoordinationProtocolMismatch is live peer coordination state this
	// client cannot safely join.
	CodeCoordinationProtocolMismatch Code = "hippo.coordination.protocol-mismatch"
	// CodeChildNotFound is a command that is not on PATH and not at the path given.
	CodeChildNotFound Code = "hippo.child.not-found"
	// CodeChildNotExecutable is a command that exists and cannot be executed.
	CodeChildNotExecutable Code = "hippo.child.not-executable"
	// CodeHostUnreadable is host evidence hippo could not collect, leaving it
	// with nothing to admit against.
	CodeHostUnreadable Code = "hippo.host.unreadable"
	// CodeEvidenceUnwritable is the evidence root refusing a write hippo needs
	// to make before it can admit work.
	CodeEvidenceUnwritable Code = "hippo.evidence.unwritable"
	// CodeEvidenceUnreadable is evidence hippo recorded earlier that it can no
	// longer read, such as a corrupt history archive. The bytes are left as
	// they are for inspection.
	CodeEvidenceUnreadable Code = "hippo.evidence.unreadable"
	// CodeSupervisionFailed is hippo losing the ability to supervise a child it
	// had already started.
	CodeSupervisionFailed Code = "hippo.supervision.failed"
	// CodeInternalFailure is a fault in hippo itself, including an unhandled
	// panic. It exists so that such a fault is a declared status rather than
	// whatever the runtime happens to do.
	CodeInternalFailure Code = "hippo.internal.failure"
)

// All is the published vocabulary in the order docs/reference/exit-codes.md
// lists it.
var All = []Code{
	CodeArgsInvalid,
	CodeConfigUnreadable,
	CodeConfigUnresolvable,
	CodeLimitCapacityDeferred,
	CodeLimitStorageBlocked,
	CodeLimitPressureShed,
	CodePolicyReplanRequired,
	CodeCoordinationProtocolMismatch,
	CodeChildNotFound,
	CodeChildNotExecutable,
	CodeHostUnreadable,
	CodeEvidenceUnwritable,
	CodeEvidenceUnreadable,
	CodeSupervisionFailed,
	CodeInternalFailure,
}

// statuses maps each code to the one status it returns. It is exhaustive by
// construction: Status panics on a code that is missing, and a unit case walks
// All so a new code cannot be added without landing here.
var statuses = map[Code]int{
	CodeArgsInvalid:                  CallerError,
	CodeConfigUnreadable:             GuardFailed,
	CodeConfigUnresolvable:           GuardFailed,
	CodeLimitCapacityDeferred:        LimitShed,
	CodeLimitStorageBlocked:          LimitShed,
	CodeLimitPressureShed:            LimitShed,
	CodePolicyReplanRequired:         GuardFailed,
	CodeCoordinationProtocolMismatch: GuardFailed,
	CodeChildNotFound:                ChildNotFound,
	CodeChildNotExecutable:           ChildNotExecutable,
	CodeHostUnreadable:               GuardFailed,
	CodeEvidenceUnwritable:           GuardFailed,
	CodeEvidenceUnreadable:           GuardFailed,
	CodeSupervisionFailed:            GuardFailed,
	CodeInternalFailure:              CallerError,
}

// retryable marks the codes whose condition can lift on its own. It is
// advisory: it tells a caller whether waiting is worth anything, and it is
// what error.retryable reports.
var retryable = map[Code]bool{
	CodeLimitCapacityDeferred: true,
	CodeLimitPressureShed:     true,
}

// Status is the exit status a code returns.
func Status(code Code) int {
	value, known := statuses[code]
	if !known {
		// Unreachable through the exported surface: Code is closed and a unit
		// case walks All. A code that arrived here anyway is a fault in hippo,
		// and reporting it as one beats inventing a status for it.
		return statuses[CodeInternalFailure]
	}

	return value
}

// Retryable reports whether waiting and trying again is worth anything.
func Retryable(code Code) bool { return retryable[code] }

// Known reports whether a code is in the published vocabulary.
func Known(code Code) bool {
	_, ok := statuses[code]

	return ok
}

// Failure is one closed failure: the reason, the sentence a person reads, and
// optionally the flag or field that caused it. It is an error so it can travel
// the paths hippo already has.
//
//nolint:errname // "Failure" is the contract's own noun for this; FailureError would say it twice.
type Failure struct {
	Code    Code
	Message string
	Field   string
}

// Fail builds a Failure whose message is formatted from the arguments. No
// message should carry a secret or an absolute path that the caller did not
// supply; the code is what a consumer branches on.
func Fail(code Code, format string, arguments ...any) Failure {
	return Failure{Code: code, Message: fmt.Sprintf(format, arguments...)}
}

// Error renders the failure the way stderr shows it: the GNU `program:
// message` form every other command-line tool uses, with the code in brackets
// so a shell script can grep for it without a parser.
func (failure Failure) Error() string {
	return fmt.Sprintf("hippo: [%s] %s", failure.Code, failure.Message)
}

// Status is the exit status this failure returns.
func (failure Failure) Status() int { return Status(failure.Code) }

// Retryable reports whether waiting and trying again is worth anything.
func (failure Failure) Retryable() bool { return Retryable(failure.Code) }
