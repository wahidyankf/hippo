package application_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

type runEntryContractFixture struct {
	*runContractFixture

	configuration                                                             application.Configuration
	absoluteError, loadError, identityError, historyError, entrySnapshotError error
	root                                                                      string
	present                                                                   bool
	legacy                                                                    int
	environment                                                               map[string]string
	identityCalls                                                             int
}

func newRunEntryContractFixture() *runEntryContractFixture {
	return &runEntryContractFixture{runContractFixture: newRunContractFixture(), root: "evidence", configuration: application.Configuration{Catalog: policy.BuiltinCatalog(), Coordination: coordination.Configuration{SchemaVersion: 1, Mode: "exclusive", MaxCPU: 8, MaxMemoryBytes: 16 * policy.GiB, BaseActiveOwners: 1, MaxActiveOwners: 2}, Hash: "hash"}}
}

func (fixture *runEntryContractFixture) AbsolutePath(path string) (string, error) {
	fixture.record("absolute")
	return "/absolute/" + path, fixture.absoluteError
}

func (fixture *runEntryContractFixture) StateRoot(environment map[string]string) string {
	fixture.record("root")
	fixture.environment = environment
	return fixture.root
}

func (fixture *runEntryContractFixture) Load(string, map[string]string) (application.Configuration, error) {
	fixture.record("load")
	return fixture.configuration, fixture.loadError
}

func (fixture *runEntryContractFixture) IdentityPresent(map[string]string, string) bool {
	fixture.record("identity-present")
	return fixture.present
}

func (fixture *runEntryContractFixture) Identity(_ map[string]string, _, source string, _ []string) (identity.Value, string, error) {
	fixture.record("resolve-identity")
	fixture.identityCalls++
	return identity.Value{SchemaVersion: identity.SchemaVersion, Source: source, Tags: map[string]string{"role": "test"}}, "private/identity.json", fixture.identityError
}

func (fixture *runEntryContractFixture) DisplayPath(string, string) string {
	return "safe-identity.json"
}

func (fixture *runEntryContractFixture) History(string, evidence.Query) ([]evidence.Summary, error) {
	fixture.record("history")
	return nil, fixture.historyError
}

func (fixture *runEntryContractFixture) Snapshot(context.Context, string, string) (coordination.ReservationTotals, error) {
	fixture.record("entry-snapshot")
	return coordination.ReservationTotals{LegacyEntries: fixture.legacy}, fixture.entrySnapshotError
}

func (fixture *runEntryContractFixture) entry() application.RunEntryServices {
	return application.RunEntryServices{Configuration: fixture, Observation: application.ObservationServices{Runtime: fixture, Evidence: fixture}, Run: fixture.services(), Collector: fixture, Environment: []string{"ROOT=first", "MALFORMED", "ROOT=last", "JOBS=1"}, Now: fixture.Now, Stderr: &fixture.stderr, PortLeaseRoot: "ports"}
}

func runEntryContractRequest() application.RunRequest {
	return application.RunRequest{Command: []string{"payload", "arg"}, Class: policy.TaskEphemeral, Source: "hippo", ConcurrencyEnvironment: []string{"JOBS"}}
}

func TestRunEntryContractPreparesExactlyOneWorkload(t *testing.T) {
	for _, mode := range []string{"exclusive-unlabeled", "exclusive-identity-present", "exclusive-tags", "schema2-explicit", "schema2-tier", "schema3-tier"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newRunEntryContractFixture()
			request := runEntryContractRequest()
			request.WorkingDir = "work"
			setupRunEntryContractSuccess(fixture, &request, mode)
			code, err := fixture.entry().Execute(context.Background(), request)
			assertRunEntryContractSuccess(t, fixture, request, mode, code, err)
		})
	}
}

func setupRunEntryContractSuccess(fixture *runEntryContractFixture, request *application.RunRequest, mode string) {
	switch mode {
	case "exclusive-unlabeled":
		request.Source = ""
	case "exclusive-identity-present":
		request.Source = ""
		fixture.present = true
	case "exclusive-tags":
		request.Source = ""
		request.Tags = []string{"role=test"}
	case "schema2-explicit":
		fixture.configuration.Coordination.SchemaVersion = 2
		fixture.configuration.Coordination.Mode = "reservation"
		request.ReserveCPU = 2
		request.ReserveMemoryMiB = 512
		request.WaitForAdmission = 3 * time.Second
	case "schema2-tier":
		fixture.configuration.Coordination.SchemaVersion = 2
		fixture.configuration.Coordination.Mode = "reservation"
		request.ResourceTier = "standard"
	case "schema3-tier":
		fixture.configuration.Coordination.SchemaVersion = 3
		fixture.configuration.Coordination.Mode = "reservation"
		request.ResourceTier = "standard"
	}
}

func assertRunEntryContractSuccess(t *testing.T, fixture *runEntryContractFixture, request application.RunRequest, mode string, code int, err error) {
	t.Helper()
	if code != 0 || err != nil || fixture.starts != 1 || fixture.finalizations != 1 || fixture.config.Command != "payload" || !reflect.DeepEqual(fixture.config.Arguments, []string{"arg"}) || fixture.config.WorkingDirectory != "/absolute/work" || fixture.config.PortLeaseRoot != "ports" || fixture.config.ConfigHash != "hash" || fixture.environment["ROOT"] != "last" || len(fixture.environment) != 2 {
		t.Fatalf("result=%d %v config=%+v trace=%v env=%v", code, err, fixture.config, fixture.trace, fixture.environment)
	}
	assertRunTraceOrder(t, fixture.trace, "absolute", "root", "load", "identity-present", "collect", "validate", "normalize", "begin-admission", "start", "finalize", "release")
	if mode == "exclusive-unlabeled" {
		if fixture.config.ReservationMetadata.Source != "unlabeled" || fixture.identityCalls != 0 {
			t.Fatalf("unlabeled identity=%+v", fixture.config.ReservationMetadata)
		}
	} else if fixture.identityCalls != 1 {
		t.Fatalf("identity calls=%d", fixture.identityCalls)
	}
	if request.WaitForAdmission > 0 && fixture.config.Policy.LeaseWait != request.WaitForAdmission {
		t.Fatalf("wait=%s want=%s", fixture.config.Policy.LeaseWait, request.WaitForAdmission)
	}
	if strings.HasPrefix(mode, "schema") && (!fixture.config.ReservationPolicy.Enabled || len(fixture.config.ReservationPolicy.Tiers) == 0 || fixture.config.ReservationPlan.Requested.CPU == 0) {
		t.Fatalf("reservation configuration=%+v plan=%+v", fixture.config.ReservationPolicy, fixture.config.ReservationPlan)
	}
}

func TestRunEntryContractRefusesBeforeLaunch(t *testing.T) {
	for _, stage := range []string{"absolute", "root", "load", "schema1-wait", "schema3-wait", "tier-wait", "identity-source", "identity-file", "probe", "profile", "cleanup", "replan", "promotion", "schema3-contention", "schema3-protocol", "schema3-legacy", "negative-cpu", "negative-memory", "exclusive-cpu", "exclusive-memory", "memory-overflow", "schema3-tier-required", "unknown-tier", "oversized-reservation"} {
		t.Run(stage, func(t *testing.T) {
			fixture := newRunEntryContractFixture()
			request := runEntryContractRequest()
			wantCode, wantFailure, wantReason := setupRunEntryContractRefusal(fixture, &request, stage)
			code, err := fixture.entry().Execute(context.Background(), request)
			if code != wantCode || err == nil || fixture.starts != 0 || fixture.finalizations != 0 {
				t.Fatalf("result=%d %v trace=%v", code, err, fixture.trace)
			}
			if wantFailure != "" {
				assertRunContractFailure(t, err, wantFailure)
			}
			if wantReason != policy.ReasonNone {
				stop, ok := errors.AsType[*policy.Stop](err)
				if !ok || stop.Reason != wantReason {
					t.Fatalf("reason=%v want=%s", err, wantReason)
				}
			}
			if stage == "identity-file" && (strings.Contains(err.Error(), "private/identity") || !strings.Contains(err.Error(), "safe-identity.json")) {
				t.Fatalf("identity path disclosure=%v", err)
			}
			if strings.HasSuffix(stage, "wait") && fixture.identityCalls != 0 {
				t.Fatalf("wait conflict read identity: %v", fixture.trace)
			}
		})
	}
}

func setupRunEntryContractRefusal(fixture *runEntryContractFixture, request *application.RunRequest, stage string) (int, status.Code, policy.Reason) {
	switch stage {
	case "absolute":
		request.WorkingDir = "work"
		fixture.absoluteError = errRunContract
		return 1, "", policy.ReasonNone
	case "root":
		fixture.root = ""
		return 1, "", policy.ReasonNone
	case "load":
		fixture.loadError = errRunContract
		return 0, status.CodeConfigUnreadable, policy.ReasonNone
	case "schema1-wait":
		request.WaitForAdmission = time.Second
	case "schema3-wait":
		fixture.configuration.Coordination.SchemaVersion = 3
		request.WaitForAdmission = time.Second
	case "tier-wait":
		fixture.configuration.Coordination.SchemaVersion = 2
		request.ResourceTier = "standard"
		request.WaitForAdmission = time.Second
	case "identity-source":
		fixture.identityError = identity.ErrSourceRequired
	case "identity-file":
		fixture.identityError = errRunContract
		return 0, status.CodeIdentityInvalid, policy.ReasonNone
	case "probe":
		fixture.collectErrorAt = 1
		fixture.collectError = errRunContract
		return 1, "", policy.ReasonNone
	case "profile":
		request.RequestedProfile = "unknown"
		return 0, "", policy.ReasonReplanRequired
	case "cleanup":
		fixture.collectHook = func(_ int, _ context.Context, reading *policy.Reading) { reading.Sample.DiskFreeBytes = new(int64(1)) }
		return 0, "", policy.ReasonStorageBlocked
	case "replan":
		request.Class = policy.TaskTransactional
		fixture.collectHook = func(_ int, _ context.Context, reading *policy.Reading) {
			reading.Sample.AvailableMemoryBytes = new(int64(1))
		}
		return 0, "", policy.ReasonReplanRequired
	case "promotion":
		fixture.configuration.Coordination.SchemaVersion = 3
		fixture.historyError = errRunContract
		return 1, "", policy.ReasonNone
	case "schema3-contention":
		fixture.configuration.Coordination.SchemaVersion = 3
		fixture.entrySnapshotError = coordination.ErrCoordinationDeferred
		return 1, "", policy.ReasonNone
	case "schema3-protocol":
		fixture.configuration.Coordination.SchemaVersion = 3
		fixture.entrySnapshotError = coordination.ErrCoordinationProtocolMismatch
		return 0, "", policy.ReasonProtocolMismatch
	case "schema3-legacy":
		fixture.configuration.Coordination.SchemaVersion = 3
		fixture.legacy = 1
		return 0, "", policy.ReasonProtocolMismatch
	default:
		return setupRunEntryContractReservationRefusal(fixture, request, stage)
	}
	return 0, status.CodeArgsInvalid, policy.ReasonNone
}

func setupRunEntryContractReservationRefusal(fixture *runEntryContractFixture, request *application.RunRequest, stage string) (int, status.Code, policy.Reason) {
	switch stage {
	case "negative-cpu":
		request.ReserveCPU = -1
	case "negative-memory":
		request.ReserveMemoryMiB = -1
	case "exclusive-cpu":
		request.ReserveCPU = 1
	case "exclusive-memory":
		request.ReserveMemoryMiB = 1
	case "memory-overflow":
		fixture.configuration.Coordination.Mode = "reservation"
		request.ReserveMemoryMiB = math.MaxInt64
	case "schema3-tier-required":
		fixture.configuration.Coordination.Mode = "reservation"
		fixture.configuration.Coordination.SchemaVersion = 3
	case "unknown-tier":
		fixture.configuration.Coordination.Mode = "reservation"
		request.ResourceTier = "unknown"
		return 0, "", policy.ReasonReplanRequired
	case "oversized-reservation":
		fixture.configuration.Coordination.Mode = "reservation"
		request.ReserveCPU = 100
		return 0, "", policy.ReasonReplanRequired
	}
	return 0, status.CodeArgsInvalid, policy.ReasonNone
}
