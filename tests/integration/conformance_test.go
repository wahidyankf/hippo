package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/conformance"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

func integrationModuleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statError := os.Stat(filepath.Join(directory, "go.mod")); statError == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("go.mod root is unavailable")
		}
		directory = parent
	}
}

func buildConformanceBinary(t *testing.T, root string) string {
	t.Helper()
	binary := filepath.Join(root, "hippo-conformance")
	command := exec.Command("go", "build", "-o", binary, "./cmd/hippo-conformance")
	command.Dir = integrationModuleRoot(t)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build conformance binary: %s: %v", output, err)
	}

	return binary
}

func initializeConformanceCheckout(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	commands := [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=HIPPO fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-q", "-m", "fixture"},
	}
	for _, arguments := range commands {
		command := exec.Command("git", arguments...)
		command.Dir = path
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("initialize checkout: %s: %v", output, err)
		}
	}
}

func compiledConformanceManifest(t *testing.T, root, hippoBinary string) (conformance.Manifest, string) {
	t.Helper()
	data, err := os.ReadFile(hippoBinary)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	manifest := conformance.Manifest{
		SchemaVersion: 1, HIPPOBinary: hippoBinary, HIPPOSHA256: hex.EncodeToString(digest[:]), SharedRoot: filepath.Join(root, "shared"),
	}
	for index := range 4 {
		path := filepath.Join(root, fmt.Sprintf("consumer-%d", index+1))
		initializeConformanceCheckout(t, path)
		manifest.Consumers = append(manifest.Consumers, conformance.Consumer{
			Name: fmt.Sprintf("consumer-%d", index+1), Path: path,
			Gates: []conformance.Command{{Arguments: []string{"true"}}},
		})
	}
	manifestPath := filepath.Join(root, "manifest.json")

	return manifest, manifestPath
}

func writeCompiledConformanceManifest(t *testing.T, path string, manifest conformance.Manifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCompiledConformanceRejectsReplacedCheckoutDirectory(t *testing.T) {
	root := t.TempDir()
	runner := buildConformanceBinary(t, root)
	manifest, manifestPath := compiledConformanceManifest(t, root, runner)
	checkout := manifest.Consumers[0].Path
	original := checkout + "-original"
	gateMarker := filepath.Join(root, "replacement-gate-started")
	manifest.Consumers[0].Bootstrap = []conformance.Command{{Arguments: []string{
		"/bin/sh", "-c", `mv "$1" "$2" && cp -R "$2" "$1"`, "conformance", checkout, original,
	}}}
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		"/bin/sh", "-c", `printf started > "$1"`, "conformance", gateMarker,
	}}}
	writeCompiledConformanceManifest(t, manifestPath, manifest)

	command := exec.Command(runner, manifestPath)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("compiled conformance accepted a replacement checkout directory")
	}
	if !bytes.Contains(output, []byte("checkout identity changed")) {
		t.Fatalf("compiled conformance did not report checkout identity replacement privately: %s", output)
	}
	if bytes.Contains(output, []byte(root)) || bytes.Contains(output, []byte(checkout)) {
		t.Fatalf("compiled checkout identity error exposed a private path: %s", output)
	}
	if _, statError := os.Stat(gateMarker); !errors.Is(statError, os.ErrNotExist) {
		t.Fatal("compiled conformance started a gate in the replacement checkout")
	}
}

func TestCompiledConformanceRejectsReplacedSharedRoot(t *testing.T) {
	for _, replacement := range []string{"directory", "symlink"} {
		t.Run(replacement, func(t *testing.T) {
			root := t.TempDir()
			runner := buildConformanceBinary(t, root)
			manifest, manifestPath := compiledConformanceManifest(t, root, runner)
			original := manifest.SharedRoot + "-original"
			replacementTarget := manifest.SharedRoot + "-replacement"
			commandMarker := filepath.Join(root, "coordination-command-started")
			script := `mv "$1" "$2" && mkdir "$1"`
			if replacement == "symlink" {
				script = `mv "$1" "$2" && mkdir "$3" && ln -s "$3" "$1"`
			}
			manifest.Consumers[0].Bootstrap = []conformance.Command{{Arguments: []string{
				"/bin/sh", "-c", script, "conformance", manifest.SharedRoot, original, replacementTarget,
			}}}
			manifest.CoordinationChecks = []conformance.Check{{
				Consumer: manifest.Consumers[0].Name,
				Command: conformance.Command{Arguments: []string{
					"/bin/sh", "-c", `printf started > "$1"`, "conformance", commandMarker,
				}},
			}}
			writeCompiledConformanceManifest(t, manifestPath, manifest)

			command := exec.Command(runner, manifestPath)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("compiled conformance accepted a replacement shared root: %s", output)
			}
			if !bytes.Contains(output, []byte("shared-root identity changed")) {
				t.Fatalf("compiled conformance did not report shared-root replacement privately: %s", output)
			}
			if bytes.Contains(output, []byte(root)) || bytes.Contains(output, []byte(manifest.SharedRoot)) {
				t.Fatalf("compiled shared-root identity error exposed a private path: %s", output)
			}
			if _, statError := os.Stat(commandMarker); !errors.Is(statError, os.ErrNotExist) {
				t.Fatal("compiled conformance started coordination against a replacement shared root")
			}
		})
	}
}

func TestCompiledConformanceScrubsCallerReservationEnvironment(t *testing.T) {
	root := t.TempDir()
	runner := buildConformanceBinary(t, root)
	manifest, manifestPath := compiledConformanceManifest(t, root, runner)
	plan := guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: 4, MemoryBytes: policy.GiB},
		Requested: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Allocated: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	caller, err := guard.AcquireReservation(
		context.Background(), manifest.SharedRoot, "", policy.TaskService, "balanced", "caller", plan, 20, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = guard.ReleaseReservation(manifest.SharedRoot, caller) })
	explicitConfig := filepath.Join(root, "operator-override.json")
	for index := range manifest.Consumers {
		manifest.Consumers[index].Gates = []conformance.Command{{Arguments: []string{
			"/bin/sh", "-c",
			`test -z "${HIPPO_SESSION+x}" && test -z "${HIPPO_PROFILE+x}" && test -z "${HIPPO_CONCURRENCY+x}" && test -z "${HIPPO_RESERVED_MEMORY_BYTES+x}" && test -z "${HIPPO_DEFAULT_CONFIG+x}" && test "$HIPPO_CONFIG" = "$1"`,
			"conformance", explicitConfig,
		}}}
	}
	writeCompiledConformanceManifest(t, manifestPath, manifest)

	command := exec.Command(runner, manifestPath)
	command.Env = append(os.Environ(),
		"HIPPO_SESSION="+caller.Token,
		"HIPPO_PROFILE=balanced",
		"HIPPO_CONCURRENCY=1",
		fmt.Sprintf("HIPPO_RESERVED_MEMORY_BYTES=%d", 256*policy.MiB),
		"HIPPO_DEFAULT_CONFIG="+filepath.Join(root, "caller-repository-default.json"),
		"HIPPO_CONFIG="+explicitConfig,
	)
	if output, runError := command.CombinedOutput(); runError != nil {
		t.Fatalf("compiled consumers inherited caller reservation state: %s: %v", output, runError)
	}
}

func TestCompiledConformancePinsVerifiedBinaryBytes(t *testing.T) {
	root := t.TempDir()
	runner := buildConformanceBinary(t, root)
	hippoBinary := filepath.Join(root, "hippo-source")
	originalMarker := filepath.Join(root, "original-ran")
	replacementMarker := filepath.Join(root, "replacement-ran")
	ready := filepath.Join(root, "ready")
	continued := filepath.Join(root, "continued")
	original := fmt.Sprintf("#!/bin/sh\nprintf original > %q\n", originalMarker)
	replacement := fmt.Sprintf("#!/bin/sh\nprintf replacement > %q\n", replacementMarker)
	if err := os.WriteFile(hippoBinary, []byte(original), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, manifestPath := compiledConformanceManifest(t, root, hippoBinary)
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		"/bin/sh", "-c", `printf ready > "$1"; while [ ! -f "$2" ]; do sleep 0.005; done; "$HIPPO_BIN"`,
		"conformance", ready, continued,
	}}}
	writeCompiledConformanceManifest(t, manifestPath, manifest)
	command := exec.Command(runner, manifestPath)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = command.Process.Kill()
			t.Fatal("compiled conformance command did not reach execution boundary")
		}
		time.Sleep(time.Millisecond)
	}
	replacementPath := filepath.Join(root, "replacement")
	if err := os.WriteFile(replacementPath, []byte(replacement), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacementPath, hippoBinary); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(continued, []byte("continue\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("compiled conformance accepted a changed source identity")
	}
	if _, err := os.Stat(originalMarker); err != nil {
		t.Fatalf("pinned original did not execute: %v", err)
	}
	if _, err := os.Stat(replacementMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("replacement bytes executed")
	}
	if strings.Contains(output.String(), root) {
		t.Fatalf("compiled identity error exposed private path: %s", output.String())
	}
}

func TestCompiledConformanceStartErrorsStayPrivate(t *testing.T) {
	root := t.TempDir()
	runner := buildConformanceBinary(t, root)
	hippoBinary := filepath.Join(root, "hippo-source")
	if err := os.WriteFile(hippoBinary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, manifestPath := compiledConformanceManifest(t, root, hippoBinary)
	privateExecutable := filepath.Join(root, "private-missing-command")
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{privateExecutable}}}
	writeCompiledConformanceManifest(t, manifestPath, manifest)
	command := exec.Command(runner, manifestPath)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("compiled conformance accepted a missing command")
	}
	message := string(output)
	if !strings.Contains(message, `consumer "consumer-1" gate`) || !strings.Contains(message, "start") {
		t.Fatalf("compiled error omitted safe metadata: %s", message)
	}
	if strings.Contains(message, privateExecutable) || strings.Contains(message, root) {
		t.Fatalf("compiled error exposed private path: %s", message)
	}
}

func TestCompiledConformanceSignalCleansDescendantsAndReconciles(t *testing.T) {
	root := t.TempDir()
	runner := buildConformanceBinary(t, root)
	hippoBinary := filepath.Join(root, "hippo-source")
	if err := os.WriteFile(hippoBinary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, manifestPath := compiledConformanceManifest(t, root, hippoBinary)
	mutation := filepath.Join(manifest.Consumers[0].Path, "mutation")
	childPIDPath := filepath.Join(root, "child-pid")
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		"/bin/sh", "-c", `printf changed > "$1"; sh -c 'trap "" TERM; printf "%s" "$$" > "$1"; while :; do sleep 0.05; done' child "$2" & wait`,
		"conformance", mutation, childPIDPath,
	}}}
	writeCompiledConformanceManifest(t, manifestPath, manifest)
	command := exec.Command(runner, manifestPath)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	childPID := 0
	for {
		data, err := os.ReadFile(childPIDPath)
		if err == nil {
			_, _ = fmt.Sscan(string(data), &childPID)
		}
		if childPID > 0 {
			break
		}
		if time.Now().After(deadline) {
			_ = command.Process.Kill()
			t.Fatal("compiled descendant did not start")
		}
		time.Sleep(time.Millisecond)
	}
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(output.String(), "checkout changed") {
			t.Fatalf("compiled cancellation did not reconcile mutation: %s: %v", output.String(), err)
		}
	case <-time.After(3 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("compiled cancellation was not bounded")
	}
	deadline = time.Now().Add(time.Second)
	for {
		err := syscall.Kill(childPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
			t.Fatal("compiled cancellation left a descendant live")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCompiledConformanceLeaderFirstCancellationRetiresDescendant(t *testing.T) {
	root := t.TempDir()
	runner := buildConformanceBinary(t, root)
	hippoBinary := filepath.Join(root, "hippo-source")
	if err := os.WriteFile(hippoBinary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, manifestPath := compiledConformanceManifest(t, root, hippoBinary)
	mutation := filepath.Join(manifest.Consumers[0].Path, "late-cancellation-mutation")
	childPIDPath := filepath.Join(root, "leader-first-child-pid")
	leaderExitPath := filepath.Join(root, "leader-exited")
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		"/bin/sh", "-c",
		`trap 'printf exited > "$3"; exit 0' TERM; sh -c 'trap "" TERM HUP; printf "%s" "$$" > "$1"; while [ ! -f "$2" ]; do sleep 0.005; done; printf late > "$3"; while :; do sleep 0.05; done' descendant "$2" "$3" "$1" </dev/null >/dev/null 2>&1 & wait`,
		"conformance", mutation, childPIDPath, leaderExitPath,
	}}}
	writeCompiledConformanceManifest(t, manifestPath, manifest)
	command := exec.Command(runner, manifestPath)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	childPID := waitForConformancePID(t, childPIDPath)
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(output.String(), "checkout changed") {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
			t.Fatalf("leader-first cancellation reconciled before late mutation: %s: %v", output.String(), err)
		}
	case <-time.After(3 * time.Second):
		_ = command.Process.Kill()
		_ = syscall.Kill(childPID, syscall.SIGKILL)
		t.Fatal("leader-first cancellation was not bounded")
	}
	if signalError := syscall.Kill(childPID, 0); !errors.Is(signalError, syscall.ESRCH) {
		_ = syscall.Kill(childPID, syscall.SIGKILL)
		t.Fatalf("leader-first descendant survived reconciliation: %v", signalError)
	}
}

func TestCompiledConformanceCompletedLeaderRetiresDescendant(t *testing.T) {
	for _, fixture := range []struct {
		name string
		exit int
	}{{name: "zero", exit: 0}, {name: "nonzero", exit: 9}} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			runner := buildConformanceBinary(t, root)
			hippoBinary := filepath.Join(root, "hippo-source")
			if err := os.WriteFile(hippoBinary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			manifest, manifestPath := compiledConformanceManifest(t, root, hippoBinary)
			mutation := filepath.Join(manifest.Consumers[0].Path, "late-completed-mutation")
			childPIDPath := filepath.Join(root, "completed-child-pid")
			script := fmt.Sprintf(
				`leader=$$; sh -c 'trap "" TERM HUP; printf "%%s" "$$" > "$1"; while kill -0 "$3" 2>/dev/null; do sleep 0.005; done; printf late > "$2"; while :; do sleep 0.05; done' descendant "$1" "$2" "$leader" </dev/null >/dev/null 2>&1 & while [ ! -s "$1" ]; do sleep 0.005; done; exit %d`,
				fixture.exit,
			)
			manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
				"/bin/sh", "-c", script, "conformance", childPIDPath, mutation,
			}}}
			writeCompiledConformanceManifest(t, manifestPath, manifest)
			command := exec.Command(runner, manifestPath)
			var output bytes.Buffer
			command.Stdout, command.Stderr = &output, &output
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			childPID := waitForConformancePID(t, childPIDPath)
			err := command.Wait()
			if err == nil || !strings.Contains(output.String(), "checkout changed") {
				_ = syscall.Kill(childPID, syscall.SIGKILL)
				t.Fatalf("completed leader reconciled before late mutation: %s: %v", output.String(), err)
			}
			if signalError := syscall.Kill(childPID, 0); !errors.Is(signalError, syscall.ESRCH) {
				_ = syscall.Kill(childPID, syscall.SIGKILL)
				t.Fatalf("completed leader descendant survived reconciliation: %v", signalError)
			}
		})
	}
}

func TestCompiledConformanceRejectsTamperedPinnedBinary(t *testing.T) {
	root := t.TempDir()
	runner := buildConformanceBinary(t, root)
	hippoBinary := filepath.Join(root, "hippo-source")
	if err := os.WriteFile(hippoBinary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, manifestPath := compiledConformanceManifest(t, root, hippoBinary)
	replacement := filepath.Join(root, "replacement")
	replacementMarker := filepath.Join(root, "tampered-pinned-binary-ran")
	if err := os.WriteFile(replacement, []byte(fmt.Sprintf("#!/bin/sh\nprintf tampered > %q\n", replacementMarker)), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest.Consumers[0].Bootstrap = []conformance.Command{{Arguments: []string{
		"/bin/sh", "-c", `directory=$(dirname "$HIPPO_BIN"); chmod 700 "$directory" "$HIPPO_BIN"; cp "$1" "$HIPPO_BIN"`, "conformance", replacement,
	}}}
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{"/bin/sh", "-c", `"$HIPPO_BIN"`}}}
	writeCompiledConformanceManifest(t, manifestPath, manifest)
	command := exec.Command(runner, manifestPath)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("tampered pinned binary was accepted: %s: %v", output, err)
	}
	if _, err := os.Stat(replacementMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("tampered pinned binary executed")
	}
	if source, err := os.ReadFile(hippoBinary); err != nil || !bytes.Equal(source, []byte("#!/bin/sh\nexit 0\n")) {
		t.Fatalf("pinned tamper changed the source: %q: %v", source, err)
	}
	if strings.Contains(string(output), root) {
		t.Fatalf("tampered pinned binary error exposed a private path: %s", output)
	}
}

func TestCompiledConformanceParallelGateCannotReplacePinnedBinary(t *testing.T) {
	root := t.TempDir()
	runner := buildConformanceBinary(t, root)
	hippoBinary := filepath.Join(root, "hippo-source")
	originalMarker := filepath.Join(root, "parallel-original-ran")
	replacementMarker := filepath.Join(root, "parallel-replacement-ran")
	original := fmt.Sprintf("#!/bin/sh\nprintf original > %q\n", originalMarker)
	if err := os.WriteFile(hippoBinary, []byte(original), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, manifestPath := compiledConformanceManifest(t, root, hippoBinary)
	replacement := filepath.Join(root, "parallel-replacement")
	if err := os.WriteFile(replacement, []byte(fmt.Sprintf("#!/bin/sh\nprintf replacement > %q\n", replacementMarker)), 0o700); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(root, "parallel-ready")
	replaced := filepath.Join(root, "parallel-replaced")
	attempt := filepath.Join(manifest.Consumers[0].Path, "parallel-replacement-attempt")
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		"/bin/sh", "-c", `while [ ! -f "$1" ]; do sleep 0.005; done; directory=$(dirname "$HIPPO_BIN"); chmod 700 "$directory" "$HIPPO_BIN"; cp "$2" "$HIPPO_BIN"; printf attempted > "$3"; printf replaced > "$4"`,
		"conformance", ready, replacement, attempt, replaced,
	}}}
	manifest.Consumers[1].Gates = []conformance.Command{{Arguments: []string{
		"/bin/sh", "-c", `printf ready > "$1"; while [ ! -f "$2" ]; do sleep 0.005; done; "$HIPPO_BIN"`,
		"conformance", ready, replaced,
	}}}
	writeCompiledConformanceManifest(t, manifestPath, manifest)
	command := exec.Command(runner, manifestPath)
	output, runError := command.CombinedOutput()
	if runError == nil || !strings.Contains(string(output), "checkout changed") {
		t.Fatalf("parallel replacement attempt was not reconciled: %s: %v", output, runError)
	}
	if _, statError := os.Stat(originalMarker); statError != nil {
		t.Fatalf("verified binary did not execute: %v: output=%s run=%v", statError, output, runError)
	}
	if _, err := os.Stat(replacementMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("parallel gate executed replacement bytes")
	}
	if strings.Contains(string(output), root) {
		t.Fatalf("parallel pinned error exposed a private path: %s", output)
	}
}

func TestCompiledConformanceCapacitySkipDoesNotHideIntegrity(t *testing.T) {
	for _, fixture := range []struct {
		name           string
		command        string
		requiredOutput string
	}{
		{
			name: "pinned_binary_tamper",
			command: `directory=$(dirname "$HIPPO_BIN"); chmod 700 "$directory" "$HIPPO_BIN"; ` +
				`printf '#!/bin/sh\nexit 9\n' > "$HIPPO_BIN"; exit 75`,
		},
		{
			name: "verified_cleanup_failure", requiredOutput: "verified HIPPO binary cleanup failed",
			command: `directory=$(dirname "$HIPPO_BIN"); chmod 700 "$directory"; rm -f "$HIPPO_BIN"; ` +
				`rmdir "$directory"; ln -s "$1-missing-target" "$directory"; exit 75`,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			runner := buildConformanceBinary(t, root)
			hippoBinary := filepath.Join(root, "hippo-source")
			if err := os.WriteFile(hippoBinary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			manifest, manifestPath := compiledConformanceManifest(t, root, hippoBinary)
			privateSentinel := filepath.Join(root, "private-capacity-skip-integrity")
			manifest.CoordinationChecks = []conformance.Check{{
				Consumer: manifest.Consumers[0].Name, AllowCapacitySkip: true,
				Command: conformance.Command{Arguments: []string{"/bin/sh", "-c", fixture.command, "conformance", privateSentinel}},
			}}
			writeCompiledConformanceManifest(t, manifestPath, manifest)
			command := exec.Command(runner, manifestPath)
			output, err := command.CombinedOutput()
			if err == nil || strings.Contains(string(output), "skipped") {
				t.Fatalf("integrity failure was treated as a capacity skip: %s: %v", output, err)
			}
			if fixture.requiredOutput != "" && !strings.Contains(string(output), fixture.requiredOutput) {
				t.Fatalf("joined integrity failure was omitted: %s", output)
			}
			if strings.Contains(string(output), root) || strings.Contains(string(output), privateSentinel) {
				t.Fatalf("capacity-skip integrity error exposed a private path: %s", output)
			}
		})
	}
}

func TestCompiledConformanceSetupErrorsStayPrivate(t *testing.T) {
	buildRoot := t.TempDir()
	runner := buildConformanceBinary(t, buildRoot)
	for _, fixture := range []struct {
		name     string
		category string
		prepare  func(*testing.T, string) string
	}{
		{
			name: "unavailable_manifest", category: "conformance manifest is unavailable",
			prepare: func(_ *testing.T, root string) string {
				return filepath.Join(root, "private-unavailable-manifest-sentinel.json")
			},
		},
		{
			name: "uncreatable_shared_root", category: "conformance shared root is unavailable",
			prepare: func(t *testing.T, root string) string {
				t.Helper()
				hippoBinary := filepath.Join(root, "hippo-source")
				if err := os.WriteFile(hippoBinary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				manifest, manifestPath := compiledConformanceManifest(t, root, hippoBinary)
				blocker := filepath.Join(root, "private-uncreatable-shared-root-sentinel")
				if err := os.Mkdir(blocker, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(blocker, 0o700) })
				manifest.SharedRoot = filepath.Join(blocker, "nested")
				writeCompiledConformanceManifest(t, manifestPath, manifest)

				return manifestPath
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			manifestPath := fixture.prepare(t, root)
			directError := conformance.Run(context.Background(), manifestPath, &bytes.Buffer{})
			if directError == nil || !strings.Contains(directError.Error(), fixture.category) || strings.Contains(directError.Error(), root) {
				t.Fatalf("direct setup error was not private: %v", directError)
			}
			command := exec.Command(runner, manifestPath)
			output, runError := command.CombinedOutput()
			if runError == nil || !strings.Contains(string(output), fixture.category) || strings.Contains(string(output), root) {
				t.Fatalf("compiled setup error was not private: %s: %v", output, runError)
			}
		})
	}
}

func waitForConformancePID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, err := os.ReadFile(path)
		var pid int
		if err == nil {
			_, _ = fmt.Sscan(string(data), &pid)
		}
		if pid > 0 {
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatal("conformance descendant did not report its PID")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCompiledConformanceCancellationPreventsNextBootstrapStart(t *testing.T) {
	root := t.TempDir()
	runner := buildConformanceBinary(t, root)
	hippoBinary := filepath.Join(root, "hippo-source")
	if err := os.WriteFile(hippoBinary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, manifestPath := compiledConformanceManifest(t, root, hippoBinary)
	mutation := filepath.Join(manifest.Consumers[0].Path, "mutation")
	nextStarted := filepath.Join(root, "next-started")
	manifest.Consumers[0].Bootstrap = []conformance.Command{
		{Arguments: []string{
			"/bin/sh", "-c", `printf changed > "$1"; kill -TERM "$PPID"; sleep 0.2`,
			"conformance", mutation,
		}},
		{Arguments: []string{"/bin/sh", "-c", `printf started > "$1"`, "conformance", nextStarted}},
	}
	writeCompiledConformanceManifest(t, manifestPath, manifest)
	command := exec.Command(runner, manifestPath)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("compiled conformance ignored bootstrap cancellation")
	}
	if _, statError := os.Stat(nextStarted); !errors.Is(statError, os.ErrNotExist) {
		t.Fatal("compiled conformance started the command after cancellation")
	}
	message := string(output)
	if !strings.Contains(message, "checkout changed") || strings.Contains(message, root) {
		t.Fatalf("compiled cancellation did not privately reconcile checkout state: %s", message)
	}
}
