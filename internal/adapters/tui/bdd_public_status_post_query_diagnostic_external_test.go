package tui_test

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
	"github.com/wahidyankf/hippo/tests/support/hostprobe"
)

// This explicitly selected diagnostic may terminate its owned status process.
// It measures an unobserved boundary, not an original acceptance scenario.
func TestTUIBDDPublicStatusPostQueryStackDiagnostic(t *testing.T) {
	selection := flag.Lookup("test.run")
	if selection == nil || selection.Value.String() != "^TestTUIBDDPublicStatusPostQueryStackDiagnostic$" {
		return
	}
	if os.Getenv("HIPPO_BDD_SUITE") != contract.E2E || runtime.GOOS != "darwin" || os.Getenv("GOMAXPROCS") != "2" {
		t.Fatal("selected status stack diagnostic requires explicit E2E/Darwin/GOMAXPROCS=2")
	}
	driver := support.NewDriver(contract.Adapter{Name: contract.E2E})
	driver.Reset()
	t.Cleanup(driver.Close)
	fixture := &publicBDDObservation{driver: driver}
	receipt := publicBDDPostQueryReceipt{ParentPID: os.Getpid(), ExitCode: -1}
	output, diagnostics := &publicBDDObservationCapture{}, &publicBDDObservationCapture{}
	var probe *support.PublicTUIHostProbe
	// Register teardown before the ledger or probe acquires its first resource.
	t.Cleanup(func() { cleanupPublicStatusDiagnostic(t, fixture, &probe, &receipt, output, diagnostics) })
	ctx, err := fixture.prepareKnownRows(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	receipt.Root, receipt.Directory, receipt.Configuration = fixture.root, fixture.directory, fixture.ledger.Configuration()
	receipt.Helper = fixture.helper
	receipt.ProbeConstructorBefore = time.Now()
	probe, err = driver.NewPublicTUIHostProbeWithHelper(ctx, fixture.directory, support.PublicTUIHostReply{CPUPercent: 20}, fixture.helper)
	receipt.ProbeConstructorAfter = time.Now()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	command, err := driver.NativeAnnotationCommand(ctx, "status", "--json", "--config", fixture.ledger.Configuration(), "--disk-path", fixture.directory)
	if err != nil {
		t.Fatal(err)
	}
	command.Dir, command.Env = fixture.directory, fixture.environment(probe)
	command.Stdin = strings.NewReader("")
	command.Stdout, command.Stderr = output, diagnostics
	command.WaitDelay = 3 * time.Second
	receipt.Arguments = slices.Clone(command.Args)
	receipt.EnvironmentScope = "known-safe explicit fixture inputs only; other entries omitted"
	for _, entry := range command.Env {
		name, _, found := strings.Cut(entry, "=")
		if found && slices.Contains([]string{"PATH", "HIPPO_ROOT", "TERM", "NO_COLOR", "HIPPO_TEST_LINUX_EVIDENCE_ROOT", "GOMAXPROCS"}, name) {
			receipt.Environment = append(receipt.Environment, entry)
		} else {
			receipt.OmittedEnvironmentEntries++
		}
	}
	receipt.ContextDeadline, receipt.ContextDeadlineAvailable = ctx.Deadline()
	if !receipt.ContextDeadlineAvailable {
		t.Fatal("original status context has no finite deadline")
	}
	receipt.StartBefore = time.Now()
	err = command.Start()
	receipt.StartAfter = time.Now()
	if err != nil {
		receipt.StartError = err.Error()
		t.Fatal(err)
	}
	receipt.PID = command.Process.Pid

	waited := make(chan publicBDDStatusCompletion, 1)
	go func() {
		err := command.Wait()
		waited <- publicBDDStatusCompletion{err: err, at: time.Now()}
	}()
	joined := false
	defer func() {
		if !joined {
			cancel()
			<-waited
		}
	}()
	finished := awaitPublicStatusDiagnostic(ctx, command, probe, &receipt, waited)
	joined = true
	// Read ProcessState and raw captures only after the one actual Wait joins.
	receipt.WaitReturned, receipt.WaitAfter = true, finished.at
	if finished.err != nil {
		receipt.WaitError = finished.err.Error()
	}
	if err := ctx.Err(); err != nil {
		receipt.ContextErrorAtWait = err.Error()
	}
	if command.ProcessState != nil {
		receipt.ProcessStateAvailable = true
		receipt.ProcessExited = command.ProcessState.Exited()
		receipt.ExitCode = command.ProcessState.ExitCode()
		receipt.ProcessState = command.ProcessState.String()
		if status, ok := command.ProcessState.Sys().(syscall.WaitStatus); ok {
			receipt.WaitStatusAvailable = true
			receipt.WaitSignaled, receipt.WaitSignal = status.Signaled(), status.Signal().String()
		}
	}
	cancel()
	requirePublicStatusDiagnosticResult(t, &receipt, output, diagnostics, finished)
}

// Validate a nonempty ordered prefix of at most the original two readings.
// At least one start/ACK pair is required; a later start may remain unacknowledged.
// Active describes a reading, not a query or a helper/status process lifetime.
func publicBDDPostQueryObservedPrefix(trace support.PublicTUIHostProbeTrace, parentPID int) bool {
	commands := hostprobe.Commands()
	eventsPerReading := 2 * len(commands)
	if parentPID < 1 || trace.Closed || trace.MaximumActive != 1 || len(trace.Events) < 2 || len(trace.Events) > 2*eventsPerReading {
		return false
	}
	active := 1
	if len(trace.Events)%eventsPerReading == 0 {
		active = 0
	}
	if trace.Readings != (len(trace.Events)-1)/eventsPerReading+1 || trace.Active != active {
		return false
	}
	seen := make(map[int]bool)
	for index, event := range trace.Events {
		query := commands[(index/2)%len(commands)]
		phase := []string{"start", "output-ack"}[index%2]
		if event.Sequence != index+1 || event.Reading != index/eventsPerReading+1 || event.Operation != query[0] || event.Phase != phase || !slices.Equal(event.Arguments, query[1:]) || event.ParentPID != parentPID || event.PID < 1 || event.ElapsedNanoseconds < 0 {
			return false
		}
		if index > 0 && event.ElapsedNanoseconds < trace.Events[index-1].ElapsedNanoseconds {
			return false
		}
		if index%2 == 0 {
			if seen[event.PID] {
				return false
			}
			seen[event.PID] = true
		} else if event.PID != trace.Events[index-1].PID {
			return false
		}
	}
	return true
}

// Validate the entire authentic two-reading event order before selecting the
// final helper's parent. This never treats ACK, Active0 or commit as child exit.
func publicBDDPostQueryFinalACK(trace support.PublicTUIHostProbeTrace, parentPID int) bool {
	commands := hostprobe.Commands()
	if parentPID < 1 || trace.Closed || trace.Readings != 2 || trace.Active != 0 || trace.MaximumActive != 1 || len(trace.Events) != 4*len(commands) {
		return false
	}
	seen := make(map[int]bool)
	for index, event := range trace.Events {
		query := commands[(index/2)%len(commands)]
		phase := []string{"start", "output-ack"}[index%2]
		if event.Sequence != index+1 || event.Reading != index/(2*len(commands))+1 || event.Operation != query[0] || event.Phase != phase || !slices.Equal(event.Arguments, query[1:]) || event.ParentPID != parentPID || event.PID < 1 || event.ElapsedNanoseconds < 0 {
			return false
		}
		if index > 0 && event.ElapsedNanoseconds < trace.Events[index-1].ElapsedNanoseconds {
			return false
		}
		if index%2 == 0 {
			if seen[event.PID] {
				return false
			}
			seen[event.PID] = true
		} else if event.PID != trace.Events[index-1].PID {
			return false
		}
	}
	return true
}

// Parent timestamps share an origin; protocol elapsed times use the distinct
// probe origin bracketed by its constructor timestamps. Neither ACK nor a
// successful diagnostic signal asserts that the final helper exited naturally.
type publicBDDPostQueryReceipt struct {
	ParentPID, PID                                                                                                int
	Root, Directory, Configuration                                                                                string
	Helper                                                                                                        support.PublicTUIHostHelper
	Arguments, Environment                                                                                        []string
	EnvironmentScope                                                                                              string
	OmittedEnvironmentEntries                                                                                     int
	ProbeConstructorBefore, ProbeConstructorAfter                                                                 time.Time
	ContextDeadline                                                                                               time.Time
	ContextDeadlineAvailable                                                                                      bool
	StartBefore, StartAfter, TimerTarget, TimerObserved, SignalAttemptedAt, WaitAfter                             time.Time
	StartError, SignalError, WaitError, ContextErrorAtWait, ProbeCloseError, FixtureCloseError                    string
	Decision, ProcessState, WaitSignal                                                                            string
	WaitPendingAtTimer, WaitPendingAtSignal, ObservedPrefixValid, FinalACKObserved, SignalDelivered, WaitReturned bool
	ProcessStateAvailable, ProcessExited, WaitStatusAvailable, WaitSignaled                                       bool
	ExitCode                                                                                                      int
	AtTimer, BeforeClose, AfterClose                                                                              support.PublicTUIHostProbeTrace
}

func cleanupPublicStatusDiagnostic(t *testing.T, fixture *publicBDDObservation, probe **support.PublicTUIHostProbe, receipt *publicBDDPostQueryReceipt, output, diagnostics *publicBDDObservationCapture) {
	t.Helper()
	cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	if *probe != nil {
		receipt.BeforeClose = (*probe).Trace()
		if err := (*probe).Close(cleanup); err != nil {
			receipt.ProbeCloseError = err.Error()
			t.Errorf("independent status probe cleanup failure: %v", err)
		}
		receipt.AfterClose = (*probe).Trace()
		if !receipt.AfterClose.Closed {
			t.Error("status diagnostic probe did not close and join its actors")
		}
	}
	if err := fixture.close(cleanup); err != nil {
		receipt.FixtureCloseError = err.Error()
		t.Errorf("independent status ledger cleanup failure: %v", err)
	}
	t.Logf("actual selected status post-query diagnostic receipt=%+v stdout=%q raw-status-stderr=%q", *receipt, output.content.Bytes(), diagnostics.content.Bytes())
}

type publicBDDStatusCompletion struct {
	err error
	at  time.Time
}

func awaitPublicStatusDiagnostic(ctx context.Context, command *exec.Cmd, probe *support.PublicTUIHostProbe, receipt *publicBDDPostQueryReceipt, waited <-chan publicBDDStatusCompletion) publicBDDStatusCompletion {
	// The original status deadline is unchanged. SIGQUIT is an intentional
	// diagnostic action 250ms before it, only after an authentic observed query ACK.
	receipt.TimerTarget = receipt.ContextDeadline.Add(-250 * time.Millisecond)
	timer := time.NewTimer(time.Until(receipt.TimerTarget))
	defer timer.Stop()
	var finished publicBDDStatusCompletion
	select {
	case finished = <-waited:
		receipt.Decision = "status Wait returned before diagnostic timer"
	case <-timer.C:
		receipt.TimerObserved = time.Now()
		select {
		case finished = <-waited:
			receipt.Decision = "status Wait already returned at diagnostic timer"
		default:
			receipt.WaitPendingAtTimer = true
			receipt.AtTimer = probe.Trace()
			receipt.FinalACKObserved = publicBDDPostQueryFinalACK(receipt.AtTimer, receipt.PID)
			receipt.ObservedPrefixValid = publicBDDPostQueryObservedPrefix(receipt.AtTimer, receipt.PID)
			finished = finishPublicStatusDiagnostic(ctx, command, receipt, waited)
		}
	}
	return finished
}

func finishPublicStatusDiagnostic(ctx context.Context, command *exec.Cmd, receipt *publicBDDPostQueryReceipt, waited <-chan publicBDDStatusCompletion) publicBDDStatusCompletion {
	var finished publicBDDStatusCompletion
	if !receipt.ObservedPrefixValid || ctx.Err() != nil {
		receipt.Decision = "authentic observed query ACK absent or original context already ended; no diagnostic signal sent"
		finished = <-waited
		return finished
	}
	select {
	case finished = <-waited:
		receipt.Decision = "status Wait returned after observed-prefix inspection; no signal sent"
	default:
		if ctx.Err() != nil {
			receipt.Decision = "original context ended after observed-prefix inspection; no diagnostic signal sent"
		} else {
			receipt.WaitPendingAtSignal = true
			receipt.Decision = "intentional SIGQUIT to owned status PID after observed query ACK; ACK does not prove child Wait"
			receipt.SignalAttemptedAt = time.Now()
			if err := command.Process.Signal(syscall.SIGQUIT); err != nil {
				receipt.SignalError = err.Error()
			} else {
				receipt.SignalDelivered = true
			}
		}
		finished = <-waited
	}
	return finished
}

func requirePublicStatusDiagnosticResult(t *testing.T, receipt *publicBDDPostQueryReceipt, output, diagnostics *publicBDDObservationCapture, finished publicBDDStatusCompletion) {
	t.Helper()
	if output.failure != nil || diagnostics.failure != nil {
		t.Fatalf("actual status capture failed: stdout=%v stderr=%v", output.failure, diagnostics.failure)
	}
	if receipt.SignalError != "" {
		t.Fatalf("owned diagnostic signal failed: %s; actual Wait=%v", receipt.SignalError, finished.err)
	}
	if receipt.SignalDelivered {
		if !strings.Contains(diagnostics.content.String(), "SIGQUIT") || !strings.Contains(diagnostics.content.String(), "goroutine") {
			t.Fatalf("intentional SIGQUIT produced no genuine bounded Go stack; actual Wait=%v", finished.err)
		}
		t.Log("intentional status diagnostic termination retained; this is no public observation/TUI acceptance")
		return
	}
	if finished.err != nil || !receipt.ProcessStateAvailable || !receipt.ProcessExited || receipt.ExitCode != 0 {
		t.Fatalf("status ended without diagnostic stack: actual Wait=%v; decision=%s", finished.err, receipt.Decision)
	}
	t.Log("actual status exited naturally before a qualifying diagnostic signal; no stack or acceptance inferred")
}
