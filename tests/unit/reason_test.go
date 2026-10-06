package unit_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/policy"
)

// legacyReasons is every internal reason with the integer v0.8.4 carried for it.
// The integers survive only where the public contract publishes them: status
// JSON's profile.exitCode, and the reservation ledger's shedding code.
var legacyReasons = []struct {
	reason policy.Reason
	code   int
}{
	{policy.ReasonNone, 0},
	{policy.ReasonStorageBlocked, 73},
	{policy.ReasonPressureShed, 74},
	{policy.ReasonCapacityDeferred, 75},
	{policy.ReasonProtocolMismatch, 76},
	{policy.ReasonReplanRequired, 78},
}

func TestReasonKeepsItsV084Integer(t *testing.T) {
	for _, row := range legacyReasons {
		encoded, err := json.Marshal(row.reason)
		if err != nil || string(encoded) != strconv.Itoa(row.code) {
			t.Errorf("reason %d encodes as %s (%v), want %d", row.reason, encoded, err, row.code)
		}
	}
}

func TestReasonEncoderRefusesAValueThatIsNoMember(t *testing.T) {
	if encoded, err := json.Marshal(policy.Reason(200)); err == nil {
		t.Errorf("a reason that is no member encoded as %s, want a refusal", encoded)
	}
}

func TestReasonDecodesOnlyItsLegacyIntegers(t *testing.T) {
	for _, row := range legacyReasons {
		var decoded policy.Reason
		if err := json.Unmarshal([]byte(strconv.Itoa(row.code)), &decoded); err != nil || decoded != row.reason {
			t.Errorf("%d decodes as reason %d (%v), want %d", row.code, decoded, err, row.reason)
		}
	}
	// 1 and 2 are statuses a handler returns, 77 sits between two reasons, and
	// the rest are not integers at all.
	for _, text := range []string{"1", "2", "72", "77", "79", "-73", "73.5", `"73"`, "true", "[]"} {
		decoded := policy.ReasonCapacityDeferred
		if err := json.Unmarshal([]byte(text), &decoded); err == nil || decoded != policy.ReasonCapacityDeferred {
			t.Errorf("%s decodes as reason %d (%v), want a refusal that leaves the reason as it was", text, decoded, err)
		}
	}
}

func TestStoppedCarriesItsReasonAndUnwrapsToItsCause(t *testing.T) {
	cause := errors.New("profile fallback cycle")
	stop := policy.Stopped(policy.ReasonReplanRequired, cause)
	if stop.Reason != policy.ReasonReplanRequired || !errors.Is(stop.Cause, cause) {
		t.Fatalf("stop = %+v, want the replan reason and its cause", stop)
	}
	if !errors.Is(stop, cause) || stop.Error() != cause.Error() {
		t.Errorf("stop does not unwrap to its cause: %v", stop)
	}

	wrapped := fmt.Errorf("verify schema-3 activation: %w", stop)
	found, isStop := errors.AsType[*policy.Stop](wrapped)
	if !isStop || found.Reason != policy.ReasonReplanRequired || !errors.Is(wrapped, cause) {
		t.Errorf("a stop inside %q was not found with its reason and cause", wrapped)
	}
}

func TestAStopWithNoCauseNamesItsReasonInWords(t *testing.T) {
	stop := policy.Stopped(policy.ReasonCapacityDeferred, nil)
	if stop.Cause != nil || errors.Unwrap(stop) != nil {
		t.Errorf("a stop given no cause carries %v", stop.Cause)
	}
	text := stop.Error()
	if !strings.Contains(text, policy.ReasonCapacityDeferred.String()) || strings.ContainsAny(text, "0123456789") {
		t.Errorf("a stop with no cause reads %q, want its reason in words and no integer", text)
	}
}

func TestEveryReasonNamesItselfWithADistinctWordAndNoInteger(t *testing.T) {
	named := map[string]policy.Reason{}
	for _, row := range legacyReasons {
		name := row.reason.String()
		if name == "" || strings.ContainsAny(name, "0123456789") {
			t.Errorf("reason %d is named %q, want words alone", row.reason, name)
		}
		if other, taken := named[name]; taken {
			t.Errorf("reasons %d and %d are both named %q", other, row.reason, name)
		}
		named[name] = row.reason
	}
	if name := policy.Reason(200).String(); name == "" || strings.ContainsAny(name, "0123456789") {
		t.Errorf("a value that is no member is named %q, want words alone", name)
	} else if member, taken := named[name]; taken {
		t.Errorf("a value that is no member is named %q, as reason %d is", name, member)
	}
}

func TestBareStopFindsOnlyATopLevelStopThatCarriesNoCause(t *testing.T) {
	bare := policy.Stopped(policy.ReasonStorageBlocked, nil)
	if stop, isBare := policy.BareStop(bare); !isBare || stop != bare {
		t.Errorf("BareStop(%v) = %v, %v, want that stop", bare, stop, isBare)
	}
	for name, err := range map[string]error{
		"nil":               nil,
		"a plain error":     errors.New("plain"),
		"a stop with cause": policy.Stopped(policy.ReasonPressureShed, errors.New("retirement unconfirmed")),
		// A wrapper or a join says more than the stop does.
		"a stop inside a wrapping error": fmt.Errorf("outer: %w", bare),
		"a stop joined with an error":    errors.Join(bare, errors.New("release failed")),
		"a stop alone inside a join":     errors.Join(bare),
	} {
		if stop, isBare := policy.BareStop(err); isBare || stop != nil {
			t.Errorf("BareStop(%s) = %v, %v, want none", name, stop, isBare)
		}
	}
}

func TestCarriesNoErrorForNothingOrATopLevelBareStopOnly(t *testing.T) {
	bare := policy.Stopped(policy.ReasonPressureShed, nil)
	for _, row := range []struct {
		name string
		err  error
		want bool
	}{
		{"no error", nil, true},
		{"a stop that says only its reason", bare, true},
		{"a plain error", errors.New("plain"), false},
		{"a stop that carries an error", policy.Stopped(policy.ReasonPressureShed, errors.New("unconfirmed")), false},
		{"a stop inside a wrapping error", fmt.Errorf("release: %w", bare), false},
		{"a stop joined with another error", errors.Join(bare, errors.New("release failed")), false},
		{"a stop alone inside a join", errors.Join(bare), false},
	} {
		if got := policy.CarriesNoError(row.err); got != row.want {
			t.Errorf("CarriesNoError(%s) = %v, want %v", row.name, got, row.want)
		}
	}
}
