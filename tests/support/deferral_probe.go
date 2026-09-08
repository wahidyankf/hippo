package support

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wahidyankf/hippo/internal/conformance"
)

// Each probe stands in for how a consumer reacts to a saturated coordination
// root. The harness saturates that root before the probe runs and frees it only
// after its hold, so these three commands cover the whole contract: one waits
// for capacity, one reads the deferral as final, and one never consults HIPPO at
// all and would otherwise pass by finishing before the question was asked.
const (
	probeWaitsForCapacity      = `while [ -f "$HIPPO_ROOT/coordination-mode.json" ]; do sleep 0.05; done`
	probeTreatsDeferralAsFinal = `if [ -f "$HIPPO_ROOT/coordination-mode.json" ]; then exit 75; fi`
	probeNeverConsultsHIPPO    = `exit 0`
)

func (driver *Driver) declareDeferralProbe(script string) error {
	driver.deferralProbeScript = script

	return nil
}

// runDeferralProbeConformance builds the smallest manifest that carries a probe
// and runs the real harness over it, so the scenario exercises the shipped check
// rather than a copy of its logic.
func (driver *Driver) runDeferralProbeConformance() error {
	root, err := os.MkdirTemp("", "hippo-probe-conformance-")
	if err != nil {
		return err
	}

	driver.temporaryPaths = append(driver.temporaryPaths, root)

	binary, err := os.Executable()
	if err != nil {
		return err
	}

	binaryData, err := os.ReadFile(binary)
	if err != nil {
		return err
	}

	digest := sha256.Sum256(binaryData)
	manifest := conformance.Manifest{
		SchemaVersion: 1,
		HIPPOBinary:   binary,
		HIPPOSHA256:   hex.EncodeToString(digest[:]),
		SharedRoot:    filepath.Join(root, "shared"),
	}

	// The harness requires its full four-consumer shape. Only the first declares a
	// probe, which keeps the scenario to one saturation hold while still running
	// the real per-consumer loop.
	for index := range 4 {
		name := fmt.Sprintf("probe-consumer-%d", index+1)
		consumerPath := filepath.Join(root, name)
		if err = os.MkdirAll(consumerPath, 0o700); err != nil {
			return err
		}

		if err = initializeFixtureCheckout(consumerPath); err != nil {
			return fmt.Errorf("create probe consumer %q: %w", name, err)
		}

		consumer := conformance.Consumer{
			Name:  name,
			Path:  consumerPath,
			Gates: []conformance.Command{{Arguments: []string{shellPath, "-c", shellExitZero}}},
		}
		if index == 0 {
			consumer.DeferralRetryProbe = conformance.Command{Arguments: []string{shellPath, "-c", driver.deferralProbeScript}}
		}

		manifest.Consumers = append(manifest.Consumers, consumer)
	}

	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(root, "manifest.json")
	if err = os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		return err
	}

	output := &bytes.Buffer{}
	driver.deferralProbeError = conformance.Run(context.Background(), manifestPath, output)
	driver.output = output.String()

	return nil
}

func (driver *Driver) requireDeferralProbeAccepted() error {
	if driver.deferralProbeError != nil {
		return fmt.Errorf("a consumer that waited for capacity was rejected: %w", driver.deferralProbeError)
	}
	if !strings.Contains(driver.output, "retried a capacity deferral") {
		return fmt.Errorf("the probe passed without recording that it retried: %s", driver.output)
	}

	return nil
}

func (driver *Driver) requireDeferralProbeRejectedForSurrender() error {
	if driver.deferralProbeError == nil {
		return errors.New("a consumer that read exit 75 as final was accepted")
	}
	if !strings.Contains(driver.deferralProbeError.Error(), "would read exit 75 as an admission") {
		return fmt.Errorf("rejected for the wrong reason: %w", driver.deferralProbeError)
	}

	return nil
}

func (driver *Driver) requireDeferralProbeRejectedForFinishingEarly() error {
	if driver.deferralProbeError == nil {
		return errors.New("a probe that never reached admission was accepted")
	}
	if !strings.Contains(driver.deferralProbeError.Error(), "before capacity could free") {
		return fmt.Errorf("rejected for the wrong reason: %w", driver.deferralProbeError)
	}

	return nil
}

// shellExitZero is the trivial payload several fixtures guard.
const shellExitZero = "exit 0"

// initializeFixtureCheckout creates the minimal clean git checkout the
// conformance harness requires of a consumer, so the three fixtures that need
// one stop repeating the same commands.
func initializeFixtureCheckout(path string) error {
	for _, arguments := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-q", "--allow-empty", "-m", fixtureOwner},
	} {
		command := GitCommand(path, arguments...)
		if output, commandError := command.CombinedOutput(); commandError != nil {
			return fmt.Errorf("%s: %w", output, commandError)
		}
	}

	return nil
}
