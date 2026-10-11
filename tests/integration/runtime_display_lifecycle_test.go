package integration_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
)

func TestRuntimeDisplayOwnedRetirementAndCancellation(t *testing.T) {
	for _, reservation := range []bool{false, true} {
		root := t.TempDir()
		config := displayRegistrationConfig(root, reservation)
		engine := runtimeadapter.NewEngine()
		owner := displayRegister(t, engine, config)
		assertRegisteredDisplay(t, root, owner.Lease.Token)
		if reservation {
			waiterEngine := runtimeadapter.NewEngine()
			admission, err := waiterEngine.BeginAdmission(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(admission.Close)
			waiter, err := admission.Attempt(t.Context(), time.Second, time.Now())
			if err != nil || !waiter.Waiting {
				t.Fatalf("waiter setup failed: %+v %v", waiter, err)
			}
			assertRegisteredDisplay(t, root, waiter.Status.RunID)
			admission.Close()
			if annotation := displayContractRead(t, root, waiter.Status.RunID); annotation.Availability != application.DisplayMissing {
				t.Fatalf("cancelled waiter kept owned label: %+v", annotation)
			}
			totals, err := engine.Snapshot(t.Context(), root, "reservation")
			if err != nil || totals.ActiveOwners != 1 || totals.WaitingOwners != 0 {
				t.Fatalf("annotation cancellation changed accounting: %+v %v", totals, err)
			}
		}
		if err := engine.Release(config, owner.Lease); err != nil {
			t.Fatal(err)
		}
		if annotation := displayContractRead(t, root, owner.Lease.Token); annotation.Availability != application.DisplayMissing {
			t.Fatalf("retired owner kept label: %+v", annotation)
		}
	}
}

func TestRuntimeDisplayInheritanceNeverOverwritesOrRemovesOwnerContext(t *testing.T) {
	for _, reservation := range []bool{false, true} {
		root := t.TempDir()
		config := displayRegistrationConfig(root, reservation)
		config.Label = displayContractLabel(t, "owner-context")
		owner := displayRegister(t, runtimeadapter.NewEngine(), config)
		path := filepath.Join(root, "run-display", owner.Lease.Token+".json")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		config.Label = displayContractLabel(t, "nested-context")
		config.Environment = []string{"HIPPO_SESSION=" + owner.Lease.Token}
		nestedEngine := runtimeadapter.NewEngine()
		admission, err := nestedEngine.BeginAdmission(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(admission.Close)
		lease := admission.Admitted
		if lease == nil {
			result, attemptError := admission.Attempt(t.Context(), time.Second, time.Now())
			if attemptError != nil {
				t.Fatal(attemptError)
			}
			lease = result.Lease
		}
		if lease == nil || !lease.Inherited || lease.Token != owner.Lease.Token {
			t.Fatalf("nested fixture did not inherit: %+v", lease)
		}
		if err = nestedEngine.Release(config, lease); err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("nested registration/retirement changed original label: %v", err)
		}
	}
}

func TestRuntimeDisplayAdvisoryWriteAndRemovalFaultsPreserveAccounting(t *testing.T) {
	for _, fault := range []string{"publish", "remove"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "run-display")
			if fault == "publish" {
				if err := os.WriteFile(path, []byte("unusable annotation directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			config := displayRegistrationConfig(root, true)
			engine := runtimeadapter.NewEngine()
			owner := displayRegister(t, engine, config)
			totals, err := engine.Snapshot(t.Context(), root, "reservation")
			if err != nil || totals.ActiveOwners != 1 || totals.Allocated != config.ReservationPlan.Allocated {
				t.Fatalf("advisory publication changed admission/accounting: %+v %v", totals, err)
			}
			if fault == "remove" {
				if err = os.Rename(path, filepath.Join(root, "retained-display")); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, []byte("unusable annotation directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err = engine.Release(config, owner.Lease); err != nil {
				t.Fatalf("advisory removal changed retirement: %v", err)
			}
			totals, err = engine.Snapshot(t.Context(), root, "reservation")
			if err != nil || totals.ActiveOwners != 0 || totals.WaitingOwners != 0 {
				t.Fatalf("advisory retirement left accounting live: %+v %v", totals, err)
			}
		})
	}
}

func TestRuntimeDisplayDoesNotExpandStrictMetadataOrStatus(t *testing.T) {
	root := t.TempDir()
	config := displayRegistrationConfig(root, true)
	config.Label = displayContractLabel(t, "display-only")
	config.ReservationMetadata.Source = "hippo"
	config.ReservationMetadata.Tags = map[string]string{"role": "test"}
	config.ReservationMetadata.Tier = "standard"
	engine := runtimeadapter.NewEngine()
	owner := displayRegister(t, engine, config)
	data, err := os.ReadFile(filepath.Join(root, "owner-metadata", owner.Lease.Token+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 9 || fields["label"] != nil || bytes.Contains(data, []byte("display-only")) {
		t.Fatalf("display annotation expanded strict owner metadata: %s", data)
	}
	totals, err := engine.Snapshot(t.Context(), root, "reservation")
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(totals)
	if err != nil || bytes.Contains(data, []byte(`"label"`)) || bytes.Contains(data, []byte("display-only")) || totals.SchemaVersion != 5 {
		t.Fatalf("display annotation expanded status schema: %s %v", data, err)
	}
}

func displayCensusRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "coordination.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := evidence.WriteDisplay(root, displayContractToken, displayContractLabel(t, "orphan-context")); err != nil {
		t.Fatal(err)
	}
	return root
}

func displayIdentityPair(t *testing.T, root string, held bool) {
	t.Helper()
	directory := filepath.Join(root, "reservation-identities")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, displayContractToken+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeError := file.Close(); closeError != nil {
			t.Error(closeError)
		}
	})
	if err = os.Link(path, filepath.Join(directory, displayContractToken+".anchor")); err != nil {
		t.Fatal(err)
	}
	if held {
		if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRuntimeDisplayIdleCensusReachesOldRetirementOrphanWithoutEpoch(t *testing.T) {
	root := t.TempDir()
	config := displayRegistrationConfig(root, true)
	engine := runtimeadapter.NewEngine()
	owner := displayRegister(t, engine, config)
	if err := engine.Release(config, owner.Lease); err != nil {
		t.Fatal(err)
	}
	// Simulate the sidecar an old binary ignored while retiring the last
	// reservation. Actual old-binary interoperability is a separate E2E check.
	if err := evidence.WriteDisplay(root, owner.Lease.Token, displayContractLabel(t, "old-retirement")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"coordination-mode.json", "reservations.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("last-owner retirement fixture retained %s: %v", name, err)
		}
	}
	totals, err := engine.Snapshot(t.Context(), root, "reservation")
	if err != nil || totals.ActiveOwners != 0 || totals.WaitingOwners != 0 || totals.Capacity.CPU != 0 {
		t.Fatalf("idle housekeeping invented accounting: %+v %v", totals, err)
	}
	if annotation := displayContractRead(t, root, owner.Lease.Token); annotation.Availability != application.DisplayMissing {
		t.Fatalf("missing-marker early return left old retirement orphan: %+v", annotation)
	}
	for _, name := range []string{"coordination-mode.json", "reservations.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("annotation housekeeping created an epoch %s: %v", name, err)
		}
	}
}

func TestRuntimeDisplayCensusRequiresUnheldCompleteIdentity(t *testing.T) {
	for _, state := range []string{"absent", "unheld", "held", "partial", "replaced"} {
		t.Run(state, func(t *testing.T) {
			root := displayCensusRoot(t)
			if state != "absent" {
				displayIdentityPair(t, root, state == "held")
			}
			anchor := filepath.Join(root, "reservation-identities", displayContractToken+".anchor")
			if state == "partial" || state == "replaced" {
				if err := os.Remove(anchor); err != nil {
					t.Fatal(err)
				}
			}
			if state == "replaced" {
				if err := os.WriteFile(anchor, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := runtimeadapter.NewEngine().Snapshot(t.Context(), root, "exclusive"); err != nil {
				t.Fatal(err)
			}
			want := application.DisplayAvailable
			if state == "absent" || state == "unheld" {
				want = application.DisplayMissing
			}
			if annotation := displayContractRead(t, root, displayContractToken); annotation.Availability != want {
				t.Fatalf("identity %s does not prove correct orphan fate: %+v want=%s", state, annotation, want)
			}
		})
	}
}

func TestRuntimeDisplayOrphanPruningRequiresSupportedAnnotation(t *testing.T) {
	valid := `{"schemaVersion":1,"runId":"` + displayContractToken + `","label":"orphan-context"}`
	cases := []struct {
		name, data string
		retain     bool
	}{
		{"supported", valid, false},
		{"future-schema", strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":2`, 1), true},
		{"malformed", "{", true},
		{"mismatched-run-id", strings.Replace(valid, displayContractToken, strings.Repeat("a", 32), 1), true},
		{"invalid-label", strings.Replace(valid, "orphan-context", "../private", 1), true},
		{"oversize", valid + strings.Repeat(" ", 513-len(valid)), true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := displayCensusRoot(t)
			path := filepath.Join(root, "run-display", displayContractToken+".json")
			if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := runtimeadapter.NewEngine().Snapshot(t.Context(), root, "exclusive"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if test.retain {
				if err != nil || string(data) != test.data {
					t.Fatalf("unverifiable annotation was pruned or changed: %v", err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("supported proven-stale annotation retained: %v", err)
			}
		})
	}
}

func TestRuntimeDisplayOwnedRetirementRemovesUnverifiableOwnedAnnotation(t *testing.T) {
	for _, reservation := range []bool{false, true} {
		root := t.TempDir()
		config := displayRegistrationConfig(root, reservation)
		engine := runtimeadapter.NewEngine()
		owner := displayRegister(t, engine, config)
		path := filepath.Join(root, "run-display", owner.Lease.Token+".json")
		if err := os.WriteFile(path, []byte(`{"schemaVersion":999}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := engine.Release(config, owner.Lease); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned retirement depended on annotation readability: %v", err)
		}
	}
}

func TestRuntimeDisplayOrphanPruningRetainsUnreadableAnnotation(t *testing.T) {
	root := displayCensusRoot(t)
	path := filepath.Join(root, "run-display", displayContractToken+".json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeadapter.NewEngine().Snapshot(t.Context(), root, "exclusive"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("unreadable sidecar was removed or replaced: %v", err)
	}
	if annotation := displayContractRead(t, root, displayContractToken); annotation.Availability != application.DisplayUnreadable {
		t.Fatalf("unreadable orphan lost safe availability: %+v", annotation)
	}
}

func TestRuntimeDisplayUnknownRegistryBlocksOrphanPruning(t *testing.T) {
	for _, state := range []string{"unsupported-ledger", "corrupt-ledger", "unsupported-session", "malformed-session", "session-link", "identity-link", "unsupported-marker", "unknown-heavy"} {
		t.Run(state, func(t *testing.T) {
			root := displayCensusRoot(t)
			setupUnknownDisplayRegistry(t, root, state)
			_, _ = runtimeadapter.NewEngine().Snapshot(t.Context(), root, "exclusive")
			if annotation := displayContractRead(t, root, displayContractToken); annotation.Availability != application.DisplayAvailable {
				t.Fatalf("unknown registry %s erased unproven orphan: %+v", state, annotation)
			}
		})
	}
}

func setupUnknownDisplayRegistry(t *testing.T, root, state string) {
	t.Helper()
	path, data := "reservations.json", `{"schemaVersion":999}`
	switch state {
	case "unsupported-ledger":
	case "corrupt-ledger":
		data = "{"
	case "unsupported-session", "malformed-session", "session-link":
		if err := os.Mkdir(filepath.Join(root, "sessions"), 0o700); err != nil {
			t.Fatal(err)
		}
		path = filepath.Join("sessions", strings.Repeat("a", 32)+".json")
		if state == "malformed-session" {
			data = "{"
		}
	case "identity-link":
		if err := os.Mkdir(filepath.Join(root, "reservation-identities"), 0o700); err != nil {
			t.Fatal(err)
		}
		path = filepath.Join("reservation-identities", displayContractToken+".lock")
	case "unsupported-marker":
		path = "coordination-mode.json"
		data = `{"schemaVersion":999,"mode":"future"}`
	case "unknown-heavy":
		if err := os.Mkdir(filepath.Join(root, "heavy.lock"), 0o700); err != nil {
			t.Fatal(err)
		}
		path = filepath.Join("heavy.lock", "unknown")
	}
	if state == "session-link" || state == "identity-link" {
		if err := os.Symlink(filepath.Join(root, "coordination.lock"), filepath.Join(root, path)); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeDisplayObservationDoesNotInitializeAbsentRoots(t *testing.T) {
	parent := t.TempDir()
	for _, root := range []string{parent, filepath.Join(parent, "absent")} {
		if _, err := runtimeadapter.NewEngine().Snapshot(t.Context(), root, "reservation"); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("ordinary absent observation created state just to prune: %v %v", entries, err)
	}
}

func TestRuntimeDisplayFailedEnqueueRetainsHeldOrphanUntilOwnedClose(t *testing.T) {
	root := t.TempDir()
	config := displayRegistrationConfig(root, true)
	owner := displayRegister(t, runtimeadapter.NewEngine(), config)
	path := filepath.Join(root, "reservations.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ledger coordination.Ledger
	if err = json.Unmarshal(data, &ledger); err != nil {
		t.Fatal(err)
	}
	ledger.NextSequence = ^uint64(0)
	data, err = json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	engine := runtimeadapter.NewEngine()
	admission, err := engine.BeginAdmission(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admission.Close)
	if _, err = admission.Attempt(t.Context(), time.Second, time.Now()); !errors.Is(err, coordination.ErrReservationDeferred) {
		t.Fatalf("exhausted sequence fixture did not refuse enqueue: %v", err)
	}
	tokens, err := evidence.DisplayTokens(root)
	if err != nil || len(tokens) != 2 {
		t.Fatalf("failed enqueue did not retain held unpublished token: %v %v", tokens, err)
	}
	orphan := tokens[0]
	if orphan == owner.Lease.Token {
		orphan = tokens[1]
	}
	if _, err = engine.Snapshot(t.Context(), root, "reservation"); err != nil {
		t.Fatal(err)
	}
	if annotation := displayContractRead(t, root, orphan); annotation.Availability != application.DisplayAvailable {
		t.Fatalf("ledger absence erased still-held prepublication identity: %+v", annotation)
	}
	admission.Close()
	if annotation := displayContractRead(t, root, orphan); annotation.Availability != application.DisplayMissing {
		t.Fatalf("owned failed enqueue did not retire its annotation: %+v", annotation)
	}
}
