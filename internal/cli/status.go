package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// bodySchemaVersion is the version of the machine-readable failure body. It is
// separate from hippo's own version so a consumer can pin what it parses
// without pinning what it runs.
const bodySchemaVersion = 1

// internalReasons maps the statuses hippo's guard and policy layers return
// among themselves to the reason each one meant. Those numbers — 73, 74, 75,
// 76, 78 — never leave the process; they survive here only as the seam that
// carries their meaning into the body, so the layers below need no rewrite
// to gain it.
var internalReasons = map[int]status.Code{
	guard.StorageBlockedExitCode:    status.CodeLimitStorageBlocked,
	guard.CapacityDeferredExitCode:  status.CodeLimitCapacityDeferred,
	guard.PressureShedExitCode:      status.CodeLimitPressureShed,
	policy.ProtocolMismatchExitCode: status.CodeCoordinationProtocolMismatch,
	policy.ReplanRequiredExitCode:   status.CodePolicyReplanRequired,
}

// failureBody is the machine-readable failure, and the only shape hippo writes
// for one. Field order here is field order on the wire.
type failureBody struct {
	SchemaVersion int              `json:"schemaVersion"`
	Command       string           `json:"command"`
	ExitCode      int              `json:"exitCode"`
	Error         failureBodyError `json:"error"`
}

type failureBodyError struct {
	Code      status.Code `json:"code"`
	Message   string      `json:"message"`
	Field     string      `json:"field,omitempty"`
	Retryable bool        `json:"retryable"`
}

// classify turns one handler's result into the failure hippo reports, or nil
// when there is nothing to report. A status a started child produced is never
// hippo's to explain, so it returns nil for those however it looks.
func classify(execution *commandExecution, err error) *status.Failure {
	if execution.childStatus != nil {
		return nil
	}

	if failure, ok := errors.AsType[status.Failure](err); ok {
		return &failure
	}

	code, internal := internalReasons[execution.exitCode]
	switch {
	case internal:
		message := reasonMessage(code)
		if err != nil {
			message = err.Error()
		}

		return &status.Failure{Code: code, Message: message}
	case err == nil:
		// A status hippo returned without an error is a result, not a failure:
		// 0 for an affirmative answer and 1 for a negative one.
		return nil
	case execution.exitCode == 0:
		// No handler set a status, so nothing hippo was asked to do ever ran:
		// Cobra rejected the invocation while parsing it.
		return &status.Failure{Code: status.CodeArgsInvalid, Message: err.Error()}
	default:
		return &status.Failure{Code: status.CodeSupervisionFailed, Message: err.Error()}
	}
}

// endedByInterruption reports whether a handler's result is only the
// cancellation a signal caused: no error, or the cancellation itself. A
// classified failure is hippo's own answer and outranks the signal.
func endedByInterruption(err error) bool {
	if err == nil {
		return true
	}
	if _, classified := errors.AsType[status.Failure](err); classified {
		return false
	}

	return errors.Is(err, context.Canceled)
}

// reasonMessage is the sentence a reason carries when the layer that decided
// returned no error of its own — which the guard does deliberately, because
// being shed against a limit is an outcome rather than a fault.
//
//nolint:exhaustive // Only the reasons the internal statuses carry reach here; the default covers the rest.
func reasonMessage(code status.Code) string {
	switch code {
	case status.CodeLimitStorageBlocked:
		return "the disk floor stopped this work; free space before retrying"
	case status.CodeLimitCapacityDeferred:
		return "capacity deferred this work; retry when the host is quieter"
	case status.CodeLimitPressureShed:
		return "host pressure shed this work after the payload ran; recover it before repeating anything"
	case status.CodeCoordinationProtocolMismatch:
		return "live peer coordination state this client cannot safely join"
	case status.CodePolicyReplanRequired:
		return "no profile admits this request as asked for"
	default:
		return "hippo could not complete this work"
	}
}

// report writes the failure the two ways a caller may read it: a single line
// on stderr, which a shell script can grep without a parser, and — when the
// caller asked for --output json — the body on stderr beneath it, which a
// consumer can parse. Both carry the same code, so neither reader is the
// second-class one.
func report(stderr io.Writer, command string, failure status.Failure, exitCode int, execution *commandExecution) {
	line := failure.Error()
	if execution.colour {
		line = "\x1b[31m" + line + "\x1b[0m"
	}
	_, _ = fmt.Fprintln(stderr, line)

	if !execution.machineReadable {
		return
	}

	body := failureBody{
		SchemaVersion: bodySchemaVersion,
		Command:       command,
		ExitCode:      exitCode,
		Error: failureBodyError{
			Code:      failure.Code,
			Message:   failure.Message,
			Field:     failure.Field,
			Retryable: failure.Retryable(),
		},
	}

	// The body is never coloured, whatever --color says: it is the reading a
	// parser does, and an escape byte in it is a parse error rather than a
	// presentation choice.
	encoder := json.NewEncoder(stderr)
	encoder.SetEscapeHTML(false)
	// Nothing useful remains if stderr itself cannot be written: the
	// diagnostic above already failed the same way, and the status is on its
	// way out regardless.
	_ = encoder.Encode(body) //nolint:errchkjson // The body is a closed struct of strings, ints and a bool.
}

// wantsColour decides whether diagnostics carry ANSI colour. hippo writes to
// a pipe far more often than to a terminal, and it cannot see the terminal
// behind an injected writer, so the default is plain text and colour is
// something a caller asks for. NO_COLOR and TERM=dumb are honoured because a
// caller that sets them has already said what it wants; an explicit
// --color=always still wins, since it is the more specific instruction.
func wantsColour(requested string, environment map[string]string) bool {
	switch strings.ToLower(requested) {
	case "always":
		return true
	case "never":
		return false
	}

	if _, present := environment["NO_COLOR"]; present {
		return false
	}

	return environment["TERM"] != "" && environment["TERM"] != "dumb" && environment["HIPPO_COLOR"] == "1"
}
