package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

type snapshotFixture struct {
	*observationFixture

	totals          coordination.ReservationTotals
	annotations     map[string]application.DisplayAnnotation
	annotationError error
	annotationIDs   []string
	annotationRoot  string
	previousCPU     []policy.CPUState
	waitDurations   []time.Duration
}

func (fixture *snapshotFixture) Snapshot(context.Context, string, string) (coordination.ReservationTotals, error) {
	fixture.calls = append(fixture.calls, "snapshot")
	return fixture.totals, nil
}

func (fixture *snapshotFixture) ReadDisplay(_ context.Context, root string, ids []string) (map[string]application.DisplayAnnotation, error) {
	fixture.calls = append(fixture.calls, "annotations")
	fixture.annotationIDs = append([]string(nil), ids...)
	fixture.annotationRoot = root
	return fixture.annotations, fixture.annotationError
}

func (fixture *snapshotFixture) Collect(ctx context.Context, previous policy.CPUState, path string) (policy.Reading, error) {
	fixture.previousCPU = append(fixture.previousCPU, append(policy.CPUState(nil), previous...))
	return fixture.observationFixture.Collect(ctx, previous, path)
}

func (fixture *snapshotFixture) Wait(ctx context.Context, duration time.Duration) error {
	fixture.waitDurations = append(fixture.waitDurations, duration)
	return fixture.observationFixture.Wait(ctx, duration)
}

func snapshotServices(fixture *snapshotFixture) application.ObservationServices {
	return application.ObservationServices{Configuration: fixture, Collector: fixture, Runtime: fixture, Evidence: fixture, Clock: fixture}
}

func TestObservationStatusFreezesReusedProviderValues(t *testing.T) {
	fixture := &snapshotFixture{
		observationFixture: newObservationFixture(),
		totals: coordination.ReservationTotals{
			Mode: "reservation", ActiveOwners: 1, WaitingOwners: 1,
			Owners:                 []coordination.ReservationEntry{{RunID: "owner", Tags: map[string]string{"role": "build"}}},
			Waiters:                []coordination.ReservationEntry{{RunID: "waiter", Position: 1, Tags: map[string]string{"role": "test"}}},
			AbandonedProcessGroups: []int{123},
		},
	}
	view, code, err := snapshotServices(fixture).Observe(context.Background(), application.StatusRequest{})
	if err != nil || code != 0 {
		t.Fatalf("observe code=%d error=%v", code, err)
	}
	*fixture.sample.CPUUtilizationPercent = 99
	fixture.sample.Capabilities[0] = "changed"
	fixture.totals.Owners[0].Tags["role"] = "changed"
	fixture.totals.Owners[0].RunID = "replacement-owner"
	fixture.totals.Waiters[0].Tags["role"] = "changed"
	fixture.totals.Waiters[0].Position = 9
	fixture.totals.AbandonedProcessGroups[0] = 999
	if *view.CPUUtilizationPercent != 20 || view.Capabilities[0] != "memory-pressure" {
		t.Errorf("completed host reading changed through provider aliases: cpu=%v capabilities=%v", *view.CPUUtilizationPercent, view.Capabilities)
	}
	if view.Coordination.Owners[0].RunID != "owner" || view.Coordination.Owners[0].Tags["role"] != "build" {
		t.Errorf("completed owner changed through provider aliases: %+v", view.Coordination.Owners[0])
	}
	if view.Coordination.Waiters[0].Position != 1 || view.Coordination.Waiters[0].Tags["role"] != "test" || view.Coordination.AbandonedProcessGroups[0] != 123 {
		t.Errorf("completed queue or housekeeping evidence changed through provider aliases: %+v", view.Coordination)
	}
}

func preparedSnapshotRequest(t *testing.T, request application.StatusRequest) application.ObservationRequest {
	t.Helper()
	prepared, err := application.PrepareObservation(request)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func snapshotLabel(t *testing.T, label string) identity.DisplayLabel {
	t.Helper()
	value, err := identity.ParseDisplayLabel(label)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestObservationSnapshotOwnsWorkflowAndFilteredGlobalAccounting(t *testing.T) {
	vector := coordination.ReservationVector{CPU: 2, MemoryBytes: policy.GiB}
	fixture := &snapshotFixture{observationFixture: newObservationFixture(), totals: coordination.ReservationTotals{
		Mode: "reservation", ActiveOwners: 2, WaitingOwners: 3,
		Capacity: coordination.ReservationVector{CPU: 8, MemoryBytes: 8 * policy.GiB}, Allocated: vector,
		Waiting: coordination.ReservationVector{CPU: 6, MemoryBytes: 3 * policy.GiB},
		Owners: []coordination.ReservationEntry{
			{RunID: "same-prefix-owner", Source: "hippo", Tags: map[string]string{"role": "build"}, Requested: vector, Allocated: vector},
			{RunID: "other-owner", Source: "rhino", Requested: vector, Allocated: vector},
		},
		Waiters: []coordination.ReservationEntry{
			{RunID: "first", Source: "rhino", Position: 1, Requested: vector},
			{RunID: "second", Source: "rhino", Position: 2, Requested: vector},
			{RunID: "same-prefix-waiter", Source: "hippo", Tags: map[string]string{"role": "build"}, Position: 3, Requested: vector},
		},
	}}
	first, second := fixture.sample, fixture.sample
	first.MeasuredAt = fixture.now.Add(-11 * time.Second).Format(time.RFC3339Nano)
	second.MeasuredAt = fixture.now.Add(-10 * time.Second).Format(time.RFC3339Nano)
	fixture.sampleSequence = []policy.Sample{first, second}
	fixture.annotations = map[string]application.DisplayAnnotation{
		"same-prefix-owner": {Label: snapshotLabel(t, "build-web"), Availability: application.DisplayAvailable},
		"absent":            {Label: snapshotLabel(t, "absent"), Availability: application.DisplayAvailable},
	}
	services := snapshotServices(fixture)
	services.Annotations = fixture
	tags := []string{"role=build"}
	request := preparedSnapshotRequest(t, application.StatusRequest{Source: "hippo", Tags: tags})
	tags[0] = "role=changed"
	snapshot, code, err := services.ObserveSnapshot(context.Background(), request)
	if err != nil || code != 0 {
		t.Fatalf("code=%d error=%v", code, err)
	}
	assertSnapshotWorkflow(t, fixture)
	if fixture.annotationRoot != "shared" || !reflect.DeepEqual(fixture.annotationIDs, []string{"same-prefix-owner", "same-prefix-waiter"}) {
		t.Fatalf("annotation scope=%s/%v", fixture.annotationRoot, fixture.annotationIDs)
	}
	if len(snapshot.Running) != 1 || len(snapshot.Queue) != 1 || snapshot.Queue[0].Entry.Position != 3 || snapshot.Running[0].Label.Label.String() != "build-web" || snapshot.Queue[0].Label.Availability != application.DisplayUnavailable {
		t.Fatalf("filtered exact-ID rows=%+v/%+v", snapshot.Running, snapshot.Queue)
	}
	if snapshot.Status.Coordination.ActiveOwners != 2 || snapshot.Status.Coordination.WaitingOwners != 3 || snapshot.Status.Coordination.Waiting != fixture.totals.Waiting || !snapshot.CapacityKnown || !snapshot.UsageKnown || !snapshot.QueueRegistered {
		t.Fatalf("filtered global accounting=%+v", snapshot)
	}
	if !snapshot.Running[0].RequestedKnown || !snapshot.Running[0].AllocatedKnown || !snapshot.Queue[0].RequestedKnown || snapshot.Queue[0].AllocatedKnown {
		t.Fatalf("resource availability=%+v/%+v", snapshot.Running, snapshot.Queue)
	}
	if snapshot.SampledAt == nil || !snapshot.SampledAt.Equal(fixture.now.Add(-10*time.Second)) {
		t.Fatalf("sample time=%v want second reading rather than completion %v", snapshot.SampledAt, fixture.now)
	}
	snapshot.Status.Coordination.Owners[0].Tags["role"] = "status-change"
	snapshot.Running[0].Entry.Tags["role"] = "row-change"
	if fixture.totals.Owners[0].Tags["role"] != "build" || snapshot.Status.Coordination.Owners[0].Tags["role"] != "status-change" {
		t.Fatal("status, rows and provider share tag storage")
	}
}

func TestObservationSnapshotAvailabilityAndOptionalTimes(t *testing.T) {
	vector := coordination.ReservationVector{CPU: 1, MemoryBytes: policy.GiB}
	for _, mode := range []string{"reservation", "exclusive"} {
		t.Run(mode, func(t *testing.T) {
			fixture := &snapshotFixture{observationFixture: newObservationFixture(), totals: coordination.ReservationTotals{Mode: mode, Owners: []coordination.ReservationEntry{{RunID: "legacy", Legacy: true, Requested: vector, Allocated: vector, Minimum: vector, Maximum: vector, RegisteredAt: "2026-10-07T01:00:00Z", Deadline: "invalid"}}}}
			snapshot, code, err := snapshotServices(fixture).ObserveSnapshot(context.Background(), preparedSnapshotRequest(t, application.StatusRequest{}))
			if err != nil || code != 0 {
				t.Fatalf("code=%d error=%v", code, err)
			}
			known := mode == "reservation"
			row := snapshot.Running[0]
			if row.RequestedKnown != known || row.AllocatedKnown != known || row.MinimumKnown != known || row.MaximumKnown != known || snapshot.UsageKnown != known || snapshot.QueueRegistered != known || snapshot.CapacityKnown {
				t.Fatalf("mode=%s availability=%+v", mode, snapshot)
			}
			if row.RegisteredAt == nil || row.Deadline != nil || row.Label.Availability != application.DisplayUnavailable {
				t.Fatalf("optional legacy evidence=%+v", row)
			}
		})
	}
	fixture := &snapshotFixture{observationFixture: newObservationFixture(), totals: coordination.ReservationTotals{Mode: "reservation"}}
	snapshot, code, err := snapshotServices(fixture).ObserveSnapshot(context.Background(), preparedSnapshotRequest(t, application.StatusRequest{}))
	if err != nil || code != 0 || !snapshot.UsageKnown || !snapshot.QueueRegistered || snapshot.CapacityKnown || len(snapshot.Running) != 0 || len(snapshot.Queue) != 0 || snapshot.Status.Coordination.Allocated != (coordination.ReservationVector{}) || snapshot.Status.Coordination.Waiting != (coordination.ReservationVector{}) {
		t.Fatalf("idle accounting=%+v code=%d error=%v", snapshot, code, err)
	}
}

func TestObservationSnapshotAnnotationFailuresAreSafeAndAdvisory(t *testing.T) {
	fixture := &snapshotFixture{observationFixture: newObservationFixture(), totals: coordination.ReservationTotals{Mode: "reservation", ActiveOwners: 1, Owners: []coordination.ReservationEntry{{RunID: "owner"}}}, annotationError: errors.New("private annotation read /fixture/private/annotation-read")}
	services := snapshotServices(fixture)
	services.Annotations = fixture
	snapshot, code, err := services.ObserveSnapshot(context.Background(), preparedSnapshotRequest(t, application.StatusRequest{}))
	if err != nil || code != 0 || snapshot.Status.Coordination.ActiveOwners != 1 || snapshot.Running[0].Label.Availability != application.DisplayUnavailable || snapshot.AnnotationAdvisory == "" || strings.Contains(snapshot.AnnotationAdvisory, "private") || strings.Contains(snapshot.AnnotationAdvisory, "/fixture/private/annotation-read") {
		t.Fatalf("advisory changed outcome or leaked payload: %+v code=%d error=%v", snapshot, code, err)
	}
}

func TestObservationSnapshotReloadsConfigurationAndPreservesStatusSchema(t *testing.T) {
	fixture := &snapshotFixture{observationFixture: newObservationFixture(), totals: coordination.ReservationTotals{Mode: "reservation", Owners: []coordination.ReservationEntry{{RunID: "owner"}}}}
	services := snapshotServices(fixture)
	services.Annotations = fixture
	request := preparedSnapshotRequest(t, application.StatusRequest{})
	for _, label := range []string{"first", "second"} {
		fixture.annotations = map[string]application.DisplayAnnotation{"owner": {Label: snapshotLabel(t, label), Availability: application.DisplayAvailable}}
		snapshot, code, err := services.ObserveSnapshot(context.Background(), request)
		if err != nil || code != 0 || snapshot.Running[0].Label.Label.String() != label {
			t.Fatalf("snapshot=%+v code=%d error=%v", snapshot, code, err)
		}
		encoded, err := json.Marshal(snapshot.Status)
		if err != nil || strings.Contains(string(encoded), "label") || strings.Contains(string(encoded), "sampledAt") || snapshot.Status.SchemaVersion != 5 {
			t.Fatalf("public status changed: %s error=%v", encoded, err)
		}
	}
	loads, roots := 0, 0
	for _, call := range fixture.calls {
		if call == "load" {
			loads++
		}
		if call == "root" {
			roots++
		}
	}
	if loads != 2 || roots != 2 {
		t.Fatalf("prepared request cached live configuration/root: calls=%v", fixture.calls)
	}
}

func TestPrepareObservationPreservesValidationPrecedenceWithoutEffects(t *testing.T) {
	for _, test := range []struct {
		request application.StatusRequest
		field   string
	}{
		{application.StatusRequest{Source: "/private", Tags: []string{"bad"}}, "--source"},
		{application.StatusRequest{Tags: []string{"bad"}}, "--tag"},
	} {
		_, err := application.PrepareObservation(test.request)
		failure, classified := errors.AsType[status.Failure](err)
		if !classified || failure.Code != status.CodeArgsInvalid || !strings.Contains(failure.Message, test.field) {
			t.Fatalf("failure=%v want %s usage refusal", err, test.field)
		}
	}
}

func TestObservationSnapshotNormalizesAnnotationAvailability(t *testing.T) {
	states := []application.DisplayAvailability{application.DisplayUnavailable, application.DisplayAvailable, application.DisplayMissing, application.DisplayInvalid, application.DisplayUnsupported, application.DisplayUnreadable, application.DisplayAvailability(255)}
	for _, availability := range states {
		t.Run(availability.String(), func(t *testing.T) {
			fixture := &snapshotFixture{observationFixture: newObservationFixture(), totals: coordination.ReservationTotals{Mode: "reservation", Owners: []coordination.ReservationEntry{{RunID: "owner"}}}, annotations: map[string]application.DisplayAnnotation{"owner": {Label: snapshotLabel(t, "build-web"), Availability: availability}}}
			services := snapshotServices(fixture)
			services.Annotations = fixture
			snapshot, code, err := services.ObserveSnapshot(context.Background(), preparedSnapshotRequest(t, application.StatusRequest{}))
			if err != nil || code != 0 {
				t.Fatalf("code=%d error=%v", code, err)
			}
			want := availability
			if availability == application.DisplayAvailability(255) {
				want = application.DisplayUnavailable
			}
			row := snapshot.Running[0]
			if row.Label.Availability != want || (availability == application.DisplayAvailable && row.Label.Label.String() != "build-web") || (availability != application.DisplayAvailable && row.Label.Label.String() != "") {
				t.Fatalf("unsafe availability/payload projection: %+v", row.Label)
			}
		})
	}
}

func TestObservationWatchIgnoresDisplayAnnotationChanges(t *testing.T) {
	fixture := &snapshotFixture{observationFixture: newObservationFixture(), totals: coordination.ReservationTotals{
		Mode: "reservation", Owners: []coordination.ReservationEntry{{RunID: "owner"}},
	}}
	fixture.annotations = map[string]application.DisplayAnnotation{"owner": {Label: snapshotLabel(t, "first"), Availability: application.DisplayAvailable}}
	services := snapshotServices(fixture)
	services.Annotations = fixture
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	fixture.cancel, fixture.cancelAt = cancel, 4
	projected, emitted := 0, 0
	code, err := services.Watch(ctx, application.StatusRequest{}, time.Second, func(view application.StatusView) ([]byte, error) {
		projected++
		return json.Marshal(view)
	}, func([]byte) error {
		emitted++
		fixture.annotations = map[string]application.DisplayAnnotation{"owner": {Label: snapshotLabel(t, "second"), Availability: application.DisplayAvailable}}
		return nil
	})
	if err != nil || code != 0 || projected != 2 || emitted != 1 || fixture.waits != 4 {
		t.Fatalf("annotation-only change affected watch: code=%d error=%v projected=%d emitted=%d waits=%d", code, err, projected, emitted, fixture.waits)
	}
}

func TestObservationStatusPreservesNilAndEmptyProviderCollections(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "nil", true: "empty"}[empty], func(t *testing.T) {
			fixture := &snapshotFixture{observationFixture: newObservationFixture(), totals: coordination.ReservationTotals{Mode: "reservation"}}
			fixture.sample.Capabilities = nil
			if empty {
				fixture.sample.Capabilities = []string{}
				fixture.totals.Owners = []coordination.ReservationEntry{}
				fixture.totals.Waiters = []coordination.ReservationEntry{}
				fixture.totals.AbandonedProcessGroups = []int{}
			}
			view, code, err := snapshotServices(fixture).Observe(t.Context(), application.StatusRequest{})
			if err != nil || code != 0 {
				t.Fatalf("code=%d error=%v", code, err)
			}
			if (view.Capabilities != nil) != empty || (view.Coordination.Owners != nil) != empty || (view.Coordination.Waiters != nil) != empty || (view.Coordination.AbandonedProcessGroups != nil) != empty {
				t.Fatalf("nil/empty source shape changed: capabilities=%#v totals=%#v", view.Capabilities, view.Coordination)
			}
			got, err := json.Marshal(view.Coordination)
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(fixture.totals)
			if err != nil || string(got) != string(want) {
				t.Fatalf("coordination JSON changed: got=%s want=%s error=%v", got, want, err)
			}
		})
	}
}

func TestObservationSnapshotOwnsEmptyAndMissingRowTags(t *testing.T) {
	fixture := &snapshotFixture{observationFixture: newObservationFixture(), totals: coordination.ReservationTotals{Mode: "reservation", Owners: []coordination.ReservationEntry{
		{RunID: "missing"}, {RunID: "empty", Tags: map[string]string{}},
	}}}
	snapshot, code, err := snapshotServices(fixture).ObserveSnapshot(t.Context(), preparedSnapshotRequest(t, application.StatusRequest{}))
	if err != nil || code != 0 {
		t.Fatalf("code=%d error=%v", code, err)
	}
	if snapshot.Status.Coordination.Owners[0].Tags != nil || snapshot.Running[0].Entry.Tags != nil || snapshot.Status.Coordination.Owners[1].Tags == nil || snapshot.Running[1].Entry.Tags == nil {
		t.Fatalf("missing/empty tags changed: status=%+v running=%+v", snapshot.Status.Coordination.Owners, snapshot.Running)
	}
	snapshot.Running[1].Entry.Tags["role"] = "row"
	if len(snapshot.Status.Coordination.Owners[1].Tags) != 0 || len(fixture.totals.Owners[1].Tags) != 0 {
		t.Fatal("empty tags share storage between rows, status or provider")
	}
}

func TestObservationSnapshotRequestsUniqueIDsWithoutChangingRows(t *testing.T) {
	fixture := &snapshotFixture{observationFixture: newObservationFixture(), totals: coordination.ReservationTotals{Mode: "reservation", Owners: []coordination.ReservationEntry{{RunID: "shared"}, {RunID: "other"}}, Waiters: []coordination.ReservationEntry{{RunID: "shared", Position: 1}}}, annotations: map[string]application.DisplayAnnotation{"shared": {Availability: application.DisplayAvailable}}}
	services := snapshotServices(fixture)
	services.Annotations = fixture
	snapshot, code, err := services.ObserveSnapshot(t.Context(), preparedSnapshotRequest(t, application.StatusRequest{}))
	if err != nil || code != 0 {
		t.Fatalf("code=%d error=%v", code, err)
	}
	if !reflect.DeepEqual(fixture.annotationIDs, []string{"shared", "other"}) || len(snapshot.Running) != 2 || len(snapshot.Queue) != 1 || snapshot.Queue[0].Entry.Position != 1 {
		t.Fatalf("annotation batching changed live row projection: ids=%v snapshot=%+v", fixture.annotationIDs, snapshot)
	}
	if snapshot.Running[0].Label.Availability != application.DisplayUnavailable || snapshot.Running[0].Label.Label.String() != "" {
		t.Fatalf("available annotation without a validated label was published: %+v", snapshot.Running[0].Label)
	}
}

func assertSnapshotWorkflow(t *testing.T, fixture *snapshotFixture) {
	t.Helper()
	if !reflect.DeepEqual(fixture.calls, []string{"load", "collect", "wait", "collect", "root", "history", "snapshot", "annotations"}) || !reflect.DeepEqual(fixture.waitDurations, []time.Duration{time.Second}) || !reflect.DeepEqual(fixture.previousCPU, []policy.CPUState{nil, {1}}) {
		t.Fatalf("shared observation workflow changed: calls=%v waits=%v CPU=%v", fixture.calls, fixture.waitDurations, fixture.previousCPU)
	}
}
