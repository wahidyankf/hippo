package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	evidenceadapter "github.com/wahidyankf/hippo/internal/adapters/evidence"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/support/runtimewiring"
)

const (
	publicTUIHostVariant      = "host"
	publicTUILegacyVariant    = "legacy"
	publicTUIMixedVariant     = "mixed"
	publicTUIMixedV085Variant = "mixed-v085"
	publicTUIMixedV084Variant = "mixed-v084"
)

// PrepareObservation changes only authentic owned input fixtures. It neither
// supplies a snapshot nor reads the selected subject's observations.
func (fixture *PublicTUILedger) PrepareObservation(ctx context.Context, variant, fault string) error {
	switch variant {
	case "known", publicTUIHostVariant, "filtered", "mode-disagreement", tuiIdleState, exclusiveMode, publicTUILegacyVariant, "identity", tuiAnnotationFaultVariant, publicTUIMixedVariant, publicTUIMixedV085Variant, publicTUIMixedV084Variant, "three", "one", tuiEmptyVariant:
	default:
		return errors.New("unknown original public observation fixture")
	}
	if err := fixture.initializeObservation(variant); err != nil {
		return err
	}
	fixture.expected = coordination.ReservationTotals{SchemaVersion: 5, Mode: reservationMode}
	if variant == tuiIdleState || variant == publicTUIMixedVariant || variant == publicTUIMixedV085Variant || variant == publicTUIMixedV084Variant || variant == tuiEmptyVariant {
		return nil
	}
	if variant == exclusiveMode {
		return fixture.prepareExclusiveObservation(ctx)
	}
	ownerCount, waiterCount := 2, 3
	if variant == "three" {
		ownerCount, waiterCount = 3, 1
	}
	if variant == "one" || variant == publicTUILegacyVariant || variant == tuiAnnotationFaultVariant {
		ownerCount, waiterCount = 1, 0
	}
	vector := coordination.ReservationVector{CPU: 2, MemoryBytes: policy.GiB}
	fixture.expected.ActiveOwners, fixture.expected.WaitingOwners, fixture.expected.Ephemeral = ownerCount, waiterCount, ownerCount
	fixture.expected.Capacity = coordination.ReservationVector{CPU: 8, MemoryBytes: 8 * policy.GiB}
	fixture.expected.Allocated = coordination.ReservationVector{CPU: 2 * ownerCount, MemoryBytes: int64(ownerCount) * policy.GiB}
	fixture.expected.Waiting = coordination.ReservationVector{CPU: 2 * waiterCount, MemoryBytes: int64(waiterCount) * policy.GiB}
	plan := coordination.ReservationPlan{Capacity: fixture.expected.Capacity, Requested: vector, Allocated: vector}
	for range ownerCount {
		if err := fixture.prepareObservationOwner(ctx, variant, plan, ownerCount, vector); err != nil {
			return err
		}
	}
	for index := range waiterCount {
		if err := fixture.prepareWaiter(ctx, plan, index+1); err != nil {
			return err
		}
	}
	for _, rows := range [][]coordination.ReservationEntry{fixture.expected.Owners, fixture.expected.Waiters} {
		for _, row := range rows {
			if err := evidenceadapter.RemoveDisplay(fixture.root, row.RunID); err != nil {
				return fmt.Errorf("remove owned display %s: %w", row.RunID, err)
			}
		}
	}
	if err := fixture.prepareObservationMetadata(variant, fault); err != nil {
		return err
	}
	return fixture.VerifyLiveness(ctx)
}

func (fixture *PublicTUILedger) ownerPID(runID string) int {
	if pid, present := fixture.foreignOwners[runID]; present {
		return pid
	}
	return os.Getpid()
}

// Labels returns an owned copy of the advisory labels.
func (fixture *PublicTUILedger) Labels() map[string]string {
	result := make(map[string]string, len(fixture.labels))
	maps.Copy(result, fixture.labels)
	return result
}

func (fixture *PublicTUILedger) writeLabel(runID, value string) error {
	label, err := identity.ParseDisplayLabel(value)
	if err != nil {
		return err
	}
	if err := evidenceadapter.WriteDisplay(fixture.root, runID, label); err != nil {
		return err
	}
	fixture.labels[runID] = value
	return nil
}

// This reproduces the owned native rekey input operation, retaining the acquired
// identity inode/anchor and changing the eventual release token together.
func (fixture *PublicTUILedger) rekeyOwner(session *runtimeadapter.Session, runID string) error {
	old := session.Token
	data, err := os.ReadFile(filepath.Join(fixture.root, "reservations.json"))
	if err != nil {
		return err
	}
	var ledger coordination.Ledger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return err
	}
	found := false
	for index := range ledger.Owners {
		if ledger.Owners[index].Token == old {
			ledger.Owners[index].Token = runID
			found = true
		}
	}
	if !found {
		return errors.New("rekey target is not this held owner")
	}
	if err := coordination.ValidateLedger(ledger); err != nil {
		return err
	}
	for _, suffix := range []string{".lock", ".anchor"} {
		if err := os.Rename(filepath.Join(fixture.root, "reservation-identities", old+suffix), filepath.Join(fixture.root, "reservation-identities", runID+suffix)); err != nil {
			return err
		}
	}
	owned, err := os.OpenRoot(fixture.root)
	if err != nil {
		return err
	}
	path := filepath.Join("owner-metadata", old+".json")
	data, readError := owned.ReadFile(path)
	if readError == nil {
		writeError := owned.WriteFile(filepath.Join("owner-metadata", runID+".json"), []byte(strings.ReplaceAll(string(data), old, runID)), 0o600)
		if writeError != nil {
			return errors.Join(writeError, owned.Close())
		}
		if err := owned.Remove(path); err != nil {
			return errors.Join(err, owned.Close())
		}
	} else if !errors.Is(readError, os.ErrNotExist) {
		return errors.Join(readError, owned.Close())
	}
	data, err = json.Marshal(ledger)
	if err != nil {
		return errors.Join(err, owned.Close())
	}
	if err := owned.WriteFile("reservations.json", data, 0o600); err != nil {
		return errors.Join(err, owned.Close())
	}
	session.Token = runID
	return owned.Close()
}

func (fixture *PublicTUILedger) writeAnnotationFault(kind, runID string) error {
	directory := filepath.Join(fixture.root, "run-display")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	path := filepath.Join(directory, runID+".json")
	switch kind {
	case annotationOperationMissing:
		return nil
	case "a symlink":
		if err := os.WriteFile(filepath.Join(directory, "private-target"), []byte("unchecked-payload"), 0o600); err != nil {
			return err
		}
		fixture.annotationFault = runID
		return os.Symlink(filepath.Join(directory, "private-target"), path)
	case tuiUnreadableAnnotation:
		fixture.annotationFault = runID
		return os.Mkdir(path, 0o700)
	case tuiMalformedAnnotation, tuiMismatchedRunID, tuiUnsupportedSchema, tuiOversizedAnnotation:
	default:
		return errors.New("unknown original annotation fault")
	}
	record := map[string]any{schemaVersionField: 1, tuiRunIDField: runID, tuiLabelField: "unchecked-payload"}
	if kind == tuiMismatchedRunID {
		record[tuiRunIDField] = strings.Repeat("f", 32)
	}
	if kind == tuiUnsupportedSchema {
		record["schemaVersion"] = 99
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if kind == tuiMalformedAnnotation {
		data = []byte(`{"label":"unchecked-payload",`)
	}
	if kind == tuiOversizedAnnotation {
		data = append(data, []byte(strings.Repeat(" ", 513))...)
	}
	return os.WriteFile(path, data, 0o600)
}

func (fixture *PublicTUILedger) removeOwnedAnnotationFault() error {
	if fixture.annotationFault == "" {
		return nil
	}
	owned, err := os.OpenRoot(fixture.root)
	if err != nil {
		return err
	}
	err = owned.Remove(filepath.Join("run-display", fixture.annotationFault+".json"))
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err == nil {
		fixture.annotationFault = ""
	}
	return errors.Join(err, owned.Close())
}

// CoordinationTree is confined setup/source evidence, not a product snapshot.
func (fixture *PublicTUILedger) CoordinationTree() (map[string]string, error) {
	owned, err := os.OpenRoot(fixture.root)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	err = fs.WalkDir(owned.FS(), ".", func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if entry.IsDir() {
			result[path] = "directory"
			return nil
		}
		data, err := owned.ReadFile(path)
		if err != nil {
			return err
		}
		result[path] = string(data)
		return nil
	})
	return result, errors.Join(err, owned.Close())
}

// RequireReadOnly checks that setup inputs retain their original bytes.
func (fixture *PublicTUILedger) RequireReadOnly(before map[string]string) error {
	after, err := fixture.CoordinationTree()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("public idle observation mutated coordination ownership")
	}
	return nil
}

// AdoptMixedOwners validates input created by two actual distinct public
// executables. It never launches an executable or supplies an observation.
// IDs/PIDs come from each command's owned readiness and actual Process receipt.
func (fixture *PublicTUILedger) AdoptMixedOwners(ctx context.Context, ids []string, pids []int) error {
	return fixture.adoptMixedOwners(ctx, ids, pids, 2, publicTUIMixedVariant)
}

// AdoptV084MixedOwners owns the public v0.8.4 compatibility route. The native
// mixed fixture keeps its separate two-CPU contract.
func (fixture *PublicTUILedger) AdoptV084MixedOwners(ctx context.Context, ids []string, pids []int) error {
	return fixture.adoptMixedOwners(ctx, ids, pids, 1, publicTUIMixedV084Variant)
}

// AdoptV085MixedOwners retains the real two-version compatibility fixture with
// one CPU per owner. The ordinary native/BDD mixed fixture remains two CPUs.
func (fixture *PublicTUILedger) AdoptV085MixedOwners(ctx context.Context, ids []string, pids []int) error {
	return fixture.adoptMixedOwners(ctx, ids, pids, 1, publicTUIMixedV085Variant)
}

func (fixture *PublicTUILedger) adoptMixedOwners(ctx context.Context, ids []string, pids []int, ownerCPU int, variant string) error {
	if fixture.variant != variant || len(ids) != 2 || len(pids) != 2 || ids[0] == ids[1] || pids[0] <= 0 || pids[1] <= 0 || pids[0] == pids[1] {
		return errors.New("mixed input requires two distinct real owned executables")
	}
	actual, err := runtimeadapter.NewEngine().Snapshot(ctx, fixture.root, reservationMode)
	if err != nil {
		return err
	}
	vector := coordination.ReservationVector{CPU: ownerCPU, MemoryBytes: policy.GiB}
	if err := validateMixedOwnerBudget(actual, runtime.NumCPU(), ownerCPU); err != nil {
		return err
	}
	for index, runID := range ids {
		found := matchesMixedPublicOwner(actual.Owners, runID, vector)
		if !found {
			return errors.New("mixed command readiness is not its real declared ledger owner")
		}
		fixture.foreignOwners[runID] = pids[index]
	}
	fixture.expected = actual
	if err := evidenceadapter.RemoveDisplay(fixture.root, ids[0]); err != nil {
		return err
	}
	fixture.labels[ids[1]] = "current-owner"
	// Both commands have already filled the real owner limit. Original three
	// waiters register through admission; the existing pause keeps their locks
	// live without churn and releases them only on owned cancellation.
	plan := coordination.ReservationPlan{Capacity: actual.Capacity, Requested: vector, Allocated: vector}
	for index := range 3 {
		if err := fixture.prepareWaiter(ctx, plan, index+1); err != nil {
			return err
		}
	}
	fixture.expected.WaitingOwners = 3
	fixture.expected.Waiting = coordination.ReservationVector{CPU: 3 * ownerCPU, MemoryBytes: 3 * policy.GiB}
	for _, row := range fixture.expected.Waiters {
		if err := evidenceadapter.RemoveDisplay(fixture.root, row.RunID); err != nil {
			return err
		}
	}
	return fixture.VerifyLiveness(ctx)
}

// validateMixedPublicOwnerBudget checks real owners against independently declared
// host/configuration inputs, before their snapshot is adopted as a preservation baseline.
func validateMixedPublicOwnerBudget(actual coordination.ReservationTotals, logicalCPUs int) error {
	return validateMixedOwnerBudget(actual, logicalCPUs, 2)
}

func validateMixedV084PublicOwnerBudget(actual coordination.ReservationTotals, logicalCPUs int) error {
	return validateMixedOwnerBudget(actual, logicalCPUs, 1)
}

func validateMixedV085PublicOwnerBudget(actual coordination.ReservationTotals, logicalCPUs int) error {
	return validateMixedOwnerBudget(actual, logicalCPUs, 1)
}

func validateMixedOwnerBudget(actual coordination.ReservationTotals, logicalCPUs, ownerCPU int) error {
	// Public reservation policy offers every logical CPU, lowered only by the
	// configured maxCpu, which this fixture's explicit configuration sets to eight.
	memoryCapacity := 8 * policy.GiB
	if ownerCPU == 1 {
		memoryCapacity = 3 * policy.GiB
	}
	capacity := coordination.ReservationVector{CPU: min(8, max(1, logicalCPUs)), MemoryBytes: memoryCapacity}
	allocated := coordination.ReservationVector{CPU: 2 * ownerCPU, MemoryBytes: 2 * policy.GiB}
	if capacity.CPU < allocated.CPU {
		return fmt.Errorf("mixed public owners require at least %d logical CPUs for two declared %d-CPU owners: host has %d", allocated.CPU, ownerCPU, logicalCPUs)
	}
	if actual.SchemaVersion != 5 || actual.Mode != reservationMode || actual.ActiveOwners != 2 || actual.WaitingOwners != 0 || actual.Capacity != capacity || actual.Allocated != allocated {
		return fmt.Errorf("real mixed commands changed declared input counts/budget: %+v", actual)
	}
	return nil
}

// RequireOwnerRetired verifies that the named fixture owner no longer exists.
func (fixture *PublicTUILedger) RequireOwnerRetired(ctx context.Context, runID string) error {
	actual, err := runtimeadapter.NewEngine().Snapshot(ctx, fixture.root, reservationMode)
	if err != nil {
		return err
	}
	for _, row := range actual.Owners {
		if row.RunID == runID {
			return errors.New("mixed public owner remained after actual Wait")
		}
	}
	return nil
}

// ProtocolSchemas returns the observed on-disk schema versions.
func (fixture *PublicTUILedger) ProtocolSchemas() (map[string]int, error) {
	owned, err := os.OpenRoot(fixture.root)
	if err != nil {
		return nil, err
	}
	result := map[string]int{}
	for _, path := range []string{coordinationModeMarker, "reservations.json"} {
		data, err := owned.ReadFile(path)
		if err != nil {
			return nil, errors.Join(err, owned.Close())
		}
		var record struct {
			SchemaVersion int `json:"schemaVersion"`
		}
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, errors.Join(err, owned.Close())
		}
		if record.SchemaVersion < 1 {
			return nil, errors.Join(errors.New("real mixed writer omitted coordination schema"), owned.Close())
		}
		result[path] = record.SchemaVersion
	}
	return result, owned.Close()
}

func (fixture *PublicTUILedger) initializeObservation(variant string) error {
	if fixture.prepared {
		return errors.New("public TUI ledger was already prepared")
	}
	fixture.prepared, fixture.variant = true, variant
	fixture.labels = map[string]string{}
	fixture.foreignOwners = map[string]int{}
	if err := publicTUIOwnedDirectory(fixture.directory); err != nil {
		return err
	}
	if err := publicTUIOwnedDirectory(fixture.root); err != nil {
		return err
	}
	entries, err := os.ReadDir(fixture.root)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("public TUI ledger refuses a nonempty setup root")
	}
	document := interruptionConfig
	if variant == exclusiveMode || variant == "mode-disagreement" {
		document = boundarySchemaOne
	}
	if variant == publicTUIMixedVariant || variant == publicTUIMixedV085Variant || variant == publicTUIMixedV084Variant {
		// Both released and current commands derive the same genuine epoch budget
		// from public configuration and real host evidence.
		memoryMiB := "8192"
		if variant == publicTUIMixedV085Variant || variant == publicTUIMixedV084Variant {
			memoryMiB = "3072"
		}
		document = strings.ReplaceAll(interruptionConfig, `"maxMemoryMiB": 16384`, `"maxMemoryMiB": `+memoryMiB)
		document = strings.ReplaceAll(document, `"maximumMemoryMiB": 16384`, `"maximumMemoryMiB": `+memoryMiB)
		document = strings.ReplaceAll(document, `"maxActiveOwners": 3`, `"maxActiveOwners": 2`)
		if variant == publicTUIMixedV085Variant || variant == publicTUIMixedV084Variant {
			// Schema three requires every tier to fit the pool, including unused
			// standard/heavy tiers. Light retains its declared one/two-GiB bounds.
			document = strings.ReplaceAll(document, `"maximumMemoryMiB": 6144`, `"maximumMemoryMiB": 3072`)
			document = strings.ReplaceAll(document, `"minimumMemoryMiB": 8192`, `"minimumMemoryMiB": 3072`)
		}
	}
	if err := os.WriteFile(fixture.configuration, []byte(document), 0o600); err != nil {
		return err
	}
	return nil
}

func (fixture *PublicTUILedger) prepareExclusiveObservation(ctx context.Context) error {
	fixture.expected.Mode = exclusiveMode
	session, err := runtimewiring.AcquireSession(ctx, fixture.root, "", policy.TaskEphemeral, 0)
	if session != nil {
		fixture.owners = append(fixture.owners, session)
	}
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("exclusive public fixture returned no held session")
	}
	fixture.expected.ActiveOwners, fixture.expected.Ephemeral, fixture.expected.LegacyEntries = 1, 1, 1
	fixture.expected.Owners = []coordination.ReservationEntry{{RunID: session.Token, State: tuiActiveState, Class: policy.TaskEphemeral, Legacy: true}}
	if err := evidenceadapter.RemoveDisplay(fixture.root, session.Token); err != nil {
		return err
	}
	return fixture.VerifyLiveness(ctx)
}

func (fixture *PublicTUILedger) prepareObservationOwner(ctx context.Context, variant string, plan coordination.ReservationPlan, ownerCount int, vector coordination.ReservationVector) error {
	metadata := coordination.ReservationMetadata{Source: tuiOwnerRole, Tags: map[string]string{tuiRoleTag: tuiOwnerRole}}
	if variant == publicTUIHostVariant {
		// Legal ordinary metadata makes real detail scrollable at 100x30:
		// four 101-cell tag rows wrap, while the identity stays below 512 bytes.
		for index := range 4 {
			metadata.Tags[fmt.Sprintf("detail-%02d-abcdefghijklmnopqrstu", index+1)] = strings.Repeat("x", 64)
		}
		for _, key := range []string{"aux-a", "aux-b", "aux-c"} {
			metadata.Tags[key] = "1"
		}
	}
	session, err := runtimewiring.AcquireReservationWithOptions(ctx, fixture.root, "", policy.TaskEphemeral, profileBalanced, "", plan, ownerCount, 0, runtimeadapter.ReservationAdmissionOptions{Metadata: metadata})
	if session != nil {
		fixture.owners = append(fixture.owners, session)
	}
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("public peer returned no held reservation")
	}
	fixture.expected.Owners = append(fixture.expected.Owners, coordination.ReservationEntry{RunID: session.Token, State: tuiActiveState, Class: policy.TaskEphemeral, Profile: profileBalanced, Source: metadata.Source, Tags: metadata.Tags, Requested: vector, Allocated: vector})
	return nil
}

func (fixture *PublicTUILedger) prepareObservationMetadata(variant, fault string) error {
	if variant == publicTUIHostVariant {
		for _, group := range []struct {
			name string
			rows []coordination.ReservationEntry
		}{{"running", fixture.expected.Owners}, {"queue", fixture.expected.Waiters}} {
			for index, row := range group.rows {
				if err := fixture.writeLabel(row.RunID, fmt.Sprintf("host-%s-%d", group.name, index+1)); err != nil {
					return err
				}
			}
		}
	}
	if variant == publicTUILegacyVariant {
		row := &fixture.expected.Owners[0]
		if err := os.Remove(filepath.Join(fixture.root, "owner-metadata", row.RunID+".json")); err != nil {
			return err
		}
		row.Source, row.Tags, row.Legacy = "", nil, true
		fixture.expected.LegacyEntries = 1
	}
	if variant == "identity" {
		for index, runID := range []string{"0123456789abcdef0000000000000001", "0123456789abcdef0000000000000002"} {
			if err := fixture.rekeyOwner(fixture.owners[index], runID); err != nil {
				return err
			}
			fixture.expected.Owners[index].RunID = runID
		}
		if err := fixture.writeLabel(fixture.expected.Owners[0].RunID, "exact-owner"); err != nil {
			return err
		}
		label, err := identity.ParseDisplayLabel("absent-owner")
		if err != nil {
			return err
		}
		if err := evidenceadapter.WriteDisplay(fixture.root, "0123456789abcdef0000000000000003", label); err != nil {
			return err
		}
	}
	if variant == tuiAnnotationFaultVariant {
		if err := fixture.writeAnnotationFault(fault, fixture.expected.Owners[0].RunID); err != nil {
			return err
		}
	}
	return nil
}

func matchesMixedPublicOwner(rows []coordination.ReservationEntry, runID string, vector coordination.ReservationVector) bool {
	found := false
	for _, row := range rows {
		if row.RunID == runID {
			found = row.Requested == vector && row.Allocated == vector && row.Class == policy.TaskEphemeral && row.Profile == profileBalanced
		}
	}
	return found
}
