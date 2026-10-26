package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

const (
	annotationBDDToken            = "0123456789abcdef0123456789abcdef"
	annotationBDDOther            = "fedcba9876543210fedcba9876543210"
	annotationBDDUnproven         = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	annotationBDDFirstWaiter      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	annotationPublishEvent        = "publish"
	annotationCommitEvent         = "commit"
	annotationAdmissionDeferral   = "admission deferral"
	annotationConfirmedForcedStop = "confirmed forced stop"
)

// annotationBDD invokes application policy at Unit, actual embedded CLI/runtime
// at Integration, and only TestMain's stamped public candidate at E2E.
// Its store callbacks mutate exactly the requested key; they contain no policy.
type annotationBDD struct {
	adapter                       contract.Adapter
	neutral                       *Driver
	kind                          application.DisplayRegistrationKind
	label, ending                 string
	annotations, before           map[string]string
	atCommit                      map[string]string
	events                        []string
	ledger                        coordination.Ledger
	request                       coordination.ReservationVector
	commits, publishes, removals  int
	root, config                  string
	jobs                          []*annotationBDDJob
	owner, target                 *annotationBDDJob
	id, staleID                   string
	unproven                      *os.File
	nativeBefore                  map[string][]byte
	unitSessionRecord             runtimeadapter.CompatibilityOwnerRecord
	unitCoordinationRecord        runtimeadapter.CoordinationMarkerRecord
	unitSession, unitCoordination []byte
}

func newAnnotationBDD(adapter contract.Adapter, neutral *Driver) *annotationBDD {
	return &annotationBDD{adapter: adapter, neutral: neutral}
}

// annotationBindings returns the display-annotation registry, creating the
// owned fixture on first use so literally constructed Drivers also resolve
// label steps. The fixture pointer outlives Driver.Reset, so registered
// handlers keep pointing at the same fixture across scenarios.
func (driver *Driver) annotationBindings() []contract.StepBinding {
	if driver.annotations == nil {
		adapter, err := contract.AdapterByName(driver.mode)
		if err != nil {
			return nil
		}
		driver.annotations = newAnnotationBDD(adapter, driver)
	}
	return driver.annotations.Bindings()
}

func (fixture *annotationBDD) Bindings() []contract.StepBinding {
	return []contract.StepBinding{
		{Pattern: `^an admitted reservation run explicitly labeled build-web$`, Handler: func() error { return fixture.prepare(application.DisplayReservationRegistration, "build-web") }},
		{Pattern: `^its owned reservation token becomes visible$`, Handler: fixture.register},
		{Pattern: `^its full run ID has a display annotation labeled build-web$`, Handler: func() error { return fixture.requireLabel("build-web") }},
		{Pattern: `^the annotation contains no command arguments or working directory$`, Handler: fixture.requirePrivacy},
		{Pattern: `^exhausted reservation capacity and a run explicitly labeled test-web$`, Handler: func() error { return fixture.prepare(application.DisplayWaiterRegistration, "test-web") }},
		{Pattern: `^that run becomes a registered waiter$`, Handler: fixture.register},
		{Pattern: `^its full waiter run ID has a display annotation labeled test-web$`, Handler: func() error { return fixture.requireLabel("test-web") }},
		{Pattern: `^the queue retains its original resource demand and FIFO position$`, Handler: fixture.requireFIFO},
		{Pattern: `^a guarded command whose executable is /usr/bin/printf and whose argument contains private text$`, Handler: func() error { return fixture.prepare(application.DisplayReservationRegistration, "") }},
		{Pattern: `^the run registers without an explicit display label$`, Handler: fixture.register},
		{Pattern: `^its annotation label is printf and contains neither the argument nor executable directory$`, Handler: fixture.requireDefault},
		{Pattern: `^an exclusive compatibility run explicitly labeled build-web$`, Handler: func() error { return fixture.prepare(application.DisplayExclusiveRegistration, "build-web") }},
		{Pattern: `^its owned session becomes visible$`, Handler: fixture.register},
		{Pattern: `^its full session run ID has a display annotation labeled build-web$`, Handler: func() error { return fixture.requireLabel("build-web") }},
		{Pattern: `^exclusive coordination and session schema remain unchanged$`, Handler: fixture.requireExclusiveSchema},
		{Pattern: `^a valid inherited session whose owner is labeled parent-build$`, Handler: fixture.prepareInherited},
		{Pattern: `^a nested guarded command requests a different display label$`, Handler: fixture.registerInherited},
		{Pattern: `^the inherited owner remains labeled parent-build$`, Handler: func() error { return fixture.requireLabel("parent-build") }},
		{Pattern: `^the nested run creates no replacement annotation$`, Handler: fixture.requireInherited},
		{Pattern: `^a registered labeled run that reaches (child completion|cancellation in queue|admission deferral|confirmed forced stop)$`, Handler: fixture.prepareRetirement},
		{Pattern: `^its owned token is removed under the coordination lock$`, Handler: fixture.retire},
		{Pattern: `^its matching display annotation is removed$`, Handler: fixture.requireRetired},
		{Pattern: `^no other owner's display annotation changes$`, Handler: fixture.requireOtherOwner},
		{Pattern: `^one proven stale owner and one live owner with display annotations$`, Handler: fixture.prepareCleanup},
		{Pattern: `^an annotation whose token is temporarily absent from the ledger$`, Handler: fixture.prepareUnproven},
		{Pattern: `^locked coordination reconciliation performs annotation cleanup$`, Handler: fixture.cleanup},
		{Pattern: `^only the proven stale owner's annotation is removed$`, Handler: fixture.requireStaleRemoved},
		{Pattern: `^the live and unproven annotations remain unchanged$`, Handler: fixture.requireCleanupRetained},
	}
}

func (fixture *annotationBDD) prepare(kind application.DisplayRegistrationKind, label string) error {
	fixture.kind, fixture.label = kind, label
	fixture.annotations = map[string]string{annotationBDDOther: "other-owner"}
	fixture.before = maps.Clone(fixture.annotations)
	fixture.request = coordination.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
	fixture.ledger = coordination.Ledger{SchemaVersion: coordination.LedgerSchemaVersion, Capacity: coordination.ReservationVector{CPU: 2, MemoryBytes: 512 * policy.MiB}}
	if fixture.adapter.Name == contract.Unit {
		return nil
	}
	if fixture.adapter.Name != contract.Integration && fixture.adapter.Name != contract.E2E {
		return errors.New("annotation fixture has no executing boundary")
	}
	if fixture.adapter.Name == contract.E2E && os.Getenv("HIPPO_BIN") == "" {
		return errors.New("annotation public fixture has no unchanged TestMain candidate")
	}
	root, err := os.MkdirTemp("", "hippo-bdd-annotation-")
	if err != nil {
		return err
	}
	fixture.root = root
	fixture.config = filepath.Join(root, "config.json")
	document := `{"schemaVersion":2,"coordination":{"mode":"reservation","maxActiveOwners":1}}`
	if kind == application.DisplayExclusiveRegistration {
		document = `{"schemaVersion":1}`
	}
	return fixture.writeNativeFile("config.json", []byte(document))
}

func (fixture *annotationBDD) readNativeFile(name string) ([]byte, error) {
	root, err := os.OpenRoot(fixture.root)
	if err != nil {
		return nil, err
	}
	data, readError := root.ReadFile(name)
	return data, errors.Join(readError, root.Close())
}

func (fixture *annotationBDD) writeNativeFile(name string, data []byte) error {
	root, err := os.OpenRoot(fixture.root)
	if err != nil {
		return err
	}
	writeError := root.WriteFile(name, data, 0o600)
	return errors.Join(writeError, root.Close())
}

func (fixture *annotationBDD) removeNativeRoot() error {
	parent, err := os.OpenRoot(filepath.Dir(fixture.root))
	if err != nil {
		return err
	}
	removeError := parent.RemoveAll(filepath.Base(fixture.root))
	return errors.Join(removeError, parent.Close())
}

func (fixture *annotationBDD) parsedLabel() (identity.DisplayLabel, error) {
	if fixture.label == "" {
		return application.ResolveDisplayLabel("/usr/bin/printf", identity.DisplayLabel{}), nil
	}
	return identity.ParseDisplayLabel(fixture.label)
}

func (fixture *annotationBDD) registrationEffects() application.DisplayRegistrationEffects {
	return application.DisplayRegistrationEffects{
		Publish: func(id string, label identity.DisplayLabel) error {
			fixture.publishes++
			fixture.events = append(fixture.events, annotationPublishEvent)
			fixture.annotations[id] = label.String()
			return nil
		},
		Commit: func() error {
			fixture.commits++
			fixture.events = append(fixture.events, annotationCommitEvent)
			fixture.atCommit = maps.Clone(fixture.annotations)
			if fixture.kind == application.DisplayExclusiveRegistration {
				var err error
				fixture.unitSession, err = fixture.unitSessionRecord.Encode()
				if err != nil {
					return err
				}
				fixture.unitCoordination, err = fixture.unitCoordinationRecord.Encode()
				return err
			}
			if err := coordination.Enqueue(&fixture.ledger, coordination.Waiter{Token: annotationBDDToken, PID: 7, Class: policy.TaskEphemeral, Profile: policy.ProfileName("balanced"), Requested: fixture.request, MaxOwners: 2}); err != nil {
				return err
			}
			_, _, err := coordination.AdmitHead(&fixture.ledger, annotationBDDToken, fixture.request, fixture.request, 2)
			return err
		},
		Remove: func(id string) error { fixture.removals++; delete(fixture.annotations, id); return nil },
	}
}

func (fixture *annotationBDD) register(ctx context.Context) error {
	if fixture.adapter.Name == contract.Unit {
		return fixture.registerUnit()
	}
	if fixture.kind == application.DisplayWaiterRegistration {
		owner, err := fixture.start(ctx, "capacity-owner", "", false)
		if err != nil {
			return err
		}
		fixture.owner = owner
		if err = fixture.awaitStarted(ctx, owner); err != nil {
			return err
		}
		first, err := fixture.start(ctx, "first-waiter", "", false)
		if err != nil {
			return err
		}
		if _, err = fixture.awaitWaiter(ctx, first, 1); err != nil {
			return err
		}
	}
	job, err := fixture.start(ctx, fixture.label, "", fixture.label == "")
	if err != nil {
		return err
	}
	fixture.target = job
	if fixture.kind == application.DisplayWaiterRegistration {
		waiter, err := fixture.awaitWaiter(ctx, job, 2)
		if err != nil {
			return err
		}
		fixture.id = waiter.Token
		ledger, err := fixture.readLedger()
		if err != nil {
			return err
		}
		fixture.ledger = ledger
		return nil
	}
	if err = fixture.awaitStarted(ctx, job); err != nil {
		return err
	}
	fixture.id = job.token
	return nil
}

func (fixture *annotationBDD) registerUnit() error {
	if fixture.kind == application.DisplayWaiterRegistration {
		fixture.ledger.Owners = []coordination.ReservationOwner{{Token: annotationBDDOther, PID: 8, Class: policy.TaskEphemeral, Profile: policy.ProfileName("balanced"), Requested: fixture.ledger.Capacity, Allocated: fixture.ledger.Capacity, Sequence: 1, MaxOwners: 2, PeakOwners: 1}}
		fixture.ledger.NextSequence = 1
		if err := coordination.Enqueue(&fixture.ledger, coordination.Waiter{Token: annotationBDDFirstWaiter, PID: 9, Class: policy.TaskEphemeral, Profile: policy.ProfileName("balanced"), Requested: fixture.request, MaxOwners: 2}); err != nil {
			return err
		}
	}
	label, err := fixture.parsedLabel()
	if err != nil {
		return err
	}
	fixture.id = annotationBDDToken
	if fixture.kind == application.DisplayExclusiveRegistration {
		fixture.unitSessionRecord = runtimeadapter.NewExclusiveSessionRecord(7, fixture.id, policy.TaskEphemeral, 13, 17)
		fixture.unitCoordinationRecord = runtimeadapter.NewExclusiveCoordinationMarker()
	}
	return application.RegisterDisplay(application.DisplayRegistration{RunID: fixture.id, Label: label, Kind: fixture.kind, Ownership: application.DisplayOwnershipOwned}, fixture.registrationEffects(), &application.DisplayAdvisories{})
}

func (fixture *annotationBDD) annotation(runID string) (string, error) {
	if fixture.adapter.Name == contract.Unit {
		label, present := fixture.annotations[runID]
		if !present {
			return "", os.ErrNotExist
		}
		return label, nil
	}
	data, err := fixture.readNativeFile(filepath.Join("run-display", runID+".json"))
	if err != nil {
		return "", err
	}
	var record struct {
		SchemaVersion int    `json:"schemaVersion"`
		RunID         string `json:"runId"`
		Label         string `json:"label"`
	}
	if err = json.Unmarshal(data, &record); err != nil {
		return "", err
	}
	if record.SchemaVersion != 1 || record.RunID != runID || len(runID) != 32 {
		return "", errors.New("native annotation does not match complete owned ID/schema")
	}
	return record.Label, nil
}

func (fixture *annotationBDD) requireLabel(want string) error {
	got, err := fixture.annotation(fixture.id)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("annotation label=%q want=%q", got, want)
	}
	if fixture.adapter.Name == contract.Unit && (fixture.atCommit[fixture.id] != want || fixture.commits != 1) {
		return errors.New("annotation absent at actual application commit observation")
	}
	return nil
}

func (fixture *annotationBDD) requirePrivacy() error {
	data, err := fixture.privacyAnnotation()
	if err != nil {
		return err
	}
	for _, private := range []string{"private argument", "private text", "/usr/bin/", fixture.root} {
		if private != "" && bytes.Contains(data, []byte(private)) {
			return errors.New("annotation contains private command/directory context")
		}
	}
	return fixture.close()
}

func (fixture *annotationBDD) privacyAnnotation() ([]byte, error) {
	if fixture.adapter.Name == contract.Unit {
		if err := fixture.requireUnitPublication(); err != nil {
			return nil, err
		}
		return []byte(fixture.annotations[fixture.id]), nil
	}
	data, err := fixture.readNativeFile(filepath.Join("run-display", fixture.id+".json"))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if len(fields) != 3 {
		return nil, errors.New("annotation gained command context fields")
	}
	return data, nil
}

func (fixture *annotationBDD) requireUnitPublication() error {
	if fixture.publishes != 1 || fixture.atCommit[fixture.id] != fixture.label || !reflect.DeepEqual(fixture.events, []string{annotationPublishEvent, annotationCommitEvent}) {
		return errors.New("publication did not precede actual ownership commit exactly once")
	}
	return nil
}

func (fixture *annotationBDD) requireFIFO() error {
	if fixture.ledger.SchemaVersion != coordination.LedgerSchemaVersion || len(fixture.ledger.Waiters) != 2 {
		return errors.New("queue lost original two FIFO waiters")
	}
	first, target := fixture.ledger.Waiters[0], fixture.ledger.Waiters[1]
	if target.Token != fixture.id || target.Requested != fixture.request || target.Sequence != first.Sequence+1 || target.Sequence != 3 {
		return fmt.Errorf("original request/FIFO changed: %+v", fixture.ledger.Waiters)
	}
	if fixture.adapter.Name == contract.Unit && (first.Token != annotationBDDFirstWaiter || target.Sequence != 3 || fixture.publishes != 1 || fixture.atCommit[fixture.id] != "test-web") {
		return errors.New("unit actual Enqueue/AdmitHead changed original FIFO/publication")
	}
	return fixture.close()
}

func (fixture *annotationBDD) requireDefault() error {
	if err := fixture.requireLabel("printf"); err != nil {
		return err
	}
	label, err := fixture.annotation(fixture.id)
	if err != nil {
		return err
	}
	if strings.Contains(label, "private") || strings.Contains(label, "/") {
		return errors.New("default label contains executable directory/argument")
	}
	if fixture.adapter.Name == contract.Unit && fixture.publishes != 1 {
		return errors.New("default label did not reach actual publication")
	}
	return fixture.close()
}

func (fixture *annotationBDD) requireExclusiveSchema() error {
	if fixture.adapter.Name == contract.Unit {
		if fixture.commits != 1 || fixture.kind != application.DisplayExclusiveRegistration || !reflect.DeepEqual(fixture.events, []string{annotationPublishEvent, annotationCommitEvent}) {
			return errors.New("exclusive production record did not pass through ordered application commit")
		}
		if err := requireUnitExclusiveWire(fixture.unitSession, fixture.unitCoordination, fixture.id); err != nil {
			return err
		}
		return fixture.close()
	}
	for path, want := range map[string]int{"coordination-mode.json": 2, filepath.Join("sessions", fixture.id+".json"): 6} {
		data, err := os.ReadFile(filepath.Join(fixture.root, path))
		if err != nil {
			return err
		}
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(data, &fields); err != nil {
			return err
		}
		if len(fields) != want || string(fields["schemaVersion"]) != "1" || fields["label"] != nil {
			return fmt.Errorf("exclusive strict schema changed: %s", data)
		}
		if path == "coordination-mode.json" && string(fields["mode"]) != `"exclusive"` {
			return errors.New("exclusive annotation changed coordination mode")
		}
	}
	return fixture.close()
}

// requireUnitExclusiveWire checks the native writer's production encodings
// against independent protocol expectations, never a copy created by Given.
func requireUnitExclusiveWire(session, marker []byte, id string) error {
	for _, record := range []struct {
		data []byte
		want map[string]string
	}{
		{session, map[string]string{"schemaVersion": "1", "pid": "7", "token": strconv.Quote(id), "class": `"ephemeral"`, "identityDevice": "13", "identityInode": "17"}},
		{marker, map[string]string{"schemaVersion": "1", "mode": `"exclusive"`}},
	} {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(record.data, &fields); err != nil {
			return err
		}
		if len(fields) != len(record.want) {
			return errors.New("exclusive production wire gained or lost fields")
		}
		for name, want := range record.want {
			if string(fields[name]) != want {
				return fmt.Errorf("exclusive production wire field %s=%s want=%s", name, fields[name], want)
			}
		}
	}
	return nil
}

func (fixture *annotationBDD) prepareInherited(ctx context.Context) error {
	if err := fixture.prepare(application.DisplayReservationRegistration, "parent-build"); err != nil {
		return err
	}
	if err := fixture.register(ctx); err != nil {
		return err
	}
	if fixture.adapter.Name == contract.Unit {
		fixture.before = maps.Clone(fixture.annotations)
		return nil
	}
	fixture.owner = fixture.target
	return fixture.captureNativeAnnotations()
}

func (fixture *annotationBDD) registerInherited(ctx context.Context) error {
	if fixture.adapter.Name == contract.Unit {
		label, err := identity.ParseDisplayLabel("nested-build")
		if err != nil {
			return err
		}
		beforePublishes := fixture.publishes
		err = application.RegisterDisplay(application.DisplayRegistration{RunID: fixture.id, Label: label, Ownership: application.DisplayOwnershipInherited}, application.DisplayRegistrationEffects{Publish: fixture.registrationEffects().Publish, Commit: func() error { fixture.atCommit = maps.Clone(fixture.annotations); return nil }, Remove: fixture.registrationEffects().Remove}, &application.DisplayAdvisories{})
		if err != nil {
			return err
		}
		if fixture.publishes != beforePublishes {
			return errors.New("nested application republished parent context")
		}
		return application.RetireDisplay(application.DisplayRetirement{RunID: fixture.id, Ownership: application.DisplayOwnershipInherited}, application.DisplayRetirementEffects{RemoveOwnership: func() (application.DisplayRetirementResult, error) {
			return application.DisplayRetirementResult{}, errors.New("inherited ownership transition must not execute")
		}, Remove: fixture.registrationEffects().Remove}, &application.DisplayAdvisories{})
	}
	job, err := fixture.start(ctx, "nested-build", fixture.id, false)
	if err != nil {
		return err
	}
	fixture.target = job
	if err = fixture.awaitStarted(ctx, job); err != nil {
		return err
	}
	if job.token != fixture.id {
		return errors.New("nested native run did not authenticate/inherit original token")
	}
	return fixture.finish(job)
}

func (fixture *annotationBDD) requireInherited() error {
	if fixture.adapter.Name == contract.Unit {
		if !reflect.DeepEqual(fixture.annotations, fixture.before) || fixture.publishes != 1 || fixture.removals != 0 {
			return errors.New("nested application replaced/removed owner annotation")
		}
	} else if err := fixture.requireNativeAnnotationsUnchanged(); err != nil {
		return err
	}
	return fixture.close()
}

func (fixture *annotationBDD) prepareRetirement(ctx context.Context, ending string) error {
	kind := application.DisplayReservationRegistration
	if ending == "cancellation in queue" || ending == annotationAdmissionDeferral {
		kind = application.DisplayWaiterRegistration
	}
	if err := fixture.prepare(kind, "retire-build"); err != nil {
		return err
	}
	fixture.ending = ending
	if fixture.adapter.Name == contract.Unit {
		if err := fixture.register(ctx); err != nil {
			return err
		}
		fixture.before = maps.Clone(fixture.annotations)
		return nil
	}
	owner, err := fixture.start(ctx, "other-owner", "", false)
	if err != nil {
		return err
	}
	fixture.owner = owner
	if err = fixture.awaitStarted(ctx, owner); err != nil {
		return err
	}
	if kind != application.DisplayWaiterRegistration {
		// Permit the target beside a real live owner; each retains its own identity.
		if err = fixture.writeNativeFile("config.json", []byte(`{"schemaVersion":2,"coordination":{"mode":"reservation","maxActiveOwners":2}}`)); err != nil {
			return err
		}
		// The first owner registered a limit of one. Restart its original
		// invocation with the same new fixture policy rather than editing ledger facts.
		if err = fixture.finish(owner); err != nil {
			return err
		}
		owner, err = fixture.start(ctx, "other-owner", "", false)
		if err != nil {
			return err
		}
		fixture.owner = owner
		if err = fixture.awaitStarted(ctx, owner); err != nil {
			return err
		}
	}
	job, err := fixture.start(ctx, "retire-build", "", false)
	if err != nil {
		return err
	}
	fixture.target = job
	if kind == application.DisplayWaiterRegistration {
		waiter, err := fixture.awaitWaiter(ctx, job, 1)
		if err != nil {
			return err
		}
		fixture.id = waiter.Token
	} else {
		if err = fixture.awaitStarted(ctx, job); err != nil {
			return err
		}
		fixture.id = job.token
	}
	return fixture.captureNativeAnnotations()
}

func (fixture *annotationBDD) retire() error {
	if fixture.adapter.Name == contract.Unit {
		return application.RetireDisplay(application.DisplayRetirement{RunID: fixture.id, Ownership: application.DisplayOwnershipOwned}, application.DisplayRetirementEffects{
			RemoveOwnership: func() (application.DisplayRetirementResult, error) {
				fixture.events = append(fixture.events, "retire-owned")
				before := len(fixture.ledger.Owners) + len(fixture.ledger.Waiters)
				fixture.ledger.Waiters = slices.DeleteFunc(fixture.ledger.Waiters, func(waiter coordination.Waiter) bool { return waiter.Token == fixture.id })
				fixture.ledger.Owners = slices.DeleteFunc(fixture.ledger.Owners, func(owner coordination.ReservationOwner) bool { return owner.Token == fixture.id })
				return application.DisplayRetirementResult{Confirmed: before > len(fixture.ledger.Owners)+len(fixture.ledger.Waiters)}, nil
			},
			Remove: func(id string) error {
				fixture.events = append(fixture.events, "retire-display")
				delete(fixture.annotations, id)
				return nil
			},
		}, &application.DisplayAdvisories{})
	}
	switch fixture.ending {
	case "child completion":
		return fixture.finish(fixture.target)
	case "cancellation in queue":
		fixture.target.cancel()
		return fixture.awaitResult(fixture.target, 130)
	case annotationConfirmedForcedStop:
		fixture.target.cancel()
		return fixture.awaitResult(fixture.target, 128+int(syscall.SIGKILL))
	case "admission deferral":
		return fixture.awaitResult(fixture.target, 124)
	default:
		return fmt.Errorf("unknown canonical retirement ending %q", fixture.ending)
	}
}

func (fixture *annotationBDD) requireRetired() error {
	_, err := fixture.annotation(fixture.id)
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("owned retirement retained annotation: %w", err)
	}
	if fixture.adapter.Name == contract.Unit {
		if len(fixture.events) < 2 || !reflect.DeepEqual(fixture.events[len(fixture.events)-2:], []string{"retire-owned", "retire-display"}) {
			return errors.New("application annotation removal preceded positive ownership retirement")
		}
		for _, owner := range fixture.ledger.Owners {
			if owner.Token == fixture.id {
				return errors.New("unit actual owner transition retained token")
			}
		}
		for _, waiter := range fixture.ledger.Waiters {
			if waiter.Token == fixture.id {
				return errors.New("unit actual waiter transition retained token")
			}
		}
		return nil
	}
	ledger, err := fixture.readLedger()
	if err != nil {
		return err
	}
	for _, owner := range ledger.Owners {
		if owner.Token == fixture.id {
			return errors.New("native released token retained ownership")
		}
	}
	for _, waiter := range ledger.Waiters {
		if waiter.Token == fixture.id {
			return errors.New("native cancelled token retained FIFO ownership")
		}
	}
	if fixture.kind == application.DisplayWaiterRegistration {
		if _, err = os.Stat(fixture.target.ready); !errors.Is(err, os.ErrNotExist) {
			return errors.New("cancelled/deferred queued invocation launched its child")
		}
	}
	if fixture.ending == "confirmed forced stop" {
		if fixture.target.childPID <= 0 {
			return errors.New("forced stop lacks actual owned child PID")
		}
		if err = syscall.Kill(fixture.target.childPID, 0); !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("forced-stop child still exists after native join: %w", err)
		}
	}
	return nil
}

func (fixture *annotationBDD) requireOtherOwner() error {
	if fixture.adapter.Name == contract.Unit {
		if fixture.annotations[annotationBDDOther] != fixture.before[annotationBDDOther] {
			return errors.New("retirement changed other owner's context")
		}
	} else {
		data, err := fixture.readNativeFile(filepath.Join("run-display", fixture.owner.token+".json"))
		if err != nil {
			return err
		}
		if !bytes.Equal(data, fixture.nativeBefore[fixture.owner.token+".json"]) {
			return errors.New("retirement changed other native owner's annotation bytes")
		}
	}
	return fixture.close()
}

func (fixture *annotationBDD) prepareCleanup(ctx context.Context) error {
	if err := fixture.prepare(application.DisplayReservationRegistration, "live-build"); err != nil {
		return err
	}
	if fixture.adapter.Name == contract.Unit {
		fixture.id = annotationBDDOther
		fixture.staleID = annotationBDDToken
		fixture.annotations = map[string]string{fixture.staleID: "stale-build", fixture.id: "live-build"}
		return nil
	}
	if err := fixture.writeNativeFile("config.json", []byte(`{"schemaVersion":2,"coordination":{"mode":"reservation","maxActiveOwners":2}}`)); err != nil {
		return err
	}
	if err := fixture.register(ctx); err != nil {
		return err
	}
	fixture.owner = fixture.target
	stale, err := fixture.start(ctx, "stale-build", "", false)
	if err != nil {
		return err
	}
	if err = fixture.awaitStarted(ctx, stale); err != nil {
		return err
	}
	fixture.staleID = stale.token
	path := filepath.Join("run-display", stale.token+".json")
	data, err := fixture.readNativeFile(path)
	if err != nil {
		return err
	}
	if err = fixture.finish(stale); err != nil {
		return err
	}
	// Recreate the exact old-retirement sidecar after its genuine owned token
	// and anchored identity retired, as an older client that ignored labels does.
	return fixture.writeNativeFile(path, data)
}

func (fixture *annotationBDD) prepareUnproven() error {
	if fixture.adapter.Name == contract.Unit {
		fixture.annotations[annotationBDDUnproven] = "held-unproven"
		fixture.before = maps.Clone(fixture.annotations)
		return nil
	}
	if fixture.id == annotationBDDUnproven || fixture.staleID == annotationBDDUnproven {
		return errors.New("fresh native owner collided with isolated unproven token")
	}
	// This genuine held, anchored identity is deliberately absent from the ledger.
	// It is native census evidence, not a token-only fake or a copied policy decision.
	directory := filepath.Join(fixture.root, "reservation-identities")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	path := filepath.Join(directory, annotationBDDUnproven+".lock")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	fixture.unproven = file
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	if err = os.Link(path, filepath.Join(directory, annotationBDDUnproven+".anchor")); err != nil {
		return err
	}
	label, err := identity.ParseDisplayLabel("held-unproven")
	if err != nil {
		return err
	}
	if err = evidence.WriteDisplay(fixture.root, annotationBDDUnproven, label); err != nil {
		return err
	}
	return fixture.captureNativeAnnotations()
}

func (fixture *annotationBDD) cleanup(ctx context.Context) error {
	if fixture.adapter.Name == contract.Unit {
		return application.CleanupDisplay(application.DisplayCleanupEffects{
			PruneTemporaries: func() error { return nil },
			Census: func() (application.DisplayCensus, error) {
				return application.DisplayCensus{Complete: true, Liveness: []application.DisplayLiveness{{RunID: fixture.staleID, Live: false}, {RunID: fixture.id, Live: false}, {RunID: fixture.id, Live: true}, {RunID: annotationBDDUnproven, Live: true}}}, nil
			},
			SupportedIDs: func() ([]string, error) { return []string{fixture.staleID, fixture.id, annotationBDDUnproven}, nil },
			Prune: func(ids []string) error {
				for _, id := range ids {
					delete(fixture.annotations, id)
				}
				return nil
			},
		})
	}
	if fixture.adapter.Name == contract.Integration {
		_, err := runtimeadapter.NewEngine().Snapshot(ctx, fixture.root, "reservation")
		return err
	}
	var output, diagnostic bytes.Buffer
	env, err := fixture.neutral.PrepareNativeAnnotationEnvironment([]string{"HIPPO_ROOT=" + fixture.root, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"})
	if err != nil {
		return err
	}
	command, err := fixture.neutral.NativeAnnotationCommand(ctx, "status", "--config", fixture.config)
	if err != nil {
		return err
	}
	command.Env = env
	command.Dir = fixture.root
	command.Stdout = &output
	command.Stderr = &diagnostic
	if err = command.Run(); err != nil {
		return fmt.Errorf("public locked reconciliation failed: %w: %s", err, diagnostic.String())
	}
	return nil
}

func (fixture *annotationBDD) requireStaleRemoved() error {
	_, err := fixture.annotation(fixture.staleID)
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("complete native/application census retained proven stale annotation: %w", err)
	}
	return nil
}

func (fixture *annotationBDD) requireCleanupRetained() error {
	if fixture.adapter.Name == contract.Unit {
		for _, id := range []string{fixture.id, annotationBDDUnproven} {
			if fixture.annotations[id] != fixture.before[id] {
				return errors.New("application census changed live/unproven annotation")
			}
		}
	} else {
		for _, id := range []string{fixture.id, annotationBDDUnproven} {
			data, err := fixture.readNativeFile(filepath.Join("run-display", id+".json"))
			if err != nil {
				return err
			}
			if !bytes.Equal(data, fixture.nativeBefore[id+".json"]) {
				return errors.New("native census changed live/unproven annotation bytes")
			}
		}
	}
	return fixture.close()
}

func (fixture *annotationBDD) readLedger() (coordination.Ledger, error) {
	var ledger coordination.Ledger
	data, err := os.ReadFile(filepath.Join(fixture.root, "reservations.json"))
	if err != nil {
		return ledger, err
	}
	if err = json.Unmarshal(data, &ledger); err != nil {
		return ledger, err
	}
	return ledger, coordination.ValidateLedger(ledger)
}

func (fixture *annotationBDD) captureNativeAnnotations() error {
	entries, err := os.ReadDir(filepath.Join(fixture.root, "run-display"))
	if err != nil {
		return err
	}
	fixture.nativeBefore = make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			data, err := fixture.readNativeFile(filepath.Join("run-display", entry.Name()))
			if err != nil {
				return err
			}
			fixture.nativeBefore[entry.Name()] = data
		}
	}
	return nil
}

func (fixture *annotationBDD) requireNativeAnnotationsUnchanged() error {
	before := fixture.nativeBefore
	if err := fixture.captureNativeAnnotations(); err != nil {
		return err
	}
	if !reflect.DeepEqual(before, fixture.nativeBefore) {
		return errors.New("nested native invocation replaced or added annotation")
	}
	return nil
}

// All waits are bounded by owned invocation contexts and actual observations.
// A Wait result is buffered exactly once and read only after the producer joins.
type annotationBDDJob struct {
	scratch, ready, release, token string
	cancel                         context.CancelFunc
	command                        *exec.Cmd
	joined                         chan struct{}
	output, diagnostic             bytes.Buffer
	code                           int
	childPID                       int
	err                            error
	pipeReader, pipeWriter         *os.File
	pipeJoined                     chan struct{}
}

type annotationBDDCollector struct{}

func (annotationBDDCollector) Collect(ctx context.Context, _ policy.CPUState, _ string) (policy.Reading, error) {
	if err := ctx.Err(); err != nil {
		return policy.Reading{}, err
	}
	return policy.Reading{Sample: policy.Sample{SchemaVersion: 3, MeasuredAt: time.Now().UTC().Format(time.RFC3339Nano), Platform: darwinPlatform, Capabilities: []string{"compressor", "memory-pressure", "swap"}, EffectiveMemoryLimitBytes: 32 * policy.GiB, PhysicalMemoryBytes: 32 * policy.GiB, AvailableMemoryBytes: new(12 * policy.GiB), AvailableNonCompressedEstimateBytes: new(12 * policy.GiB), MemoryPressureLevel: new(1), CompressorAvailable: new(true), CompressorPayloadBytes: new(int64(0)), AvailableParallelism: 8, CPUUtilizationPercent: new(0.0), DiskFreeBytes: new(40 * policy.GiB), DiskTotalBytes: new(512 * policy.GiB), SwapIns: new(int64(0)), SwapOuts: new(int64(0)), SwapFreeBytes: new(2 * policy.GiB), SwapState: idleSwapState}}, nil
}

// annotationInvocationContext mirrors the public process entry's signal cause
// only for embedded Integration invocations; the real public process is signaled.
func annotationInvocationContext(parent context.Context, adapter contract.Adapter) (context.Context, context.CancelFunc) {
	bounded, cancelBound := context.WithTimeout(parent, 45*time.Second)
	if adapter.Name != contract.Integration {
		return bounded, cancelBound
	}
	ctx, interrupt := context.WithCancelCause(bounded)
	return ctx, func() {
		interrupt(status.Interruption{Signal: int(syscall.SIGINT)})
		cancelBound()
	}
}

func (fixture *annotationBDD) start(parent context.Context, label, inherited string, printf bool) (*annotationBDDJob, error) {
	if fixture.adapter.Name == contract.Unit {
		return nil, errors.New("unit must not enter native fixture")
	}
	ctx, cancel := annotationInvocationContext(parent, fixture.adapter)
	job := &annotationBDDJob{cancel: cancel, joined: make(chan struct{})}
	fixture.jobs = append(fixture.jobs, job)
	directory, err := os.MkdirTemp(fixture.root, "invocation-")
	if err != nil {
		cancel()
		close(job.joined)
		return nil, err
	}
	job.scratch = directory
	job.ready = filepath.Join(directory, "ready")
	job.release = filepath.Join(directory, "release")
	environment := []string{"HIPPO_ROOT=" + fixture.root, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "ANNOTATION_READY=" + job.ready, "ANNOTATION_RELEASE=" + job.release}
	if inherited != "" {
		environment = append(environment, "HIPPO_SESSION="+inherited)
	}
	args := []string{"run", "--config", fixture.config, "--cwd", directory, "--disk-path", directory, "--profile", profileConstrained, taskClassFlag, "service"}
	if label != "" {
		args = append(args, "--label", label)
	}
	if fixture.kind != application.DisplayExclusiveRegistration {
		args = append(args, "--reserve-cpu", "1", "--reserve-memory-mib", "256")
	}
	if fixture.ending == "admission deferral" && label == "retire-build" {
		args = append(args, "--wait-for-admission", "5s")
	}
	terminationTrap := `trap 'exit 130' TERM; `
	if fixture.ending == "confirmed forced stop" && label == "retire-build" {
		terminationTrap = `trap '' TERM; `
	}
	script := terminationTrap + `printf '%s' "$$" > "$ANNOTATION_READY.pid"; printf '%s' "$HIPPO_SESSION" > "$ANNOTATION_READY.tmp"; /bin/mv "$ANNOTATION_READY.tmp" "$ANNOTATION_READY"; while [ ! -e "$ANNOTATION_RELEASE" ]; do /bin/sleep 0.01; done; exit 0`
	args = append(args, "--", "/bin/sh", "-c", script, "private argument")
	var stdout io.Writer = &job.output
	if printf {
		// The actual printf child blocks on its owned output pipe until assertions
		// inspect the live annotation, rather than racing a fast child exit.
		job.pipeReader, job.pipeWriter, err = os.Pipe()
		if err != nil {
			cancel()
			close(job.joined)
			return nil, err
		}
		stdout = job.pipeWriter
		args = args[:len(args)-5]
		args = append(args, "--", "/usr/bin/printf", "%1048576s", "private text")
	}
	if fixture.adapter.Name == contract.E2E {
		if err := fixture.startPublicJob(ctx, job, args, environment, stdout); err != nil {
			cancel()
			close(job.joined)
			return nil, err
		}
	} else {
		go func() {
			defer close(job.joined)
			boundary := bootstrap.WithDefaults(cli.Application{Stdin: strings.NewReader(""), Stdout: stdout, Stderr: &job.diagnostic, Environment: environment, Collector: annotationBDDCollector{}, Sleep: func(time.Duration) {}})
			job.code, job.err = boundary.Run(ctx, args)
		}()
	}
	return job, nil
}

func (fixture *annotationBDD) startPublicJob(ctx context.Context, job *annotationBDDJob, args, environment []string, stdout io.Writer) error {
	environment, err := fixture.neutral.PrepareNativeAnnotationEnvironment(environment)
	if err != nil {
		return err
	}
	command, err := fixture.neutral.NativeAnnotationCommand(ctx, args...)
	if err != nil {
		return err
	}
	job.command = command
	command.Dir = job.scratch
	command.Env = environment
	command.Stdin = strings.NewReader("")
	command.Stdout = stdout
	command.Stderr = &job.diagnostic
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		err := command.Process.Signal(os.Interrupt)
		if errors.Is(err, os.ErrProcessDone) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 15 * time.Second
	if err = command.Start(); err != nil {
		return err
	}
	go func() {
		defer close(job.joined)
		job.err = command.Wait()
		if job.err != nil {
			if exitError, ok := errors.AsType[*exec.ExitError](job.err); ok {
				job.code = exitError.ExitCode()
			} else {
				job.code = 125
			}
		}
	}()
	return nil
}

func (fixture *annotationBDD) await(ctx context.Context, job *annotationBDDJob, observe func() (bool, error)) error {
	bound := time.NewTimer(30 * time.Second)
	defer bound.Stop()
	ticks := time.NewTicker(time.Millisecond)
	defer ticks.Stop()
	for {
		ready, err := observe()
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-bound.C:
			return errors.New("actual native annotation rendezvous exceeded bound")
		case <-job.joined:
			return fmt.Errorf("native guard exited before rendezvous: %d/%w/%s", job.code, job.err, job.diagnostic.String())
		case <-ticks.C:
		}
	}
}

func (fixture *annotationBDD) awaitStarted(ctx context.Context, job *annotationBDDJob) error {
	return fixture.await(ctx, job, func() (bool, error) {
		if job.pipeReader != nil {
			ledger, err := fixture.readLedger()
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			for _, owner := range ledger.Owners {
				job.token = owner.Token
				return true, nil
			}
			return false, nil
		}
		data, err := os.ReadFile(job.ready)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(data) != 32 {
			return false, errors.New("native child exposed incomplete owned token")
		}
		job.token = string(data)
		pid, err := os.ReadFile(job.ready + ".pid")
		if err != nil {
			return false, err
		}
		job.childPID, err = strconv.Atoi(string(pid))
		if err != nil || job.childPID <= 0 {
			return false, errors.New("native child exposed invalid owned PID")
		}
		return true, nil
	})
}

func (fixture *annotationBDD) awaitWaiter(ctx context.Context, job *annotationBDDJob, position int) (coordination.Waiter, error) {
	var waiter coordination.Waiter
	err := fixture.await(ctx, job, func() (bool, error) {
		ledger, err := fixture.readLedger()
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(ledger.Waiters) < position {
			return false, nil
		}
		waiter = ledger.Waiters[position-1]
		job.token = waiter.Token
		return true, nil
	})
	return waiter, err
}

func (fixture *annotationBDD) finish(job *annotationBDDJob) error {
	if job.pipeReader != nil && job.pipeJoined == nil {
		job.pipeJoined = make(chan struct{})
		go func() {
			defer close(job.pipeJoined)
			_, err := io.Copy(io.Discard, job.pipeReader)
			if err != nil && !errors.Is(err, os.ErrClosed) {
				job.cancel()
			}
		}()
	} else if job.pipeReader == nil {
		if err := os.WriteFile(job.release, nil, 0o600); err != nil {
			return err
		}
	}
	return fixture.awaitResult(job, 0)
}

func (fixture *annotationBDD) awaitResult(job *annotationBDDJob, want int) error {
	bound := time.NewTimer(30 * time.Second)
	defer bound.Stop()
	select {
	case <-job.joined:
	case <-bound.C:
		job.cancel()
		return errors.New("owned native annotation producer did not join")
	}
	if job.pipeWriter != nil {
		if err := job.pipeWriter.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			return err
		}
		if job.pipeJoined != nil {
			select {
			case <-job.pipeJoined:
			case <-bound.C:
				return errors.New("owned printf output consumer did not join")
			}
		}
	}
	job.cancel()
	if job.code != want {
		return fmt.Errorf("actual native outcome=%d want=%d: %w/%s", job.code, want, job.err, job.diagnostic.String())
	}
	return nil
}

func (fixture *annotationBDD) close() error {
	var failures []error
	// End queued children first, while the capacity owner is still held. This
	// prevents fixture teardown from accidentally admitting cancelled waiters.
	for _, job := range slices.Backward(fixture.jobs) {
		if err := fixture.closeJob(job); err != nil {
			failures = append(failures, err)
		}
	}
	if fixture.unproven != nil {
		if err := fixture.unproven.Close(); err != nil {
			failures = append(failures, err)
		}
		fixture.unproven = nil
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	fixture.jobs = nil
	if fixture.root != "" {
		if err := fixture.removeNativeRoot(); err != nil {
			return err
		}
		fixture.root = ""
	}
	return nil
}

func (fixture *annotationBDD) closeJob(job *annotationBDDJob) error {
	var failures []error

	select {
	case <-job.joined:
	default:
		if job.pipeReader != nil {
			if err := fixture.finish(job); err != nil {
				failures = append(failures, err)
			}
		} else {
			job.cancel()
			if err := fixture.awaitResult(job, 130); err != nil {
				failures = append(failures, err)
			}
		}
	}
	if job.pipeReader != nil {
		if err := job.pipeReader.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			failures = append(failures, err)
		}
	}
	if job.pipeWriter != nil {
		if err := job.pipeWriter.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			failures = append(failures, err)
		}
	}
	if job.pipeJoined != nil {
		bound := time.NewTimer(5 * time.Second)
		select {
		case <-job.pipeJoined:
		case <-bound.C:
			failures = append(failures, errors.New("owned printf consumer did not join during teardown"))
		}
		bound.Stop()
	}
	job.cancel()
	return errors.Join(failures...)
}

func (fixture *annotationBDD) Reset() {
	if err := fixture.close(); err != nil {
		panic(fmt.Sprintf("owned annotation fixture cleanup failed: %v", err))
	}
	adapter, neutral := fixture.adapter, fixture.neutral
	*fixture = annotationBDD{adapter: adapter, neutral: neutral}
}

func (fixture *annotationBDD) Close() {
	if err := fixture.close(); err != nil {
		panic(fmt.Sprintf("owned annotation fixture cleanup failed: %v", err))
	}
}
