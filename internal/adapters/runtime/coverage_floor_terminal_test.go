package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/internal/adapters/system"
)

func TestFloorForegroundProcessGroupReportsNonTerminal(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "foreground-*.lock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if _, err := ForegroundProcessGroup(file); err == nil {
		t.Fatal("ForegroundProcessGroup on a regular file = nil, want error")
	}
}

func TestFloorCurrentProcessGroupMatchesTheKernel(t *testing.T) {
	if got, want := CurrentProcessGroup(), syscall.Getpgrp(); got != want {
		t.Fatalf("CurrentProcessGroup = %d, want %d", got, want)
	}
}

func TestFloorWithoutEnvironmentStripsTheName(t *testing.T) {
	environment := []string{"KEEP=1", "HIPPO_X=1", "HIPPO_X=2", "OTHER=HIPPO_X=3"}
	got := withoutEnvironment(environment, "HIPPO_X")
	for _, entry := range got {
		if entry == "HIPPO_X=1" || entry == "HIPPO_X=2" {
			t.Fatalf("withoutEnvironment kept %q", entry)
		}
	}
	if len(got) != 2 {
		t.Fatalf("withoutEnvironment = %q, want the two unrelated entries", got)
	}
	if len(environment) != 4 {
		t.Fatal("withoutEnvironment modified its input")
	}
}

func TestFloorReportLifetimeHandshakeEncodes(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "handshake-*.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if err := reportLifetimeHandshake(file, lifetimeHandshake{ProcessGroup: 42}); err != nil {
		t.Fatalf("reportLifetimeHandshake: %v", err)
	}
}

func TestFloorChildUsageRecordOnNilIsANoOp(t *testing.T) {
	var usage *childUsage
	usage.record(nil)
	if _, measured := usage.snapshot(); measured {
		t.Error("nil usage snapshot measured, want unmeasured")
	}
}

func TestFloorProcessGroupAliveRefusesInvalidGroups(t *testing.T) {
	for _, group := range []int{0, -3} {
		if alive, err := processGroupAlive(group); err == nil || alive {
			t.Fatalf("processGroupAlive(%d) = %v, %v, want an error", group, alive, err)
		}
	}
}

func TestFloorProcessGroupAliveSeesARunningGroup(t *testing.T) {
	command := exec.Command("sleep", "30")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_, _ = command.Process.Wait()
	})
	alive, err := processGroupAlive(command.Process.Pid)
	if err != nil || !alive {
		t.Fatalf("processGroupAlive(running) = %v, %v, want true, nil", alive, err)
	}
}

func TestFloorProcessGroupAliveMissesAnAbsentGroup(t *testing.T) {
	alive, err := processGroupAlive(1 << 30)
	if err != nil || alive {
		t.Fatalf("processGroupAlive(absent) = %v, %v, want false, nil", alive, err)
	}
}

func TestFloorWaitForProcessGroupRetirementOnAbsentGroup(t *testing.T) {
	if err := waitForProcessGroupRetirement(1 << 30); err != nil {
		t.Fatalf("waitForProcessGroupRetirement(absent): %v", err)
	}
}

func TestFloorWaitForProcessGroupRetirementWaitsOutAGroup(t *testing.T) {
	command := exec.Command("sleep", "30")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	group := command.Process.Pid
	retired := make(chan error, 1)
	go func() { retired <- waitForProcessGroupRetirement(group) }()
	time.Sleep(50 * time.Millisecond)
	_ = syscall.Kill(-group, syscall.SIGKILL)
	_, _ = command.Process.Wait()
	select {
	case err := <-retired:
		if err != nil {
			t.Fatalf("waitForProcessGroupRetirement: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("waitForProcessGroupRetirement did not return after the group died")
	}
}

func TestFloorWaitStatusCodeReportsExitStatus(t *testing.T) {
	command := exec.Command("sh", "-c", "exit 3")
	err := command.Run()
	if err == nil {
		t.Fatal("sh -c 'exit 3' succeeded, want an exit error")
	}
	if got := waitStatusCode(err); got != 3 {
		t.Fatalf("waitStatusCode(exit 3) = %d, want 3", got)
	}
	if got := waitStatusCode(nil); got != 0 {
		t.Fatalf("waitStatusCode(nil) = %d, want 0", got)
	}
	if got := waitStatusCode(errLifetimeHandshakeDeadline); got != 1 {
		t.Fatalf("waitStatusCode(generic) = %d, want 1", got)
	}
}

func TestFloorWaitStatusCodeReportsSignal(t *testing.T) {
	command := exec.Command("sh", "-c", "kill -TERM $$")
	err := command.Run()
	if err == nil {
		t.Fatal("sh -c 'kill -TERM $$' succeeded, want an exit error")
	}
	if got, want := waitStatusCode(err), 128+int(syscall.SIGTERM); got != want {
		t.Fatalf("waitStatusCode(SIGTERM) = %d, want %d", got, want)
	}
}

func TestFloorNewEvidenceWriterRefusesUnwritableRoot(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "root-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	if _, err := NewEvidenceWriter(marker.Name()+"/evidence", "floor", evidence.Limits{}); err == nil {
		t.Fatal("NewEvidenceWriter under a file = nil, want error")
	}
}

func TestFloorSetReservationContextOnNilIsANoOp(_ *testing.T) {
	writer := &EvidenceWriter{}
	writer.SetReservationContext(nil, 0, evidence.BudgetOutcome(0))
	var nilWriter *EvidenceWriter
	nilWriter.SetReservationContext(&Session{}, 0, evidence.BudgetOutcome(0))
}

func TestFloorSystemForegroundGroupReportsNonTerminal(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "system-fg-*.lock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	port := system.Production
	if _, err := port.Terminal.ForegroundGroup(file); err == nil {
		t.Fatal("foregroundGroup on a regular file = nil, want error")
	}
	if err := port.Terminal.SetForegroundGroup(file, 1); err == nil {
		t.Fatal("setForegroundGroup on a regular file = nil, want error")
	}
}
