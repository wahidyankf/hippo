package integration_test

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestInteractiveChildOwnsControllingTerminal(t *testing.T) {
	if os.Getenv("HIPPO_PTY_HELPER") == "1" {
		return
	}

	scriptPath, err := exec.LookPath("script")
	if err != nil {
		t.Skip("script utility is unavailable")
	}

	runHIPPOPTYCase(t, scriptPath, "success")
}

func TestInteractiveChildRestoresTerminalOnEveryExitPath(t *testing.T) {
	if os.Getenv("HIPPO_PTY_HELPER") == "1" {
		return
	}
	scriptPath, err := exec.LookPath("script")
	if err != nil {
		t.Skip("script utility is unavailable")
	}
	for _, mode := range []string{"failure", "cancel", "forced-stop"} {
		t.Run(mode, func(t *testing.T) { runHIPPOPTYCase(t, scriptPath, mode) })
	}
}

func runHIPPOPTYCase(t *testing.T, scriptPath, mode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	temporaryRoot := t.TempDir()
	resultPath := temporaryRoot + "/terminal-result"
	readyPath := temporaryRoot + "/terminal-ready"
	args := ptyScriptArguments(t, os.Args[0], "-test.run=^TestHIPPOPTYHelper$")
	command := exec.CommandContext(ctx, scriptPath, args...)
	command.Env = append(os.Environ(),
		"HIPPO_PTY_HELPER=1",
		"HIPPO_PTY_ROOT="+temporaryRoot,
		"HIPPO_PTY_RESULT="+resultPath,
		"HIPPO_PTY_READY="+readyPath,
		"HIPPO_PTY_MODE="+mode,
	)
	inputReader, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = inputReader.Close() }()
	command.Stdin = inputReader
	go func() {
		// Wait for the child to announce it is about to read rather than guessing
		// a fixed delay: helper startup and PTY allocation are not time-bounded
		// under load, and writing early loses the input before any read begins.
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, statError := os.Stat(readyPath); statError == nil {
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
		_, _ = inputWriter.WriteString("terminal-input\n")
	}()
	// Closing the write end early can hand `script` an EOF that tears the PTY
	// session down before the child consumes the line, so it stays open until
	// the guarded command has exited.
	defer func() { _ = inputWriter.Close() }()

	output, runErr := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("interactive child did not complete before the PTY deadline; it may have stopped on SIGTTIN: %s", output)
	}
	if runErr != nil {
		t.Fatalf("PTY helper failed: %v\n%s", runErr, output)
	}
	result, err := os.ReadFile(resultPath)
	if err != nil || string(result) != "terminal-input" {
		t.Fatalf("interactive child did not read from its controlling terminal: result=%q error=%v\n%s", result, err, output)
	}
}

func TestHIPPOPTYHelper(t *testing.T) {
	if os.Getenv("HIPPO_PTY_HELPER") != "1" {
		return
	}

	evidenceRoot := os.Getenv("HIPPO_PTY_ROOT")
	if evidenceRoot == "" {
		t.Fatal("HIPPO_PTY_ROOT is required")
	}
	resultPath := os.Getenv("HIPPO_PTY_RESULT")
	if resultPath == "" {
		t.Fatal("HIPPO_PTY_RESULT is required")
	}
	readyPath := os.Getenv("HIPPO_PTY_READY")
	if readyPath == "" {
		t.Fatal("HIPPO_PTY_READY is required")
	}
	mode := os.Getenv("HIPPO_PTY_MODE")
	readLoop := "printf ready > \"$HIPPO_PTY_READY\"; value=; while [ -z \"$value\" ]; do IFS= read -r value || :; done"
	childScript := readLoop + "; printf '%s' \"$value\" > \"$HIPPO_PTY_RESULT\""
	expectedExit := 0
	switch mode {
	case "", "success":
	case "failure":
		childScript += "; exit 7"
		expectedExit = 7
	case "cancel":
		childScript += "; sleep 5"
		expectedExit = 143
	case "forced-stop":
		// The trap must be installed before the result write: cancellation is
		// triggered by that file appearing, so a later trap would leave a window
		// where TERM still kills the child and never proves forced escalation.
		childScript = readLoop + "; trap '' TERM; printf '%s' \"$value\" > \"$HIPPO_PTY_RESULT\"; sleep 5"
		expectedExit = 137
	default:
		t.Fatalf("unknown PTY helper mode %q", mode)
	}

	base := time.Now().UTC()
	collector := &integrationCollector{samples: []policy.Sample{
		integrationSample(base),
		integrationSample(base.Add(time.Millisecond)),
		integrationSample(base.Add(2 * time.Millisecond)),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if mode == "cancel" || mode == "forced-stop" {
		go func() {
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					result, readError := os.ReadFile(resultPath) //nolint:gosec // The probe reads only the fixture-owned temporary result path.
					if readError == nil && len(result) > 0 {
						cancel()

						return
					}
				}
			}
		}()
	}
	ptyPolicy := fastPolicy()
	ptyPolicy.TerminationGrace = 200 * time.Millisecond

	runStarted := time.Now()
	exitCode, runErr := guard.Run(ctx, guard.RunConfig{
		Command:      "/bin/sh",
		Arguments:    []string{"-c", childScript},
		ChildStdin:   os.Stdin,
		ChildStdout:  os.Stdout,
		ChildStderr:  os.Stderr,
		Collector:    collector,
		EvidenceRoot: evidenceRoot,
		Policy:       ptyPolicy,
		Resolution: policy.Resolution{
			RequestedProfile: "balanced",
			ResolvedProfile:  "balanced",
			Concurrency:      4,
		},
		TaskClass: policy.TaskEphemeral,
		Now:       func() time.Time { return base },
	})
	if runErr != nil {
		t.Fatalf("guarded PTY command returned an error after %s (context=%v): %v", time.Since(runStarted), ctx.Err(), runErr)
	}
	if exitCode != expectedExit {
		t.Fatalf("guarded PTY command exited with %d, expected %d", exitCode, expectedExit)
	}
	foregroundGroup, err := guard.ForegroundProcessGroup(os.Stdin)
	if err != nil || foregroundGroup != guard.CurrentProcessGroup() {
		t.Fatalf("original foreground process group was not restored: foreground=%d process=%d error=%v", foregroundGroup, guard.CurrentProcessGroup(), err)
	}
}

func ptyScriptArguments(t *testing.T, command string, arguments ...string) []string {
	t.Helper()

	switch runtime.GOOS {
	case "darwin", "freebsd", "openbsd", "netbsd":
		return append([]string{"-q", "/dev/null", command}, arguments...)
	case "linux":
		parts := append([]string{command}, arguments...)
		for index := range parts {
			parts[index] = shellQuote(parts[index])
		}
		return []string{"-q", "-e", "-c", strings.Join(parts, " "), "/dev/null"}
	default:
		t.Skipf("PTY integration test is unsupported on %s", runtime.GOOS)
		return nil
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
