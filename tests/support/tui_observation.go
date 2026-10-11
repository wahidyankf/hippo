package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	evidencedomain "github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// tuiObservationState records the outward observation capabilities. The real
// application workflow, filtering, freezing and legacy status/watch projection
// are the Unit subjects; this state never implements their decisions.
const (
	tuiCollectCall           = "collect"
	tuiRoleTag               = "role"
	tuiMatchingSource        = "matching"
	tuiMalformedAnnotation   = "malformed"
	tuiSymlinkAnnotation     = "a symlink"
	tuiAnnotationReadCall    = "annotations"
	tuiNativeProfileFlag     = "--profile"
	tuiNativeLabelFlag       = "--label"
	tuiConfigurationReadCall = "load"
	tuiRootReadCall          = "root"
	tuiRuntimeSnapshotCall   = "snapshot"
	tuiClockWaitCall         = "wait"
	tuiHistoryReadCall       = "history"
	tuiChangedSourceValue    = "changed"
)

type tuiObservationState struct {
	native                               *tuiNativeObservation
	sample                               policy.Sample
	totals                               coordination.ReservationTotals
	annotations                          map[string]application.DisplayAnnotation
	annotationError                      error
	annotationState                      string
	configurationMode                    string
	request                              application.StatusRequest
	snapshot, original                   application.ObservationSnapshot
	calls                                []string
	collectionTimes                      []time.Time
	now                                  time.Time
	waits, observations, annotationReads int
	watchCancel                          context.CancelFunc
	watchWaitBase                        int
	watchProjected                       int
	statusJSON                           []byte
	watchBodies                          [][]byte
}

func (driver *Driver) tuiObservationBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^two running jobs and three registered FIFO waiters with known reservation vectors$`, driver.prepareTUIKnownRows),
		step(`^one TUI observation completes$`, driver.observeTUI),
		step(`^both running identities and all three original FIFO positions retain their resource values$`, driver.requireTUIRows),
		step(`^two running jobs and three FIFO waiters with only the third waiter matching source and tag filters$`, driver.prepareTUIFilteredRows),
		step(`^the filtered TUI observation completes$`, driver.observeTUI),
		step(`^only the matching row is visible at position 3 and global capacity and usage remain unfiltered$`, driver.requireTUIFilteredRows),
		step(`^a host collector with two readings and a shared root whose active mode differs from configuration$`, driver.prepareTUIModeDifference),
		step(`^two host readings one second apart and the active coordination mode determine the displayed snapshot$`, driver.requireTUIObservationWorkflow),
		step(`^an empty reservation root with no active epoch$`, driver.prepareTUIIdleRoot),
		step(`^Running and Queue are empty with zero known demand and unavailable epoch capacity$`, driver.requireTUIIdleSnapshot),
		step(`^the observation creates no owner waiter annotation or coordination epoch$`, driver.requireTUIObservationReadOnly),
		step(`^an exclusive compatibility root with a live owner and no registered FIFO queue$`, driver.prepareTUIExclusiveRoot),
		step(`^the owner is visible and reservation vectors and unregistered waiting demand are unavailable$`, driver.requireTUIExclusiveSnapshot),
		step(`^a reservation owner whose optional metadata and display annotation are missing$`, driver.prepareTUILegacyRow),
		step(`^its live run ID and ledger allocation remain known while its label is unavailable$`, driver.requireTUILegacyRow),
		step(`^a completed TUI observation with host metrics row tags and ordered jobs$`, driver.prepareTUICompletedObservation),
		step(`^the collector and coordination provider reuse and change their previous values$`, driver.mutateTUIObservationSources),
		step(`^the completed observation retains its original values and independently owned row tags$`, driver.requireTUIFrozenObservation),
		step(`^live job IDs sharing a short prefix and annotations for one live ID and one absent ID$`, driver.prepareTUIIdentityJoin),
		step(`^only the exact matching live ID receives its label without adding or reordering jobs$`, driver.requireTUIIdentityJoin),
		step(`^a live job whose display annotation is (missing|malformed|mismatched run ID|unsupported schema|larger than 512 bytes|unreadable|a symlink)$`, driver.prepareTUIUnavailableAnnotation),
		step(`^its label is unavailable and the live identity vectors and global accounting remain unchanged$`, driver.requireTUIUnavailableAnnotation),
		step(`^the annotation advisory exposes no unchecked payload or private path$`, driver.requireTUIPrivateAdvisory),
		step(`^old and current owners sharing unchanged coordination schemas$`, driver.prepareTUIMixedRows),
		step(`^current observation reads the owners and their queued work$`, driver.observeTUI),
		step(`^owners without display annotations remain visible with unavailable labels$`, driver.requireTUIMixedLabels),
		step(`^admission accounting and FIFO ordering remain unchanged$`, driver.requireTUIRows),
		step(`^stable observation evidence whose display annotation changes between requests$`, driver.prepareTUIStatusCompatibility),
		step(`^JSON status and watch inspect that evidence$`, driver.inspectTUIStatusCompatibility),
		step(`^status retains schema version 5 without display annotation fields$`, driver.requireTUIStatusCompatibility),
		step(`^watch emits only the first unchanged schema five snapshot$`, driver.requireTUIWatchCompatibility),
	}
}

func (driver *Driver) prepareTUIKnownRows() error {
	if driver.mode == contract.E2E {
		return errors.New("public observation requires owned command/frame fixture")
	}
	now := time.Unix(100, 0).UTC()
	state := &tuiObservationState{now: now, configurationMode: reservationMode, annotations: map[string]application.DisplayAnnotation{}}
	state.sample = healthySample(now)
	state.sample.MeasuredAt = now.Format(time.RFC3339Nano)
	vector := coordination.ReservationVector{CPU: 2, MemoryBytes: policy.GiB}
	state.totals = coordination.ReservationTotals{SchemaVersion: 5, Mode: reservationMode, ActiveOwners: 2, WaitingOwners: 3, Capacity: coordination.ReservationVector{CPU: 8, MemoryBytes: 8 * policy.GiB}, Allocated: coordination.ReservationVector{CPU: 4, MemoryBytes: 2 * policy.GiB}, Waiting: coordination.ReservationVector{CPU: 6, MemoryBytes: 3 * policy.GiB}}
	for i := range 2 {
		state.totals.Owners = append(state.totals.Owners, coordination.ReservationEntry{RunID: fmt.Sprintf("%032x", i+1), Class: policy.TaskEphemeral, Source: tuiOwnerRole, Allocated: vector, Requested: vector, Tags: map[string]string{tuiRoleTag: tuiOwnerRole}})
	}
	for i := range 3 {
		state.totals.Waiters = append(state.totals.Waiters, coordination.ReservationEntry{RunID: fmt.Sprintf("%032x", i+10), Class: policy.TaskEphemeral, Source: tuiOtherRole, Position: i + 1, Requested: vector, Tags: map[string]string{tuiRoleTag: tuiOtherRole}})
	}
	driver.tuiObservation = state
	return nil
}

func (state *tuiObservationState) Load(path string, environment map[string]string) (application.Configuration, error) {
	state.calls = append(state.calls, tuiConfigurationReadCall)
	if state.native != nil {
		return state.native.configuration.Load(path, environment)
	}
	return application.Configuration{Catalog: policy.BuiltinCatalog(), Coordination: coordination.Configuration{SchemaVersion: 3, Mode: state.configurationMode, BaseActiveOwners: 1, MaxActiveOwners: 2}, Hash: "stable-observation"}, nil
}

func (*tuiObservationState) Identity(map[string]string, string, string, []string) (identity.Value, string, error) {
	return identity.Value{}, "", nil
}

func (state *tuiObservationState) StateRoot(map[string]string) string {
	state.calls = append(state.calls, tuiRootReadCall)
	if state.native != nil {
		return state.native.root
	}
	return "memory-observation"
}
func (*tuiObservationState) AbsolutePath(path string) (string, error) { return path, nil }
func (state *tuiObservationState) Collect(context.Context, policy.CPUState, string) (policy.Reading, error) {
	state.calls = append(state.calls, tuiCollectCall)
	state.collectionTimes = append(state.collectionTimes, state.now)
	return policy.Reading{Sample: state.sample, CPUState: policy.CPUState{1}}, nil
}

func (state *tuiObservationState) Snapshot(ctx context.Context, root, mode string) (coordination.ReservationTotals, error) {
	state.calls = append(state.calls, tuiRuntimeSnapshotCall)
	state.observations++
	if state.native != nil {
		return state.native.runtime.Snapshot(ctx, root, mode)
	}
	return state.totals, nil
}

func (state *tuiObservationState) History(root string, query evidencedomain.Query) ([]evidencedomain.Summary, error) {
	state.calls = append(state.calls, tuiHistoryReadCall)
	if state.native != nil {
		return state.native.history.History(root, query)
	}
	return nil, nil
}
func (state *tuiObservationState) Now() time.Time { return state.now }
func (state *tuiObservationState) Wait(ctx context.Context, duration time.Duration) error {
	state.calls = append(state.calls, tuiClockWaitCall)
	state.now = state.now.Add(duration)
	state.waits++
	if state.watchCancel != nil && state.waits-state.watchWaitBase == 4 {
		state.watchCancel()
	}
	return ctx.Err()
}

func (*tuiObservationState) Ticker(time.Duration) application.ReleaseTicker {
	return application.ReleaseTicker{Ticks: make(chan time.Time), Stop: func() {}}
}

func (state *tuiObservationState) ReadDisplay(ctx context.Context, root string, ids []string) (map[string]application.DisplayAnnotation, error) {
	state.calls = append(state.calls, tuiAnnotationReadCall)
	state.annotationReads++
	if state.native != nil {
		return state.readNativeDisplay(ctx, root, ids)
	}
	return state.annotations, state.annotationError
}

func (state *tuiObservationState) services() application.ObservationServices {
	return application.ObservationServices{Configuration: state, Collector: state, Runtime: state, Evidence: state, Clock: state, Annotations: state}
}

func (driver *Driver) observeTUI(ctx context.Context) error {
	state := driver.tuiObservation
	if state == nil {
		return errors.New("TUI observation was not prepared")
	}
	if driver.mode == contract.Integration && state.native == nil {
		if err := driver.prepareTUINativeObservation(ctx); err != nil {
			return err
		}
	}
	request, err := application.PrepareObservation(state.request)
	if err != nil {
		return err
	}
	snapshot, code, err := state.services().ObserveSnapshot(ctx, request)
	if err != nil || code != 0 {
		return fmt.Errorf("observation failed code=%d: %w", code, err)
	}
	state.snapshot = snapshot
	return nil
}

// TUIObservationWithSampleTime lets the package-local model component consume
// the real application projection, including malformed timestamp handling.
func (driver *Driver) TUIObservationWithSampleTime(ctx context.Context, value string) (application.ObservationSnapshot, error) {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return application.ObservationSnapshot{}, err
	}
	state := driver.tuiObservation
	switch value {
	case "absent":
		state.sample.MeasuredAt = ""
	case tuiMalformedAnnotation:
		state.sample.MeasuredAt = "not-a-time"
	default:
		return application.ObservationSnapshot{}, fmt.Errorf("unknown canonical sample time %q", value)
	}
	if err := driver.observeTUI(ctx); err != nil {
		return application.ObservationSnapshot{}, err
	}
	if state.snapshot.Status.MeasuredAt != state.sample.MeasuredAt {
		return application.ObservationSnapshot{}, errors.New("application replaced the supplied measurement time")
	}
	return state.snapshot, nil
}

func (driver *Driver) requireTUIRows() error {
	state := driver.tuiObservation
	if state == nil {
		return errors.New("no observation")
	}
	snapshot := state.snapshot
	if len(snapshot.Running) != 2 || len(snapshot.Queue) != 3 {
		return errors.New("observation lost running or FIFO rows")
	}
	for i, row := range snapshot.Running {
		want := state.totals.Owners[i]
		if !reflect.DeepEqual(row.Entry, want) || !row.AllocatedKnown || !row.RequestedKnown {
			return fmt.Errorf("owner %d lost identity/allocation", i)
		}
	}
	for i, row := range snapshot.Queue {
		want := state.totals.Waiters[i]
		if !reflect.DeepEqual(row.Entry, want) || row.Entry.Position != i+1 || !row.RequestedKnown {
			return fmt.Errorf("waiter %d lost identity/position/demand", i)
		}
	}
	return driver.requireTUIGlobalTotals()
}

func (driver *Driver) requireTUIGlobalTotals() error {
	state := driver.tuiObservation
	got, want := state.snapshot.Status.Coordination, state.totals
	if got.ActiveOwners != want.ActiveOwners || got.WaitingOwners != want.WaitingOwners || got.Capacity != want.Capacity || got.Allocated != want.Allocated || got.Waiting != want.Waiting {
		return errors.New("observation changed global capacity, allocation or waiting demand")
	}
	return nil
}

func (driver *Driver) prepareTUIFilteredRows() error {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return err
	}
	state := driver.tuiObservation
	state.totals.Waiters[2].Source = tuiMatchingSource
	state.totals.Waiters[2].Tags = map[string]string{tuiRoleTag: tuiMatchingSource}
	state.request = application.StatusRequest{Source: tuiMatchingSource, Tags: []string{"role=matching"}}
	return nil
}

func (driver *Driver) requireTUIFilteredRows() error {
	state := driver.tuiObservation
	if len(state.snapshot.Running) != 0 || len(state.snapshot.Queue) != 1 || state.snapshot.Queue[0].Entry.Position != 3 || !reflect.DeepEqual(state.snapshot.Queue[0].Entry, state.totals.Waiters[2]) {
		return errors.New("filter invented queue position or retained unrelated rows")
	}
	return driver.requireTUIGlobalTotals()
}

func (driver *Driver) prepareTUIModeDifference() error {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return err
	}
	driver.tuiObservation.configurationMode = exclusiveMode
	return nil
}

func (driver *Driver) requireTUIObservationWorkflow() error {
	state := driver.tuiObservation
	want := []string{tuiConfigurationReadCall, tuiCollectCall, tuiClockWaitCall, tuiCollectCall, tuiRootReadCall, tuiHistoryReadCall, tuiRuntimeSnapshotCall, tuiAnnotationReadCall}
	if state.native != nil && state.configurationMode == exclusiveMode {
		configuration, err := state.native.configuration.Load(state.native.config, nil)
		if err != nil {
			return err
		}
		if configuration.Coordination.Mode == state.snapshot.Status.Coordination.Mode {
			return errors.New("native fixture did not exercise active/configuration mode disagreement")
		}
		want = []string{tuiConfigurationReadCall, tuiCollectCall, tuiClockWaitCall, tuiCollectCall, tuiRootReadCall, tuiRuntimeSnapshotCall, tuiAnnotationReadCall}
	}
	if !reflect.DeepEqual(state.calls, want) || len(state.collectionTimes) != 2 || state.collectionTimes[1].Sub(state.collectionTimes[0]) != time.Second || state.snapshot.Status.Coordination.Mode != reservationMode || !state.snapshot.QueueRegistered {
		return fmt.Errorf("shared observation workflow changed: calls=%v times=%v mode=%s", state.calls, state.collectionTimes, state.snapshot.Status.Coordination.Mode)
	}
	return driver.requireTUIRows()
}

func (driver *Driver) prepareTUIIdleRoot() error {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return err
	}
	driver.tuiObservation.totals = coordination.ReservationTotals{Mode: reservationMode, SchemaVersion: 5}
	return nil
}

func (driver *Driver) requireTUIIdleSnapshot() error {
	state := driver.tuiObservation.snapshot
	if len(state.Running) != 0 || len(state.Queue) != 0 || !state.UsageKnown || !state.QueueRegistered || state.CapacityKnown || state.Status.Coordination.Allocated != (coordination.ReservationVector{}) || state.Status.Coordination.Waiting != (coordination.ReservationVector{}) {
		return errors.New("idle observation invented epoch capacity or unknown zero demand")
	}
	return nil
}

func (driver *Driver) requireTUIObservationReadOnly() error {
	if driver.mode == contract.Integration {
		return driver.requireTUINativeReadOnly()
	}
	if err := driver.requireTUIIdleSnapshot(); err != nil {
		return err
	}
	want := []string{tuiConfigurationReadCall, tuiCollectCall, tuiClockWaitCall, tuiCollectCall, tuiRootReadCall, tuiHistoryReadCall, tuiRuntimeSnapshotCall, tuiAnnotationReadCall}
	if !reflect.DeepEqual(driver.tuiObservation.calls, want) {
		return errors.New("observation requested an outward ownership mutation")
	}
	return nil
}

func (driver *Driver) prepareTUIExclusiveRoot() error {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return err
	}
	state := driver.tuiObservation
	state.configurationMode = exclusiveMode
	state.totals.Mode = exclusiveMode
	state.totals.Owners = state.totals.Owners[:1]
	state.totals.ActiveOwners = 1
	state.totals.Waiters = nil
	state.totals.WaitingOwners = 0
	return nil
}

func (driver *Driver) requireTUIExclusiveSnapshot() error {
	state := driver.tuiObservation.snapshot
	if len(state.Running) != 1 || len(state.Queue) != 0 || state.QueueRegistered || state.CapacityKnown || state.UsageKnown || state.Running[0].AllocatedKnown || state.Running[0].RequestedKnown {
		return errors.New("exclusive view invented registered FIFO or reservation vectors")
	}
	return nil
}

func (driver *Driver) prepareTUILegacyRow() error {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return err
	}
	state := driver.tuiObservation
	state.totals.Owners = state.totals.Owners[:1]
	state.totals.Owners[0].Source = ""
	state.totals.Owners[0].Tags = nil
	state.totals.Owners[0].Legacy = true
	return nil
}

func (driver *Driver) requireTUILegacyRow() error {
	state := driver.tuiObservation
	if len(state.snapshot.Running) != 1 {
		return errors.New("legacy owner lost")
	}
	row := state.snapshot.Running[0]
	if row.Entry.RunID != state.totals.Owners[0].RunID || row.Entry.Allocated != state.totals.Owners[0].Allocated || !row.AllocatedKnown || row.Label.Availability == application.DisplayAvailable || row.Label.Label.String() != "" {
		return errors.New("legacy row lost allocation or invented label")
	}
	return nil
}

func (driver *Driver) prepareTUICompletedObservation(ctx context.Context) error {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return err
	}
	if err := driver.observeTUI(ctx); err != nil {
		return err
	}
	data, err := json.Marshal(driver.tuiObservation.snapshot)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &driver.tuiObservation.original)
}

func (driver *Driver) mutateTUIObservationSources() error {
	state := driver.tuiObservation
	if state.native != nil {
		if err := state.native.mutateSources(); err != nil {
			return err
		}
		actual, err := state.native.runtime.Snapshot(context.Background(), state.native.root, reservationMode)
		if err != nil {
			return err
		}
		if len(actual.Owners) != 2 || actual.Owners[0].RunID == state.original.Running[0].Entry.RunID || actual.Owners[0].Tags[tuiRoleTag] != tuiChangedSourceValue || len(actual.Waiters) != 3 || actual.Waiters[0].RunID != state.original.Queue[2].Entry.RunID {
			return errors.New("native source mutation did not change actual owner metadata and FIFO evidence")
		}
	}
	if state.sample.AvailableMemoryBytes != nil {
		*state.sample.AvailableMemoryBytes = 1
	}
	if state.sample.CPUUtilizationPercent != nil {
		*state.sample.CPUUtilizationPercent = 99
	}
	state.sample.Capabilities = append(state.sample.Capabilities, tuiChangedSourceValue)
	state.totals.Owners[0].Tags[tuiRoleTag] = tuiChangedSourceValue
	state.totals.Owners[0].RunID = tuiChangedSourceValue
	state.totals.Waiters[0], state.totals.Waiters[2] = state.totals.Waiters[2], state.totals.Waiters[0]
	return nil
}

func (driver *Driver) requireTUIFrozenObservation() error {
	state := driver.tuiObservation
	got, err := json.Marshal(state.snapshot)
	if err != nil {
		return err
	}
	want, err := json.Marshal(state.original)
	if err != nil {
		return err
	}
	if string(got) != string(want) {
		return errors.New("completed snapshot aliased reused collector/provider values")
	}
	return nil
}

func (driver *Driver) prepareTUIIdentityJoin() error {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return err
	}
	state := driver.tuiObservation
	state.totals.Owners[0].RunID = "0123456789abcdef0000000000000001"
	state.totals.Owners[1].RunID = "0123456789abcdef0000000000000002"
	label, err := identity.ParseDisplayLabel("exact-owner")
	if err != nil {
		return err
	}
	state.annotations[state.totals.Owners[0].RunID] = application.DisplayAnnotation{Availability: application.DisplayAvailable, Label: label}
	state.annotations["0123456789abcdef0000000000000003"] = application.DisplayAnnotation{Availability: application.DisplayAvailable, Label: label}
	return nil
}

func (driver *Driver) requireTUIIdentityJoin() error {
	if err := driver.requireTUIRows(); err != nil {
		return err
	}
	state := driver.tuiObservation.snapshot
	if state.Running[0].Label.Label.String() != "exact-owner" || state.Running[0].Label.Availability != application.DisplayAvailable || state.Running[1].Label.Label.String() != "" || state.Running[1].Label.Availability == application.DisplayAvailable {
		return errors.New("annotation joined a short prefix or absent identity")
	}
	return nil
}

func (driver *Driver) prepareTUIUnavailableAnnotation(annotationState string) error {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return err
	}
	state := driver.tuiObservation
	state.totals.Owners = state.totals.Owners[:1]
	state.annotationState = annotationState
	availability := application.DisplayInvalid
	switch annotationState {
	case annotationOperationMissing:
		availability = application.DisplayMissing
	case tuiUnsupportedSchema:
		availability = application.DisplayUnsupported
	case tuiUnreadableAnnotation, tuiSymlinkAnnotation:
		availability = application.DisplayUnreadable
	}
	// Unit receives the reader's typed outcome and proves normalization, never
	// claims it opened/parsed the filesystem source. Integration owns that read.
	unsafe, err := identity.ParseDisplayLabel("unchecked-payload")
	if err != nil {
		return err
	}
	state.annotations[state.totals.Owners[0].RunID] = application.DisplayAnnotation{Availability: availability, Label: unsafe}
	return nil
}

func (driver *Driver) requireTUIUnavailableAnnotation() error {
	state := driver.tuiObservation
	if len(state.snapshot.Running) != 1 {
		return errors.New("unusable label removed live owner")
	}
	row := state.snapshot.Running[0]
	if row.Label.Label.String() != "" || row.Label.Availability == application.DisplayAvailable || !row.AllocatedKnown || !reflect.DeepEqual(row.Entry, state.totals.Owners[0]) {
		return errors.New("unusable annotation altered live accounting")
	}
	return driver.requireTUIGlobalTotals()
}

func (driver *Driver) requireTUIPrivateAdvisory() error {
	state := driver.tuiObservation.snapshot
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "unchecked-payload") || strings.Contains(state.AnnotationAdvisory, "/private/") || strings.Contains(state.AnnotationAdvisory, "raw-error") {
		return errors.New("unchecked display annotation leaked into normalized view/advisory")
	}
	return nil
}

func (driver *Driver) prepareTUIMixedRows() error {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return err
	}
	state := driver.tuiObservation
	label, err := identity.ParseDisplayLabel("current-owner")
	if err != nil {
		return err
	}
	state.annotations[state.totals.Owners[1].RunID] = application.DisplayAnnotation{Availability: application.DisplayAvailable, Label: label}
	return nil
}

func (driver *Driver) requireTUIMixedLabels() error {
	if err := driver.requireTUIRows(); err != nil {
		return err
	}
	state := driver.tuiObservation.snapshot
	if state.Running[0].Label.Availability == application.DisplayAvailable || state.Running[0].Label.Label.String() != "" || state.Running[1].Label.Label.String() != "current-owner" {
		return errors.New("mixed version observation requires optional label metadata")
	}
	return nil
}

func (driver *Driver) prepareTUIStatusCompatibility() error {
	if err := driver.prepareTUIKnownRows(); err != nil {
		return err
	}
	state := driver.tuiObservation
	label, err := identity.ParseDisplayLabel("first-owner")
	if err != nil {
		return err
	}
	state.annotations[state.totals.Owners[0].RunID] = application.DisplayAnnotation{Availability: application.DisplayAvailable, Label: label}
	return nil
}

func (driver *Driver) inspectTUIStatusCompatibility(ctx context.Context) error {
	state := driver.tuiObservation
	if err := driver.observeTUI(ctx); err != nil {
		return err
	}
	if state.snapshot.Running[0].Label.Label.String() != "first-owner" {
		return errors.New("initial annotation never joined")
	}
	if state.native != nil {
		return driver.inspectTUINativeStatusCompatibility(ctx)
	}
	var err error
	state.statusJSON, err = json.Marshal(state.snapshot.Status)
	if err != nil {
		return err
	}
	watchContext, cancel := context.WithCancel(ctx)
	defer cancel()
	state.watchCancel = cancel
	state.watchWaitBase = state.waits
	code, err := state.services().Watch(watchContext, state.request, time.Second, func(view application.StatusView) ([]byte, error) {
		state.watchProjected++
		label, err := identity.ParseDisplayLabel(fmt.Sprintf("changed-%d", state.watchProjected))
		if err != nil {
			return nil, err
		}
		state.annotations[state.totals.Owners[0].RunID] = application.DisplayAnnotation{Availability: application.DisplayAvailable, Label: label}
		if state.native != nil {
			if err := state.native.writeLabel(state.totals.Owners[0].RunID, label); err != nil {
				return nil, err
			}
		}
		return json.Marshal(view)
	}, func(body []byte) error {
		state.watchBodies = append(state.watchBodies, append([]byte(nil), body...))
		return nil
	})
	if err != nil || code != 0 {
		return fmt.Errorf("watch failed code=%d: %w", code, err)
	}
	return nil
}

func (driver *Driver) requireTUIStatusCompatibility() error {
	state := driver.tuiObservation
	var body map[string]any
	if err := json.Unmarshal(state.statusJSON, &body); err != nil {
		return err
	}
	if body["schemaVersion"] != float64(5) {
		return errors.New("status schema changed")
	}
	for _, private := range []string{"first-owner", "changed-", "displayAnnotation", "availability", tuiLabelField} {
		if strings.Contains(string(state.statusJSON), private) {
			return fmt.Errorf("status leaked annotation field %q", private)
		}
	}
	return nil
}

func (driver *Driver) requireTUIWatchCompatibility() error {
	state := driver.tuiObservation
	if state.watchProjected != 2 || len(state.watchBodies) != 1 || string(state.watchBodies[0]) != string(state.statusJSON) || state.annotationReads < 3 {
		return fmt.Errorf("watch did not inspect changed labels while suppressing unchanged status: projected=%d emitted=%d reads=%d", state.watchProjected, len(state.watchBodies), state.annotationReads)
	}
	return driver.requireTUIStatusCompatibility()
}

func (state *tuiObservationState) readNativeDisplay(ctx context.Context, root string, ids []string) (map[string]application.DisplayAnnotation, error) {
	values, err := state.native.display.ReadDisplay(ctx, root, ids)
	if err != nil {
		return values, err
	}
	if state.native.watching {
		state.watchProjected++
		label, err := identity.ParseDisplayLabel(fmt.Sprintf("changed-%d", state.watchProjected))
		if err != nil {
			return nil, err
		}
		if err := state.native.writeLabel(state.totals.Owners[0].RunID, label); err != nil {
			return nil, err
		}
	}
	return values, nil
}
