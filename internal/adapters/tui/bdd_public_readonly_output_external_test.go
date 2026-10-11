package tui_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// Scoped CLI regressions; eventual restoration does not prove restore-before-
// report ordering or close the original six terminal-fault acceptance rows.
func runPublicReadOnlyOutput(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.E2E || os.Getenv("HIPPO_BDD_ADAPTER") != contract.E2E {
		t.Fatal("readonly public CLI regressions require explicit E2E isolation")
	}
	for _, machineReadable := range []bool{false, true} {
		name := "plain"
		if machineReadable {
			name = "global-json"
		}
		t.Run(name, func(t *testing.T) {
			runPublicReadOnlyOutputCase(t, machineReadable)
		})
	}
}

// Keep the original native privacy prefix refusal and reject non-text controls.
// Newlines remain available for the exact diagnostic/JSON line-count oracle.
func publicReadOnlyDiagnosticSafe(text string, privatePaths ...string) bool {
	if !utf8.ValidString(text) || strings.Contains(text, "/private/") || strings.Contains(text, "/dev/") {
		return false
	}
	for _, character := range text {
		if unicode.IsControl(character) && character != '\n' {
			return false
		}
	}
	for _, path := range privatePaths {
		if path != "" && strings.Contains(text, path) {
			return false
		}
	}
	return true
}

func TestPublicReadOnlyOutputDiagnosticPrivacy(t *testing.T) {
	const safe = "hippo.supervision.failed: supervision failed\n"
	if !publicReadOnlyDiagnosticSafe(safe, "/owned/state") {
		t.Fatal("ordinary safe diagnostic refused")
	}
	for _, bad := range []string{
		"hippo.supervision.failed: /private/other-secret\n",
		"hippo.supervision.failed: /dev/other\n",
		"hippo.supervision.failed: /owned/state\n",
		"hippo.supervision.failed: failed\x00\n",
		"hippo.supervision.failed: failed\x1b\n",
		"hippo.supervision.failed: failed\r\n",
		"hippo.supervision.failed: failed\u0085\n",
		"hippo.supervision.failed: failed\xff\n",
	} {
		if publicReadOnlyDiagnosticSafe(bad, "/owned/state") {
			t.Fatalf("privacy/control negative accepted: %q", bad)
		}
	}
}

func runPublicReadOnlyOutputCase(t *testing.T, machineReadable bool) {
	t.Helper()
	driver := support.NewDriver(contract.Adapter{Name: contract.E2E})
	driver.Reset()
	t.Cleanup(driver.Close)
	fixture := &publicBDDObservation{driver: driver}
	// Close in the test body while its existing parent context is still live.
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
		defer stop()
		if err := fixture.close(cleanup); err != nil {
			t.Errorf("owned readonly fixture cleanup: %v", err)
		}
	}()
	ctx, err := fixture.prepareCase(t.Context(), "idle", "")
	if err != nil {
		t.Fatal(err)
	}
	fixture.probe, err = driver.NewPublicTUIHostProbeWithHelper(ctx, fixture.directory, support.PublicTUIHostReply{CPUPercent: 20}, fixture.helper)
	if err != nil {
		t.Fatal(err)
	}
	fixture.screen = tui.NewBDDNativeTransportScreen(120, 32)
	arguments := []string{"tui", "--config", fixture.ledger.Configuration(), "--disk-path", fixture.directory}
	if machineReadable {
		arguments = append([]string{"--output", "json"}, arguments...)
	}
	fixture.transport, err = support.StartPublicTUIReadOnlyOutput(ctx, support.PublicTUIStart{Driver: driver, Mode: contract.E2E, Executable: os.Getenv("HIPPO_BIN"), Root: fixture.root, Directory: fixture.directory, Arguments: arguments, Environment: fixture.environment(fixture.probe), Width: 120, Height: 32, Feed: fixture.screen.Feed, ResizeScreen: fixture.screen.Resize, Finish: fixture.screen.Finish})
	if err != nil {
		t.Fatal(err)
	}
	cleanup, stop := context.WithTimeout(ctx, 3*time.Second)
	result, err := fixture.transport.WaitAndDrain(cleanup)
	stop()
	t.Logf("actual cold readonly CLI receipt: %+v", result)
	if err != nil {
		t.Fatal(err)
	}
	if !result.NaturalExit() || result.ExitCode != 125 || !result.Joined() || result.HolderExitCode != 0 || !result.PostChildAcknowledged || !result.HolderAliveAtPostChild || !result.TerminalAfterAvailable || result.TerminalBefore != result.TerminalAfter || result.NativeFailure != nil {
		t.Fatalf("readonly CLI failure/restoration/lifetime refused: %+v", result)
	}
	if len(result.Stdout) != 0 || len(result.HolderStderr) != 0 || !publicReadOnlyDiagnosticSafe(string(result.Stderr), fixture.root, fixture.directory, result.Identity.Slave) {
		t.Fatalf("readonly failure leaked output, colour or private paths: stdout=%q stderr=%q holder=%q", result.Stdout, result.Stderr, result.HolderStderr)
	}
	requirePublicReadOnlyDiagnostic(t, result.Stderr, machineReadable)
}

func requirePublicReadOnlyDiagnostic(t *testing.T, stderr []byte, machineReadable bool) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(string(stderr)), "\n")
	want := 1
	if machineReadable {
		want = 2
	}
	if len(lines) != want || !strings.Contains(lines[0], "hippo.supervision.failed") || strings.Count(lines[0], "hippo.supervision.failed") != 1 {
		t.Fatalf("wrong diagnostic/body count: %q", stderr)
	}
	if machineReadable {
		var body struct {
			SchemaVersion int    `json:"schemaVersion"`
			Command       string `json:"command"`
			ExitCode      int    `json:"exitCode"`
			Error         struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(lines[1]), &body); err != nil {
			t.Fatal(err)
		}
		if body.SchemaVersion != 1 || body.Command != "hippo tui" || body.ExitCode != 125 || body.Error.Code != "hippo.supervision.failed" || strings.Contains(string(stderr), `"schemaVersion":5`) {
			t.Fatalf("wrong global JSON failure body: %q", stderr)
		}
	}
}
