package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/policy"
)

// validFloorPlan satisfies PlanBounds: requested at the immutable floors and
// inside capacity.
func validFloorPlan() ReservationPlan {
	return ReservationPlan{
		Capacity:  ReservationVector{CPU: 8, MemoryBytes: 8 * 1024 * 1024 * 1024},
		Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
}

func TestFloorRememberSessionNilIsNil(t *testing.T) {
	if got := NewEngine().rememberSession(nil); got != nil {
		t.Fatalf("rememberSession(nil) = %+v, want nil", got)
	}
}

func TestFloorBeginAdmissionRefusesCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewEngine().BeginAdmission(ctx, application.RunConfig{}); err == nil {
		t.Fatal("BeginAdmission on a cancelled context = nil, want error")
	}
}

func TestFloorBeginAdmissionRefusesNegativeWait(t *testing.T) {
	config := application.RunConfig{}
	config.Policy.LeaseWait = -time.Second
	if _, err := NewEngine().BeginAdmission(context.Background(), config); err == nil {
		t.Fatal("BeginAdmission with negative wait = nil, want error")
	}
}

func TestFloorBeginAdmissionDefaultsClock(t *testing.T) {
	admission, err := NewEngine().BeginAdmission(context.Background(), application.RunConfig{
		EvidenceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BeginAdmission without Now: %v", err)
	}
	if admission.Attempt == nil || admission.Receipt == nil || admission.Close == nil {
		t.Fatal("BeginAdmission without Now did not return the exclusive admission")
	}
}

func TestFloorBeginAdmissionRefusesUnwritableExclusiveRoot(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "root-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	if _, err := NewEngine().BeginAdmission(context.Background(), application.RunConfig{
		EvidenceRoot: filepath.Join(marker.Name(), "evidence"),
	}); err == nil {
		t.Fatal("BeginAdmission under a file = nil, want error")
	}
}

func TestFloorBeginAdmissionReportsInheritedLockFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "coordination.lock"), 0o750); err != nil {
		t.Fatal(err)
	}
	config := application.RunConfig{
		EvidenceRoot:      root,
		ReservationPolicy: ReservationPolicy{Enabled: true},
		ReservationPlan:   validFloorPlan(),
		Environment:       []string{"HIPPO_SESSION=0123456789abcdef0123456789abcdef"},
	}
	if _, err := NewEngine().BeginAdmission(context.Background(), config); err == nil {
		t.Fatal("BeginAdmission with an unopenable coordination lock = nil, want error")
	}
}

func TestFloorInheritedAdmissionReportsLedgerFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "reservations.json"), 0o750); err != nil {
		t.Fatal(err)
	}
	state := &admissionState{
		system: system.Production,
		config: application.RunConfig{EvidenceRoot: root},
	}
	if _, err := state.inheritedAdmission(
		context.Background(), "0123456789abcdef0123456789abcdef", time.Second,
	); err == nil {
		t.Fatal("inheritedAdmission with an unreadable ledger = nil, want error")
	}
}

func TestFloorReceiptWithoutReservationPolicyIsNil(t *testing.T) {
	state := &admissionState{config: application.RunConfig{}}
	if err := state.receipt("refused", time.Now()); err != nil {
		t.Fatalf("receipt without a reservation policy = %v, want nil", err)
	}
}

func TestFloorAdmissionCloseTwiceKeepsClosed(t *testing.T) {
	state := &admissionState{}
	state.close()
	state.close()
	if !state.closed {
		t.Error("admission did not stay closed after a second close")
	}
}

func TestFloorEngineNilLeasesAreNoOps(t *testing.T) {
	engine := NewEngine()
	config := application.RunConfig{}
	if err := engine.Release(config, nil); err != nil {
		t.Errorf("Release(nil) = %v, want nil", err)
	}
	if err := engine.Abandon(nil); err != nil {
		t.Errorf("Abandon(nil) = %v, want nil", err)
	}
	if err := engine.ReleasePort(config, nil); err != nil {
		t.Errorf("ReleasePort(nil) = %v, want nil", err)
	}
	if err := engine.AbandonPort(nil); err != nil {
		t.Errorf("AbandonPort(nil) = %v, want nil", err)
	}
}

func TestFloorEngineStartRefusesUnknownLease(t *testing.T) {
	_, err := NewEngine().Start(context.Background(), application.RunConfig{}, &application.Lease{Token: "missing"}, nil)
	if err == nil {
		t.Fatal("Start with an unknown lease = nil, want error")
	}
}

func TestFloorEngineStopRefusesUnknownWorkload(t *testing.T) {
	_, err := NewEngine().Stop(application.RunConfig{}, &application.Workload{ID: "missing"})
	if err == nil {
		t.Fatal("Stop with an unknown workload = nil, want error")
	}
}

func TestFloorEngineActivateRefusesUnknownWorkload(t *testing.T) {
	err := NewEngine().Activate(application.RunConfig{}, &application.Lease{Token: "x"}, &application.Workload{ID: "missing"})
	if err == nil {
		t.Fatal("Activate with an unknown workload = nil, want error")
	}
}

func TestFloorSnapshotWithoutRootReturnsTotals(t *testing.T) {
	totals, err := NewEngine().Snapshot(context.Background(), "", "exclusive")
	if err != nil {
		t.Fatalf("Snapshot without a root: %v", err)
	}
	if totals.SchemaVersion != 5 {
		t.Errorf("Snapshot without a root schema = %d, want 5", totals.SchemaVersion)
	}
}

func TestFloorSnapshotReportsMarkerFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "coordination-mode.json"), []byte("["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEngine().Snapshot(context.Background(), root, "reservation"); err == nil {
		t.Fatal("Snapshot with a corrupt marker = nil, want error")
	}
}

func TestFloorSelectVictimEmergencyTakesThePort(t *testing.T) {
	_, _, err := NewEngine().SelectVictim(t.TempDir(), ShedCausePressure, true)
	if err != nil {
		t.Fatalf("SelectVictim emergency: %v", err)
	}
}

func TestFloorSelectVictimStandardTakesThePort(t *testing.T) {
	_, _, err := NewEngine().SelectVictim(t.TempDir(), ShedCausePressure, false)
	if err != nil {
		t.Fatalf("SelectVictim standard: %v", err)
	}
}

func TestFloorEngineOpenRefusesUnwritableRoot(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "root-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	if _, err := NewEngine().Open(application.RunConfig{
		EvidenceRoot: filepath.Join(marker.Name(), "evidence"),
		Now:          time.Now,
	}); err == nil {
		t.Fatal("Open under a file = nil, want error")
	}
}

func TestFloorBeginPortRefusesInvalidRequest(t *testing.T) {
	if _, err := NewEngine().BeginPort(application.RunConfig{LeasePort: 0}); err == nil {
		t.Fatal("BeginPort with port 0 = nil, want error")
	}
}

func TestFloorBeginPortRefusesUnwritableRoot(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "root-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	_, err = NewEngine().BeginPort(application.RunConfig{
		LeasePort: 8080, LeaseMinimum: 1, LeaseMaximum: 65535, LeaseOwner: "fixture",
		PortLeaseRoot: filepath.Join(marker.Name(), "leases"),
	})
	if err == nil {
		t.Fatal("BeginPort under a file = nil, want error")
	}
}
