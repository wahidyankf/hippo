package tui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/tests/support"
)

// Public observation assertions consume only actual status JSON and decoded
// current terminal bytes. This passive object supplies no application observer.
type publicBDDObservation struct {
	driver                          *support.Driver
	root, directory                 string
	ledger                          *support.PublicTUILedger
	probe                           *support.PublicTUIHostProbe
	helper                          support.PublicTUIHostHelper
	transport                       *support.PublicTUITransport
	screen                          *tui.BDDNativeTransportScreen
	cancel                          context.CancelFunc
	status                          application.StatusView
	statusPID                       int
	statusOutput, statusDiagnostics []byte
	completed                       []publicBDDObservationReceipt
	retained                        bool
	variant, annotationFault        string
	filters                         []string
	statusTrace                     support.PublicTUIHostProbeTrace
	idleBefore                      map[string]string
	frames                          []tui.BDDNativeTransportFrame
	mixed                           []*publicBDDMixedJob
	mixedSchemas                    map[string]int
	width, height                   int
	family                          *publicBDDInteractive
}

func (fixture *publicBDDObservation) handler(pattern string) any {
	switch pattern {
	case `^two running jobs and three registered FIFO waiters with known reservation vectors$`:
		return fixture.prepareKnownRows
	case `^one TUI observation completes$`:
		return fixture.observe
	case `^both running identities and all three original FIFO positions retain their resource values$`:
		return fixture.requireRows
	default:
		return fixture.neutralHandler(pattern)
	}
}

func (fixture *publicBDDObservation) prepareKnownRows(ctx context.Context) (context.Context, error) {
	return fixture.prepareCase(ctx, "known", "")
}

func (fixture *publicBDDObservation) prepareCase(ctx context.Context, variant, fault string) (context.Context, error) {
	if fixture.ledger != nil || fixture.cancel != nil {
		return ctx, errors.New("public observation fixture has unretired ownership")
	}
	fixture.variant, fixture.annotationFault = variant, fault
	fixture.helper = support.PublicTUIHostHelper{Executable: os.Getenv("HIPPO_PUBLIC_HOST_HELPER"), SHA256: os.Getenv("HIPPO_PUBLIC_HOST_HELPER_SHA256")}
	ctx, fixture.cancel = context.WithTimeout(ctx, 20*time.Second)
	var err error
	fixture.root, fixture.directory, err = fixture.driver.PreparePublicTUIRoot()
	if err != nil {
		return ctx, err
	}
	fixture.ledger, err = fixture.driver.NewPublicTUILedger(ctx, fixture.root, fixture.directory)
	if err != nil {
		return ctx, err
	}
	if err := fixture.ledger.PrepareObservation(ctx, variant, fault); err != nil {
		return ctx, err
	}
	if variant == "filtered" {
		fixture.filters = []string{"--source", "matching", "--tag", "role=matching"}
	}
	if variant == "idle" {
		fixture.idleBefore, err = fixture.ledger.CoordinationTree()
		return ctx, err
	}
	if variant == "mixed" {
		return ctx, fixture.prepareMixedOwners(ctx)
	}
	return ctx, nil
}

func (fixture *publicBDDObservation) environment(probe *support.PublicTUIHostProbe) []string {
	environment := []string{"HIPPO_ROOT=" + fixture.root, "TERM=xterm-ghostty", "NO_COLOR=1", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	// Retain the admitted caller's scheduler limit for the public child and
	// its real collector helpers; actual NumCPU and host readings remain real.
	if limit := os.Getenv("GOMAXPROCS"); limit != "" {
		environment = append(environment, "GOMAXPROCS="+limit)
	}
	return probe.Environment(environment)
}

// publicBDDStatusReceipt retains observed command and collector facts only.
// Zero/unavailable process fields do not assert that a child ever started.
// Probe.Trace is bounded by its existing 8192-event limit.
type publicBDDStatusReceipt struct {
	Root, Directory, Configuration                        string
	Stage                                                 string
	Arguments, Environment                                []string
	EnvironmentScope                                      string
	OmittedEnvironmentEntries                             int
	PID                                                   int
	ContextDeadline                                       time.Time
	ContextDeadlineAvailable                              bool
	StartedAt, WaitFinishedAt                             time.Time
	StartError, WaitError, ContextErrorAtWait, CloseError string
	WaitReturned, ProcessStateAvailable, ProcessExited    bool
	ExitCode                                              int
	BeforeClose, AfterClose                               support.PublicTUIHostProbeTrace
	Helper                                                support.PublicTUIHostHelper
}

// Status uses an independent collector stream. Its two real readings cannot
// inflate the TUI stream used to measure observation or refresh scheduling.
func (fixture *publicBDDObservation) observeStatus(parent context.Context) (result error) {
	receipt := publicBDDStatusReceipt{
		Root: fixture.root, Directory: fixture.directory, Configuration: fixture.ledger.Configuration(),
		Stage: "preparing-probe", ExitCode: -1, Helper: fixture.helper,
	}
	probe, err := fixture.driver.NewPublicTUIHostProbeWithHelper(parent, fixture.directory, support.PublicTUIHostReply{CPUPercent: 20}, fixture.helper)
	if err != nil {
		return err
	}
	defer func() {
		receipt.BeforeClose = probe.Trace()
		cleanup, stop := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
		defer stop()
		closeError := probe.Close(cleanup)
		receipt.AfterClose = probe.Trace()
		if closeError != nil {
			receipt.CloseError = closeError.Error()
		}
		// Preserve the first failure and each independent cleanup error. The
		// original Godog When transcript reports this receipt even if the
		// canonical selection subsequently stops before returning its driver.
		result = errors.Join(result, closeError)
		if result != nil {
			result = fmt.Errorf("%w; actual public status receipt=%+v", result, receipt)
		}
	}()
	ctx, stop := context.WithTimeout(parent, 3*time.Second)
	defer stop()
	receipt.ContextDeadline, receipt.ContextDeadlineAvailable = ctx.Deadline()
	receipt.Stage = "preparing-command"
	command, err := fixture.driver.NativeAnnotationCommand(ctx, "status", "--json", "--config", fixture.ledger.Configuration(), "--disk-path", fixture.directory)
	if err != nil {
		return err
	}
	command.Dir, command.Env = fixture.directory, fixture.environment(probe)
	command.Stdin = strings.NewReader("")
	output, diagnostics := &publicBDDObservationCapture{}, &publicBDDObservationCapture{}
	command.Stdout, command.Stderr = output, diagnostics
	command.WaitDelay = 3 * time.Second
	// This command receives only the explicitly constructed fixture environment.
	// Do not copy the invoking process's environment into diagnostic evidence.
	receipt.Arguments = append([]string(nil), command.Args...)
	receipt.EnvironmentScope = "known-safe explicit fixture inputs only; other entries omitted"
	recordPublicStatusEnvironment(&receipt, command.Env)
	receipt.Stage = "starting-command"
	receipt.StartedAt = time.Now()
	if err = command.Start(); err != nil {
		receipt.StartError = err.Error()
		return err
	}
	fixture.statusPID = command.Process.Pid
	receipt.PID = fixture.statusPID
	receipt.Stage = "waiting-command"
	err = command.Wait()
	receipt.WaitReturned = true
	receipt.WaitFinishedAt = time.Now()
	receipt.Stage = "wait-returned"
	if err != nil {
		receipt.WaitError = err.Error()
	}
	if contextError := ctx.Err(); contextError != nil {
		receipt.ContextErrorAtWait = contextError.Error()
	}
	if command.ProcessState != nil {
		receipt.ProcessStateAvailable = true
		receipt.ProcessExited = command.ProcessState.Exited()
		receipt.ExitCode = command.ProcessState.ExitCode()
	}
	fixture.statusOutput = bytes.Clone(output.content.Bytes())
	fixture.statusDiagnostics = bytes.Clone(diagnostics.content.Bytes())
	if err != nil || command.ProcessState == nil || !command.ProcessState.Exited() || command.ProcessState.ExitCode() != 0 || output.failure != nil || diagnostics.failure != nil || len(fixture.statusDiagnostics) != 0 {
		return errors.Join(fmt.Errorf("selected public status pid=%d failed: %w; stdout=%q stderr=%q", fixture.statusPID, publicBDDDiagnosticCause(err), fixture.statusOutput, fixture.statusDiagnostics), output.failure, diagnostics.failure)
	}
	receipt.Stage = "decoding-status"
	decoder := json.NewDecoder(bytes.NewReader(fixture.statusOutput))
	if err := decoder.Decode(&fixture.status); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("public status produced an extra JSON value")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	if fixture.status.SchemaVersion != 5 || fixture.status.AvailableParallelism < 1 || fixture.status.MeasuredAt == "" {
		return errors.New("selected public status lost actual host/schema evidence")
	}
	receipt.Stage = "checking-readings"
	trace := probe.Trace()
	if trace.Readings != 2 || trace.Active != 0 || trace.MaximumActive != 1 {
		return fmt.Errorf("public status did not settle exactly two actual readings: %+v", trace)
	}
	fixture.statusTrace = trace
	receipt.Stage = "status-complete"
	return nil
}

func recordPublicStatusEnvironment(receipt *publicBDDStatusReceipt, environment []string) {
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			receipt.OmittedEnvironmentEntries++
			continue
		}
		switch name {
		case "PATH", "HIPPO_ROOT", "TERM", "NO_COLOR", "HIPPO_TEST_LINUX_EVIDENCE_ROOT", "GOMAXPROCS":
			receipt.Environment = append(receipt.Environment, entry)
		default:
			// Omit both name and value; never assert a complete inherited
			// environment or expose credentials from a future caller.
			receipt.OmittedEnvironmentEntries++
		}
	}
}

type publicBDDObservationCapture struct {
	content bytes.Buffer
	failure error
}

func (capture *publicBDDObservationCapture) Write(data []byte) (int, error) {
	const maximum = 8 * 1024 * 1024
	if capture.failure != nil {
		return 0, capture.failure
	}
	if len(data) > maximum-capture.content.Len() {
		capture.failure = errors.New("public observation raw capture exceeds 8 MiB")
		return 0, capture.failure
	}
	return capture.content.Write(data)
}

func (fixture *publicBDDObservation) observe(ctx context.Context) error {
	if fixture.ledger == nil || fixture.cancel == nil {
		return errors.New("public ledger was not prepared")
	}
	if err := fixture.observeStatus(ctx); err != nil {
		return err
	}
	if err := fixture.startInteractive(ctx, 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false); err != nil {
		return err
	}
	ready, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	if err := fixture.transport.AwaitReady(ready, func() (bool, error) {
		frame := fixture.screen.Snapshot()
		if frame.OverflowWrites != 0 {
			return false, errors.New("public observation overflowed the actual terminal")
		}
		text := strings.Join(frame.Rows, "\n")
		return frame.Alternate && frame.Width == fixture.width && frame.Height == fixture.height && strings.Contains(text, "HIPPO | Running and Queue") && strings.Contains(text, fixture.panelHeader("Running", fixture.visibleRows(false), 0)) && strings.Contains(text, fixture.panelHeader("Queue", fixture.visibleRows(true), 0)) && strings.Contains(text, "LIVE") && !strings.Contains(text, "refreshing") && !strings.Contains(text, "ERROR"), nil
	}); err != nil {
		return err
	}
	trace := fixture.probe.Trace()
	if trace.Readings != 2 || trace.Active != 0 || trace.MaximumActive != 1 {
		return fmt.Errorf("public TUI completed frame lacked two settled actual readings: %+v", trace)
	}
	return nil
}

func (fixture *publicBDDObservation) await(ctx context.Context, present, absent []string) (tui.BDDNativeTransportFrame, error) {
	return fixture.awaitFrame(ctx, present, absent, false, 0)
}

// Resize cases assert the accepted current repaint, not cleared historic cells.
// A full clear alone is not a completed frame: visible predicates are required.
func (fixture *publicBDDObservation) awaitRepaint(ctx context.Context, present, absent []string, after uint64) (tui.BDDNativeTransportFrame, error) {
	return fixture.awaitFrame(ctx, present, absent, true, after)
}

func (fixture *publicBDDObservation) awaitFrame(ctx context.Context, present, absent []string, repaint bool, after uint64) (tui.BDDNativeTransportFrame, error) {
	bound, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		frame := fixture.screen.Snapshot()
		if err := fixture.transport.OutputError(); err != nil {
			return frame, err
		}
		if !repaint && frame.OverflowWrites != 0 {
			return frame, fmt.Errorf("public current-frame viewport overflow: writes=%s resizeOrder=%q native=%s", frame.OverflowEvidence, fixture.family.resizeEvidence, fixture.transport.ReadinessEvidence())
		}
		text := strings.Join(frame.Rows, "\n")
		matches := frame.Alternate && frame.Width == fixture.width && frame.Height == fixture.height
		if repaint {
			matches = matches && frame.FullClearSequence > after && frame.CurrentOverflowWrites == 0
		}
		for _, fragment := range present {
			matches = matches && strings.Contains(text, fragment)
		}
		for _, fragment := range absent {
			matches = matches && !strings.Contains(text, fragment)
		}
		if matches {
			fixture.frames = append(fixture.frames, frame)
			return frame, nil
		}
		if result := fixture.transport.Result(); result.Reaped || result.OutputEOF {
			return frame, errors.New("public child ended before current-frame evidence")
		}
		select {
		case <-bound.Done():
			return frame, fmt.Errorf("current public frame %q: %w; rows=%q currentOverflow=%d history=%s clearSequence=%d after=%d", present, bound.Err(), frame.Rows, frame.CurrentOverflowWrites, frame.OverflowEvidence, frame.FullClearSequence, after)
		case <-ticker.C:
		}
	}
}

func (fixture *publicBDDObservation) key(ctx context.Context, key string, present ...string) error {
	bound, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stop()
	if err := fixture.transport.SendKey(bound, []byte(key)); err != nil {
		return err
	}
	_, err := fixture.await(bound, present, nil)
	return err
}

func (fixture *publicBDDObservation) requireRows(ctx context.Context) error {
	if err := fixture.checkRows(ctx); err != nil {
		return err
	}
	return fixture.finishObservation(ctx)
}

func (fixture *publicBDDObservation) close(ctx context.Context) error {
	var result error
	var beforeCleanup support.PublicTUIResult
	if fixture.transport != nil {
		beforeCleanup = fixture.transport.Result()
		result = errors.Join(result, fixture.transport.Close(ctx))
	}
	if fixture.probe != nil {
		result = errors.Join(result, fixture.probe.Close(ctx))
	}
	// Retain the actual probe closure result, including on unready abort paths.
	// Taking the value before Close would leave a permanently stale Closed flag.
	if fixture.transport != nil && !fixture.retained {
		fixture.completed = append(fixture.completed, publicBDDObservationReceipt{StatusPID: fixture.statusPID, StatusOutput: bytes.Clone(fixture.statusOutput), StatusDiagnostics: bytes.Clone(fixture.statusDiagnostics), Expected: fixture.ledger.Expected(), BeforeCleanup: beforeCleanup, Candidate: fixture.transport.Result(), Collector: fixture.probe.Trace(), Helper: fixture.helper})
		fixture.retained = true
	}
	if fixture.ledger != nil {
		result = errors.Join(result, fixture.ledger.Close(ctx))
	}
	queuedJoined := fixture.ledger == nil || fixture.ledger.QueuedActorsJoined()
	result = errors.Join(result, fixture.closeMixedJobs(ctx, queuedJoined))
	if fixture.family != nil && fixture.family.guardedFinish != nil {
		result = errors.Join(result, fixture.family.guardedFinish())
	}
	if fixture.cancel != nil {
		fixture.cancel()
	}
	return result
}

type publicBDDObservationReceipt struct {
	StatusPID                       int
	StatusOutput, StatusDiagnostics []byte
	Expected                        coordination.ReservationTotals
	BeforeCleanup, Candidate        support.PublicTUIResult
	Collector                       support.PublicTUIHostProbeTrace
	Helper                          support.PublicTUIHostHelper
}

func (fixture *publicBDDObservation) finish(ctx context.Context) error {
	action, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	err := fixture.transport.SendKey(action, []byte("q"))
	stop()
	if err != nil {
		return err
	}
	drain, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	if _, err := fixture.transport.AwaitCandidateExit(drain); err != nil {
		return err
	}
	// Settle the actual collector before allowing the stable holder to release
	// its same-slave restoration witness. One original drain budget covers both.
	if err := fixture.probe.Close(drain); err != nil {
		return err
	}
	result, err := fixture.transport.WaitAndDrain(drain)
	if err != nil {
		return err
	}
	if !result.Joined() || !result.NaturalExit() || result.ExitCode != 0 || result.HolderExitCode != 0 || !result.HolderAliveAtPostChild || !result.PostChildAcknowledged || len(result.Stderr) != 0 || !result.TerminalAfterAvailable || result.TerminalBefore.Mode != result.TerminalAfter.Mode || result.TerminalBefore.Foreground != result.TerminalAfter.Foreground {
		return fmt.Errorf("original public observation did not naturally quit/restore/reap/drain: %+v", result)
	}
	return nil
}

// finalizeScenario runs before Godog cancels its context. An assertion failure
// must not turn a still-owned terminal into a stale transport in the next Reset.
// Natural q and independent cleanup share the original three-second allowance.
func (fixture *publicBDDObservation) finalizeScenario(ctx context.Context) error {
	cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	var result error
	if fixture.transport != nil && !fixture.transport.Result().Reaped && fixture.transport.Result().TerminalReadyAvailable {
		result = fixture.finish(cleanup)
	}
	result = errors.Join(result, fixture.close(cleanup))
	if result != nil {
		return fmt.Errorf("public scenario finalization: %w", result)
	}
	*fixture = publicBDDObservation{driver: fixture.driver, completed: fixture.completed}
	return nil
}

func (fixture *publicBDDObservation) Reset() {
	cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	if err := fixture.close(cleanup); err != nil {
		panic(fmt.Errorf("public observation reset: %w", err))
	}
	*fixture = publicBDDObservation{driver: fixture.driver, completed: fixture.completed}
}

func (fixture *publicBDDObservation) Close() {
	cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	if err := fixture.close(cleanup); err != nil {
		panic(fmt.Errorf("public observation cleanup: %w", err))
	}
}

// Failure receipts also describe abnormal process state with a nil Wait error.
// Preserve the previous <nil> diagnostic while wrapping every actual cause.
func publicBDDDiagnosticCause(err error) error {
	if err == nil {
		return errors.New("<nil>")
	}
	return err
}
