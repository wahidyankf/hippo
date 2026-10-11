package support

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These controls exercise receipt discrimination, not a replacement E2E subject.
func TestPublicTUIHolderProtocolRejectsStaleAndMissingWitnesses(t *testing.T) {
	holder := PublicTUIHolderIdentity{PID: 7, Session: 7, Group: 7, Nonce: strings.Repeat("a", 32), Device: 2, Inode: 3, UID: 501}
	baseline := publicHolderReport{Version: 1, Sequence: 1, Nonce: holder.Nonce, Phase: "baseline", Holder: holder}
	if err := validatePublicHolderReport(baseline, holder, 1); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*publicHolderReport)
	}{
		{"nonce", func(value *publicHolderReport) { value.Nonce = strings.Repeat("b", 32) }},
		{"duplicate-sequence", func(value *publicHolderReport) { value.Sequence = 0 }},
		{"out-of-order", func(value *publicHolderReport) { value.Phase = "post-child" }},
		{"foreign-holder", func(value *publicHolderReport) { value.Holder.PID++ }},
		{"foreign-pty", func(value *publicHolderReport) { value.Holder.Inode++ }},
		{"version", func(value *publicHolderReport) { value.Version++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := baseline
			test.change(&value)
			if validatePublicHolderReport(value, holder, 1) == nil {
				t.Fatal("foreign receipt accepted")
			}
		})
	}
	control := publicHolderControl{Version: 1, Sequence: 2, Nonce: holder.Nonce, Action: "teardown", Accepted: true}
	if err := validatePublicHolderControl(control, holder.Nonce, 2, "teardown"); err != nil {
		t.Fatal(err)
	}
	control.Sequence = 1
	if validatePublicHolderControl(control, holder.Nonce, 2, "teardown") == nil {
		t.Fatal("stale ACK accepted")
	}
	packet, err := publicHolderPacket(baseline)
	if err != nil {
		t.Fatal(err)
	}
	var decoded publicHolderReport
	if err := readPublicHolderPacket(bytes.NewReader(packet), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{nil, packet[:3], packet[:len(packet)-1], {0, 1, 0, 1}, {0, 0, 0, 1, '!'}} {
		if readPublicHolderPacket(bytes.NewReader(bad), &decoded) == nil {
			t.Fatal("missing/oversized/malformed receipt accepted")
		}
	}
	if validatePublicHolderJSON([]byte(`{"Sequence":1,"Sequence":2}`)) == nil {
		t.Fatal("duplicate JSON identity key accepted")
	}
	ended := make(chan struct{})
	close(ended)
	transport := &PublicTUITransport{reportDone: ended}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if transport.awaitPublicPhase(ctx, make(chan struct{})) == nil {
		t.Fatal("holder death/report EOF qualified a missing wait witness")
	}
}

func TestPublicTUIHolderRestorationUsesFullModeAndForeground(t *testing.T) {
	baseline := PublicTUITerminalState{Foreground: 7}
	baseline.Mode.Lflag = syscall.ISIG | syscall.ICANON | syscall.ECHO
	baseline.Mode.Cc[0] = 3
	if err := validatePublicTUIRestoration(baseline, baseline); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*PublicTUITerminalState)
	}{
		{"mode-flag", func(value *PublicTUITerminalState) { value.Mode.Lflag ^= syscall.ECHO }},
		{"control-character", func(value *PublicTUITerminalState) { value.Mode.Cc[0]++ }},
		{"foreground", func(value *PublicTUITerminalState) { value.Foreground = 8 }},
		{"synthetic-zero", func(value *PublicTUITerminalState) { value.Foreground = 0 }},
		{"raw-ready", func(value *PublicTUITerminalState) {
			value.RawInput = true
			value.Mode.Lflag &^= syscall.ISIG | syscall.ICANON | syscall.ECHO
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			observed := baseline
			test.change(&observed)
			if validatePublicTUIRestoration(baseline, observed) == nil {
				t.Fatal("changed full native state normalized into restoration")
			}
		})
	}
}

func TestPublicTUIHolderStatusCannotReplaceCandidateOrEOF(t *testing.T) {
	facts := PublicTUIWaitFacts{PID: 8, ExitCode: 143, Exited: true, Signal: -1, RawStatus: 143 << 8}
	result := PublicTUIResult{Identity: PublicTUIIdentity{NativeTUIIdentity: NativeTUIIdentity{PID: 8}}, CandidateWait: facts, Reaped: true, ExitCode: 143, HolderExitCode: 0}
	if !result.NaturalExit() || result.Joined() {
		t.Fatal("candidate nonzero or missing real EOF was confused with holder0")
	}
	result.CandidateWait.PID = 7
	if result.NaturalExit() {
		t.Fatal("foreign wait PID represented natural candidate exit")
	}
	result.CandidateWait = facts
	result.CandidateWait.Forced = true
	if result.NaturalExit() {
		t.Fatal("forced child represented natural candidate exit")
	}
	result.CandidateWait = facts
	result.CandidateWait.RawStatus = 0
	if result.NaturalExit() {
		t.Fatal("holder0 raw status replaced candidate143")
	}
	complete := PublicTUIResult{Reaped: true, OutputEOF: true, ErrorEOF: true, ReadersJoined: true, HolderReaped: true, HolderWaitJoined: true, HolderErrorEOF: true, HolderErrorJoined: true, ReportEOF: true, ReportJoined: true}
	if !complete.Joined() {
		t.Fatal("complete lifetime facts refused")
	}
	for _, test := range []struct {
		name   string
		remove func(*PublicTUIResult)
	}{
		{"holder-reap", func(r *PublicTUIResult) { r.HolderReaped = false }},
		{"holder-wait-join", func(r *PublicTUIResult) { r.HolderWaitJoined = false }},
		{"holder-error-eof", func(r *PublicTUIResult) { r.HolderErrorEOF = false }},
		{"holder-error-reader", func(r *PublicTUIResult) { r.HolderErrorJoined = false }},
		{"report-eof", func(r *PublicTUIResult) { r.ReportEOF = false }},
		{"report-reader", func(r *PublicTUIResult) { r.ReportJoined = false }},
		{"extra-writer-withholds-eof", func(r *PublicTUIResult) { r.OutputEOF = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := complete
			test.remove(&result)
			if result.Joined() {
				t.Fatal("missing actual waiter/EOF accepted")
			}
		})
	}
	cause := errors.New("post-child foreground ENOTTY")
	transport := &PublicTUITransport{reportError: cause, nativeFailure: &PublicTUINativeFailure{Phase: "post-child", Operation: "foreground", Descriptor: 3, Errno: int(syscall.ENOTTY), Detail: cause.Error()}}
	retained := transport.Result()
	if retained.TerminalAfterAvailable || retained.NativeFailure == nil || !errors.Is(transport.OutputError(), cause) {
		t.Fatal("unavailable native getter became synthetic full-state witness")
	}
}

// SH-1: cleanup retains each first actual close error after an earlier failure,
// attempts duplicate closes at their original points, and ignores only their
// replacement results. This seam supplies close outcomes, not process facts.
func TestPublicTUIHolderCleanupRetainsFirstCloseResults(t *testing.T) {
	firstFailure := errors.New("post-child foreground failed")
	retained := firstFailure
	for _, handle := range []string{"control descriptor", "report descriptor", "controlling slave descriptor", "candidate stderr descriptor"} {
		t.Run(handle, func(t *testing.T) {
			firstClose := errors.New("first actual close failed")
			duplicate := errors.New("descriptor already closed")
			owner := publicHolderCloseResult{handle: handle}
			attempts := 0
			closeHandle := func() error {
				attempts++
				if attempts == 1 {
					return firstClose
				}
				return duplicate
			}
			actualFirst := owner.close(closeHandle)
			if !errors.Is(actualFirst, firstClose) || !strings.Contains(actualFirst.Error(), handle) {
				t.Fatal("first close error lost its handle context")
			}
			retained = retainPublicHolderCloseError(retained, actualFirst)
			actualAgain := owner.close(closeHandle)
			if !errors.Is(actualAgain, actualFirst) || attempts != 2 || errors.Is(actualAgain, duplicate) {
				t.Fatal("duplicate close was omitted or replaced the first actual result")
			}
			if repeated := retainPublicHolderCloseError(retained, actualAgain); repeated.Error() != retained.Error() {
				t.Fatal("retained first close was duplicated as a fresh failure")
			}
			if !errors.Is(retained, firstFailure) || !errors.Is(retained, firstClose) {
				t.Fatal("earlier failure or actual close failure discarded")
			}
		})
	}
	if retained.Error() == firstFailure.Error() {
		t.Fatal("all actual handle close failures were discarded")
	}
	owner := publicHolderCloseResult{handle: "successful descriptor"}
	attempts := 0
	closeHandle := func() error {
		attempts++
		if attempts == 1 {
			return nil
		}
		return errors.New("expected duplicate close")
	}
	firstError := owner.close(closeHandle)
	secondError := owner.close(closeHandle)
	if firstError != nil || secondError != nil || attempts != 2 {
		t.Fatal("successful first close became duplicate-close failure or skipped an attempt")
	}
}

// The real original startup RED supplies the parent/session mismatch regression.
// This control retains failed-getter context and missing facts; it supplies no
// replacement native success, process identity, wait result or EOF witness.
func TestPublicTUIParentNativeStartupFailureRetainsContext(t *testing.T) {
	first := publicTUIParentNativeError("baseline-readback", "foreground-master", 12, syscall.ENOTTY)
	cleanup := errors.New("actual cleanup close failed")
	transport := &PublicTUITransport{
		holderStderr: append([]byte("discarded-prefix"), bytes.Repeat([]byte{'h'}, 2048)...),
		reports:      bytes.Repeat([]byte{0xff}, 2048), controls: bytes.Repeat([]byte{'c'}, 2048), stderr: []byte("candidate diagnostic"),
	}
	diagnostics := transport.publicStartupDiagnostics()
	retained := publicTUIStartupFailure("baseline-readback", errors.Join(first, cleanup), publicTUIStartupEvidence{}, diagnostics)
	if !errors.Is(retained, syscall.ENOTTY) || !errors.Is(retained, cleanup) {
		t.Fatal("first getter failure or cleanup failure was discarded")
	}
	for _, required := range []string{"stage=baseline-readback", "getter=foreground-master", "fd=12", "errno=", "HolderSessionAvailable:false", "HolderGroupAvailable:false", "CandidateReceiptAvailable:false", "BaselineAvailable:false", "holderStderrSuffix=", "reportsSuffix=", "controlsSuffix=", "candidate diagnostic"} {
		if !strings.Contains(retained.Error(), required) {
			t.Fatalf("startup context omitted %q", required)
		}
	}
	if strings.Contains(diagnostics, "discarded-prefix") || len(diagnostics) > 16*1024 {
		t.Fatal("startup diagnostics lost the bounded-suffix contract")
	}
	capture := bytes.Repeat([]byte{'x'}, 2048)
	suffix := publicTUIStartupCapture(capture)
	if len(suffix) != 1024 {
		t.Fatal("startup retained an unbounded capture")
	}
	suffix[0] = 'y'
	if capture[1024] != 'x' {
		t.Fatal("diagnostic suffix aliases mutable transport capture")
	}
}
