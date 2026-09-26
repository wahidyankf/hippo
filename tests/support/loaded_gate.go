package support

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/status"
)

// loadSaturatedVariable is how scripts/test-loaded.sh tells the end-to-end
// fixtures that it keeps every core busy for the whole gate.
const loadSaturatedVariable = "HIPPO_LOAD_SATURATED"

// capacityDeferralMessage is the guard's progress sentence when admission never
// came. It is prose for a person reading the log, not the contract.
const capacityDeferralMessage = "HIPPO deferred task: safe admission was not reached."

// capacityDeferralReason is the closed reason a capacity deferral names on
// stderr, and the part of the output the exit-status contract promises.
const capacityDeferralReason = "hippo: [" + string(status.CodeLimitCapacityDeferred) + "]"

// loadedGateSaturation is the only form the declaration may take: set when, and
// only when, the busy workers cover every core.
var loadedGateSaturation = regexp.MustCompile(
	`if \[ "\$workers" -ge "\$cores" \]; then\s+HIPPO_LOAD_SATURATED=1\s+export HIPPO_LOAD_SATURATED\s+fi`,
)

// acceptsSaturatedDeferralV04 is the whole decision the loaded fixtures make. A
// deferral passes only on a host the loaded gate declared saturated, only with
// the documented caller status for a limit (124) naming
// hippo.limit.capacity-deferred, and only for a child that never started, so a
// child that ran and then failed still fails. The reason, not the progress
// sentence, is what it matches: 124 is also a pressure shed, and only the
// reason tells the two apart.
func acceptsSaturatedDeferralV04(declared bool, exitCode int, output []byte, childStarted, neverStartedReceipt bool) bool {
	return declared && !childStarted && neverStartedReceipt &&
		exitCode == status.LimitShed &&
		bytes.Contains(output, []byte(capacityDeferralReason))
}

func (driver *Driver) inspectLoadedGate() error {
	data, err := os.ReadFile(filepath.Join(toolRoot(), "scripts", "test-loaded.sh"))
	if err != nil {
		return err
	}

	driver.loadedGateScript = string(data)
	driver.saturationReaders = nil

	for _, directory := range []string{"cmd", "internal"} {
		if err := filepath.WalkDir(filepath.Join(toolRoot(), directory), driver.recordSaturationReader); err != nil {
			return err
		}
	}

	return nil
}

func (driver *Driver) recordSaturationReader(path string, entry fs.DirEntry, walkError error) error {
	if walkError != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
		return walkError
	}

	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if bytes.Contains(source, []byte(loadSaturatedVariable)) {
		relative, relativeError := filepath.Rel(toolRoot(), path)
		if relativeError != nil {
			return relativeError
		}

		driver.saturationReaders = append(driver.saturationReaders, relative)
	}

	return nil
}

func (driver *Driver) requireSaturationCoversEveryCore() error {
	if driver.loadedGateScript == "" {
		return errors.New("the loaded gate was not inspected")
	}

	if !loadedGateSaturation.MatchString(driver.loadedGateScript) {
		return errors.New("scripts/test-loaded.sh does not declare saturation only when its busy workers cover every core")
	}

	if count := strings.Count(driver.loadedGateScript, loadSaturatedVariable+"="); count != 1 {
		return fmt.Errorf("scripts/test-loaded.sh assigns %s %d times, want exactly once", loadSaturatedVariable, count)
	}

	return nil
}

func (driver *Driver) requireProductIgnoresSaturation() error {
	if driver.loadedGateScript == "" {
		return errors.New("the loaded gate was not inspected")
	}

	if len(driver.saturationReaders) > 0 {
		return fmt.Errorf("product code reads %s: %s", loadSaturatedVariable, strings.Join(driver.saturationReaders, ", "))
	}

	return nil
}

func (driver *Driver) declareLoadSaturation() {
	driver.loadSaturationDeclared = true
	driver.guardedChildStarted = false
}

func (driver *Driver) undeclaredLoadSaturation() {
	driver.loadSaturationDeclared = false
	driver.guardedChildStarted = false
}

func (driver *Driver) markGuardedChildStarted() {
	driver.guardedChildStarted = true
}

// capacityDeferralOutput is the stderr a compiled guard writes when host
// admission never came: its progress sentence, then the closed reason.
const capacityDeferralOutput = capacityDeferralMessage + "\n" + capacityDeferralReason +
	" capacity deferred this work; retry when the host is quieter\n"

func (driver *Driver) documentedCapacityDeferral() {
	driver.deferralAccepted = acceptsSaturatedDeferralV04(
		driver.loadSaturationDeclared, status.LimitShed,
		[]byte(capacityDeferralOutput), driver.guardedChildStarted, true,
	)
}

// otherRefusals tries every near miss of the documented deferral: its reason
// with another exit, and its exit with another reason, only the progress
// sentence, or nothing.
func (driver *Driver) otherRefusals() {
	nearMisses := []struct {
		exitCode            int
		output              string
		neverStartedReceipt bool
	}{
		{exitCode: 1, output: capacityDeferralOutput, neverStartedReceipt: true},
		{exitCode: guard.CapacityDeferredExitCode, output: capacityDeferralOutput, neverStartedReceipt: true},
		{exitCode: status.GuardFailed, output: capacityDeferralOutput, neverStartedReceipt: true},
		{exitCode: status.LimitShed, output: "HIPPO stayed deferred across 4 attempts in 1m0s.", neverStartedReceipt: true},
		{exitCode: status.LimitShed, output: capacityDeferralMessage, neverStartedReceipt: true},
		{exitCode: status.LimitShed, output: "hippo: [hippo.limit.pressure-shed] host pressure shed this work", neverStartedReceipt: true},
		{exitCode: status.LimitShed, output: "", neverStartedReceipt: true},
		{exitCode: status.LimitShed, output: capacityDeferralOutput},
	}

	driver.deferralAccepted = false

	for _, nearMiss := range nearMisses {
		if acceptsSaturatedDeferralV04(
			driver.loadSaturationDeclared, nearMiss.exitCode, []byte(nearMiss.output), driver.guardedChildStarted,
			nearMiss.neverStartedReceipt,
		) {
			driver.deferralAccepted = true
		}
	}
}

func (driver *Driver) protocolMismatchRefusal() {
	driver.deferralAccepted = acceptsSaturatedDeferralV04(
		driver.loadSaturationDeclared,
		status.GuardFailed,
		[]byte("HIPPO protocol mismatch: incompatible peer coordination.\n"+
			"hippo: ["+string(status.CodeCoordinationProtocolMismatch)+"] live peer coordination state this client cannot safely join\n"),
		driver.guardedChildStarted, true,
	)
}

func (driver *Driver) requireDeferralAccepted() error {
	if !driver.deferralAccepted {
		return errors.New("the documented deferral on a declared saturated host was refused")
	}

	return nil
}

func (driver *Driver) requireRunRefused() error {
	if driver.deferralAccepted {
		return errors.New("the fixture accepted a run it must refuse")
	}

	return nil
}
