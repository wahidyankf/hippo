package tui_test

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// This measures one external collector helper, not a public observation or TUI.
// It uses the pinned external helper and existing wrapper/IPC protocol only.
func TestTUIBDDPublicPSHelperStartupDiagnostic(t *testing.T) {
	selection := flag.Lookup("test.run")
	if selection == nil || selection.Value.String() != "^TestTUIBDDPublicPSHelperStartupDiagnostic$" {
		return
	}
	if os.Getenv("HIPPO_BDD_SUITE") != contract.E2E || runtime.GOOS != "darwin" {
		t.Fatal("PS helper startup diagnostic requires explicit E2E mode on Darwin")
	}
	if os.Getenv("GOMAXPROCS") != "2" {
		t.Fatal("PS helper diagnostic requires the admitted GOMAXPROCS=2 limit")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	driver := support.NewDriver(contract.Adapter{Name: contract.E2E})
	t.Cleanup(driver.Close)
	root, directory, err := driver.PreparePublicTUIRoot()
	if err != nil {
		t.Fatal(err)
	}
	receipt := publicBDDPSStartupReceipt{
		ParentPID: os.Getpid(), ParentExecutable: executable, Root: root, Directory: directory,
		ExitCode: -1, ConstructorBefore: time.Now(),
	}
	helper := support.PublicTUIHostHelper{Executable: os.Getenv("HIPPO_PUBLIC_HOST_HELPER"), SHA256: os.Getenv("HIPPO_PUBLIC_HOST_HELPER_SHA256")}
	probe, err := driver.NewPublicTUIHostProbeWithHelper(t.Context(), directory, support.PublicTUIHostReply{CPUPercent: 20}, helper)
	receipt.ConstructorAfter = time.Now()
	if err != nil {
		t.Fatal(err)
	}
	output, diagnostics := &publicBDDObservationCapture{}, &publicBDDObservationCapture{}
	// Install teardown and failure receipts before the next fallible acquisition.
	registerPublicPSStartupCleanup(t, probe, &receipt, output, diagnostics)
	environment := probe.Environment([]string{
		"HIPPO_ROOT=" + root, "TERM=xterm-ghostty", "NO_COLOR=1",
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "GOMAXPROCS=2", "GODEBUG=inittrace=1",
	})
	// Preserve inherited race settings; the existing wrapper's helper-only
	// atexit option stays exactly as implemented by the accepted fixture.
	inheritedRace := os.Getenv("GORACE")
	if inheritedRace != "" {
		environment = append(environment, "GORACE="+inheritedRace)
	}
	var wrapper string
	for _, entry := range environment {
		if path, ok := strings.CutPrefix(entry, "PATH="); ok {
			wrapper = filepath.Join(strings.Split(path, string(os.PathListSeparator))[0], "ps")
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, wrapper, "-A", "-o", "%cpu=") // #nosec G702 -- Probe owns the fixed 0700 ps wrapper with quoted authenticated helper; query argv, finite context and actual Wait are retained.
	command.Dir, command.Env = directory, environment
	command.Stdin = strings.NewReader("")
	command.Stdout, command.Stderr = output, diagnostics
	command.WaitDelay = 3 * time.Second
	// These are all explicitly constructed safe entries, not inherited os.Environ.
	receipt.Arguments, receipt.Environment = append([]string(nil), command.Args...), append([]string(nil), environment...)
	receipt.ContextDeadline, receipt.ContextDeadlineAvailable = ctx.Deadline()
	receipt.StartBefore = time.Now()
	err = command.Start()
	receipt.StartAfter = time.Now()
	if err != nil {
		receipt.StartError = err.Error()
		t.Fatal(err)
	}
	receipt.PID = command.Process.Pid
	err = command.Wait()
	receipt.WaitAfter = time.Now()
	receipt.WaitReturned = true
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
	if err != nil || !receipt.ProcessExited || receipt.ExitCode != 0 || output.failure != nil || diagnostics.failure != nil {
		t.Fatalf("actual PS helper did not exit0 within original3s command bound: wait=%v output=%v stderr=%v", err, output.failure, diagnostics.failure)
	}
	trace := probe.Trace()
	requirePublicPSQueryTrace(t, trace, receipt, inheritedRace)
	if output.content.String() != strconv.Itoa(20*runtime.NumCPU())+".0\n" {
		t.Fatal("actual single PS helper output changed the existing physical NumCPU evidence")
	}
	if !strings.Contains(diagnostics.content.String(), "init ") || !strings.Contains(diagnostics.content.String(), " ms clock, ") {
		t.Fatal("helper-only inittrace stderr has no actual package initialization records")
	}
}

func registerPublicPSStartupCleanup(t *testing.T, probe *support.PublicTUIHostProbe, receipt *publicBDDPSStartupReceipt, output, diagnostics *publicBDDObservationCapture) {
	t.Helper()
	t.Cleanup(func() {
		receipt.BeforeClose = probe.Trace()
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		closeError := probe.Close(cleanup)
		receipt.AfterClose = probe.Trace()
		if closeError != nil {
			receipt.CloseError = closeError.Error()
		}
		t.Logf("actual single PS helper startup receipt=%+v stdout=%q raw-inittrace-stderr=%q", *receipt, output.content.Bytes(), diagnostics.content.Bytes())
		if closeError != nil {
			t.Errorf("PS helper independent cleanup failure: %v", closeError)
		}
		if !receipt.AfterClose.Closed {
			t.Error("owned PS helper fixture did not join its actors and close")
		}
	})
}

func requirePublicPSQueryTrace(t *testing.T, trace support.PublicTUIHostProbeTrace, receipt publicBDDPSStartupReceipt, inheritedRace string) {
	t.Helper()
	// A single ps query deliberately does not finish the seven-command cohort.
	// Active1 here is truthful fixture state, never a leaked helper/Collect result.
	if trace.Readings != 1 || trace.Active != 1 || trace.MaximumActive != 1 || len(trace.Events) != 2 {
		t.Fatalf("single PS query lost its actual bounded protocol trace: %+v", trace)
	}
	expectedRace := "atexit_sleep_ms=0"
	if inheritedRace != "" {
		expectedRace = inheritedRace + " " + expectedRace
	}
	for index, event := range trace.Events {
		phase := []string{"start", "output-ack"}[index]
		if event.Sequence != index+1 || event.Reading != 1 || event.Operation != "ps" || event.Phase != phase || event.PID != receipt.PID || event.ParentPID != receipt.ParentPID || event.RaceOptions != expectedRace || strings.Join(event.Arguments, " ") != "-A -o %cpu=" || event.ElapsedNanoseconds < 0 {
			t.Fatalf("helper protocol identity/argv/order/race settings changed: %+v", event)
		}
		if index > 0 && event.ElapsedNanoseconds < trace.Events[index-1].ElapsedNanoseconds {
			t.Fatal("helper protocol time moved backwards")
		}
	}
}

// Protocol timestamps are relative to a probe origin between these constructor
// timestamps. Start and Wait are parent observations. ACK is not helper exit;
// Wait is its actual process completion witness. Inittrace has its own origin.
type publicBDDPSStartupReceipt struct {
	ParentPID, PID                            int
	ParentExecutable, Root, Directory         string
	Arguments, Environment                    []string
	ConstructorBefore, ConstructorAfter       time.Time
	ContextDeadline                           time.Time
	ContextDeadlineAvailable                  bool
	StartBefore, StartAfter, WaitAfter        time.Time
	StartError, WaitError, ContextErrorAtWait string
	WaitReturned, ProcessStateAvailable       bool
	ProcessExited                             bool
	ExitCode                                  int
	CloseError                                string
	BeforeClose, AfterClose                   support.PublicTUIHostProbeTrace
}
