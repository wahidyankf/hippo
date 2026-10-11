package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

func displayRegistrationConfig(root string, reservation bool) application.RunConfig {
	vector := coordination.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
	return application.RunConfig{
		Command: "/usr/bin/printf", Arguments: []string{"private-argument"},
		WorkingDirectory: "/private/fixture-directory", EvidenceRoot: root,
		TaskClass: policy.TaskService, Policy: policy.Policy{LeaseWait: time.Second},
		Resolution:        policy.Resolution{ResolvedProfile: "balanced"},
		ReservationPolicy: coordination.ReservationPolicy{Enabled: reservation, MaxActiveOwners: 20},
		ReservationPlan:   coordination.ReservationPlan{Capacity: vector, Requested: vector, Allocated: vector},
		Now:               time.Now,
	}
}

func displayRegister(t *testing.T, engine *runtime.Engine, config application.RunConfig) application.AdmissionResult {
	t.Helper()
	admission, err := engine.BeginAdmission(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admission.Close)
	result, err := admission.Attempt(t.Context(), time.Second, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Lease != nil {
		t.Cleanup(func() {
			if releaseError := engine.Release(config, result.Lease); releaseError != nil {
				t.Error(releaseError)
			}
		})
	}
	return result
}

func assertRegisteredDisplay(t *testing.T, root, runID string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "run-display", runID+".json"))
	if err != nil {
		t.Fatalf("visible run %s has no executable-only display annotation: %v", runID, err)
	}
	var record struct {
		SchemaVersion int    `json:"schemaVersion"`
		RunID         string `json:"runId"`
		Label         string `json:"label"`
	}
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.SchemaVersion != 1 || record.RunID != runID || record.Label != "printf" {
		t.Fatalf("visible run has wrong derived display annotation: %+v", record)
	}
}

func TestRuntimeDisplayPublishedAtFirstVisibleRegistration(t *testing.T) {
	for _, reservation := range []bool{false, true} {
		name := "exclusive"
		if reservation {
			name = "reservation"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			config := displayRegistrationConfig(root, reservation)
			engine := runtime.NewEngine()
			result := displayRegister(t, engine, config)
			if result.Lease == nil || result.Waiting {
				t.Fatalf("free capacity did not produce one owner: %+v", result)
			}
			assertRegisteredDisplay(t, root, result.Lease.Token)
		})
	}
}

func TestRuntimeDisplayPublishedAtFirstVisibleWaiter(t *testing.T) {
	root := t.TempDir()
	config := displayRegistrationConfig(root, true)
	owner := displayRegister(t, runtime.NewEngine(), config)
	if owner.Lease == nil {
		t.Fatalf("capacity fixture has no owner: %+v", owner)
	}
	waiter := displayRegister(t, runtime.NewEngine(), config)
	if !waiter.Waiting || waiter.Lease != nil || waiter.Status.RunID == "" || waiter.Status.Position != 1 {
		t.Fatalf("exhausted capacity did not expose the first FIFO waiter: %+v", waiter)
	}
	assertRegisteredDisplay(t, root, waiter.Status.RunID)
}
