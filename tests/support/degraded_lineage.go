package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/config"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

// Degraded admission belongs to the built-in balanced profile and every
// configured profile whose extends lineage reaches it. These bindings drive the
// guard with a configured catalog, and supervise children admitted under
// normal pressure through a stable or unsafe macOS warning.
const (
	degradedAdmissionLine = "HIPPO admitting ephemeral child under stable macOS warning pressure with concurrency 1."
	lineageSampleCount    = 20
	// lineagePrefixPayload is the compressor payload the healthy prefix holds,
	// so a stable warning reads as flat and the consumer's observed growth to
	// 12.9 GB reads as growth past the warning threshold.
	lineagePrefixPayload   = int64(11_100_000_000)
	lineageGrowingPayload  = int64(12_900_000_000)
	lineageSpareChild      = `sleep 0.5; printf d > "$CHILD_COMPLETED"`
	lineageShedChild       = `sleep 10; printf d > "$CHILD_COMPLETED"`
	derivedFromConstrained = "an ephemeral child of a profile derived from constrained"
	serviceOfBalanced      = "a service child of the built-in balanced profile"
)

// lineageScenario carries the resolution and class a supervised child runs
// under, and the pressure the host turns to once that child is admitted.
type lineageScenario struct {
	resolution policy.Resolution
	taskClass  policy.TaskClass
	pressure   policy.Sample
	// spared marks the child the exemption covers, which runs briefly so its
	// own exit is observable; every other child outlives the scenario.
	spared bool
}

// advancingCollector returns its samples in order and then repeats its final
// sample forever, stamping every reading one step after the last so a trend
// window keeps advancing instead of collapsing onto one timestamp.
type advancingCollector struct {
	samples []policy.Sample
	then    policy.Sample
	base    time.Time
	step    time.Duration
	index   int
}

func (collector *advancingCollector) Collect(ctx context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	if err := ctx.Err(); err != nil {
		return policy.Reading{}, err
	}

	sample := collector.then
	if collector.index < len(collector.samples) {
		sample = collector.samples[collector.index]
	}
	sample.MeasuredAt = collector.base.Add(time.Duration(collector.index) * collector.step).UTC().Format(time.RFC3339Nano)
	collector.index++

	return policy.Reading{CPUState: previous, Sample: sample}, nil
}

// derivedProfileConfiguration writes a schema-2 configuration whose default
// profile is a local profile extending base, and selects it for the scenario.
func (driver *Driver) derivedProfileConfiguration(base string) error {
	directory, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.configPath = filepath.Join(directory, "hippo.json")
	document := fmt.Sprintf(`{"schemaVersion":2,"defaultProfile":"local-%[1]s","profiles":{"local-%[1]s":{"extends":%[1]q}}}`, base)

	return os.WriteFile(driver.configPath, []byte(document), 0o600)
}

// resolveConfigured resolves the scenario configuration's default profile the
// way the command line does for an ephemeral run.
func (driver *Driver) resolveConfigured(taskClass policy.TaskClass, sample policy.Sample) (policy.Resolution, error) {
	loaded, err := config.Load(driver.configPath, true)
	if err != nil {
		return policy.Resolution{}, err
	}

	return loaded.Catalog.Resolve("", taskClass, sample)
}

// guardUnderConfiguration runs ephemeral work under the configured profile
// while the host holds the scenario's warning window, then keeps holding it.
func (driver *Driver) guardUnderConfiguration() error {
	if len(driver.samples) == 0 {
		return errors.New("no host window was prepared")
	}

	last := driver.samples[len(driver.samples)-1]
	resolution, err := driver.resolveConfigured(policy.TaskEphemeral, last)
	if err != nil {
		return err
	}

	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	resourcePolicy := resolution.Policy
	resourcePolicy.SampleInterval = time.Millisecond
	resourcePolicy.AdmissionWindow = 100 * time.Millisecond
	resourcePolicy.TerminationGrace = time.Millisecond
	resourcePolicy.LeaseWait = time.Second
	stderr := &bytes.Buffer{}

	code, runError := guard.Run(context.Background(), guard.RunConfig{
		Command:                shellPath,
		Arguments:              []string{"-c", `[ "$HIPPO_CONCURRENCY" = 1 ]`},
		TaskClass:              policy.TaskEphemeral,
		Environment:            os.Environ(),
		EvidenceRoot:           root,
		DiskPath:               ".",
		Collector:              &advancingCollector{samples: driver.samples, then: last, base: time.Unix(0, 0), step: time.Second},
		Policy:                 resourcePolicy,
		Resolution:             resolution,
		RetirementConfirmation: fixtureLivenessWait,
		Sleep:                  time.Sleep,
		Now:                    time.Now,
		Stderr:                 stderr,
	})

	driver.resolution = resolution
	driver.exitCode = code
	driver.errorOutput = stderr.String()

	return runError
}

func (driver *Driver) requireConfiguredDegradedAdmission() error {
	if driver.exitCode != 0 || !strings.Contains(driver.errorOutput, degradedAdmissionLine) {
		return fmt.Errorf("profile %q: exit=%d stderr=%q", driver.resolution.ResolvedProfile, driver.exitCode, driver.errorOutput)
	}

	return nil
}

// requireConfiguredDeferral holds the guard to its capacity-deferral status,
// the one the command line reports as 124 naming hippo.limit.capacity-deferred,
// as requireDeferred does for a held lease.
func (driver *Driver) requireConfiguredDeferral() error {
	if driver.exitCode != guard.CapacityDeferredExitCode || strings.Contains(driver.errorOutput, degradedAdmissionLine) {
		return fmt.Errorf("profile %q: exit=%d stderr=%q", driver.resolution.ResolvedProfile, driver.exitCode, driver.errorOutput)
	}

	return nil
}

// lineageHealthySample is a healthy Darwin reading whose compressor payload
// already sits where the warning that follows will hold it.
func lineageHealthySample() policy.Sample {
	sample := healthySample(time.Unix(0, 0))
	sample.CompressorPayloadBytes = new(lineagePrefixPayload)

	return sample
}

// lineageStableWarning is a stable macOS warning reading: level 2, headroom at
// the warning-admission floor, flat compressor payload and swap-outs.
func lineageStableWarning() policy.Sample {
	sample := lineageHealthySample()
	sample.MemoryPressureLevel = new(2)
	sample.AvailableMemoryBytes = new(8 * policy.GiB)
	sample.AvailableNonCompressedEstimateBytes = new(8 * policy.GiB)

	return sample
}

// balancedEphemeralChild admits an ephemeral child of the built-in balanced
// profile on healthy evidence.
func (driver *Driver) balancedEphemeralChild() error {
	resolution, err := policy.BuiltinCatalog().Resolve(profileBalanced, policy.TaskEphemeral, lineageHealthySample())
	if err != nil {
		return err
	}

	driver.lineage = lineageScenario{resolution: resolution, taskClass: policy.TaskEphemeral, spared: true}

	return driver.prepareGuardedExecution(nil)
}

// childOutsideExemption admits a child the exemption does not cover: ephemeral
// work of a configured profile derived from constrained, or a service of the
// built-in balanced profile.
func (driver *Driver) childOutsideExemption(child string) error {
	switch child {
	case derivedFromConstrained:
		if err := driver.derivedProfileConfiguration(profileConstrained); err != nil {
			return err
		}
		resolution, err := driver.resolveConfigured(policy.TaskEphemeral, lineageHealthySample())
		if err != nil {
			return err
		}
		driver.lineage = lineageScenario{resolution: resolution, taskClass: policy.TaskEphemeral}
	case serviceOfBalanced:
		resolution, err := policy.BuiltinCatalog().Resolve(profileBalanced, policy.TaskService, lineageHealthySample())
		if err != nil {
			return err
		}
		driver.lineage = lineageScenario{resolution: resolution, taskClass: policy.TaskService}
	default:
		return fmt.Errorf("unknown child %q", child)
	}

	return driver.prepareGuardedExecution(nil)
}

// holdStableWarning turns the host to a stable macOS warning and runs a child
// that finishes long after the class grace yet well inside the scenario.
func (driver *Driver) holdStableWarning() error {
	driver.lineage.pressure = lineageStableWarning()
	script := lineageShedChild
	if driver.lineage.spared {
		script = lineageSpareChild
	}

	return driver.superviseLineageChild(script)
}

// showUnsafePressure turns the host to a warning that is not stable, or to
// critical memory, or to a disk below its warning reserve.
func (driver *Driver) showUnsafePressure(pressure string) error {
	sample := lineageStableWarning()

	switch pressure {
	case "growing compressor payload":
		sample.CompressorPayloadBytes = new(lineageGrowingPayload)
	case "growing swap-outs":
		sample.SwapOuts = new(*sample.SwapOuts + 128*policy.MiB / *sample.PageSizeBytes)
	case "critical memory pressure":
		sample.MemoryPressureLevel = new(4)
		sample.AvailableMemoryBytes = new(3 * policy.GiB)
		sample.AvailableNonCompressedEstimateBytes = new(3 * policy.GiB)
	case "disk below its warning reserve":
		sample.DiskFreeBytes = new(25 * policy.GiB)
	default:
		return fmt.Errorf("unknown pressure %q", pressure)
	}

	driver.lineage.pressure = sample

	return driver.superviseLineageChild(lineageShedChild)
}

// superviseLineageChild admits the scenario child on a healthy prefix, then
// holds the scenario pressure. The trend window spans fifteen readings a
// millisecond apart, and each reading waits a real millisecond, so a
// three-millisecond class grace always ends before a step in compressor
// payload or swap-outs leaves the window.
func (driver *Driver) superviseLineageChild(script string) error {
	resourcePolicy := fastBehaviourPolicy()
	resourcePolicy.TrendWindow = 15 * time.Millisecond
	resourcePolicy.EphemeralWarningGrace = 3 * time.Millisecond
	resourcePolicy.ServiceWarningGrace = 3 * time.Millisecond

	prefix := make([]policy.Sample, lineageSampleCount)
	for index := range prefix {
		prefix[index] = lineageHealthySample()
	}

	completed := filepath.Join(driver.leaseRoot, "completed")
	stderr := &bytes.Buffer{}
	code, err := guard.Run(context.Background(), guard.RunConfig{
		Command:                shellPath,
		Arguments:              []string{"-c", script},
		TaskClass:              driver.lineage.taskClass,
		Environment:            append(os.Environ(), "CHILD_COMPLETED="+completed),
		EvidenceRoot:           driver.leaseRoot,
		DiskPath:               ".",
		Collector:              &advancingCollector{samples: prefix, then: driver.lineage.pressure, base: time.Unix(0, 0), step: time.Millisecond},
		Policy:                 resourcePolicy,
		Resolution:             driver.lineage.resolution,
		RetirementConfirmation: fixtureLivenessWait,
		Sleep:                  time.Sleep,
		Now:                    time.Now,
		Stderr:                 stderr,
	})

	driver.exitCode = code
	driver.errorOutput = stderr.String()
	if _, completedError := os.Stat(completed); completedError == nil {
		driver.childCompleted = true
	}

	return err
}

func (driver *Driver) requireChildFinished() error {
	if driver.exitCode != 0 || !driver.childCompleted {
		return fmt.Errorf("exit=%d completed=%t stderr=%q", driver.exitCode, driver.childCompleted, driver.errorOutput)
	}

	return nil
}

// requireLineageShed holds a shed to its cause: storage pressure leaves the
// guard as its storage status, every other pressure as the pressure-shed one.
func (driver *Driver) requireLineageShed(reason string) error {
	if driver.childCompleted {
		return fmt.Errorf("child finished instead of being shed: exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}

	switch reason {
	case "hippo.limit.pressure-shed":
		return driver.requireShed()
	case "hippo.limit.storage-blocked":
		if driver.exitCode != guard.StorageBlockedExitCode {
			return fmt.Errorf("got exit %d stderr=%q", driver.exitCode, driver.errorOutput)
		}

		return requireShedReasonsAtBoundary()
	default:
		return fmt.Errorf("unknown reason %q", reason)
	}
}

// requireStatusDegradedAdmission reads the JSON status profile and requires
// degraded admission exactly when the resolved profile derives from balanced.
func (driver *Driver) requireStatusDegradedAdmission() error {
	document := struct {
		Profile map[string]any `json:"profile"`
	}{}
	if err := json.Unmarshal([]byte(driver.output), &document); err != nil {
		return fmt.Errorf("status output is not JSON: %w: %q", err, driver.output)
	}

	resolved, _ := document.Profile["resolvedProfile"].(string)
	degraded, present := document.Profile["degradedAdmission"].(bool)
	if !present {
		return fmt.Errorf("status profile has no boolean degradedAdmission: %v", document.Profile)
	}
	if degraded != (resolved == "local-balanced") {
		return fmt.Errorf("profile %q reported degradedAdmission=%t", resolved, degraded)
	}

	return nil
}
