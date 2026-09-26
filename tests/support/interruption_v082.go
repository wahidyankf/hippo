package support

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
	releaseguard "github.com/wahidyankf/hippo/internal/release"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

// interruptionScenario is the state the signal and refusal scenarios share:
// which observer is under test, the isolated root, the reservation holders
// that keep a run queued, and whether a release capture finished before
// exiting.
type interruptionScenario struct {
	command         string
	root            string
	workingDir      string
	childMarker     string
	holders         []*guard.Session
	captureFinished bool
	// diskPath is the measured path when it differs from root, and locked
	// holds the directories a refusal scenario made read-only.
	diskPath string
	locked   []string
}

const (
	interruptedRunSource   = "interrupted-run"
	signalInterrupt        = "SIGINT"
	signalTerminate        = "SIGTERM"
	hippoDiagnosticPrefix  = "hippo:"
	neverStartedState      = "never-started"
	admissionCancelled     = "admission-cancelled"
	queuedNotice           = "HIPPO queued run="
	interruptionTimeLimit  = time.Minute
	interruptionConfigName = "hippo.machine.json"
	// isolatedPath is the whole PATH a scenario that runs HIPPO in isolation
	// hands it, so no caller tool shadows a system one.
	isolatedPath = "PATH=/usr/bin:/bin:/usr/sbin:/sbin"
)

// interruptionConfig is a schema-3 reservation configuration whose base owner
// limit is two, so two live owners are enough to keep a third run queued.
const interruptionConfig = `{
  "schemaVersion": 3,
  "coordination": {
    "mode": "reservation",
    "maxCpu": 8,
    "maxMemoryMiB": 16384,
    "baseActiveOwners": 2,
    "maxActiveOwners": 3,
    "promotion": {"completedRuns": 25, "minimumSources": 3, "minimumAvailableMemoryMiB": 10240, "maximumCpuP95Percent": 75},
    "emergencyAvailableMemoryMiB": 6144,
    "tiers": {
      "light": {"minimumCpu": 1, "maximumCpu": 2, "minimumMemoryMiB": 1024, "maximumMemoryMiB": 2048, "queueDeadline": "30m"},
      "standard": {"minimumCpu": 2, "maximumCpu": 4, "minimumMemoryMiB": 3072, "maximumMemoryMiB": 6144, "queueDeadline": "90m"},
      "heavy": {"minimumCpu": 4, "maximumCpu": 8, "minimumMemoryMiB": 8192, "maximumMemoryMiB": 16384, "queueDeadline": "4h"}
    }
  }
}
`

func (driver *Driver) interruptionBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^watch has printed its first status snapshot$`, driver.watchStartedV082),
		step(`^watch receives (SIGINT|SIGTERM) while it waits for the next snapshot$`, driver.signalWatchV082),
		step(`^(watch|monitor) exits (130|143) with no hippo diagnostic$`, driver.requireObserverSignalStatusV082),
		step(`^stable host state for (watch|monitor) sampling$`, driver.observerSamplingV082),
		step(`^(watch|monitor) is interrupted by SIGINT while it collects a host sample$`, driver.interruptObserverSampleV082),
		step(`^a release capture that runs until it is stopped$`, driver.releaseCaptureV082),
		step(`^the release capture is ended by (SIGTERM|its duration)$`, driver.endReleaseCaptureV082),
		step(`^release monitoring finishes its capture and exits (143|0)$`, driver.requireReleaseCaptureStatusV082),
		step(`^a reservation root whose live owners fill its capacity and owner limit$`, driver.filledReservationRootV082),
		step(`^a queued guarded run receives SIGINT before it is admitted$`, driver.signalQueuedRunV082),
		step(`^a granted run still sampling the host before admission$`, driver.samplingRunV082),
		step(`^the guarded run receives SIGTERM before its child starts$`, driver.signalSamplingRunV082),
		step(
			`^the run exits (130|143) with no hippo diagnostic, its child never starts, and its receipt records never-started admission-cancelled$`,
			driver.requireInterruptedRunV082,
		),
		step(`^its lifetime summary records the outcome admission-cancelled$`, driver.requireCancelledSummaryV082),
		step(`^no lifetime summary is written, because a queued waiter collects no host evidence$`, driver.requireNoSummaryV082),
	}
}

// interruptedSummaries reads the lifetime summaries the interrupted run's
// root holds, by source.
func (driver *Driver) interruptedSummaries() (map[string]string, error) {
	rows, err := evidence.ReadHistory(driver.interruption.root, evidence.Query{})
	if err != nil {
		return nil, err
	}
	outcomes := map[string]string{}
	for _, row := range rows {
		outcomes[row.Source] = row.Outcome
	}

	return outcomes, nil
}

func (driver *Driver) requireCancelledSummaryV082() error {
	outcomes, err := driver.interruptedSummaries()
	if err != nil {
		return err
	}
	if len(outcomes) != 1 || outcomes[interruptedRunSource] != admissionCancelled {
		return fmt.Errorf("the cancelled run summarized as %v, want %s=%s", outcomes, interruptedRunSource, admissionCancelled)
	}

	return nil
}

func (driver *Driver) requireNoSummaryV082() error {
	outcomes, err := driver.interruptedSummaries()
	if err != nil {
		return err
	}
	if len(outcomes) != 0 {
		return fmt.Errorf("a queued waiter that collected no evidence wrote summaries: %v", outcomes)
	}

	return nil
}

func signalNamed(name string) syscall.Signal {
	if name == signalTerminate {
		return syscall.SIGTERM
	}

	return syscall.SIGINT
}

// interruptionEnvironment is the whole environment a compiled HIPPO sees, so
// no ambient configuration or state root reaches the scenario.
func (driver *Driver) interruptionEnvironment() []string {
	return []string{
		"HIPPO_ROOT=" + driver.interruption.root, "HOME=" + driver.interruption.root,
		isolatedPath, "CHILD_MARKER=" + driver.interruption.childMarker,
	}
}

func (driver *Driver) prepareInterruption(command string) error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.interruption.command, driver.interruption.root = command, root
	driver.interruption.childMarker = filepath.Join(root, "child-started")
	driver.samples = []policy.Sample{healthySample(time.Now())}

	return nil
}

func (driver *Driver) watchStartedV082() error {
	if err := driver.prepareInterruption(watchCommandName); err != nil {
		return err
	}
	if driver.mode == contract.E2E {
		return driver.compiledBinary()
	}

	return nil
}

// signalWatchV082 stops watch while it waits between snapshots. Through the
// compiled binary the signal is a real one, sent once the first snapshot has
// reached stdout; in process it is the cancellation the entry point makes of
// one, delivered at the interval pause.
func (driver *Driver) signalWatchV082(name string) error {
	if driver.mode == contract.E2E {
		return driver.signalCompiled(
			exec.Command(driver.binary, watchCommandName, intervalFlagName, "1h"), signalNamed(name), readsFirstStdoutLine,
		)
	}
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	sleeps := 0
	driver.runInterruptible(ctx, []string{watchCommandName, intervalFlagName, "1s"}, &sequenceCollector{samples: driver.samples},
		func(time.Duration) {
			// The first pause is inside the status snapshot; the second is
			// the interval between snapshots.
			sleeps++
			if sleeps == 2 {
				interrupt(status.Interruption{Signal: signalNamed(name)})
			}
		})

	return nil
}

func (driver *Driver) runInterruptible(ctx context.Context, arguments []string, collector policy.Collector, sleep func(time.Duration)) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := (cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: driver.interruptionEnvironment(),
		Collector: collector, Sleep: sleep,
	}).Run(ctx, arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()
}

// readsFirstStdoutLine and readsQueuedNotice say when a compiled command has
// reached the state a scenario wants to interrupt.
const (
	readsFirstStdoutLine = iota
	readsQueuedNotice
)

// signalCompiled starts a compiled HIPPO, waits until it proves it is running
// -- its first stdout line, or its queued notice on stderr -- then sends the
// signal and records the status a shell would see.
func (driver *Driver) signalCompiled(command *exec.Cmd, signal syscall.Signal, ready int) error {
	command.Env = driver.interruptionEnvironment()
	command.Dir = driver.interruption.root
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	watched, pipeError := command.StdoutPipe()
	if ready == readsQueuedNotice {
		command.Stdout = stdout
		watched, pipeError = command.StderrPipe()
	} else {
		command.Stderr = stderr
	}
	if pipeError != nil {
		return pipeError
	}
	if err := command.Start(); err != nil {
		return err
	}
	// The reader owns the pipe until it reaches end of file, and only then may
	// Wait close it; waiting first would drop whatever the command wrote last.
	seen, drained := make(chan bool, 1), make(chan struct{})
	var captured bytes.Buffer
	go func() {
		defer close(drained)
		reader := bufio.NewReader(watched)
		found := false
		for {
			line, readError := reader.ReadString('\n')
			captured.WriteString(line)
			if !found && (ready == readsFirstStdoutLine && line != "" || strings.Contains(line, queuedNotice)) {
				found = true
				seen <- true
			}
			if readError != nil {
				if !found {
					seen <- false
				}

				return
			}
		}
	}()
	select {
	case ok := <-seen:
		if !ok {
			<-drained
			_ = command.Wait()

			return fmt.Errorf("the compiled command ended before it was ready: stdout=%q stderr=%q captured=%q",
				stdout.String(), stderr.String(), captured.String())
		}
	case <-time.After(interruptionTimeLimit):
		_ = command.Process.Kill()
		<-drained
		_ = command.Wait()

		return fmt.Errorf("the compiled command was not ready within %s: %q", interruptionTimeLimit, captured.String())
	}
	if err := command.Process.Signal(signal); err != nil {
		return err
	}
	<-drained
	waitError := command.Wait()
	driver.exitCode = 0
	if exitError := (&exec.ExitError{}); errors.As(waitError, &exitError) {
		driver.exitCode = exitError.ExitCode()
		if waitStatus, ok := exitError.Sys().(syscall.WaitStatus); ok && waitStatus.Signaled() {
			driver.exitCode = 128 + int(waitStatus.Signal())
		}
	} else if waitError != nil {
		return waitError
	}
	if ready == readsQueuedNotice {
		driver.output, driver.errorOutput = stdout.String(), captured.String()
	} else {
		driver.output, driver.errorOutput = captured.String(), stderr.String()
	}

	return nil
}

func (driver *Driver) requireObserverSignalStatusV082(command, want string) error {
	expected, err := strconv.Atoi(want)
	if err != nil {
		return err
	}
	if command != driver.interruption.command {
		return fmt.Errorf("the scenario ran %s, not %s", driver.interruption.command, command)
	}
	if driver.exitCode != expected {
		return fmt.Errorf("%s exited %d, want %d: %q", command, driver.exitCode, expected, driver.errorOutput)
	}
	if strings.Contains(driver.errorOutput, hippoDiagnosticPrefix) {
		return fmt.Errorf("an interrupted %s reported a hippo failure: %q", command, driver.errorOutput)
	}
	if strings.TrimSpace(driver.output) == "" {
		return fmt.Errorf("%s printed nothing before the signal", command)
	}

	return nil
}

func (driver *Driver) observerSamplingV082(command string) error {
	return driver.prepareInterruption(command)
}

// interruptingSampler reports a stable host until its interruptAt-th
// collection, which is cut short by the signal the way a killed host probe
// is: it cancels the invocation and fails with the probe's own error.
type interruptingSampler struct {
	sample      policy.Sample
	calls       int
	interruptAt int
	interrupt   context.CancelCauseFunc
}

func (sampler *interruptingSampler) Collect(_ context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	sampler.calls++
	if sampler.calls == sampler.interruptAt {
		sampler.interrupt(status.Interruption{Signal: syscall.SIGINT})

		return policy.Reading{}, errors.New("available memory estimate is unavailable")
	}

	return policy.Reading{CPUState: previous, Sample: sampler.sample}, nil
}

func (driver *Driver) interruptObserverSampleV082(command string) error {
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	// Each watch snapshot collects twice, so its third collection starts the
	// second snapshot; monitor collects once per transition check.
	interruptAt, arguments := 3, []string{watchCommandName, intervalFlagName, "1s"}
	if command == monitorCommandName {
		interruptAt, arguments = 2, []string{monitorCommandName, intervalFlagName, "1ms"}
	}
	driver.runInterruptible(ctx, arguments,
		&interruptingSampler{sample: driver.samples[0], interruptAt: interruptAt, interrupt: interrupt},
		func(time.Duration) {})

	return nil
}

func (driver *Driver) releaseCaptureV082() error {
	return driver.prepareInterruption("release monitor")
}

// endReleaseCaptureV082 stands in for the capture itself: it runs until its
// context ends, then finishes, which is what the real capture does when it
// writes its summary on the way out.
func (driver *Driver) endReleaseCaptureV082(end string) error {
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	duration := "0"
	if end != signalTerminate {
		duration = "1"
	}
	root := driver.interruption.root
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := (cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: driver.interruptionEnvironment(),
		Collector: &sequenceCollector{samples: driver.samples},
		MonitorRelease: func(monitorContext context.Context, _ releaseguard.MonitorConfig) error {
			if end == signalTerminate {
				interrupt(status.Interruption{Signal: syscall.SIGTERM})
			}
			<-monitorContext.Done()
			driver.interruption.captureFinished = true

			return nil
		},
	}).Run(ctx, []string{
		"release", monitorCommandName, "--output", filepath.Join(root, "raw.jsonl"),
		"--summary", filepath.Join(root, "summary.json"), "--deployment-root", root,
		"--health-url", "http://127.0.0.1:9/health", "--routed-origin", "https://example.test",
		"--duration-ms", duration,
	})
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

func (driver *Driver) requireReleaseCaptureStatusV082(want string) error {
	expected, err := strconv.Atoi(want)
	if err != nil {
		return err
	}
	if !driver.interruption.captureFinished {
		return errors.New("release monitoring exited before finishing its capture")
	}
	if driver.exitCode != expected || driver.errorOutput != "" {
		return fmt.Errorf("release monitoring exited %d with %q, want %d and no diagnostic",
			driver.exitCode, driver.errorOutput, expected)
	}

	return nil
}

// filledReservationRootV082 fills both the ledger vector and the base owner
// limit with two live owners from this process, so the run under test can
// only queue behind them whatever the host it plans against.
func (driver *Driver) filledReservationRootV082() error {
	if err := driver.prepareInterruption(runCommandName); err != nil {
		return err
	}
	root := driver.interruption.root
	driver.interruption.workingDir = filepath.Join(root, "checkout")
	if err := os.MkdirAll(driver.interruption.workingDir, 0o700); err != nil {
		return err
	}
	configPath := filepath.Join(driver.interruption.workingDir, interruptionConfigName)
	if err := os.WriteFile(configPath, []byte(interruptionConfig), 0o600); err != nil {
		return err
	}
	for _, source := range []string{"first-holder", "second-holder"} {
		holder, err := guard.AcquireReservationWithOptions(
			context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
			guard.ReservationPlan{
				Capacity:  guard.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB},
				Requested: guard.ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
				Allocated: guard.ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
			},
			3, 0, guard.ReservationAdmissionOptions{Metadata: guard.ReservationMetadata{Source: source, Tier: "light"}},
		)
		if err != nil || holder == nil {
			return fmt.Errorf("hold reservation capacity: session=%v error=%w", holder != nil, err)
		}
		driver.interruption.holders = append(driver.interruption.holders, holder)
	}
	if driver.mode == contract.E2E {
		return driver.compiledBinary()
	}

	return nil
}

func (driver *Driver) interruptedRunArguments() []string {
	arguments := []string{runCommandName, sourceFlagName, interruptedRunSource, diskPathFlag, driver.interruption.root}
	if driver.interruption.workingDir != "" {
		arguments = append(arguments,
			"--config", filepath.Join(driver.interruption.workingDir, interruptionConfigName),
			"--cwd", driver.interruption.workingDir, "--resource-tier", "light")
	}

	return append(arguments, "--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"`)
}

func (driver *Driver) signalQueuedRunV082() error {
	if driver.mode == contract.E2E {
		return driver.signalCompiled(
			exec.Command(driver.binary, driver.interruptedRunArguments()...), syscall.SIGINT, readsQueuedNotice,
		)
	}
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	// The first pause is the queued waiter's retry wait.
	driver.runInterruptible(ctx, driver.interruptedRunArguments(), &sequenceCollector{samples: driver.samples},
		func(time.Duration) { interrupt(status.Interruption{Signal: syscall.SIGINT}) })
	if !strings.Contains(driver.errorOutput, queuedNotice) {
		return fmt.Errorf("the run was never queued: %q", driver.errorOutput)
	}

	return nil
}

func (driver *Driver) samplingRunV082() error {
	return driver.prepareInterruption(runCommandName)
}

func (driver *Driver) signalSamplingRunV082() error {
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	// Admission needs consecutive host samples, so the first pause is the wait
	// between the first sample and the second.
	driver.runInterruptible(ctx, driver.interruptedRunArguments(), &sequenceCollector{samples: driver.samples},
		func(time.Duration) { interrupt(status.Interruption{Signal: syscall.SIGTERM}) })

	return nil
}

func (driver *Driver) requireInterruptedRunV082(want string) error {
	expected, err := strconv.Atoi(want)
	if err != nil {
		return err
	}
	if driver.exitCode != expected {
		return fmt.Errorf("the interrupted run exited %d, want %d: %q", driver.exitCode, expected, driver.errorOutput)
	}
	if strings.Contains(driver.errorOutput, hippoDiagnosticPrefix) {
		return fmt.Errorf("the interrupted run reported a hippo failure: %q", driver.errorOutput)
	}
	if _, statError := os.Stat(driver.interruption.childMarker); !errors.Is(statError, os.ErrNotExist) {
		return fmt.Errorf("the interrupted run started its child: %w", statError)
	}

	return requireInterruptionReceipt(driver.interruption.root)
}

func requireInterruptionReceipt(root string) error {
	entries, err := os.ReadDir(filepath.Join(root, "receipts"))
	if err != nil {
		return fmt.Errorf("no receipt was written: %w", err)
	}
	for _, entry := range entries {
		data, readError := os.ReadFile(filepath.Join(root, "receipts", entry.Name()))
		if readError != nil {
			return readError
		}
		var receipt guard.SafetyReceipt
		if json.Unmarshal(data, &receipt) != nil || receipt.Source != interruptedRunSource {
			continue
		}
		if receipt.State != neverStartedState || receipt.Reason != admissionCancelled {
			return fmt.Errorf("the receipt records %s/%s, want %s/%s",
				receipt.State, receipt.Reason, neverStartedState, admissionCancelled)
		}

		return nil
	}

	return fmt.Errorf("no receipt from %s among %d entries", interruptedRunSource, len(entries))
}
