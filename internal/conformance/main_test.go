package conformance //nolint:testpackage // Main is exercised with the same manifest the command reads.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/status"
)

// blockedGateManifest writes a four-consumer manifest whose first gate marks
// that it started and then runs until it is stopped.
func blockedGateManifest(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(truePath)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "hippo-fixture")
	if err = os.WriteFile(filepath.Clean(binary), data, 0o700); err != nil { //nolint:gosec // The fixture writes inside its own temporary directory.
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	manifest := Manifest{
		SchemaVersion: 1, HIPPOBinary: binary, HIPPOSHA256: hex.EncodeToString(digest[:]),
		SharedRoot: filepath.Join(root, "shared"),
	}
	started := filepath.Join(root, "gate-started")
	for index := range 4 {
		path := filepath.Join(root, fmt.Sprintf("consumer-%d", index+1))
		initializeFixtureCheckout(t, path)
		gate := Command{Arguments: []string{"true"}}
		if index == 0 {
			gate = Command{Arguments: []string{"/bin/sh", "-c", `printf started > "$1"; exec sleep 30`, "gate", started}}
		}
		manifest.Consumers = append(manifest.Consumers, Consumer{
			Name: fmt.Sprintf("consumer-%d", index+1), Path: path, Gates: []Command{gate},
		})
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	if err = os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	return manifestPath, started
}

func initializeFixtureCheckout(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=HIPPO fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-q", "-m", "fixture"},
	} {
		command := exec.Command("git", append([]string{"-C", path}, arguments...)...)
		// A hook's GIT_DIR would redirect this at the repository under test.
		command.Env = removeEnvironment(os.Environ(), "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("initialize checkout: %s: %v", output, err)
		}
	}
}

// runUntilGateStarts runs Main and cancels it with cause once the blocked gate
// has started.
func runUntilGateStarts(t *testing.T, cause error) (int, string) {
	t.Helper()
	manifestPath, started := blockedGateManifest(t)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	done := make(chan int, 1)
	go func() { done <- Main(ctx, []string{manifestPath}, stdout, stderr) }()
	deadline := time.Now().Add(time.Minute)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel(nil)
			<-done
			t.Fatalf("the gate never started: %q", stderr.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel(cause)
	select {
	case code := <-done:
		return code, stderr.String()
	case <-time.After(time.Minute):
		t.Fatal("hippo-conformance did not return after cancellation")
	}

	return 0, ""
}

func TestMainEndsASignalledRunWithTheSignalStatus(t *testing.T) {
	// A conformance run a signal stopped reached no verdict. Exit 1 would
	// read as "the consumers do not conform", which the run never found out.
	for _, signal := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			code, stderr := runUntilGateStarts(t, status.Interruption{Signal: signal})
			if want := 128 + int(signal); code != want {
				t.Fatalf("a run stopped by %s exited %d, want %d: %q", signal, code, want, stderr)
			}
			// What the harness found while stopping is still reported.
			if !strings.Contains(stderr, `consumer "consumer-1" gate`) {
				t.Fatalf("a stopped run hid what it found: %q", stderr)
			}
		})
	}
}

func TestMainReportsACancelledRunWithoutASignalAsAFailure(t *testing.T) {
	// Only a signal changes the status. A run cancelled for any other reason
	// did not conform, as before.
	code, stderr := runUntilGateStarts(t, nil)
	if code != 1 || stderr == "" {
		t.Fatalf("a cancelled run exited %d with %q, want 1 and its diagnostic", code, stderr)
	}
}
