package support

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// The release monitor's endpoint variables, and the endpoints the scenario gives
// them: the test endpoints the release steps already use.
const (
	healthURLEnvironment       = "HIPPO_HEALTH_URL"
	routedOriginEnvironment    = "HIPPO_ROUTED_ORIGIN"
	identityEnvironment        = "HIPPO_IDENTITY"
	defaultIdentityEnvironment = "HIPPO_DEFAULT_IDENTITY"
)

func (driver *Driver) configEnvironmentAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a schema two configuration file$`, func() error {
			return driver.stageTierDocument(`{"schemaVersion":2}`)
		}),
		step(`^status reports the SHA-256 of that file's bytes as its configHash$`, driver.requireConfigHashOfFile),
		step(`^HIPPO_IDENTITY names an identity from source ([a-z-]+) and HIPPO_DEFAULT_IDENTITY names one from source ([a-z-]+)$`, driver.stageIdentityEnvironment),
		step(`^a guarded run of a passing command is requested without a source flag$`, driver.runPassingWithoutSource),
		step(`^the history of that run names ([a-z-]+) and does not name ([a-z-]+)$`, driver.requireHistoryNamesSource),
		step(`^HIPPO_HEALTH_URL names a health endpoint and HIPPO_ROUTED_ORIGIN names a routed origin$`, driver.stageEndpointEnvironment),
		step(`^release monitoring is requested with output paths and no endpoint flags$`, driver.releaseMonitorFromEnvironment),
		step(`^the release monitor is given the health endpoint and the routed origin those variables name$`, driver.requireMonitorEndpoints),
	}
}

// requireConfigHashOfFile holds the configHash status reports to the SHA-256 of
// the configuration file's bytes, which configuration.md promises.
func (driver *Driver) requireConfigHashOfFile() error {
	if driver.exitCode != 0 {
		return fmt.Errorf("status exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}
	document, err := os.ReadFile(driver.configPath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(document)
	var payload struct {
		ConfigHash string `json:"configHash"`
	}
	if err = json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return err
	}
	if want := hex.EncodeToString(sum[:]); payload.ConfigHash != want {
		return fmt.Errorf("configHash %q, want the SHA-256 of the file %q", payload.ConfigHash, want)
	}

	return nil
}

// stageIdentityEnvironment writes two identity files, one per source, and names
// them as HIPPO_IDENTITY and HIPPO_DEFAULT_IDENTITY for the run that follows.
func (driver *Driver) stageIdentityEnvironment(fromEnvironment, fromDefault string) error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root
	environmentPath := filepath.Join(root, "identity-environment.json")
	defaultPath := filepath.Join(root, "identity-default.json")
	if err = os.WriteFile(environmentPath, []byte(fmt.Sprintf(`{"schemaVersion":1,"source":%q}`, fromEnvironment)), 0o600); err != nil {
		return err
	}
	if err = os.WriteFile(defaultPath, []byte(fmt.Sprintf(`{"schemaVersion":1,"source":%q}`, fromDefault)), 0o600); err != nil {
		return err
	}
	driver.addCLI.identityEnvironment = map[string]string{
		identityEnvironment:        environmentPath,
		defaultIdentityEnvironment: defaultPath,
	}

	return nil
}

// runPassingWithoutSource guards a child that exits zero, with no --source flag,
// so the identity variables decide the run's source.
func (driver *Driver) runPassingWithoutSource() error {
	values := map[string]string{hippoRootEnvironment: driver.leaseRoot}
	maps.Copy(values, driver.addCLI.identityEnvironment)

	return driver.runCommandWith(values, runCommandName, diskPathFlag, driver.leaseRoot, "--", shellPath, "-c", "true")
}

// runCommandWith runs one invocation with the given variables set: in process at
// the unit and integration boundaries, through the compiled binary at the
// end-to-end one. A guarded child's own evidence is the only output the scenario reads.
func (driver *Driver) runCommandWith(values map[string]string, arguments ...string) error {
	if driver.mode == contract.E2E {
		return driver.runBinaryWith(values, arguments...)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := bootstrap.WithDefaults(system.Production, cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: append(environmentPairs(values), isolatedPath),
		Collector: &sequenceCollector{samples: []policy.Sample{healthySample(time.Now()), healthySample(time.Now())}},
		Sleep:     func(time.Duration) {},
	}).Run(context.Background(), arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

// requireHistoryNamesSource reads thirty days of history from the scenario's root
// and holds the one run to the source it took and not the other.
func (driver *Driver) requireHistoryNamesSource(want, unwanted string) error {
	if err := driver.requestJSONHistory(); err != nil {
		return err
	}
	if driver.exitCode != 0 {
		return fmt.Errorf("history exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}
	if !strings.Contains(driver.output, `"source":"`+want+`"`) || strings.Contains(driver.output, `"source":"`+unwanted+`"`) {
		return fmt.Errorf("history does not name %s alone: %s", want, driver.output)
	}

	return nil
}

// stageEndpointEnvironment names the release monitor's two endpoint variables with
// the endpoints the release steps already use, which both pass the monitor's checks.
func (driver *Driver) stageEndpointEnvironment() error {
	driver.addCLI.endpointEnvironment = map[string]string{
		healthURLEnvironment:    testHealthURL,
		routedOriginEnvironment: testRoutedOrigin,
	}

	return nil
}

// releaseMonitorFromEnvironment runs the release monitor with its output paths
// and no endpoint flags, and captures the configuration the boundary hands it.
// The compiled binary runs its own monitor, so only the in-process boundary can
// read that configuration.
func (driver *Driver) releaseMonitorFromEnvironment() error {
	if driver.mode == contract.E2E {
		return errors.New("the end-to-end boundary runs the monitor itself and does not expose its configuration")
	}
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root
	values := map[string]string{hippoRootEnvironment: root}
	maps.Copy(values, driver.addCLI.endpointEnvironment)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := bootstrap.WithDefaults(system.Production, cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: environmentPairs(values),
		MonitorRelease: func(_ context.Context, configuration application.MonitorConfig) error {
			driver.addCLI.capturedMonitor = &configuration

			return nil
		},
	}).Run(context.Background(), []string{
		releaseCommandName, monitorCommandName,
		outputFlag, filepath.Join(root, "samples.jsonl"),
		summaryFlag, filepath.Join(root, "summary.json"),
		deploymentRootFlag, root,
	})
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

// requireMonitorEndpoints holds the captured monitor to the two endpoints the
// variables named, with no flag supplying either.
func (driver *Driver) requireMonitorEndpoints() error {
	captured := driver.addCLI.capturedMonitor
	if driver.exitCode != 0 || captured == nil {
		return fmt.Errorf("exit=%d stderr=%q, the monitor was not given a configuration", driver.exitCode, driver.errorOutput)
	}
	if captured.HealthURL != testHealthURL || captured.RoutedOrigin != testRoutedOrigin {
		return fmt.Errorf("monitor health URL %q and routed origin %q, want %q and %q",
			captured.HealthURL, captured.RoutedOrigin, testHealthURL, testRoutedOrigin)
	}

	return nil
}
