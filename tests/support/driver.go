package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/policy"
	releaseguard "github.com/wahidyankf/hippo/internal/release"
	"github.com/wahidyankf/hippo/tests/contract"
)

const (
	taskClassEphemeral    = "ephemeral"
	taskClassFlag         = "--class"
	diskPathFlag          = "--disk-path"
	profileBalanced       = "balanced"
	profileConstrained    = "constrained"
	profileMinimal        = "minimal"
	jsonFlag              = "--json"
	statusCommandName     = "status"
	monitorCommandName    = "monitor"
	runCommandName        = "run"
	usageBlockMarker      = "Usage:"
	versionCommandName    = "version"
	releaseCommandName    = "release"
	releaseAssessName     = "assess"
	outputFlag            = "--output"
	summaryFlag           = "--summary"
	deploymentRootFlag    = "--deployment-root"
	healthURLFlag         = "--health-url"
	routedOriginFlag      = "--routed-origin"
	testHealthURL         = "http://127.0.0.1/health"
	testRoutedOrigin      = "https://service.example"
	callerWorkersName     = "CALLER_WORKERS"
	toolWorkersName       = "TOOL_WORKERS"
	unexpectedArgument    = "unexpected"
	shellPath             = "/bin/sh"
	configFlag            = "--config"
	conformanceLabel      = "conformance"
	e2eMode               = "e2e"
	exclusiveMode         = "exclusive"
	reservationMode       = "reservation"
	fixtureOwner          = "fixture"
	capacityField         = "capacity"
	classField            = "class"
	profileField          = "profile"
	sequenceField         = "sequence"
	ownersField           = "owners"
	waitersField          = "waiters"
	schemaVersionField    = "schemaVersion"
	nextSequenceField     = "nextSequence"
	maxActiveOwnersFld    = "maxActiveOwners"
	succeedsOutcome       = "succeeds"
	trueCommandName       = "true"
	childStartedScript    = `printf started > "$CHILD_MARKER"`
	schemaOneDocument     = `{"schemaVersion":1}`
	pidField              = "pid"
	tokenField            = "token"
	requestedField        = "requested"
	childStartedArgScript = `printf started > "$1"`
)

// interruptReadinessWait bounds how long a fixture waits for a guarded child to
// publish readiness before interrupting it. It is generous because it only has
// to outlast admission and process start on a busy host, and a fixture that
// exceeds it fails with its own diagnostic rather than hanging.
const interruptReadinessWait = 30 * time.Second

type sequenceCollector struct {
	samples     []policy.Sample
	index       int
	failureFrom int
	cancel      context.CancelFunc
	cancelAfter int
}

func (collector *sequenceCollector) Collect(ctx context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	if err := ctx.Err(); err != nil {
		return policy.Reading{}, err
	}
	if collector.failureFrom > 0 && collector.index >= collector.failureFrom {
		return policy.Reading{}, errors.New("injected host evidence failure")
	}
	if len(collector.samples) == 0 {
		return policy.Reading{}, errors.New("no samples")
	}

	index := min(collector.index, len(collector.samples)-1)
	collector.index++
	if collector.cancel != nil && collector.index >= collector.cancelAfter {
		collector.cancel()
	}

	return policy.Reading{
		CPUState: previous,
		Sample:   collector.samples[index],
	}, nil
}

// Driver carries isolated scenario state for one adapter suite.
type Driver struct {
	mode                    string
	samples                 []policy.Sample
	assessment              policy.Assessment
	admitted, accepted      bool
	exitCode                int
	output, errorOutput     string
	binary, summaryPath     string
	temporaryPaths          []string
	lifecycleOK             bool
	cacheRoot               string
	historicalCaches        []string
	lintConfiguration       string
	lintCommand             string
	strictAdapters          bool
	unitExemptionsForbidden bool
	approvedExemptions      bool
	serialCompliance        bool
	e2ePlacement            bool
	conventionalCommits     bool
	stagedFormatting        bool
	pushQuickGate           bool
	coreCoverage            bool
	resolution              policy.Resolution
	requestedProfile        string
	taskClass               policy.TaskClass
	effectiveMemory         int64
	linuxMemInfo            string
	linuxCgroupLimit        int64
	configPath              string
	privateArtifacts        bool
	exampleTracked          bool
	applicationLayout       bool
	leaseRoot               string
	leaseHolder             int
	heavySession            *guard.Session
	serviceSessions         []*guard.Session
	coordinationMarker      []byte
	coordinationRequests    int
	coordinationDeferrals   int
	inheritedSessions       bool
	forceStopElapsed        time.Duration
	runtimeFailureOutput    string
	runtimeFailureExit      int
	usageErrorOutput        string
	terminationSignals      int
	supervisionFailure      error
	childReaped             bool
	evidenceRoot            string
	evidenceIdentifier      string
	evidenceSampleCount     int
	evidenceWriters         []*evidence.Writer
	excessEvidencePath      string
	excessEvidenceError     error
	inactiveEvidencePaths   []string
	operandCommandsRejected bool
	childInput              string
	childEnvironment        []string
	concurrencyEnvironment  []string
	childResolution         policy.Resolution
	releaseMonitorRun       func() error
	releaseMonitorError     error
	releaseRawPath          string
	releaseSummaryPath      string
	releaseArguments        []string
	releaseCollector        *sequenceCollector
	streamCloseCalls        int
	invalidMappingsRejected bool
	v04Error                error
	v04Session              *guard.Session
	v04State                []byte
	v04Action               func(string) error
	v04Scenario             string
}

type failingStream struct {
	closeCalls *int
}

func (stream *failingStream) Write([]byte) (int, error) {
	return 0, errors.New("injected downstream write failure")
}

func (stream *failingStream) Close() error {
	(*stream.closeCalls)++

	return nil
}

// NewDriver returns isolated scenario state for one behavior adapter.
func NewDriver(adapter contract.Adapter) *Driver {
	return &Driver{mode: adapter.Name}
}

func healthySample(at time.Time) policy.Sample {
	return policy.Sample{
		SchemaVersion:                       3,
		MeasuredAt:                          at.UTC().Format(time.RFC3339Nano),
		Platform:                            "darwin",
		Capabilities:                        []string{"compressor", "memory-pressure", "swap"},
		EffectiveMemoryLimitBytes:           32 * policy.GiB,
		AvailableMemoryBytes:                new(12 * policy.GiB),
		AvailableNonCompressedEstimateBytes: new(12 * policy.GiB),
		MemoryPressureLevel:                 new(1),
		CompressorAvailable:                 new(true),
		CompressorPayloadBytes:              new(7 * policy.GiB),
		PhysicalMemoryBytes:                 32 * policy.GiB,
		AvailableParallelism:                8,
		CPUUtilizationPercent:               new(20.0),
		DiskFreeBytes:                       new(40 * policy.GiB),
		DiskTotalBytes:                      new(512 * policy.GiB),
		PageSizeBytes:                       new(int64(16_384)),
		SwapIns:                             new(int64(10)),
		SwapOuts:                            new(int64(20)),
		SwapFreeBytes:                       new(2 * policy.GiB),
		SwapState:                           "idle",
	}
}

// Reset returns the shared driver to an isolated scenario state.
func (driver *Driver) Reset() {
	driver.cleanup()
	mode := driver.mode

	*driver = Driver{mode: mode}

	if mode == contract.E2E {
		driver.binary = os.Getenv("HIPPO_BIN")
	}
}

func (driver *Driver) cleanup() {
	for _, writer := range driver.evidenceWriters {
		_ = writer.Close()
	}
	driver.evidenceWriters = nil

	for _, path := range driver.temporaryPaths {
		_ = os.RemoveAll(path)
	}
	driver.temporaryPaths = nil
}

// Close removes temporary scenario resources.
func (driver *Driver) Close() {
	driver.cleanup()
}

func toolRoot() string {
	return filepath.Clean(filepath.Join("..", ".."))
}

func environmentWith(values map[string]string) []string {
	prefixes := make([]string, 0, len(values))

	for name := range values {
		prefixes = append(prefixes, name+"=")
	}

	environment := make([]string, 0, len(os.Environ())+len(values))

	for _, value := range os.Environ() {
		replaced := false

		for _, prefix := range prefixes {
			if strings.HasPrefix(value, prefix) {
				replaced = true
				break
			}
		}
		if !replaced {
			environment = append(environment, value)
		}
	}

	for name, value := range values {
		environment = append(environment, name+"="+value)
	}

	return environment
}

func (driver *Driver) threeHealthy() {
	base := time.Unix(0, 0)
	driver.samples = []policy.Sample{healthySample(base), healthySample(base.Add(time.Second)), healthySample(base.Add(2 * time.Second))}
}

func stableWarningSamples(base time.Time, interval time.Duration) []policy.Sample {
	samples := make([]policy.Sample, 16)

	for index := range samples {
		sample := healthySample(base.Add(time.Duration(index) * interval))
		level := 2
		available := 8 * policy.GiB
		sample.MemoryPressureLevel = &level
		sample.AvailableMemoryBytes = &available
		sample.AvailableNonCompressedEstimateBytes = &available
		samples[index] = sample
	}

	return samples
}

func (driver *Driver) stableDarwinWarning() {
	driver.taskClass = taskClassEphemeral
	driver.samples = stableWarningSamples(time.Unix(0, 0), time.Second)
}

func (driver *Driver) growingDarwinWarning() {
	driver.stableDarwinWarning()
	payload := *driver.samples[0].CompressorPayloadBytes + 2*policy.GiB
	driver.samples[len(driver.samples)-1].CompressorPayloadBytes = &payload
}

func (driver *Driver) strictDarwinWarning() {
	driver.stableDarwinWarning()
	driver.taskClass = "transactional"
}

func (driver *Driver) swapGrowth() {
	first := healthySample(time.Unix(0, 0))
	second := healthySample(time.Unix(15, 0))
	first.SwapOuts = new(int64(0))
	second.SwapOuts = new(128 * policy.MiB / 16_384)
	driver.samples = []policy.Sample{first, second}
}

func (driver *Driver) compressorGrowth() {
	first := healthySample(time.Unix(0, 0))
	second := healthySample(time.Unix(15, 0))
	first.CompressorPayloadBytes = new(11 * policy.GiB)
	second.CompressorPayloadBytes = new(12 * policy.GiB)
	driver.samples = []policy.Sample{first, second}
}

func (driver *Driver) assessAdmission() {
	if len(driver.samples) == 0 {
		driver.assessment = policy.ResourceAssessment(nil, policy.DefaultPolicy())

		return
	}

	resolution, err := policy.BuiltinCatalog().Resolve(driver.requestedProfile, driver.taskClass, driver.samples[len(driver.samples)-1])
	if err != nil {
		driver.errorOutput = err.Error()
		driver.exitCode = policy.ReplanRequiredExitCode

		return
	}

	driver.resolution = resolution
	driver.exitCode = resolution.ExitCode
	driver.assessment = policy.ResourceAssessment(driver.samples, resolution.Policy)
	driver.admitted = resolution.ExitCode == 0 && policy.AdmissionReady(driver.samples, resolution.Policy)
	if !driver.admitted &&
		driver.taskClass == taskClassEphemeral &&
		resolution.ResolvedProfile == profileBalanced &&
		policy.WarningAdmissionReady(driver.samples, resolution.Policy) {
		driver.resolution.Concurrency = 1
		driver.admitted = true
	}
}

func (driver *Driver) assessPressure() {
	if len(driver.samples) == 0 {
		driver.assessment = policy.ResourceAssessment(nil, policy.DefaultPolicy())

		return
	}

	resolution, err := policy.BuiltinCatalog().Resolve(driver.requestedProfile, driver.taskClass, driver.samples[len(driver.samples)-1])
	if err != nil {
		driver.errorOutput = err.Error()

		return
	}

	driver.resolution = resolution
	driver.assessment = policy.ResourceAssessment(driver.samples, resolution.Policy)
}

func (driver *Driver) requireAdmitted() error {
	if !driver.admitted {
		return errors.New("work was not admitted")
	}

	return nil
}

func (driver *Driver) requireDegradedAdmitted() error {
	if !driver.admitted || driver.resolution.Concurrency != 1 {
		return fmt.Errorf("got admitted=%t resolution=%+v", driver.admitted, driver.resolution)
	}
	return nil
}

func (driver *Driver) requireDegradedDeferred() error {
	if driver.admitted {
		return fmt.Errorf("unsafe degraded work was admitted with %+v", driver.resolution)
	}
	return nil
}

func (driver *Driver) requireStorageBlocked() error {
	if !driver.assessment.StorageBlocked || driver.exitCode != 73 {
		return fmt.Errorf("got %+v and exit %d", driver.assessment, driver.exitCode)
	}
	return nil
}

func (driver *Driver) requireReason(reason string) error {
	if driver.assessment.State != "warning" || driver.assessment.Reason != reason {
		return fmt.Errorf("got %+v", driver.assessment)
	}
	return nil
}

func (driver *Driver) temporaryRoot() (string, error) {
	directory, err := os.MkdirTemp("", "hippo-lease-")
	if err != nil {
		return "", err
	}

	driver.temporaryPaths = append(driver.temporaryPaths, directory)

	return directory, nil
}

func fastBehaviourPolicy() policy.Policy {
	policy := policy.DefaultPolicy()
	policy.SampleInterval = time.Millisecond
	policy.AdmissionWindow = time.Second
	policy.TerminationGrace = time.Millisecond
	policy.LeaseWait = time.Second

	return policy
}

func (driver *Driver) prepareGuardedExecution(samples []policy.Sample) error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.leaseRoot = root
	driver.samples = samples

	return nil
}

func (driver *Driver) runGuardedShell(script string, environment []string) error {
	if environment == nil {
		environment = os.Environ()
	}

	stderr := &bytes.Buffer{}
	started := time.Now()
	code, err := guard.Run(context.Background(), guard.RunConfig{
		Command:      shellPath,
		Arguments:    []string{"-c", script},
		TaskClass:    taskClassEphemeral,
		Environment:  environment,
		EvidenceRoot: driver.leaseRoot,
		DiskPath:     ".",
		Collector:    &sequenceCollector{samples: driver.samples},
		Policy:       fastBehaviourPolicy(),
		Sleep:        func(time.Duration) {},
		Now:          time.Now,
		Stderr:       stderr,
	})

	driver.forceStopElapsed = time.Since(started)
	driver.exitCode = code
	driver.errorOutput = stderr.String()

	return err
}

func (driver *Driver) admittedWithoutConcurrencyMappings() error {
	base := time.Unix(0, 0)
	driver.childEnvironment = []string{"PATH=" + os.Getenv("PATH")}
	driver.childResolution = policy.Resolution{
		RequestedProfile: profileBalanced,
		ResolvedProfile:  profileBalanced,
		FallbackChain:    []string{profileBalanced},
		Concurrency:      4,
	}

	return driver.prepareGuardedExecution([]policy.Sample{
		healthySample(base),
		healthySample(base.Add(time.Second)),
		healthySample(base.Add(2 * time.Second)),
	})
}

func (driver *Driver) admittedWithConcurrencyMappings() error {
	if err := driver.admittedWithoutConcurrencyMappings(); err != nil {
		return err
	}

	driver.childEnvironment = append(driver.childEnvironment, callerWorkersName+"=3")
	driver.concurrencyEnvironment = []string{toolWorkersName, callerWorkersName, toolWorkersName}

	return nil
}

func (driver *Driver) inspectGuardedEnvironment() error {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	arguments := make([]string, 0, 9+(2*len(driver.concurrencyEnvironment)))
	arguments = append(arguments, runCommandName, taskClassFlag, taskClassEphemeral, diskPathFlag, ".")
	for _, name := range driver.concurrencyEnvironment {
		arguments = append(arguments, "--concurrency-env", name)
	}
	arguments = append(
		arguments,
		"--",
		shellPath,
		"-c",
		`printf '%s:%s:%s:%s:%s:%s\n' "$HIPPO_PROFILE" "$HIPPO_CONCURRENCY" "${TOOL_WORKERS-unset}" "${CALLER_WORKERS-unset}" "${NX_PARALLEL-unset}" "${GOMAXPROCS-unset}"`,
	)
	environment := append([]string{}, driver.childEnvironment...)
	environment = append(environment, "HIPPO_ROOT="+driver.leaseRoot)
	code, err := (cli.Application{
		Stdout:      stdout,
		Stderr:      stderr,
		Environment: environment,
		Collector:   &sequenceCollector{samples: driver.samples},
		Sleep:       func(time.Duration) {},
	}).Run(context.Background(), arguments)

	driver.exitCode = code
	driver.output = stdout.String()
	driver.errorOutput = stderr.String()

	return err
}

func (driver *Driver) requireCanonicalConcurrencyOnly() error {
	if driver.exitCode != 0 || driver.output != "balanced:7:unset:unset:unset:unset\n" {
		return fmt.Errorf("exit=%d stdout=%q stderr=%q", driver.exitCode, driver.output, driver.errorOutput)
	}

	return nil
}

func (driver *Driver) requireMappedConcurrency() error {
	if driver.exitCode != 0 || driver.output != "balanced:7:7:3:unset:unset\n" {
		return fmt.Errorf("exit=%d stdout=%q stderr=%q", driver.exitCode, driver.output, driver.errorOutput)
	}

	return nil
}

func (driver *Driver) invalidConcurrencyMappings() error {
	return driver.admittedWithoutConcurrencyMappings()
}

func (driver *Driver) requestInvalidConcurrencyMappings() {
	marker := filepath.Join(driver.leaseRoot, "child-started")
	driver.invalidMappingsRejected = true

	for _, name := range []string{"9INVALID", "HIPPO_SESSION"} {
		code, err := guard.Run(context.Background(), guard.RunConfig{
			Command:                shellPath,
			Arguments:              []string{"-c", childStartedScript},
			TaskClass:              taskClassEphemeral,
			Environment:            environmentWith(map[string]string{"CHILD_MARKER": marker}),
			ConcurrencyEnvironment: []string{name},
			EvidenceRoot:           driver.leaseRoot,
			DiskPath:               ".",
			Collector:              &sequenceCollector{samples: driver.samples},
			Policy:                 fastBehaviourPolicy(),
			Resolution:             driver.childResolution,
			Sleep:                  func(time.Duration) {},
			Now:                    time.Now,
			Stderr:                 &bytes.Buffer{},
		})
		if code != 1 || err == nil {
			driver.invalidMappingsRejected = false
			driver.errorOutput = fmt.Sprintf("name=%q exit=%d error=%v", name, code, err)

			return
		}
	}

	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		driver.invalidMappingsRejected = false
		driver.errorOutput = "invalid mapping started its child"
	}
}

func (driver *Driver) requireInvalidMappingsRejected() error {
	if !driver.invalidMappingsRejected {
		return errors.New(driver.errorOutput)
	}

	return nil
}

func (driver *Driver) admittedGuardedStreams() error {
	if err := driver.admittedWithoutConcurrencyMappings(); err != nil {
		return err
	}

	driver.childInput = "hello-pipeline\n"

	return nil
}

func (driver *Driver) copyGuardedStreams() error {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	environment := append([]string{}, driver.childEnvironment...)
	environment = append(environment, "HIPPO_ROOT="+driver.leaseRoot)
	code, err := (cli.Application{
		Stdin:       strings.NewReader(driver.childInput),
		Stdout:      stdout,
		Stderr:      stderr,
		Environment: environment,
		Collector:   &sequenceCollector{samples: driver.samples},
		Sleep:       func(time.Duration) {},
	}).Run(context.Background(), []string{
		runCommandName, taskClassFlag, taskClassEphemeral, diskPathFlag, ".",
		"--", shellPath, "-c",
		`IFS= read -r value; printf 'stdout:%s\n' "$value"; printf 'stderr:%s\n' "$value" >&2`,
	})

	driver.exitCode = code
	driver.output = stdout.String()
	driver.errorOutput = stderr.String()

	return err
}

func (driver *Driver) requireComposableStreams() error {
	if driver.exitCode != 0 || driver.output != "stdout:hello-pipeline\n" || driver.errorOutput != "stderr:hello-pipeline\n" {
		return fmt.Errorf("exit=%d stdout=%q stderr=%q", driver.exitCode, driver.output, driver.errorOutput)
	}

	return nil
}

func (driver *Driver) emptyCoordinationRoot() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.leaseRoot = root

	return nil
}

func (driver *Driver) acquireExclusiveCompatibility() error {
	session, err := guard.AcquireSession(context.Background(), driver.leaseRoot, "", taskClassEphemeral, time.Second)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("exclusive compatibility work was deferred")
	}

	driver.heavySession = session

	return nil
}

func (driver *Driver) requireExclusiveCoordination() error {
	data, err := os.ReadFile(filepath.Join(driver.leaseRoot, "coordination-mode.json"))
	if err != nil {
		return fmt.Errorf("read coordination marker: %w", err)
	}

	var marker struct {
		SchemaVersion int    `json:"schemaVersion"`
		Mode          string `json:"mode"`
	}
	if err = json.Unmarshal(data, &marker); err != nil {
		return fmt.Errorf("decode coordination marker: %w", err)
	}
	if marker.SchemaVersion != 1 || marker.Mode != exclusiveMode {
		return fmt.Errorf("unexpected coordination marker: %+v", marker)
	}

	return nil
}

func (driver *Driver) releaseFinalCoordinationSession() error {
	if err := guard.ReleaseSession(driver.leaseRoot, driver.heavySession); err != nil {
		return err
	}
	driver.heavySession = nil

	if _, err := os.Stat(filepath.Join(driver.leaseRoot, "coordination-mode.json")); err == nil {
		return errors.New("coordination marker remained after final release")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect coordination marker after final release: %w", err)
	}

	return nil
}

func (driver *Driver) reservationCoordination() error {
	if err := driver.emptyCoordinationRoot(); err != nil {
		return err
	}

	driver.coordinationMarker = []byte("{\"schemaVersion\":1,\"mode\":\"reservation\"}\n")

	return os.WriteFile(
		filepath.Join(driver.leaseRoot, "coordination-mode.json"),
		driver.coordinationMarker,
		0o600,
	)
}

func (driver *Driver) requestEveryCompatibilityClass() error {
	classes := []policy.TaskClass{policy.TaskEphemeral, policy.TaskTransactional, policy.TaskService}
	driver.coordinationRequests = len(classes)
	if driver.mode == contract.E2E {
		return driver.requestEveryCompatibilityClassE2E(classes)
	}

	for _, class := range classes {
		session, err := guard.AcquireSession(context.Background(), driver.leaseRoot, "", class, 0)
		if session != nil {
			_ = guard.ReleaseSession(driver.leaseRoot, session)
		}
		if err != nil && session == nil && strings.Contains(err.Error(), "reservation mode is active") {
			driver.coordinationDeferrals++
			driver.supervisionFailure = errors.Join(driver.supervisionFailure, err)
		}
	}
	if driver.coordinationDeferrals == driver.coordinationRequests {
		driver.exitCode = guard.CapacityDeferredExitCode
	}

	return nil
}

func (driver *Driver) requestEveryCompatibilityClassE2E(classes []policy.TaskClass) error {
	for _, class := range classes {
		stderr := &bytes.Buffer{}
		command := exec.Command(
			driver.binary,
			"run",
			"--class", string(class),
			diskPathFlag, ".",
			"--", shellPath, "-c", "exit 99",
		)
		command.Env = environmentWith(map[string]string{"HIPPO_ROOT": driver.leaseRoot})
		command.Stderr = stderr

		err := command.Run()
		var exitError *exec.ExitError
		if errors.As(err, &exitError) &&
			exitError.ExitCode() == guard.CapacityDeferredExitCode &&
			strings.Contains(stderr.String(), "reservation mode is active") {
			driver.coordinationDeferrals++
			driver.supervisionFailure = errors.Join(
				driver.supervisionFailure,
				fmt.Errorf("%s compatibility deferral: %w", class, err),
			)
		}
		driver.errorOutput += stderr.String()
	}
	if driver.coordinationDeferrals == driver.coordinationRequests {
		driver.exitCode = guard.CapacityDeferredExitCode
	}

	return nil
}

func (driver *Driver) requireEveryCoordinationOwnerDeferred() error {
	if driver.exitCode != guard.CapacityDeferredExitCode ||
		driver.coordinationRequests != 3 ||
		driver.coordinationDeferrals != driver.coordinationRequests ||
		driver.supervisionFailure == nil {
		return fmt.Errorf(
			"exit=%d requests=%d deferrals=%d error=%w",
			driver.exitCode,
			driver.coordinationRequests,
			driver.coordinationDeferrals,
			driver.supervisionFailure,
		)
	}

	return nil
}

func (driver *Driver) requireReservationCoordinationUnchanged() error {
	data, err := os.ReadFile(filepath.Join(driver.leaseRoot, "coordination-mode.json"))
	if err != nil {
		return err
	}
	if !bytes.Equal(data, driver.coordinationMarker) {
		return fmt.Errorf("reservation coordination marker changed from %q to %q", driver.coordinationMarker, data)
	}

	return nil
}

func (driver *Driver) liveLease() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.leaseRoot = root
	driver.leaseHolder = os.Getpid()

	holder, err := guard.AcquireSession(context.Background(), root, "", taskClassEphemeral, time.Second)
	if err != nil {
		return err
	}
	if holder == nil {
		return errors.New("the first heavy owner was deferred")
	}

	driver.heavySession = holder

	return nil
}

func (driver *Driver) waitLease() error {
	second, err := guard.AcquireSession(context.Background(), driver.leaseRoot, "", taskClassEphemeral, 200*time.Millisecond)
	if err != nil {
		return err
	}

	if second != nil {
		driver.exitCode = 0

		return nil
	}

	driver.exitCode = guard.CapacityDeferredExitCode
	driver.errorOutput = guard.DescribeHeavyLease(driver.leaseRoot)

	return nil
}

func (driver *Driver) requireDeferred() error {
	if driver.exitCode != 75 {
		return fmt.Errorf("got exit %d", driver.exitCode)
	}
	return nil
}

func (driver *Driver) requireDeferralNamesHolder() error {
	if !strings.Contains(driver.errorOutput, "heavy-work lease") ||
		!strings.Contains(driver.errorOutput, strconv.Itoa(driver.leaseHolder)) {
		return fmt.Errorf("deferral did not name the holder: %q", driver.errorOutput)
	}
	return nil
}

func (driver *Driver) liveServiceLease() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.leaseRoot = root

	service, err := guard.AcquireSession(context.Background(), root, "", "service", time.Second)
	if err != nil {
		return err
	}
	if service == nil {
		return errors.New("service session was deferred")
	}

	driver.serviceSessions = []*guard.Session{service}

	return nil
}

func (driver *Driver) heavyRequestsLease() error {
	heavy, err := guard.AcquireSession(context.Background(), driver.leaseRoot, "", taskClassEphemeral, time.Second)
	if err != nil {
		return err
	}

	driver.heavySession = heavy

	return nil
}

func (driver *Driver) requireHeavyAcquired() error {
	if driver.heavySession == nil {
		return errors.New("heavy work was deferred by a service lease")
	}
	return nil
}

func (driver *Driver) twoServiceSessions() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.leaseRoot = root

	for range 2 {
		service, acquireError := guard.AcquireSession(context.Background(), root, "", "service", time.Second)
		if acquireError != nil {
			return acquireError
		}

		if service == nil {
			return errors.New("a concurrent service session was deferred")
		}
		driver.serviceSessions = append(driver.serviceSessions, service)
	}

	return nil
}

func (driver *Driver) validateInheritedSessions() {
	driver.inheritedSessions = true

	for _, service := range driver.serviceSessions {
		if !guard.InheritedSession(driver.leaseRoot, service.Token) {
			driver.inheritedSessions = false
		}
	}
}

func (driver *Driver) requireInheritedSessionsValid() error {
	if len(driver.serviceSessions) != 2 || !driver.inheritedSessions {
		return errors.New("concurrent service sessions were not independently inheritable")
	}
	return nil
}

func (driver *Driver) inherited() error {
	base := time.Now()
	if err := driver.prepareGuardedExecution([]policy.Sample{healthySample(base)}); err != nil {
		return err
	}

	session, err := guard.AcquireSession(context.Background(), driver.leaseRoot, "", taskClassEphemeral, time.Second)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("inherited session owner was not admitted")
	}

	driver.heavySession = session

	return nil
}

func (driver *Driver) successfulChild() error {
	err := driver.runGuardedShell(
		"exit 0",
		[]string{"HIPPO_SESSION=" + driver.heavySession.Token},
	)
	driver.inheritedSessions = guard.InheritedSession(driver.leaseRoot, driver.heavySession.Token)

	return err
}

func (driver *Driver) requirePreserved() error {
	if driver.exitCode != 0 || !driver.inheritedSessions {
		return fmt.Errorf("exit=%d inherited-session-owned=%t", driver.exitCode, driver.inheritedSessions)
	}
	return nil
}

func (driver *Driver) givenAdmitted() error {
	base := time.Now()

	return driver.prepareGuardedExecution([]policy.Sample{
		healthySample(base),
		healthySample(base.Add(time.Millisecond)),
		healthySample(base.Add(2 * time.Millisecond)),
	})
}

func (driver *Driver) child17() error {
	return driver.runGuardedShell("exit 17", nil)
}

func (driver *Driver) require17() error {
	if driver.exitCode != 17 {
		return fmt.Errorf("got exit %d", driver.exitCode)
	}
	return nil
}

func (driver *Driver) stubbornChild() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.leaseRoot = root

	return nil
}

func (driver *Driver) interruptGuard() error {
	base := time.Now()
	resourcePolicy := policy.DefaultPolicy()
	resourcePolicy.SampleInterval = time.Millisecond
	resourcePolicy.AdmissionWindow = time.Second
	resourcePolicy.TerminationGrace = 200 * time.Millisecond
	resourcePolicy.LeaseWait = time.Second

	marker := filepath.Join(driver.leaseRoot, "terminations")
	ready := filepath.Join(driver.leaseRoot, "trap-installed")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The scenario needs a child that is genuinely ignoring termination when the
	// interrupt arrives. A wall-clock delay cannot promise that: on a loaded host
	// the signal can land after the shell is running but before it installs its
	// trap, which kills the child by SIGTERM's default action and leaves nothing
	// for the scenario to observe. Interrupting on the child's own readiness mark
	// establishes the Given instead of assuming it.
	interrupted := make(chan time.Time, 1)

	go func() {
		defer cancel()

		interrupted <- awaitMarker(ready, interruptReadinessWait)
	}()

	// The child waits without forking. A forked foreground child shares the
	// payload process group, so one group SIGTERM kills it too and the shell then
	// runs its trap a second time to report the child that died from that signal,
	// which makes the trap count an unreliable witness for how often the guard
	// actually signalled. A builtin-only wait counts kernel deliveries exactly,
	// and its bound stops a guard that never force-stops from leaving a spinning
	// orphan behind.
	_, err := guard.Run(ctx, guard.RunConfig{
		Command:      shellPath,
		Arguments:    []string{"-c", `trap 'printf x >> "$GUARD_TERM_MARKER"' TERM; printf r > "$GUARD_READY_MARKER"; attempt=0; while [ "$attempt" -lt 2000000 ]; do attempt=$((attempt+1)); done`},
		TaskClass:    taskClassEphemeral,
		Environment:  append(os.Environ(), "GUARD_TERM_MARKER="+marker, "GUARD_READY_MARKER="+ready),
		EvidenceRoot: driver.leaseRoot,
		DiskPath:     ".",
		Collector: &sequenceCollector{samples: []policy.Sample{
			healthySample(base),
			healthySample(base.Add(time.Millisecond)),
			healthySample(base.Add(2 * time.Millisecond)),
		}},
		Policy: resourcePolicy,
		Sleep:  func(time.Duration) {},
		Now:    time.Now,
		Stderr: &bytes.Buffer{},
	})

	// The bound belongs to the stop, not to however long a busy host took to admit
	// and start the child, so it is measured from the interrupt.
	driver.forceStopElapsed = time.Since(<-interrupted)

	if err != nil {
		return err
	}

	if _, readyError := os.Stat(ready); readyError != nil {
		return fmt.Errorf("child never installed its termination trap: %w", readyError)
	}

	delivered, readError := os.ReadFile(marker)
	if readError != nil {
		return fmt.Errorf("child recorded no termination signal: %w", readError)
	}

	driver.terminationSignals = len(delivered)

	return nil
}

// awaitMarker waits for a child readiness mark and reports when the wait ended.
// It returns on the deadline as well, so a child that never publishes readiness
// still reaches its own explicit assertion instead of hanging the suite.
func awaitMarker(path string, wait time.Duration) time.Time {
	deadline := time.Now().Add(wait)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			break
		}

		time.Sleep(time.Millisecond)
	}

	return time.Now()
}

func (driver *Driver) requireForceStopped() error {
	if driver.terminationSignals != 1 {
		return fmt.Errorf("guard delivered %d termination signals, want exactly 1", driver.terminationSignals)
	}

	if driver.forceStopElapsed > 1500*time.Millisecond {
		return fmt.Errorf("a child ignoring SIGTERM was not force-stopped: guard returned %s after the interrupt", driver.forceStopElapsed)
	}
	return nil
}

func (driver *Driver) stubbornCollectorFailureChild() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.leaseRoot = root

	return nil
}

func (driver *Driver) loseHostEvidence() error {
	base := time.Now()
	resourcePolicy := fastBehaviourPolicy()
	resourcePolicy.SampleInterval = 20 * time.Millisecond
	resourcePolicy.TerminationGrace = 50 * time.Millisecond
	pidPath := filepath.Join(driver.leaseRoot, "child.pid")
	collector := &sequenceCollector{
		samples: []policy.Sample{
			healthySample(base),
			healthySample(base.Add(time.Millisecond)),
			healthySample(base.Add(2 * time.Millisecond)),
		},
		failureFrom: 3,
	}

	code, runError := guard.Run(context.Background(), guard.RunConfig{
		Command:      shellPath,
		Arguments:    []string{"-c", `trap '' TERM; printf '%s' "$$" > "$GUARD_CHILD_PID"; while :; do sleep 1; done`},
		TaskClass:    policy.TaskEphemeral,
		Environment:  append(os.Environ(), "GUARD_CHILD_PID="+pidPath),
		EvidenceRoot: driver.leaseRoot,
		DiskPath:     ".",
		Collector:    collector,
		Policy:       resourcePolicy,
		Sleep:        func(time.Duration) {},
		Now:          time.Now,
		Stderr:       &bytes.Buffer{},
	})

	driver.exitCode = code
	driver.supervisionFailure = runError

	pidData, readError := os.ReadFile(pidPath)
	if readError != nil {
		return fmt.Errorf("read guarded child PID: %w", readError)
	}

	pid, parseError := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if parseError != nil {
		return fmt.Errorf("parse guarded child PID: %w", parseError)
	}

	driver.childReaped = errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)

	return nil
}

func (driver *Driver) requireReapedBeforeRelease() error {
	if driver.supervisionFailure == nil {
		return fmt.Errorf("exit=%d without a supervision error", driver.exitCode)
	}
	if driver.exitCode != 1 || !strings.Contains(driver.supervisionFailure.Error(), "injected host evidence failure") {
		return fmt.Errorf("exit=%d with unexpected supervision error: %w", driver.exitCode, driver.supervisionFailure)
	}
	if !driver.childReaped {
		return errors.New("guarded child remained live after supervision returned")
	}
	if _, err := os.Stat(filepath.Join(driver.leaseRoot, "heavy.lock")); err == nil {
		return errors.New("heavy lease remained after child reaping")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect heavy lease after child reaping: %w", err)
	}

	session, err := guard.AcquireSession(context.Background(), driver.leaseRoot, "", policy.TaskEphemeral, 0)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("heavy lease was released before it became reacquirable")
	}

	return guard.ReleaseSession(driver.leaseRoot, session)
}

func (driver *Driver) smallEvidenceStream() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.evidenceRoot = root
	driver.evidenceIdentifier = "bounded-lifetime"
	driver.evidenceSampleCount = 40

	return nil
}

func (driver *Driver) overflowEvidenceChunks() error {
	writer, err := guard.NewEvidenceWriter(
		driver.evidenceRoot,
		driver.evidenceIdentifier,
		evidence.Limits{ChunkBytes: 1024, Chunks: 5},
	)
	if err != nil {
		return err
	}

	for index := range driver.evidenceSampleCount {
		sample := healthySample(time.Unix(int64(index), 0))
		if err := writer.Append(sample); err != nil {
			return err
		}
	}

	_, err = writer.Finalize(policy.TaskEphemeral, "passed", 0)

	return err
}

func (driver *Driver) requireBoundedEvidenceChunks() error {
	pattern := filepath.Join(driver.evidenceRoot, driver.evidenceIdentifier+"*.jsonl")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	if len(paths) != 5 {
		return fmt.Errorf("retained %d raw evidence chunks, want 5: %v", len(paths), paths)
	}

	expected := map[string]bool{
		driver.evidenceIdentifier + ".jsonl":   true,
		driver.evidenceIdentifier + ".1.jsonl": true,
		driver.evidenceIdentifier + ".2.jsonl": true,
		driver.evidenceIdentifier + ".3.jsonl": true,
		driver.evidenceIdentifier + ".4.jsonl": true,
	}
	for _, path := range paths {
		info, statError := os.Stat(path)
		if statError != nil {
			return statError
		}
		if !expected[filepath.Base(path)] || info.Size() > 1024 {
			return fmt.Errorf("unexpected evidence chunk %s (%d bytes)", path, info.Size())
		}
	}

	return nil
}

func (driver *Driver) requireLifetimeEvidenceSummary() error {
	path := filepath.Join(driver.evidenceRoot, driver.evidenceIdentifier+".summary.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var summary guard.EvidenceSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return err
	}
	if summary.SampleCount != driver.evidenceSampleCount {
		return fmt.Errorf("summary counted %d samples, want %d", summary.SampleCount, driver.evidenceSampleCount)
	}

	return nil
}

func (driver *Driver) twentyLiveEvidenceStreams() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.evidenceRoot = root
	for index := range 20 {
		path := filepath.Join(root, fmt.Sprintf("live-%02d.jsonl", index))
		writer, writerError := evidence.NewWriter(path, evidence.Limits{ChunkBytes: 1024, Chunks: 5})
		if writerError != nil {
			return fmt.Errorf("open live evidence stream %d: %w", index+1, writerError)
		}
		driver.evidenceWriters = append(driver.evidenceWriters, writer)
	}

	return nil
}

func (driver *Driver) startExcessEvidenceStream() {
	driver.excessEvidencePath = filepath.Join(driver.evidenceRoot, "live-over-limit.jsonl")
	writer, err := evidence.NewWriter(driver.excessEvidencePath, evidence.Limits{ChunkBytes: 1024, Chunks: 5})
	driver.excessEvidenceError = err
	if writer != nil {
		driver.evidenceWriters = append(driver.evidenceWriters, writer)
	}
}

func (driver *Driver) requireExcessEvidenceRejected() error {
	if driver.excessEvidenceError == nil || !strings.Contains(driver.excessEvidenceError.Error(), "live evidence session limit") {
		if driver.excessEvidenceError == nil {
			return errors.New("twenty-first evidence writer was admitted")
		}

		return fmt.Errorf("unexpected twenty-first writer result: %w", driver.excessEvidenceError)
	}
	if _, err := os.Stat(driver.excessEvidencePath); err == nil {
		return errors.New("rejected writer created raw evidence")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect rejected writer path: %w", err)
	}

	return nil
}

func (driver *Driver) inactiveEvidenceAboveBudget() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.evidenceRoot = root
	base := time.Unix(2_000_000, 0)
	for index := range 3 {
		path := filepath.Join(root, fmt.Sprintf("inactive-%d.jsonl", index))
		file, createError := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if createError != nil {
			return createError
		}
		if truncateError := file.Truncate(20 * 1024 * 1024); truncateError != nil {
			_ = file.Close()

			return truncateError
		}
		if closeError := file.Close(); closeError != nil {
			return closeError
		}

		modified := base.Add(time.Duration(index) * time.Second)
		if chtimesError := os.Chtimes(path, modified, modified); chtimesError != nil {
			return chtimesError
		}
		driver.inactiveEvidencePaths = append(driver.inactiveEvidencePaths, path)
	}

	return nil
}

func (driver *Driver) enforceEvidenceRetention() error {
	return evidence.Cleanup(driver.evidenceRoot, time.Unix(2_000_100, 0))
}

func (driver *Driver) requireInactiveEvidencePruned() error {
	if len(driver.inactiveEvidencePaths) != 3 {
		return fmt.Errorf("prepared %d inactive evidence files, want 3", len(driver.inactiveEvidencePaths))
	}
	if _, err := os.Stat(driver.inactiveEvidencePaths[0]); !errors.Is(err, os.ErrNotExist) {
		return errors.New("oldest inactive evidence was retained")
	}

	var total int64
	for _, path := range driver.inactiveEvidencePaths[1:] {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		total += info.Size()
	}
	if total > 50*1024*1024 {
		return fmt.Errorf("inactive evidence retained %d bytes", total)
	}

	return nil
}

func (driver *Driver) criticalChild() error {
	base := time.Now()
	healthy := healthySample(base)
	critical := healthySample(base.Add(3 * time.Millisecond))
	level := 4
	critical.MemoryPressureLevel = &level

	return driver.prepareGuardedExecution([]policy.Sample{
		healthy,
		healthySample(base.Add(time.Millisecond)),
		healthySample(base.Add(2 * time.Millisecond)),
		critical,
	})
}

func (driver *Driver) observeCritical() error {
	return driver.runGuardedShell("sleep 10", nil)
}

func (driver *Driver) requireShed() error {
	if driver.exitCode != 75 {
		return fmt.Errorf("got exit %d", driver.exitCode)
	}
	if driver.forceStopElapsed >= 3*time.Second {
		return fmt.Errorf("critical child was not shed promptly: %s", driver.forceStopElapsed)
	}

	return nil
}

func (driver *Driver) degradedGrowthChild() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.leaseRoot = root

	return nil
}

func (driver *Driver) observeDegradedWarning() error {
	base := time.Now()
	resourcePolicy := policy.DefaultPolicy()
	resourcePolicy.SampleInterval = time.Millisecond
	resourcePolicy.TrendWindow = 15 * time.Millisecond
	resourcePolicy.AdmissionWindow = 30 * time.Millisecond
	resourcePolicy.EphemeralWarningGrace = 3 * time.Millisecond
	resourcePolicy.TerminationGrace = time.Millisecond
	resourcePolicy.LeaseWait = time.Second

	samples := stableWarningSamples(base, time.Millisecond)

	for index := range 20 {
		sample := samples[len(samples)-1]
		sample.MeasuredAt = base.Add(time.Duration(16+index) * time.Millisecond).UTC().Format(time.RFC3339Nano)
		payload := *samples[0].CompressorPayloadBytes + 2*policy.GiB
		sample.CompressorPayloadBytes = &payload
		samples = append(samples, sample)
	}

	stderr := &bytes.Buffer{}

	code, runError := guard.Run(context.Background(), guard.RunConfig{
		Command:                shellPath,
		Arguments:              []string{"-c", `[ "$HIPPO_CONCURRENCY" = 1 ] && [ "$TOOL_WORKERS" = 1 ] && [ "$CALLER_WORKERS" = 1 ]; sleep 5`},
		TaskClass:              taskClassEphemeral,
		Environment:            environmentWith(map[string]string{"TOOL_WORKERS": "7", "CALLER_WORKERS": "3"}),
		ConcurrencyEnvironment: []string{"TOOL_WORKERS", "CALLER_WORKERS"},
		EvidenceRoot:           driver.leaseRoot,
		DiskPath:               ".",
		Collector:              &sequenceCollector{samples: samples},
		Policy:                 resourcePolicy,
		Resolution: policy.Resolution{
			RequestedProfile: profileBalanced,
			ResolvedProfile:  profileBalanced,
			FallbackChain:    []string{profileBalanced},
			Concurrency:      7,
		},
		Sleep:  func(time.Duration) {},
		Now:    time.Now,
		Stderr: stderr,
	})

	driver.exitCode = code
	driver.errorOutput = stderr.String()

	return runError
}

func (driver *Driver) requireDegradedShed() error {
	if driver.exitCode != guard.CapacityDeferredExitCode ||
		!strings.Contains(driver.errorOutput, "admitting") ||
		!strings.Contains(driver.errorOutput, "shedding") {
		return fmt.Errorf("exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}
	return nil
}

func (driver *Driver) compiledBinary() error {
	if driver.mode != contract.E2E {
		driver.binary = "in-process"

		return nil
	}

	driver.binary = os.Getenv("HIPPO_BIN")
	if driver.binary == "" {
		return errors.New("HIPPO_BIN is required")
	}

	return nil
}

func (driver *Driver) runBinary(arguments ...string) {
	driver.runBinaryWithInput("", arguments...)
}

func (driver *Driver) runBinaryWithInput(input string, arguments ...string) {
	command := exec.Command(driver.binary, arguments...)
	command.Stdin = strings.NewReader(input)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	driver.output = stdout.String()
	driver.errorOutput = stderr.String()

	if err != nil {
		driver.exitCode = 1
		exitError := &exec.ExitError{}
		if errors.As(err, &exitError) {
			driver.exitCode = exitError.ExitCode()
		}
	}
}

func (driver *Driver) jsonStatus() error {
	if driver.mode == contract.E2E {
		driver.runBinary(statusCommandName, jsonFlag, diskPathFlag, ".")

		return nil
	}

	base := time.Unix(0, 0)
	collector := &sequenceCollector{samples: []policy.Sample{healthySample(base), healthySample(base.Add(time.Second))}}
	var stdout, stderr bytes.Buffer
	code, err := (cli.Application{
		Stdout:    &stdout,
		Stderr:    &stderr,
		Collector: collector,
		Sleep:     func(time.Duration) {},
	}).Run(context.Background(), []string{statusCommandName, jsonFlag, diskPathFlag, "."})

	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return err
}

func (driver *Driver) jsonVersion() error {
	if driver.mode == contract.E2E {
		driver.runBinary("version", jsonFlag)

		return nil
	}

	var stdout bytes.Buffer
	code, err := (cli.Application{
		Stdout:  &stdout,
		Stderr:  &bytes.Buffer{},
		Version: "v0.0.0-test",
		Commit:  strings.Repeat("0", 40),
	}).Run(context.Background(), []string{versionCommandName, jsonFlag})

	driver.exitCode, driver.output = code, stdout.String()

	return err
}

func (driver *Driver) requireVersion() error {
	var payload struct {
		SchemaVersion int    `json:"schemaVersion"`
		Version       string `json:"version"`
		Commit        string `json:"commit"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return err
	}

	if driver.exitCode != 0 ||
		payload.SchemaVersion != 1 ||
		payload.Version != "v0.0.0-test" ||
		payload.Commit != strings.Repeat("0", 40) {
		return fmt.Errorf("invalid version: exit=%d payload=%+v", driver.exitCode, payload)
	}
	return nil
}

func (driver *Driver) requireStatus() error {
	var payload struct {
		SchemaVersion int                      `json:"schemaVersion"`
		Resource      *policy.Assessment       `json:"resource"`
		Profile       *policy.Resolution       `json:"profile"`
		Capabilities  []string                 `json:"capabilities"`
		Coordination  *guard.ReservationTotals `json:"coordination"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return err
	}
	if driver.exitCode != 0 ||
		payload.SchemaVersion != 4 ||
		payload.Resource == nil ||
		payload.Profile == nil ||
		payload.Coordination == nil || payload.Coordination.SchemaVersion != 4 ||
		len(payload.Capabilities) == 0 {
		return fmt.Errorf("invalid status: exit=%d payload=%+v", driver.exitCode, payload)
	}
	return nil
}

func (driver *Driver) runCLI(arguments ...string) error {
	if driver.mode == contract.E2E {
		driver.runBinary(arguments...)

		return nil
	}

	var stdout, stderr bytes.Buffer
	exitCode, err := (cli.Application{
		Stdout: &stdout,
		Stderr: &stderr,
	}).Run(context.Background(), arguments)

	driver.exitCode = exitCode
	driver.output = stdout.String()
	driver.errorOutput = stderr.String()

	return err
}

func (driver *Driver) requestRuntimeFailureAndUsageError() error {
	// An unresolvable guarded command fails only after its arguments were
	// accepted, so it is a runtime failure and not a caller usage mistake.
	// Cobra renders the error on the error stream and the usage block on the
	// output stream, so both are inspected together.
	_ = driver.runCLI(runCommandName, diskPathFlag, ".", "--", "/nonexistent/guarded-command")
	driver.runtimeFailureOutput = driver.output + driver.errorOutput
	driver.runtimeFailureExit = driver.exitCode
	// Omitting the command separator is a usage mistake, which must still show
	// the caller how the command is used.
	_ = driver.runCLI(runCommandName)
	driver.usageErrorOutput = driver.output + driver.errorOutput

	return nil
}

func (driver *Driver) requireUsageOnlyForUsageErrors() error {
	if driver.runtimeFailureExit == 0 {
		return errors.New("an unresolvable guarded command exited successfully")
	}
	if strings.Contains(driver.runtimeFailureOutput, usageBlockMarker) {
		return fmt.Errorf("a runtime failure printed the usage block: %q", driver.runtimeFailureOutput)
	}
	if !strings.Contains(driver.usageErrorOutput, usageBlockMarker) {
		return fmt.Errorf("a usage error omitted the usage block: %q", driver.usageErrorOutput)
	}

	return nil
}

func (driver *Driver) rootHelp() error {
	return driver.runCLI("--help")
}

func (driver *Driver) requireHelp() error {
	commands := []string{"completion", monitorCommandName, releaseCommandName, "run", statusCommandName, versionCommandName}
	if driver.exitCode != 0 {
		return fmt.Errorf("help exited %d: %s", driver.exitCode, driver.errorOutput)
	}

	for _, command := range commands {
		if !strings.Contains(driver.output, command) {
			return fmt.Errorf("help does not list %q: %s", command, driver.output)
		}
	}

	return nil
}

func (driver *Driver) requireHIPPOExpansion() error {
	const expansion = "HIPPO — Host Infrastructure Pressure & Process Orchestrator"
	if driver.exitCode != 0 || !strings.Contains(driver.output, expansion) {
		return fmt.Errorf("help does not identify HIPPO: exit=%d output=%q", driver.exitCode, driver.output)
	}

	return nil
}

func (driver *Driver) releaseHelp() error {
	return driver.runCLI(releaseCommandName, "--help")
}

func (driver *Driver) requireReleaseHelp() error {
	commands := []string{releaseAssessName, "check", monitorCommandName}
	if driver.exitCode != 0 {
		return fmt.Errorf("release help exited %d: %s", driver.exitCode, driver.errorOutput)
	}

	for _, command := range commands {
		if !strings.Contains(driver.output, command) {
			return fmt.Errorf("release help does not list %q: %s", command, driver.output)
		}
	}

	return nil
}

func (driver *Driver) zshCompletion() error {
	return driver.runCLI("completion", "zsh")
}

func (driver *Driver) requireZshCompletion() error {
	if driver.exitCode != 0 || !strings.Contains(driver.output, "#compdef hippo") {
		return fmt.Errorf("exit=%d output=%q error=%q", driver.exitCode, driver.output, driver.errorOutput)
	}

	return nil
}

func (driver *Driver) unknownCommand() {
	_ = driver.runCLI("not-a-command")
}

func (driver *Driver) requireCobraDiagnostic() error {
	if driver.exitCode != 1 ||
		!strings.Contains(driver.errorOutput, "unknown command \"not-a-command\"") ||
		!strings.Contains(driver.errorOutput, "hippo --help") {
		return fmt.Errorf("exit=%d error=%q", driver.exitCode, driver.errorOutput)
	}

	return nil
}

func (driver *Driver) invalidRun() error {
	if driver.mode == contract.E2E {
		driver.runBinary(runCommandName, taskClassFlag, taskClassEphemeral)

		return nil
	}

	_, err := (cli.Application{
		Stdout:    &bytes.Buffer{},
		Stderr:    &bytes.Buffer{},
		Collector: &sequenceCollector{},
	}).Run(context.Background(), []string{runCommandName, taskClassFlag, taskClassEphemeral})
	if err != nil {
		driver.errorOutput = err.Error()
		driver.exitCode = 1
	}

	return nil
}

func (driver *Driver) requireValidation() error {
	if driver.exitCode == 0 || !strings.Contains(driver.errorOutput, "run requires -- followed by a command") {
		return fmt.Errorf("exit=%d error=%q", driver.exitCode, driver.errorOutput)
	}
	return nil
}

func (driver *Driver) requestOperandFreeCommandsWithArguments() {
	commands := [][]string{
		{versionCommandName, unexpectedArgument},
		{statusCommandName, unexpectedArgument},
		{monitorCommandName, unexpectedArgument},
		{releaseCommandName, "check", unexpectedArgument},
		{releaseCommandName, "assess", unexpectedArgument},
		{releaseCommandName, monitorCommandName, unexpectedArgument},
	}

	driver.operandCommandsRejected = true
	for _, arguments := range commands {
		driver.exitCode = 0
		driver.output = ""
		driver.errorOutput = ""
		err := driver.runCLI(arguments...)

		diagnostic := driver.errorOutput
		if err != nil {
			diagnostic += err.Error()
		}
		if driver.exitCode != 1 ||
			!strings.Contains(diagnostic, "unknown command") && !strings.Contains(diagnostic, "accepts 0 arg") {
			driver.operandCommandsRejected = false
			driver.errorOutput = fmt.Sprintf("arguments=%v exit=%d diagnostic=%q", arguments, driver.exitCode, diagnostic)

			return
		}
	}
}

func (driver *Driver) requireOperandFreeCommandsRejected() error {
	if !driver.operandCommandsRejected {
		return errors.New(driver.errorOutput)
	}

	return nil
}

func healthySummary() policy.ReleaseSummary {
	return policy.ReleaseSummary{
		SchemaVersion:                          5,
		SampleCount:                            3,
		AvailableParallelism:                   12,
		AvailableNonCompressedEstimateMinBytes: 13 * policy.GiB,
		MemoryPressureLevelMax:                 1,
		CompressorAvailableAll:                 true,
		CompressorPayloadPeakBytes:             7 * policy.GiB,
		CPUUtilizationP95Percent:               50,
		DiskFreeMinBytes:                       30 * policy.GiB,
		SwapFreeMinBytes:                       2 * policy.GiB,
		HealthLatencyP95Ms:                     25,
		RoutedJourneyLatencyP95Ms:              50,
		RoutedJourneyLatencyMaxMs:              100,
	}
}

func (driver *Driver) summary(healthFailures int) error {
	directory, err := os.MkdirTemp("", "hippo-bdd-")
	if err != nil {
		return err
	}

	driver.temporaryPaths = append(driver.temporaryPaths, directory)
	driver.summaryPath = filepath.Join(directory, "summary.json")
	summary := healthySummary()
	summary.HealthFailures = healthFailures
	value, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("encode release summary: %w", err)
	}

	return os.WriteFile(driver.summaryPath, value, 0o600)
}

func (driver *Driver) assessSummary() error {
	if driver.mode == contract.E2E {
		driver.runBinary("release", "assess", "--summary", driver.summaryPath)
		driver.accepted = driver.exitCode == 0

		return nil
	}

	_, err := releaseguard.AssessFile(driver.summaryPath)
	driver.accepted = err == nil

	return nil
}

func (driver *Driver) requireAccepted() error {
	if !driver.accepted {
		return errors.New("release evidence was rejected")
	}
	return nil
}

func (driver *Driver) healthySummaryInput() error {
	value, err := json.Marshal(healthySummary())
	if err != nil {
		return err
	}

	driver.childInput = string(value)

	return nil
}

func (driver *Driver) assessSummaryInput() error {
	arguments := []string{releaseCommandName, releaseAssessName, summaryFlag, "-"}
	if driver.mode == contract.E2E {
		driver.runBinaryWithInput(driver.childInput, arguments...)
	} else {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		code, err := (cli.Application{
			Stdin:  strings.NewReader(driver.childInput),
			Stdout: stdout,
			Stderr: stderr,
		}).Run(context.Background(), arguments)
		driver.exitCode = code
		driver.output = stdout.String()
		driver.errorOutput = stderr.String()
		if err != nil {
			return err
		}
	}

	driver.accepted = driver.exitCode == 0

	return nil
}

func (driver *Driver) requireAcceptedOutput() error {
	if !driver.accepted || driver.errorOutput != "" {
		return fmt.Errorf("exit=%d stdout=%q stderr=%q", driver.exitCode, driver.output, driver.errorOutput)
	}

	var payload struct {
		Accepted      bool `json:"accepted"`
		SchemaVersion int  `json:"schemaVersion"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return err
	}
	if !payload.Accepted || payload.SchemaVersion != 5 {
		return fmt.Errorf("unexpected assessment output: %+v", payload)
	}

	return nil
}

func (driver *Driver) repeatedChangingStates() {
	base := time.Unix(0, 0)
	healthy := healthySample(base)
	repeated := healthySample(base.Add(time.Second))
	warning := healthySample(base.Add(2 * time.Second))
	level := 2
	warning.MemoryPressureLevel = &level
	driver.samples = []policy.Sample{healthy, repeated, warning}
}

func (driver *Driver) monitorJSON() error {
	ctx, cancel := context.WithCancel(context.Background())
	collector := &sequenceCollector{samples: driver.samples, cancel: cancel, cancelAfter: len(driver.samples)}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	code, err := (cli.Application{
		Stdout:    stdout,
		Stderr:    stderr,
		Collector: collector,
	}).Run(ctx, []string{monitorCommandName, jsonFlag, "--interval", "1ms", diskPathFlag, "."})
	driver.exitCode = code
	driver.output = stdout.String()
	driver.errorOutput = stderr.String()

	return err
}

func (driver *Driver) requireJSONTransitions() error {
	lines := strings.Split(strings.TrimSpace(driver.output), "\n")
	if driver.exitCode != 0 || len(lines) != 2 {
		return fmt.Errorf("exit=%d lines=%d stdout=%q stderr=%q", driver.exitCode, len(lines), driver.output, driver.errorOutput)
	}

	states := []policy.State{}
	for _, line := range lines {
		var payload struct {
			SchemaVersion int          `json:"schemaVersion"`
			MeasuredAt    string       `json:"measuredAt"`
			State         policy.State `json:"state"`
			Profile       string       `json:"profile"`
		}
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			return err
		}
		if payload.SchemaVersion != 1 || payload.MeasuredAt == "" || payload.Profile == "" {
			return fmt.Errorf("incomplete transition: %+v", payload)
		}
		states = append(states, payload.State)
	}
	if states[0] != policy.StateNormal || states[1] != policy.StateWarning {
		return fmt.Errorf("unexpected states: %v", states)
	}

	return nil
}

func (driver *Driver) releasePathsWithoutEndpoints() error {
	directory, err := os.MkdirTemp("", "hippo-release-inputs-")
	if err != nil {
		return err
	}

	driver.temporaryPaths = append(driver.temporaryPaths, directory)
	driver.leaseRoot = directory

	return nil
}

func runReleaseMonitorCLI(arguments []string, standardOutput io.Writer) (int, string, error) {
	standardError := &bytes.Buffer{}
	application := cli.Application{
		Stdout:      standardOutput,
		Stderr:      standardError,
		Environment: []string{},
		Collector:   &sequenceCollector{samples: []policy.Sample{healthySample(time.Unix(0, 0))}},
		MonitorRelease: func(ctx context.Context, config releaseguard.MonitorConfig) error {
			monitorContext, cancel := context.WithCancel(ctx)
			config.Collector = &sequenceCollector{
				samples:     []policy.Sample{healthySample(time.Unix(0, 0))},
				cancel:      cancel,
				cancelAfter: 1,
			}
			config.Interval = time.Millisecond
			config.ServiceRSS = func(context.Context) int64 { return 4096 }
			config.Health = func(context.Context) (int, float64) { return 200, 2.5 }
			config.RoutedHealth = func(context.Context) (int, float64) { return 200, 75 }
			config.LoadAverage = func(context.Context) float64 { return 1.5 }

			return releaseguard.RunMonitor(monitorContext, config)
		},
	}

	code, err := application.Run(context.Background(), arguments)

	return code, standardError.String(), err
}

func (driver *Driver) boundedReleaseRawOutput() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.releaseSummaryPath = filepath.Join(root, "summary.json")
	stdout := &bytes.Buffer{}
	driver.releaseMonitorRun = func() error {
		arguments := []string{
			releaseCommandName, monitorCommandName,
			outputFlag, "-",
			summaryFlag, driver.releaseSummaryPath,
			deploymentRootFlag, root,
			healthURLFlag, testHealthURL,
			routedOriginFlag, testRoutedOrigin,
		}
		code, standardError, runError := runReleaseMonitorCLI(arguments, stdout)
		driver.exitCode = code
		driver.output = stdout.String()
		driver.errorOutput = standardError

		return runError
	}

	return nil
}

func (driver *Driver) boundedReleaseSummaryOutput() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.releaseRawPath = filepath.Join(root, "samples.jsonl")
	stdout := &bytes.Buffer{}
	driver.releaseMonitorRun = func() error {
		arguments := []string{
			releaseCommandName, monitorCommandName,
			outputFlag, driver.releaseRawPath,
			summaryFlag, "-",
			deploymentRootFlag, root,
			healthURLFlag, testHealthURL,
			routedOriginFlag, testRoutedOrigin,
		}
		code, standardError, runError := runReleaseMonitorCLI(arguments, stdout)
		driver.exitCode = code
		driver.output = stdout.String()
		driver.errorOutput = standardError

		return runError
	}

	return nil
}

func (driver *Driver) completeReleaseMonitor() {
	if driver.releaseMonitorRun == nil {
		driver.releaseMonitorError = errors.New("release monitor was not prepared")

		return
	}

	driver.releaseMonitorError = driver.releaseMonitorRun()
}

func decodeJSONLines(value string) ([]map[string]any, error) {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	result := make([]map[string]any, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		payload := map[string]any{}
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			return nil, err
		}
		result = append(result, payload)
	}

	return result, nil
}

func (driver *Driver) requireRawStandardOutput() error {
	if driver.releaseMonitorError != nil {
		return driver.releaseMonitorError
	}

	records, err := decodeJSONLines(driver.output)
	if err != nil {
		return fmt.Errorf("decode raw output: %w", err)
	}
	if len(records) != 1 {
		return fmt.Errorf("raw output records=%d output=%q", len(records), driver.output)
	}
	if _, err := releaseguard.AssessFile(driver.releaseSummaryPath); err != nil {
		return err
	}

	return nil
}

func (driver *Driver) requireSummaryStandardOutput() error {
	if driver.releaseMonitorError != nil {
		return driver.releaseMonitorError
	}

	var summary policy.ReleaseSummary
	if err := json.Unmarshal([]byte(driver.output), &summary); err != nil {
		return err
	}
	if summary.SchemaVersion != 5 || summary.SampleCount != 1 {
		return fmt.Errorf("unexpected summary: %+v", summary)
	}

	data, err := os.ReadFile(driver.releaseRawPath)
	if err != nil {
		return err
	}
	records, err := decodeJSONLines(string(data))
	if err != nil {
		return fmt.Errorf("decode raw file: %w", err)
	}
	if len(records) != 1 {
		return fmt.Errorf("raw file records=%d", len(records))
	}

	return nil
}

func (driver *Driver) mixedReleaseStandardOutput() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	driver.releaseCollector = &sequenceCollector{samples: []policy.Sample{healthySample(time.Unix(0, 0))}}
	driver.releaseArguments = []string{
		releaseCommandName, monitorCommandName,
		outputFlag, "-",
		summaryFlag, "-",
		deploymentRootFlag, root,
		healthURLFlag, testHealthURL,
		routedOriginFlag, testRoutedOrigin,
	}

	return nil
}

func (driver *Driver) requireMixedOutputRejected() error {
	if driver.exitCode != 1 || !strings.Contains(driver.errorOutput, "cannot both use standard output") {
		return fmt.Errorf("exit=%d error=%q", driver.exitCode, driver.errorOutput)
	}
	if driver.releaseCollector != nil && driver.releaseCollector.index != 0 {
		return fmt.Errorf("collected %d samples before rejecting output", driver.releaseCollector.index)
	}

	return nil
}

func (driver *Driver) failingReleaseRawOutput() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}

	stream := &failingStream{closeCalls: &driver.streamCloseCalls}
	driver.releaseMonitorRun = func() error {
		arguments := []string{
			releaseCommandName, monitorCommandName,
			outputFlag, "-",
			summaryFlag, filepath.Join(root, "summary.json"),
			deploymentRootFlag, root,
			healthURLFlag, testHealthURL,
			routedOriginFlag, testRoutedOrigin,
		}
		code, standardError, runError := runReleaseMonitorCLI(arguments, stream)
		driver.exitCode = code
		driver.errorOutput = standardError

		return runError
	}

	return nil
}

func (driver *Driver) requireStreamFailure() error {
	if driver.releaseMonitorError == nil || !strings.Contains(driver.releaseMonitorError.Error(), "downstream write failure") {
		return fmt.Errorf("unexpected monitor error: %w", driver.releaseMonitorError)
	}
	if driver.streamCloseCalls != 0 {
		return fmt.Errorf("caller-owned stream closed %d times", driver.streamCloseCalls)
	}

	return nil
}

func (driver *Driver) requestReleaseMonitoring() {
	arguments := driver.releaseArguments
	if len(arguments) == 0 {
		arguments = []string{
			"release", "monitor",
			outputFlag, filepath.Join(driver.leaseRoot, "samples.jsonl"),
			summaryFlag, filepath.Join(driver.leaseRoot, "summary.json"),
			deploymentRootFlag, driver.leaseRoot,
			"--duration-ms", "1",
		}
	}
	if driver.mode == contract.E2E {
		driver.runBinary(arguments...)

		return
	}

	base := time.Unix(0, 0)
	collector := driver.releaseCollector
	if collector == nil {
		collector = &sequenceCollector{samples: []policy.Sample{healthySample(base)}}
	}
	code, err := (cli.Application{
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		Environment: []string{},
		Collector:   collector,
	}).Run(context.Background(), arguments)

	driver.exitCode = code
	if err != nil {
		driver.errorOutput = err.Error()
	}
}

func (driver *Driver) requireMissingHealthURL() error {
	if driver.exitCode == 0 || !strings.Contains(driver.errorOutput, "health URL") {
		return fmt.Errorf("exit=%d error=%q", driver.exitCode, driver.errorOutput)
	}
	return nil
}

func (driver *Driver) releaseHost() {
	driver.samples = []policy.Sample{capacitySample(5*policy.GiB, 700*policy.MiB)}
}

func (driver *Driver) assessRelease() {
	driver.resolution, _ = policy.BuiltinCatalog().Resolve(profileBalanced, "release", driver.samples[len(driver.samples)-1])
	driver.exitCode = driver.resolution.ExitCode
}

func (driver *Driver) requireReleaseCPU() error {
	if driver.exitCode != policy.ReplanRequiredExitCode || driver.resolution.Decision != "replan" {
		return fmt.Errorf("got %+v", driver.resolution)
	}
	return nil
}

func (driver *Driver) failedSummary() error {
	return driver.summary(1)
}

func (driver *Driver) slowRoutedSummary() error {
	directory, err := os.MkdirTemp("", "hippo-bdd-")
	if err != nil {
		return err
	}

	driver.temporaryPaths = append(driver.temporaryPaths, directory)
	driver.summaryPath = filepath.Join(directory, "summary.json")
	summary := healthySummary()
	summary.SchemaVersion = 4
	summary.RoutedJourneyLatencyP95Ms = 501
	summary.RoutedJourneyLatencyMaxMs = 2_001

	value, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("encode release summary: %w", err)
	}

	return os.WriteFile(driver.summaryPath, value, 0o600)
}

func (driver *Driver) requireRejected() error {
	if driver.accepted {
		return errors.New("release evidence was accepted")
	}
	return nil
}

func (driver *Driver) inspectBuildCaching() error {
	command := exec.Command("git", "check-ignore", "--quiet", "dist/hippo_test_linux_amd64.tar.gz")
	command.Dir = toolRoot()

	driver.lifecycleOK = command.Run() == nil

	return nil
}

func (driver *Driver) requireBuildCacheDisabled() error {
	if !driver.lifecycleOK {
		return errors.New("generated release binaries are not ignored")
	}
	return nil
}

func (driver *Driver) runTemporaryHarness(testExit int) error {
	temporaryParent, err := os.MkdirTemp("", "hippo-e2e-parent-")
	if err != nil {
		return err
	}

	fakeDirectory, err := os.MkdirTemp("", "hippo-fake-go-")
	if err != nil {
		_ = os.RemoveAll(temporaryParent)
		return err
	}

	driver.temporaryPaths = append(driver.temporaryPaths, temporaryParent, fakeDirectory)
	fakeGo := filepath.Join(fakeDirectory, "go")

	program := `#!/bin/sh
set -eu
case "$1" in
  build)
    shift
    output=
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-o" ]; then
        output=$2
        break
      fi
      shift
    done
    [ -n "$output" ]
    printf '#!/bin/sh\nexit 0\n' > "$output"
    chmod 700 "$output"
    ;;
  test)
    [ -x "${HIPPO_BIN:-}" ]
    exit "${FAKE_GO_TEST_EXIT:-0}"
    ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(fakeGo, []byte(program), 0o700); err != nil {
		return err
	}

	command := exec.Command(filepath.Join(toolRoot(), "tests", e2eMode, "run.sh"))
	command.Env = environmentWith(map[string]string{
		"FAKE_GO_TEST_EXIT":     strconv.Itoa(testExit),
		"HIPPO_E2E_TEMP_PARENT": temporaryParent,
		"HIPPO_GO_BINARY":       fakeGo,
	})

	output, runError := command.CombinedOutput()
	if testExit == 0 && runError != nil {
		return fmt.Errorf("temporary E2E harness failed: %w: %s", runError, output)
	}
	if testExit != 0 {
		var exitError *exec.ExitError
		if !errors.As(runError, &exitError) || exitError.ExitCode() != testExit {
			return fmt.Errorf("temporary E2E harness returned %w, want exit %d: %s", runError, testExit, output)
		}
	}

	entries, err := os.ReadDir(temporaryParent)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("temporary E2E parent retained %d artifact(s)", len(entries))
	}

	return nil
}

func (driver *Driver) inspectE2ELifecycle() error {
	if err := driver.runTemporaryHarness(0); err != nil {
		return err
	}

	if err := driver.runTemporaryHarness(23); err != nil {
		return err
	}

	driver.lifecycleOK = true

	return nil
}

func (driver *Driver) requireE2ECleanup() error {
	if !driver.lifecycleOK {
		return errors.New("temporary E2E binary was not removed after every outcome")
	}
	return nil
}

func (driver *Driver) historicalGenerations() error {
	cacheRoot, err := os.MkdirTemp("", "hippo-cache-")
	if err != nil {
		return err
	}

	driver.cacheRoot = cacheRoot
	driver.temporaryPaths = append(driver.temporaryPaths, cacheRoot)

	platform := filepath.Join(cacheRoot, runtime.GOOS+"-"+runtime.GOARCH)
	if err := os.MkdirAll(platform, 0o700); err != nil {
		return err
	}

	for index, character := range []string{"1", "2", "3", "4"} {
		name := strings.Repeat(character, 64)
		directory := filepath.Join(platform, name)
		if err := os.Mkdir(directory, 0o700); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(directory, "hippo"), []byte("historical"), 0o700); err != nil {
			return err
		}

		modified := time.Unix(int64(index+1), 0)
		if err := os.Chtimes(directory, modified, modified); err != nil {
			return err
		}
		driver.historicalCaches = append(driver.historicalCaches, name)
	}

	retentionLock := filepath.Join(platform, ".retention.lock")
	if err := os.Mkdir(retentionLock, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(retentionLock, pidField), []byte("999999999\n"), 0o600); err != nil {
		return err
	}

	return nil
}

func (driver *Driver) runCurrentBootstrap() error {
	if driver.cacheRoot == "" {
		return errors.New("bootstrap cache root was not prepared")
	}

	summaryPath := filepath.Join(driver.cacheRoot, "healthy-summary.json")
	value, err := json.Marshal(healthySummary())
	if err != nil {
		return err
	}
	if err := os.WriteFile(summaryPath, value, 0o600); err != nil {
		return err
	}

	command := exec.Command(filepath.Join(toolRoot(), "hippo"), "release", "assess", "--summary", summaryPath)
	command.Env = environmentWith(map[string]string{"HIPPO_BUILD_CACHE": driver.cacheRoot})

	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("current bootstrap failed: %w: %s", err, output)
	}

	platform := filepath.Join(driver.cacheRoot, runtime.GOOS+"-"+runtime.GOARCH)
	entries, err := os.ReadDir(platform)
	if err != nil {
		return err
	}

	remaining := map[string]bool{}
	generation := regexp.MustCompile(`^[0-9a-f]{64}$`)

	for _, entry := range entries {
		if entry.IsDir() && generation.MatchString(entry.Name()) {
			remaining[entry.Name()] = true
		}
	}
	if len(remaining) != 3 {
		return fmt.Errorf("retained %d bootstrap generations, want 3", len(remaining))
	}
	if remaining[driver.historicalCaches[0]] || remaining[driver.historicalCaches[1]] {
		return errors.New("oldest bootstrap generations were retained")
	}
	if !remaining[driver.historicalCaches[2]] || !remaining[driver.historicalCaches[3]] {
		return errors.New("two most recent historical generations were removed")
	}

	driver.lifecycleOK = true

	return nil
}

func (driver *Driver) requireRetention() error {
	if !driver.lifecycleOK {
		return errors.New("bootstrap retention did not keep current plus two recent generations")
	}
	return nil
}

func (driver *Driver) inspectLintEnforcement() error {
	configuration, err := os.ReadFile(filepath.Join(toolRoot(), ".golangci.yml"))
	if err != nil {
		return err
	}
	driver.lintConfiguration = string(configuration)

	quickData, err := os.ReadFile(filepath.Join(toolRoot(), "scripts", "test-quick.sh"))
	if err != nil {
		return err
	}

	driver.lintCommand = string(quickData)

	return nil
}

func (driver *Driver) requireExhaustiveLint() error {
	required := []string{"default: all", "max-issues-per-linter: 0", "max-same-issues: 0"}
	for _, value := range required {
		if !strings.Contains(driver.lintConfiguration, value) {
			return fmt.Errorf("lint configuration does not enforce %q", value)
		}
	}

	return nil
}

func (driver *Driver) requirePackageDocumentation() error {
	if strings.Contains(driver.lintConfiguration, "- stylecheck") {
		return errors.New("stylecheck is disabled")
	}

	if !strings.Contains(driver.lintConfiguration, "name: exported") || !strings.Contains(driver.lintConfiguration, "severity: error") {
		return errors.New("exported documentation is not an error")
	}
	return nil
}

func (driver *Driver) requireModuleScopedLint() error {
	if !strings.Contains(driver.lintCommand, "go tool golangci-lint run") || strings.Contains(driver.lintCommand, "../") {
		return errors.New("lint is not scoped to the standalone module")
	}
	return nil
}

func (driver *Driver) inspectBehaviourCoverage() error {
	driver.strictAdapters = true

	for _, adapter := range contract.Adapters() {
		driver.strictAdapters = driver.strictAdapters && contract.SuiteOptions(adapter).Strict
	}

	unitAdapter, err := contract.AdapterByName(contract.Unit)
	if err != nil {
		return err
	}

	driver.unitExemptionsForbidden = unitAdapter.ExemptionTag == "" && len(contract.ApprovedExemptions[contract.Unit]) == 0
	driver.approvedExemptions = true

	for _, exemptions := range contract.ApprovedExemptions {
		for _, exemption := range exemptions {
			driver.approvedExemptions = driver.approvedExemptions &&
				strings.TrimSpace(exemption.Boundary) != "" &&
				strings.TrimSpace(exemption.Reason) != ""
		}
	}

	fullData, err := os.ReadFile(filepath.Join(toolRoot(), "scripts", "test.sh"))
	if err != nil {
		return err
	}

	quickData, err := os.ReadFile(filepath.Join(toolRoot(), "scripts", "test-quick.sh"))
	if err != nil {
		return err
	}

	fullText, quickText := string(fullData), string(quickData)
	effectiveText := quickText + fullText

	unit := strings.Index(effectiveText, "HIPPO_BDD_ADAPTER=unit")
	integration := strings.Index(effectiveText, "HIPPO_BDD_ADAPTER=integration")
	e2e := strings.Index(effectiveText, "HIPPO_BDD_ADAPTER=e2e")
	driver.serialCompliance = unit >= 0 && unit < integration && integration < e2e
	driver.e2ePlacement = strings.Contains(fullText, "./tests/e2e/run.sh") && !strings.Contains(quickText, "./tests/e2e/run.sh")

	return nil
}

func (driver *Driver) requireStrictAdapters() error {
	if !driver.strictAdapters {
		return errors.New("one or more behavior adapters do not use strict step resolution")
	}
	return nil
}

func (driver *Driver) requirePairedBindings() error {
	return errors.Join(contract.ValidateHandlers(driver.Bindings())...)
}

func (driver *Driver) requireNoUnitExemptions() error {
	if !driver.unitExemptionsForbidden {
		return errors.New("unit behavior adapter permits exemptions")
	}

	return nil
}

func (driver *Driver) requireApprovedExemptions() error {
	if !driver.approvedExemptions {
		return errors.New("behavior exemptions do not name a concrete boundary and reason")
	}
	return nil
}

func (driver *Driver) requireSerialCompliance() error {
	if !driver.serialCompliance {
		return errors.New("behavior compliance does not run every adapter serially")
	}
	return nil
}

func (driver *Driver) requireE2EPlacement() error {
	if !driver.e2ePlacement {
		return errors.New("full end-to-end behavior is not isolated from quick checks")
	}
	return nil
}

func (driver *Driver) inspectContributorEnforcement() error {
	root := toolRoot()

	read := func(parts ...string) (string, error) {
		data, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))

		return string(data), err
	}

	commitHook, err := read(".husky", "commit-msg")
	if err != nil {
		return err
	}

	preCommitHook, err := read(".husky", "pre-commit")
	if err != nil {
		return err
	}

	prePushHook, err := read(".husky", "pre-push")
	if err != nil {
		return err
	}

	stagedConfig, err := read("lint-staged.config.mjs")
	if err != nil {
		return err
	}

	workflow, err := read(".github", "workflows", "ci.yml")
	if err != nil {
		return err
	}

	manifest, err := read("package.json")
	if err != nil {
		return err
	}

	quick, err := read("scripts", "test-quick.sh")
	if err != nil {
		return err
	}

	driver.conventionalCommits = strings.Contains(commitHook, "commitlint --edit") &&
		strings.Contains(workflow, "commitlint --from") &&
		strings.Contains(workflow, "Validate pull request commits") &&
		strings.Contains(workflow, "Validate pushed commits")
	driver.stagedFormatting = strings.Contains(preCommitHook, "lint-staged") &&
		strings.Contains(stagedConfig, `"**/*.go"`) &&
		strings.Contains(stagedConfig, "goimports -w") &&
		strings.Contains(stagedConfig, "gofumpt -w") &&
		strings.Contains(stagedConfig, `"**/*.sh"`) &&
		strings.Contains(stagedConfig, "shfmt -w") &&
		strings.Contains(stagedConfig, `"**/*.{json,md,yaml,yml}"`) &&
		strings.Contains(stagedConfig, "prettier --write")
	driver.pushQuickGate = strings.Contains(prePushHook, "npm run test:quick") &&
		strings.Contains(manifest, `"test:quick": "./scripts/test-quick.sh"`) &&
		!strings.Contains(prePushHook, "npm exec -- nx") &&
		!strings.Contains(prePushHook, "npx nx") &&
		!strings.Contains(manifest, `"nx":`)
	driver.coreCoverage = strings.Contains(quick, "--minimum 99") &&
		strings.Contains(quick, "internal/policy,./internal/config,./internal/host") &&
		strings.Contains(quick, "internal/host/collector.go,internal/host/linux_parsers.go")

	return nil
}

func (driver *Driver) requireConventionalCommits() error {
	if !driver.conventionalCommits {
		return errors.New("the commit hook and CI do not invoke conventional commit validation")
	}
	return nil
}

func (driver *Driver) requireStagedFormatting() error {
	if !driver.stagedFormatting {
		return errors.New("pre-commit does not invoke formatting for every supported staged file type")
	}
	return nil
}

func (driver *Driver) requirePushQuickGate() error {
	if !driver.pushQuickGate {
		return errors.New("pre-push does not invoke the direct no-Nx quick gate")
	}
	return nil
}

func (driver *Driver) requireCoreCoverage() error {
	if !driver.coreCoverage {
		return errors.New("the quick gate does not invoke deterministic core coverage at 99 percent")
	}
	return nil
}

func capacitySample(memory, available int64) policy.Sample {
	sample := healthySample(time.Unix(0, 0))
	sample.Platform = "linux"
	sample.Capabilities = []string{"cgroup-v2", "memory-psi"}
	sample.PhysicalMemoryBytes = memory
	sample.EffectiveMemoryLimitBytes = memory
	sample.AvailableMemoryBytes = &available
	sample.AvailableNonCompressedEstimateBytes = nil
	sample.CompressorAvailable = nil
	sample.CompressorPayloadBytes = nil
	sample.SwapState = "unavailable"

	return sample
}

func (driver *Driver) smallRunner() {
	driver.samples = []policy.Sample{capacitySample(5*policy.GiB, 800*policy.MiB)}
}

func (driver *Driver) requireConstrained() error {
	if driver.resolution.ResolvedProfile != "constrained" || driver.exitCode != 0 {
		return fmt.Errorf("got profile %q exit %d", driver.resolution.ResolvedProfile, driver.exitCode)
	}
	return nil
}

func (driver *Driver) tinyMachine() {
	driver.samples = []policy.Sample{capacitySample(policy.GiB, 200*policy.MiB)}
}

func (driver *Driver) requireMinimal() error {
	if driver.resolution.ResolvedProfile != "minimal" || driver.resolution.Concurrency != 1 || driver.exitCode != 0 {
		return fmt.Errorf("got %+v exit %d", driver.resolution, driver.exitCode)
	}
	return nil
}

func (driver *Driver) exhaustedDisk() {
	sample := capacitySample(policy.GiB, 512*policy.MiB)
	sample.DiskFreeBytes = new(200 * policy.MiB)
	sample.DiskTotalBytes = new(policy.GiB)
	driver.samples = []policy.Sample{sample}
}

func (driver *Driver) strictTransaction() {
	driver.taskClass = "transactional"
	driver.samples = []policy.Sample{capacitySample(5*policy.GiB, 700*policy.MiB)}
}

func (driver *Driver) requireReplan() error {
	if driver.exitCode != policy.ReplanRequiredExitCode || driver.resolution.Decision != "replan" {
		return fmt.Errorf("got %+v exit %d", driver.resolution, driver.exitCode)
	}
	return nil
}

func (driver *Driver) linuxCgroupCapacity() {
	driver.linuxMemInfo = "MemTotal: 16777216 kB\nMemAvailable: 8388608 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n"
	driver.linuxCgroupLimit = 4 * policy.GiB
}

func (driver *Driver) collectLinuxEvidence() error {
	memory, err := host.ParseMemInfo(driver.linuxMemInfo)
	if err != nil {
		return err
	}

	driver.effectiveMemory = host.EffectiveMemoryLimit(memory.Total, driver.linuxCgroupLimit, 0)

	return nil
}

func (driver *Driver) requireFourGiB() error {
	if driver.effectiveMemory != 4*policy.GiB {
		return fmt.Errorf("effective memory is %d", driver.effectiveMemory)
	}
	return nil
}

func (driver *Driver) linuxWithoutSwap() {
	driver.samples = []policy.Sample{capacitySample(4*policy.GiB, 3*policy.GiB)}
}

func (driver *Driver) requireSwapUnavailable() error {
	if driver.assessment.State == "critical" || driver.assessment.SwapState != "unavailable" {
		return fmt.Errorf("got %+v", driver.assessment)
	}
	return nil
}

func (driver *Driver) linuxPSIWarning() {
	sample := capacitySample(4*policy.GiB, 3*policy.GiB)
	sample.MemoryPSISomeAvg10 = new(10.0)
	driver.samples = []policy.Sample{sample}
}

func (driver *Driver) requirePSIWarning() error {
	if driver.assessment.State != "warning" || driver.assessment.Reason != "memory-psi" {
		return fmt.Errorf("got %+v", driver.assessment)
	}
	return nil
}

func (driver *Driver) invalidExplicitConfig() {
	directory, err := os.MkdirTemp("", "hippo-config-")
	if err != nil {
		driver.errorOutput = err.Error()

		return
	}

	driver.temporaryPaths = append(driver.temporaryPaths, directory)
	driver.configPath = filepath.Join(directory, "invalid.json")

	if err := os.WriteFile(driver.configPath, []byte(`{"schemaVersion":1,"unknown":true}`), 0o600); err != nil {
		driver.errorOutput = err.Error()
	}
}

func (driver *Driver) statusWithConfig() {
	if driver.mode == contract.E2E {
		driver.runBinary(statusCommandName, jsonFlag, configFlag, driver.configPath)

		return
	}

	base := time.Unix(0, 0)
	collector := &sequenceCollector{samples: []policy.Sample{healthySample(base), healthySample(base.Add(time.Second))}}
	code, err := (cli.Application{
		Stdout:    &bytes.Buffer{},
		Stderr:    &bytes.Buffer{},
		Collector: collector,
		Sleep:     func(time.Duration) {},
	}).Run(context.Background(), []string{statusCommandName, jsonFlag, configFlag, driver.configPath})

	driver.exitCode = code
	if err != nil {
		driver.errorOutput = err.Error()
	}
}

func (driver *Driver) requireConfigExit() error {
	if driver.exitCode != policy.ReplanRequiredExitCode || !strings.Contains(driver.errorOutput, "unknown") {
		return fmt.Errorf("exit=%d error=%q", driver.exitCode, driver.errorOutput)
	}
	return nil
}

func repositoryRoot() string {
	return toolRoot()
}

func (driver *Driver) inspectArtifactPolicy() {
	root := repositoryRoot()
	ignoredPaths := []string{
		"hippo.local.json",
		".env",
		".env.local",
		".cache/example",
		"dist/example",
		"coverage/example",
		"local-tmp/example",
		"generated-reports/example",
		"node_modules/example",
		"example.test",
	}

	driver.privateArtifacts = true
	for _, path := range ignoredPaths {
		ignore := exec.Command("git", "check-ignore", "--quiet", path)
		ignore.Dir = root
		if ignore.Run() != nil {
			driver.privateArtifacts = false
		}
	}

	tracked := exec.Command(
		"git",
		"ls-files",
		".env",
		".env.*",
		".cache/**",
		"dist/**",
		"coverage/**",
		"local-tmp/**",
		"generated-reports/**",
		"node_modules/**",
		"*.test",
	)
	tracked.Dir = root
	trackedOutput, trackedError := tracked.Output()
	driver.privateArtifacts = driver.privateArtifacts && trackedError == nil && len(bytes.TrimSpace(trackedOutput)) == 0

	examplePath := "hippo.local.json.example"
	example := exec.Command("git", "ls-files", "--error-unmatch", examplePath)
	example.Dir = root
	driver.exampleTracked = example.Run() == nil

	moduleData, moduleError := os.ReadFile(filepath.Join(root, "go.mod"))
	packageData, packageError := os.ReadFile(filepath.Join(root, "package.json"))
	manifest := map[string]any{}
	manifestError := json.Unmarshal(packageData, &manifest)
	private, privateOK := manifest["private"].(bool)
	_, hasWorkspaces := manifest["workspaces"]
	hasNxDependency := false

	for _, section := range []string{"dependencies", "devDependencies", "peerDependencies", "optionalDependencies"} {
		dependencies, ok := manifest[section].(map[string]any)
		if !ok {
			continue
		}

		_, hasNxDependency = dependencies["nx"]
		if hasNxDependency {
			break
		}
	}

	driver.applicationLayout = moduleError == nil &&
		bytes.Contains(moduleData, []byte("module github.com/wahidyankf/hippo")) &&
		packageError == nil && manifestError == nil && privateOK && private &&
		!hasWorkspaces && !hasNxDependency &&
		!pathExists(filepath.Join(root, "project.json")) &&
		!pathExists(filepath.Join(root, "nx.json")) &&
		pathExists(filepath.Join(root, "specs", "behaviours"))
}

func pathExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func (driver *Driver) requirePrivateArtifacts() error {
	if !driver.privateArtifacts {
		return errors.New("local config, generated artifacts, or generated reports are not ignored and untracked")
	}
	return nil
}

func (driver *Driver) requireExampleTracked() error {
	if !driver.exampleTracked {
		return errors.New("local config example is not tracked")
	}
	return nil
}

func (driver *Driver) requireApplicationLayout() error {
	if !driver.applicationLayout {
		return errors.New("HIPPO is not a standalone Go module with co-owned specifications")
	}
	return nil
}
