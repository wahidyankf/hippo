package integration_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/wahidyankf/hippo/tests/support"
	"github.com/wahidyankf/hippo/tests/support/runtimewiring"

	guard "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// runCompiledHIPPO runs the built binary in environment and returns its exit status and output.
func runCompiledHIPPO(t *testing.T, binary string, environment []string, arguments ...string) (int, string) {
	t.Helper()
	command := exec.Command(binary, arguments...)
	command.Env = environment
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	if err == nil {
		return status.Success, output.String()
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("run %v: %v", arguments, err)
	}

	return exitError.ExitCode(), output.String()
}

// A smaller client holds a live owner on a shared root, so the epoch's capacity is
// that smaller pool. A larger client then asks for a minimum only its own pool
// holds. The request must be deferred and must not leave a ledger that the next
// command or the owner's release cannot read.
func TestCompiledClientLargerThanSharedPoolDefersWithoutPoisoningRoot(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "hippo")
	// A stamped test build samples the fixed host evidence below instead of the live
	// host, so only the shared ledger decides admission here. Against the live host,
	// the CPU that parallel test packages use on a small runner defers the final run
	// at the host profile's admission, and that says nothing about the shared pool.
	build := exec.Command("go", "build", "-ldflags", support.StampedBuildLinkerFlags(), "-o", binary, "./cmd/hippo")
	build.Dir = integrationModuleRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build HIPPO: %s: %v", output, err)
	}
	shared := filepath.Join(root, "shared")
	environment, releaseHost, err := support.QuietHostEnvironment(append(os.Environ(), "HIPPO_ROOT="+shared))
	if err != nil {
		t.Fatalf("publish the fixed host evidence: %v", err)
	}
	defer releaseHost()
	configPath := filepath.Join(root, "hippo.json")
	if err := os.WriteFile(configPath, []byte(`{"schemaVersion":2,"coordination":{"maxCpu":2,"maxMemoryMiB":1024}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	owner, err := runtimewiring.AcquireReservation(
		context.Background(), shared, "", policy.TaskService, "balanced", "",
		guard.ReservationPlan{
			Capacity:  guard.ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
			Requested: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
			Allocated: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		}, 20, 0,
	)
	if err != nil || owner == nil {
		t.Fatalf("smaller client did not hold the shared root: session=%+v error=%v", owner, err)
	}
	defer func() { _ = guard.ReleaseReservation(shared, owner) }()

	// The outcome does not depend on the deadline: this minimum cannot fit while the
	// owner lives, so the request defers at whichever deadline it is given. A short
	// one keeps the test from waiting out the five-minute default.
	marker := filepath.Join(root, "child-started")
	larger := []string{
		"run", "--config", configPath, "--disk-path", ".", "--reserve-cpu", "2", "--reserve-memory-mib", "256",
		"--wait-for-admission", "50ms", "--", "/bin/sh", "-c", "printf started > '" + marker + "'",
	}
	if code, output := runCompiledHIPPO(t, binary, environment, larger...); code != status.LimitShed {
		t.Fatalf("larger client exit=%d output=%q, want the capacity deferral %d", code, output, status.LimitShed)
	}
	if _, err = os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deferred client started its child: stat error=%v", err)
	}
	if code, output := runCompiledHIPPO(t, binary, environment, "status", "--config", configPath, "--json"); code != status.Success {
		t.Fatalf("status after the deferral exit=%d output=%q, want a readable shared root", code, output)
	}
	if err = guard.ReleaseReservation(shared, owner); err != nil {
		t.Fatalf("live owner could not release after the deferral: %v", err)
	}
	if code, output := runCompiledHIPPO(t, binary, environment, larger...); code != status.Success {
		t.Fatalf("larger client after the owner released exit=%d output=%q, want admission", code, output)
	}
	if _, err = os.Stat(marker); err != nil {
		t.Fatalf("admitted client did not run its child: %v", err)
	}
}
