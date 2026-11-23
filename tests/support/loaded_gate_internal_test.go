package support

import (
	"testing"

	"github.com/wahidyankf/hippo/internal/status"
)

// TestLoadedGateDeclaresSaturationOnlyWhenEveryCoreIsBusy covers the removed scenario "The loaded gate declares
// saturation only when every core is busy". The committed script declares saturation only under the full-core test,
// product code never reads the declaration, and a script that relaxes the test is refused.
func TestLoadedGateDeclaresSaturationOnlyWhenEveryCoreIsBusy(t *testing.T) {
	driver := &Driver{}
	if err := driver.inspectLoadedGate(); err != nil {
		t.Fatal(err)
	}

	if err := driver.requireSaturationCoversEveryCore(); err != nil {
		t.Fatalf("the committed loaded gate fails the saturation step: %v", err)
	}

	if err := driver.requireProductIgnoresSaturation(); err != nil {
		t.Fatalf("product code reads the saturation declaration: %v", err)
	}

	relaxed := &Driver{loadedGateScript: replaceOnce(t, driver.loadedGateScript, `-ge "$cores"`, `-gt "$cores"`)}
	if err := relaxed.requireSaturationCoversEveryCore(); err == nil {
		t.Fatal("a declaration that is not gated on every core was accepted")
	}

	reading := &Driver{loadedGateScript: driver.loadedGateScript, saturationReaders: []string{"internal/example/reader.go"}}
	if err := reading.requireProductIgnoresSaturation(); err == nil {
		t.Fatal("product code that reads the saturation declaration was accepted")
	}
}

// documentedDeferral is the stderr of a compiled guard whose admission never came: its progress sentence, then the
// closed reason the exit-status contract promises.
func documentedDeferral() []byte {
	return []byte(capacityDeferralOutput)
}

// TestSaturatedHostDeferralIsAcceptedOnlyAsDocumented covers the removed scenario "A saturated host deferral is
// accepted only as documented". The documented deferral on a declared saturated host, from a child that never
// started, is the one accepted outcome.
func TestSaturatedHostDeferralIsAcceptedOnlyAsDocumented(t *testing.T) {
	if !acceptsSaturatedDeferralV04(true, status.LimitShed, documentedDeferral(), false, true) {
		t.Fatal("the documented deferral on a declared saturated host was refused")
	}
}

// TestDeferralStaysFailureWhereSaturationIsUndeclared covers the removed scenario "A deferral stays a failure where
// saturation is undeclared". The same documented deferral is refused when the loaded gate declared nothing.
func TestDeferralStaysFailureWhereSaturationIsUndeclared(t *testing.T) {
	if acceptsSaturatedDeferralV04(false, status.LimitShed, documentedDeferral(), false, true) {
		t.Fatal("a deferral on a host the loaded gate did not declare saturated was accepted")
	}
}

// TestSaturatedHostAcceptsNoOtherRefusal covers the removed scenario "A saturated host accepts no other refusal".
// Each near miss of the documented deferral is refused on a declared saturated host.
func TestSaturatedHostAcceptsNoOtherRefusal(t *testing.T) {
	nearMisses := []struct {
		name                string
		exitCode            int
		output              string
		neverStartedReceipt bool
	}{
		{"another exit with the reason", 1, capacityDeferralOutput, true},
		{"the internal integer as exit", internalDeferralStatus, capacityDeferralOutput, true},
		{"a guard failure exit with the reason", status.GuardFailed, capacityDeferralOutput, true},
		{"the exit without the reason", status.LimitShed, "HIPPO stayed deferred across 4 attempts in 1m0s.", true},
		{"the progress sentence alone", status.LimitShed, capacityDeferralMessage, true},
		{"another limit reason", status.LimitShed, "hippo: [hippo.limit.pressure-shed] host pressure shed this work", true},
		{"no output", status.LimitShed, "", true},
		{"no never-started receipt", status.LimitShed, capacityDeferralOutput, false},
	}

	for _, nearMiss := range nearMisses {
		t.Run(nearMiss.name, func(t *testing.T) {
			if acceptsSaturatedDeferralV04(true, nearMiss.exitCode, []byte(nearMiss.output), false, nearMiss.neverStartedReceipt) {
				t.Fatal("a refusal other than the documented deferral was accepted")
			}
		})
	}
}

// TestProtocolMismatchIsNeverAcceptedAsCapacitySkip covers the removed scenario "A protocol mismatch is never accepted
// as a capacity skip". A coordination protocol mismatch is a failure whatever exit it carries.
func TestProtocolMismatchIsNeverAcceptedAsCapacitySkip(t *testing.T) {
	mismatch := []byte("HIPPO protocol mismatch: incompatible peer coordination.\n" +
		"hippo: [" + string(status.CodeCoordinationProtocolMismatch) + "] live peer coordination state this client cannot safely join\n")

	for _, exitCode := range []int{status.GuardFailed, status.LimitShed} {
		if acceptsSaturatedDeferralV04(true, exitCode, mismatch, false, true) {
			t.Fatalf("a protocol mismatch exiting %d was accepted as a capacity skip", exitCode)
		}
	}
}

// TestSaturatedDeferralIsRefusedOnceGuardedChildStarted covers the removed scenario "A saturated deferral is refused
// once the guarded child started". A child that signalled readiness has started, so the deferral is no longer a skip.
func TestSaturatedDeferralIsRefusedOnceGuardedChildStarted(t *testing.T) {
	if acceptsSaturatedDeferralV04(true, status.LimitShed, documentedDeferral(), true, true) {
		t.Fatal("a deferral after the guarded child started was accepted")
	}
}
