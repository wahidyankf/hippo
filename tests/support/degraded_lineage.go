package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	// ownerShareLimit is the most owners one shared root admits, so the most
	// shares an automatic reservation can be divided into.
	ownerShareLimit = 20
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

// logicalClock is the clock the admission guard reads: time passes only when
// the guard pauses, so the admission window counts the guard's own readings and
// no runner, however slow, can exhaust it before the readings decide admission.
type logicalClock struct{ now time.Time }

func (clock *logicalClock) Now() time.Time { return clock.now }

func (clock *logicalClock) Sleep(duration time.Duration) { clock.now = clock.now.Add(duration) }

// advancingCollector returns its samples in order and then repeats its final
// sample forever, stamping every reading one step after the last so a trend
// window keeps advancing instead of collapsing onto one timestamp.
type advancingCollector struct {
	samples []policy.Sample
	then    policy.Sample
	base    time.Time
	step    time.Duration
	index   int
	stall   time.Duration
}

func (collector *advancingCollector) Collect(ctx context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	if err := ctx.Err(); err != nil {
		return policy.Reading{}, err
	}

	time.Sleep(collector.stall)

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
	return driver.writeDerivedProfile(base, "")
}

// derivedProfileWithoutFallback is derivedProfileConfiguration for a profile
// that clears the fallback it would inherit, so nothing stands behind it.
func (driver *Driver) derivedProfileWithoutFallback(base string) error {
	return driver.writeDerivedProfile(base, `,"fallback":""`)
}

// derivedProfileFallingBackTo is derivedProfileConfiguration for a profile that
// names a fallback of its own, so it does not end its chain.
func (driver *Driver) derivedProfileFallingBackTo(base, fallback string) error {
	return driver.writeDerivedProfile(base, fmt.Sprintf(`,"fallback":%q`, fallback))
}

// writeDerivedProfile writes the configuration for a local profile extending
// base, with any further keys of that profile in extra.
func (driver *Driver) writeDerivedProfile(base, extra string) error {
	directory, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.configPath = filepath.Join(directory, "hippo.json")
	driver.configDocument = fmt.Sprintf(
		`{"schemaVersion":2,"defaultProfile":"local-%[1]s","profiles":{"local-%[1]s":{"extends":%[1]q%[2]s}}}`, base, extra,
	)

	return os.WriteFile(driver.configPath, []byte(driver.configDocument), 0o600)
}

// resolveProfile resolves the requested profile, or the default one when none is
// requested, against the scenario's configuration the way the command line does:
// through config.Load, which returns the built-in catalog when the scenario names
// no configuration. Every scenario that depends on a profile resolves here, so
// none resolves against a catalog the command line would not have built.
func (driver *Driver) resolveProfile(requested policy.ProfileName, taskClass policy.TaskClass, sample policy.Sample) (policy.Resolution, error) {
	loaded, err := config.Load(driver.configPath, driver.configPath != "")
	if err != nil {
		return policy.Resolution{}, err
	}

	return loaded.Catalog.Resolve(requested, taskClass, sample)
}

// resolveConfigured resolves the scenario configuration's default profile the
// way the command line does for an ephemeral run.
func (driver *Driver) resolveConfigured(taskClass policy.TaskClass, sample policy.Sample) (policy.Resolution, error) {
	return driver.resolveProfile("", taskClass, sample)
}

// guardUnderConfiguration runs ephemeral work under the configured profile
// while the host holds the scenario's warning window, then keeps holding it.
func (driver *Driver) guardUnderConfiguration() error {
	if len(driver.samples) == 0 {
		return errors.New("no host window was prepared")
	}

	resolution, err := driver.resolveConfigured(policy.TaskEphemeral, driver.samples[len(driver.samples)-1])
	if err != nil {
		return err
	}

	return driver.guardUnder(resolution)
}

// guardUnder runs ephemeral work under the resolution while the host holds the
// scenario's warning window, then keeps holding it. Its hundred-millisecond
// admission window is logical, so it admits at the sixteenth reading and defers
// after the hundred and first, whatever the host's speed.
func (driver *Driver) guardUnder(resolution policy.Resolution) error {
	last := driver.samples[len(driver.samples)-1]
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
	clock := &logicalClock{now: time.Now()}
	collector := &advancingCollector{
		samples: driver.samples, then: last, base: time.Unix(0, 0), step: time.Second, stall: driver.readingStall,
	}

	code, runError := guard.Run(context.Background(), guard.RunConfig{
		Command:                shellPath,
		Arguments:              []string{"-c", `[ "$HIPPO_CONCURRENCY" = 1 ]`},
		TaskClass:              policy.TaskEphemeral,
		Environment:            os.Environ(),
		EvidenceRoot:           root,
		DiskPath:               ".",
		Collector:              collector,
		Policy:                 resourcePolicy,
		Resolution:             resolution,
		RetirementConfirmation: fixtureLivenessWait,
		Sleep:                  clock.Sleep,
		Now:                    clock.Now,
		Stderr:                 stderr,
	})

	driver.resolution = resolution
	driver.errorOutput = stderr.String()

	return driver.recordGuardResult(code, runError)
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
	if driver.reason != policy.ReasonCapacityDeferred || strings.Contains(driver.errorOutput, degradedAdmissionLine) {
		return fmt.Errorf("profile %q: reason=%d exit=%d stderr=%q", driver.resolution.ResolvedProfile, driver.reason, driver.exitCode, driver.errorOutput)
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

// extendedBuiltin names the built-in profile a configured profile of the
// scenario's outline extends, or none for the built-in balanced profile itself.
func extendedBuiltin(profile string) (string, error) {
	switch profile {
	case "the built-in balanced profile":
		return "", nil
	case "a configured profile that extends balanced":
		return profileBalanced, nil
	case "a configured profile that extends constrained":
		return profileConstrained, nil
	case "a configured profile that extends minimal":
		return profileMinimal, nil
	default:
		return "", fmt.Errorf("unknown profile %q", profile)
	}
}

// hostWithProfileOfNoOwnerShare prepares healthy host capacity and the profile
// the outline names, resolved through the configuration it writes, so the only
// thing that can say how many owners share that capacity is the profile.
func (driver *Driver) hostWithProfileOfNoOwnerShare(profile string) error {
	base, err := extendedBuiltin(profile)
	if err != nil {
		return err
	}
	if base != "" {
		if err = driver.derivedProfileConfiguration(base); err != nil {
			return err
		}
	}

	sample := v04ReservationSample()
	driver.samples = []policy.Sample{sample}
	driver.resolution, err = driver.resolveProfile("", policy.TaskEphemeral, sample)

	return err
}

// planAutomaticReservation plans the reservation in the guard with an empty
// owner share map, which is the case a profile's lineage has to answer.
func (driver *Driver) planAutomaticReservation() error {
	settings := guard.ReservationPolicy{Enabled: true, MaxActiveOwners: ownerShareLimit, OwnerShares: map[policy.ProfileName]int{}}
	driver.automaticPlan, driver.automaticPlanError = guard.PlanReservation(
		driver.samples[0], driver.resolution, settings, 0, 0,
	)

	return nil
}

// requireOwnerShares holds the planned vector to the given number of owner
// shares of the safe capacity, and names the number it was divided into.
func (driver *Driver) requireOwnerShares(shares string) error {
	want, err := strconv.Atoi(shares)
	if err != nil {
		return err
	}
	if driver.automaticPlanError != nil {
		return driver.automaticPlanError
	}

	sample := driver.samples[0]
	cpuCapacity := int64(max(guard.MinimumReservationCPU, sample.AvailableParallelism-1))
	memoryCapacity := sample.EffectiveMemoryLimitBytes - driver.resolution.MemoryReserve
	vector := func(owners int) guard.ReservationVector {
		return guard.ReservationVector{
			CPU:         int(max(int64(guard.MinimumReservationCPU), exactCeilingV04(cpuCapacity, int64(owners)))),
			MemoryBytes: max(guard.MinimumReservationMemoryBytes, exactCeilingV04(memoryCapacity, int64(owners))),
		}
	}
	if driver.automaticPlan.Requested == vector(want) {
		return nil
	}

	for owners := 1; owners <= ownerShareLimit; owners++ {
		if driver.automaticPlan.Requested == vector(owners) {
			return fmt.Errorf("profile %q divided capacity into %d owner shares instead of %d",
				driver.resolution.ResolvedProfile, owners, want)
		}
	}

	return fmt.Errorf("profile %q planned %+v, want %d owner shares", driver.resolution.ResolvedProfile, driver.automaticPlan.Requested, want)
}

// balancedEphemeralChild admits an ephemeral child of the built-in balanced
// profile on healthy evidence.
func (driver *Driver) balancedEphemeralChild() error {
	resolution, err := driver.resolveProfile(profileBalanced, policy.TaskEphemeral, lineageHealthySample())
	if err != nil {
		return err
	}

	driver.lineage = lineageScenario{resolution: resolution, taskClass: policy.TaskEphemeral, spared: true}

	return driver.prepareGuardedExecution(nil)
}

// balancedLineageChild admits an ephemeral child of the profile the outline
// names, the built-in balanced profile or a configured one extending it, on
// healthy evidence. Both resolve through config.Load.
func (driver *Driver) balancedLineageChild(profile string) error {
	base, err := extendedBuiltin(profile)
	if err != nil {
		return err
	}
	if base != "" {
		if err = driver.derivedProfileConfiguration(base); err != nil {
			return err
		}
	}

	resolution, err := driver.resolveProfile("", policy.TaskEphemeral, lineageHealthySample())
	if err != nil {
		return err
	}
	if base != "" && resolution.ResolvedProfile != policy.ProfileName("local-"+base) {
		return fmt.Errorf("the configuration resolved %q, want its profile extending %s", resolution.ResolvedProfile, base)
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
		resolution, err := driver.resolveProfile(profileBalanced, policy.TaskService, lineageHealthySample())
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

	driver.errorOutput = stderr.String()
	if _, completedError := os.Stat(completed); completedError == nil {
		driver.childCompleted = true
	}

	return driver.recordGuardResult(code, err)
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
		if driver.reason != policy.ReasonStorageBlocked {
			return fmt.Errorf("got reason %d exit %d stderr=%q", driver.reason, driver.exitCode, driver.errorOutput)
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
