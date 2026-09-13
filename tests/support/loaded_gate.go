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
	"github.com/wahidyankf/hippo/internal/policy"
)

// loadSaturatedVariable is how scripts/test-loaded.sh tells the end-to-end
// fixtures that it keeps every core busy for the whole gate.
const loadSaturatedVariable = "HIPPO_LOAD_SATURATED"

// capacityDeferralMessage is the guard's documented refusal when admission never came.
const capacityDeferralMessage = "HIPPO deferred task: safe admission was not reached."

// loadedGateSaturation is the only form the declaration may take: set when, and
// only when, the busy workers cover every core.
var loadedGateSaturation = regexp.MustCompile(
	`if \[ "\$workers" -ge "\$cores" \]; then\s+HIPPO_LOAD_SATURATED=1\s+export HIPPO_LOAD_SATURATED\s+fi`,
)

// acceptsSaturatedDeferralV04 is the whole decision the loaded fixtures make. A
// deferral passes only on a host the loaded gate declared saturated, only with
// the guard's documented exit code and message, and only for a child that never
// started, so a child that ran and then failed still fails.
func acceptsSaturatedDeferralV04(declared bool, exitCode int, output []byte, childStarted bool) bool {
	return declared && !childStarted &&
		exitCode == guard.CapacityDeferredExitCode &&
		bytes.Contains(output, []byte(capacityDeferralMessage))
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

func (driver *Driver) documentedCapacityDeferral() {
	driver.deferralAccepted = acceptsSaturatedDeferralV04(
		driver.loadSaturationDeclared, guard.CapacityDeferredExitCode,
		[]byte(capacityDeferralMessage+"\n"), driver.guardedChildStarted,
	)
}

// otherRefusals tries every near miss of the documented deferral: its message
// with another exit, and its exit with another message or none.
func (driver *Driver) otherRefusals() {
	nearMisses := []struct {
		exitCode int
		output   string
	}{
		{exitCode: 1, output: capacityDeferralMessage},
		{exitCode: policy.ReplanRequiredExitCode, output: capacityDeferralMessage},
		{exitCode: guard.CapacityDeferredExitCode, output: "HIPPO stayed deferred across 4 attempts in 1m0s."},
		{exitCode: guard.CapacityDeferredExitCode, output: ""},
	}

	driver.deferralAccepted = false

	for _, nearMiss := range nearMisses {
		if acceptsSaturatedDeferralV04(
			driver.loadSaturationDeclared, nearMiss.exitCode, []byte(nearMiss.output), driver.guardedChildStarted,
		) {
			driver.deferralAccepted = true
		}
	}
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
