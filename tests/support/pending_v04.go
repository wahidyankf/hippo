package support

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	resourceconfig "github.com/wahidyankf/hippo/internal/config"
	"github.com/wahidyankf/hippo/internal/conformance"
	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

func (driver *Driver) preparePendingV04() error {
	if driver.evidenceRoot != "" {
		return nil
	}
	root, err := os.MkdirTemp("", "hippo-v04-behaviour-")
	if err != nil {
		return err
	}
	driver.evidenceRoot = root
	driver.temporaryPaths = append(driver.temporaryPaths, root)

	return nil
}

func v04ReservationPolicy() guard.ReservationPolicy {
	return guard.ReservationPolicy{
		Enabled: true, MaxActiveOwners: 20,
		OwnerShares: map[string]int{profileBalanced: 4, profileConstrained: 2, profileMinimal: 1},
	}
}

func v04ReservationSample() policy.Sample {
	sample := healthySample(time.Now())
	sample.AvailableParallelism = 9
	sample.EffectiveMemoryLimitBytes = 32 * policy.GiB

	return sample
}

func v04FastPolicy() policy.Policy {
	result := policy.DefaultPolicy()
	result.SampleInterval = time.Millisecond
	result.AdmissionWindow = time.Second
	result.TerminationGrace = time.Millisecond
	result.LeaseWait = time.Second

	return result
}

func v04Plan(cpu int, memory int64) guard.ReservationPlan {
	vector := guard.ReservationVector{CPU: cpu, MemoryBytes: memory}

	return guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: 4, MemoryBytes: policy.GiB},
		Requested: vector, Allocated: vector,
	}
}

func requireV04Planning() error {
	settings := v04ReservationPolicy()
	for profile, expectedCPU := range map[string]int{profileBalanced: 2, profileConstrained: 4, profileMinimal: 8} {
		plan, err := guard.PlanReservation(
			v04ReservationSample(),
			policy.Resolution{ResolvedProfile: profile, MemoryReserve: 4 * policy.GiB},
			settings, 0, 0,
		)
		if err != nil || plan.Requested.CPU != expectedCPU {
			return fmt.Errorf("%s automatic share: %+v: %w", profile, plan, err)
		}
	}
	return nil
}

func requireV04ExplicitPlanning() error {
	plan, err := guard.PlanReservation(
		v04ReservationSample(), policy.Resolution{ResolvedProfile: profileBalanced, MemoryReserve: 4 * policy.GiB},
		v04ReservationPolicy(), 1, 256*policy.MiB,
	)
	if err != nil || plan.Requested != (guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}) || plan.Allocated != plan.Requested {
		return fmt.Errorf("explicit reservation was not fixed unchanged: %+v: %w", plan, err)
	}

	return nil
}

func requireV04ReservationFloors() error {
	for _, request := range []guard.ReservationVector{
		{CPU: -1, MemoryBytes: 256 * policy.MiB},
		{CPU: 1, MemoryBytes: 255 * policy.MiB},
	} {
		if _, err := guard.PlanReservation(
			v04ReservationSample(), policy.Resolution{ResolvedProfile: profileBalanced, MemoryReserve: 4 * policy.GiB},
			v04ReservationPolicy(), request.CPU, request.MemoryBytes,
		); !errors.Is(err, guard.ErrReservationReplan) {
			return fmt.Errorf("unsafe reservation %+v did not require replanning: %w", request, err)
		}
	}

	return nil
}

func requireV04Configuration(root string) error {
	exclusivePath := filepath.Join(root, "exclusive.json")
	reservationPath := filepath.Join(root, "reservation.json")
	if err := os.WriteFile(exclusivePath, []byte(schemaOneDocument), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(reservationPath, []byte(`{"schemaVersion":2,"coordination":{"maxCpu":8,"maxMemoryMiB":4096,"maxActiveOwners":8}}`), 0o600); err != nil {
		return err
	}
	exclusive, err := resourceconfig.Load(exclusivePath, true)
	if err != nil {
		return err
	}
	reservation, err := resourceconfig.Load(reservationPath, true)
	if err != nil {
		return err
	}
	if exclusive.Coordination.Mode != exclusiveMode || reservation.Coordination.Mode != reservationMode ||
		reservation.Coordination.MaxCPU != 8 || reservation.Coordination.MaxMemoryBytes != 4*policy.GiB {
		return errors.New("schema 1/2 coordination compatibility failed")
	}

	return nil
}

func requireV04Environment() error {
	environment, err := guard.ReservationEnvironment(
		[]string{"LOWER=1", "HIGHER=9"}, policy.Resolution{ResolvedProfile: profileConstrained},
		guard.ReservationVector{CPU: 2, MemoryBytes: 512 * policy.MiB}, []string{"MISSING", "LOWER", "HIGHER"},
	)
	if err != nil {
		return err
	}
	for _, expected := range []string{"HIPPO_CONCURRENCY=2", "HIPPO_RESERVED_MEMORY_BYTES=536870912", "MISSING=2", "LOWER=1", "HIGHER=2"} {
		if !slices.Contains(environment, expected) {
			return fmt.Errorf("fixed allocation environment lacks %q", expected)
		}
	}
	if _, err = guard.ReservationEnvironment(
		[]string{"BAD=zero"}, policy.Resolution{ResolvedProfile: profileBalanced},
		guard.ReservationVector{CPU: 2, MemoryBytes: 512 * policy.MiB}, []string{"BAD"},
	); err == nil {
		return errors.New("malformed concurrency mapping was accepted")
	}

	return nil
}

func requireV04AllClasses(root string) error {
	plan := v04Plan(1, 256*policy.MiB)
	sessions := make([]*guard.Session, 0, 3)
	for _, class := range []policy.TaskClass{policy.TaskService, policy.TaskEphemeral, policy.TaskTransactional} {
		session, err := guard.AcquireReservation(context.Background(), root, "", class, profileBalanced, "", plan, 20, 0)
		if err != nil || session == nil {
			return fmt.Errorf("admit %s: %w", class, err)
		}
		sessions = append(sessions, session)
	}
	defer func() {
		for _, session := range sessions {
			_ = guard.ReleaseReservation(root, session)
		}
	}()

	totals, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || totals.SchemaVersion != 4 || totals.ActiveOwners != 3 ||
		totals.Service != 1 || totals.Ephemeral != 1 || totals.Transactional != 1 || totals.Allocated.CPU != 3 {
		return fmt.Errorf("all-class schema 4 totals: %+v: %w", totals, err)
	}

	return nil
}

func requireV04AtomicAdmission(root string) error {
	owner, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, profileBalanced, "",
		v04Plan(3, 512*policy.MiB), 20, 0,
	)
	if err != nil || owner == nil {
		return fmt.Errorf("create asymmetric owner: %w", err)
	}
	defer func() { _ = guard.ReleaseReservation(root, owner) }()
	candidate, acquireError := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(2, 256*policy.MiB), 20, 0,
	)
	if candidate != nil || !errors.Is(acquireError, guard.ErrReservationDeferred) {
		return fmt.Errorf("asymmetric vector was partially admitted: session=%+v error=%w", candidate, acquireError)
	}
	totals, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || totals.Allocated != owner.Allocation {
		return fmt.Errorf("failed vector changed allocation: totals=%+v error=%w", totals, err)
	}

	return nil
}

func requireV04ImpossibleReservation(root string) error {
	plan := v04Plan(1, 256*policy.MiB)
	plan.Requested.CPU, plan.Allocated.CPU = 5, 5
	session, err := guard.AcquireReservation(context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "", plan, 20, 0)
	if session != nil || !errors.Is(err, guard.ErrReservationReplan) {
		return fmt.Errorf("impossible reservation did not replan: session=%+v error=%w", session, err)
	}

	return nil
}

func requireV04TemporaryExhaustion(root string) error {
	owner, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, profileBalanced, "",
		v04Plan(4, policy.GiB), 20, 0,
	)
	if err != nil || owner == nil {
		return fmt.Errorf("create exhausted capacity: %w", err)
	}
	defer func() { _ = guard.ReleaseReservation(root, owner) }()
	started := time.Now()
	session, acquireError := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 30*time.Millisecond,
	)
	if session != nil || !errors.Is(acquireError, guard.ErrReservationDeferred) {
		return fmt.Errorf("temporary exhaustion did not defer: session=%+v error=%w", session, acquireError)
	}
	if time.Since(started) < 25*time.Millisecond {
		return errors.New("temporary exhaustion did not honor its bounded wait")
	}

	return nil
}

func requireV04FIFO(root string) error {
	owner, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, profileBalanced, "",
		v04Plan(2, 512*policy.MiB), 20, 0,
	)
	if err != nil || owner == nil {
		return fmt.Errorf("create FIFO owner: %w", err)
	}
	result, errorsFound := make(chan *guard.Session, 1), make(chan error, 1)
	go func() {
		session, acquireError := guard.AcquireReservation(
			context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
			v04Plan(3, 512*policy.MiB), 20, time.Second,
		)
		result <- session
		errorsFound <- acquireError
	}()
	deadline := time.Now().Add(fixtureLivenessWait)
	for {
		totals, statusError := guard.ReservationStatus(context.Background(), root)
		if statusError == nil && totals.WaitingOwners == 1 {
			break
		}
		if time.Now().After(deadline) {
			_ = guard.ReleaseReservation(root, owner)

			return errors.New("large FIFO head did not enqueue")
		}
		time.Sleep(time.Millisecond)
	}
	small, smallError := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskTransactional, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 50*time.Millisecond,
	)
	if small != nil || !errors.Is(smallError, guard.ErrReservationDeferred) && !guard.IsCoordinationDeferred(smallError) {
		_ = guard.ReleaseReservation(root, owner)

		return fmt.Errorf("small waiter bypassed FIFO head: session=%+v error=%w", small, smallError)
	}
	if err = releaseContendedReservation(root, owner); err != nil {
		return err
	}
	large, largeError := <-result, <-errorsFound
	if large == nil || largeError != nil {
		return fmt.Errorf("FIFO head was not admitted after release: session=%+v error=%w", large, largeError)
	}

	return guard.ReleaseReservation(root, large)
}

func requireV04Inheritance(root string) error {
	plan := v04Plan(1, 256*policy.MiB)
	owner, err := guard.AcquireReservation(context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "", plan, 20, 0)
	if err != nil || owner == nil {
		return fmt.Errorf("create inheritable reservation: %w", err)
	}
	defer func() { _ = guard.ReleaseReservation(root, owner) }()
	before, err := guard.ReservationStatus(context.Background(), root)
	if err != nil {
		return err
	}
	inherited, err := guard.AcquireReservation(
		context.Background(), root, owner.Token, policy.TaskEphemeral, profileMinimal, "",
		v04Plan(4, policy.GiB), 20, 0,
	)
	if err != nil || inherited == nil || !inherited.Inherited || inherited.Allocation != plan.Allocated {
		return fmt.Errorf("inherited reservation expanded or failed: session=%+v error=%w", inherited, err)
	}
	after, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || after.ActiveOwners != before.ActiveOwners || after.Allocated != before.Allocated {
		return fmt.Errorf("inheritance double reserved: before=%+v after=%+v error=%w", before, after, err)
	}

	return nil
}

func requireV04StaleIdentity(root string) error {
	tokenValue := strings.Repeat("a", 32)
	floor := guard.ReservationVector{CPU: 4, MemoryBytes: policy.GiB}
	ledger := map[string]any{
		schemaVersionField: 2, capacityField: floor, nextSequenceField: 1,
		ownersField: []any{map[string]any{
			tokenField: tokenValue, pidField: os.Getpid(), classField: string(policy.TaskEphemeral), profileField: profileBalanced,
			requestedField: floor, "allocated": floor, sequenceField: 1, maxActiveOwnersFld: 20,
		}},
		waitersField: []any{},
	}
	data, err := json.Marshal(ledger)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "coordination-mode.json"), []byte("{\"schemaVersion\":1,\"mode\":\"reservation\"}\n"), 0o600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "reservations.json"), append(data, '\n'), 0o600); err != nil {
		return err
	}
	fresh, err := guard.AcquireReservation(
		context.Background(), root, tokenValue, policy.TaskTransactional, profileMinimal, "",
		v04Plan(4, policy.GiB), 20, 0,
	)
	if err != nil || fresh == nil || fresh.Inherited {
		return fmt.Errorf("stale PID-reuse record retained capacity: session=%+v error=%w", fresh, err)
	}

	return guard.ReleaseReservation(root, fresh)
}

func requireV04ThresholdAuthority(root string) error {
	critical := healthySample(time.Now())
	level := 4
	critical.MemoryPressureLevel = &level
	childMarker := filepath.Join(root, "child-started")
	policySettings := v04FastPolicy()
	policySettings.AdmissionWindow = 0
	exitCode, err := guard.Run(context.Background(), guard.RunConfig{
		Command: shellPath, Arguments: []string{"-c", childStartedScript}, TaskClass: policy.TaskEphemeral,
		Environment: []string{"CHILD_MARKER=" + childMarker}, EvidenceRoot: root,
		Collector: &sequenceCollector{samples: []policy.Sample{critical}}, Policy: policySettings,
		Resolution:        policy.Resolution{RequestedProfile: profileBalanced, ResolvedProfile: profileBalanced, Concurrency: 1},
		ReservationPolicy: v04ReservationPolicy(), ReservationPlan: v04Plan(1, 256*policy.MiB),
		ChildStdin: bytes.NewBuffer(nil), ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
	})
	if err != nil || exitCode != guard.CapacityDeferredExitCode {
		return fmt.Errorf("threshold-blocked reservation did not defer: exit=%d error=%w", exitCode, err)
	}
	if _, statError := os.Stat(childMarker); !errors.Is(statError, os.ErrNotExist) {
		return errors.New("threshold-blocked reservation started its child")
	}
	if _, statError := os.Stat(filepath.Join(root, "reservations.json")); !errors.Is(statError, os.ErrNotExist) {
		return errors.New("threshold-blocked reservation left ledger allocation")
	}

	return nil
}

func acquireV04Victim(root string, class policy.TaskClass, processGroup int) (*guard.Session, error) {
	session, err := guard.AcquireReservation(
		context.Background(), root, "", class, profileBalanced, "", v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		return nil, err
	}
	if err = guard.ActivateReservation(root, session, processGroup); err != nil {
		_ = guard.ReleaseReservation(root, session)

		return nil, err
	}

	return session, nil
}

func requireV04NewestEphemeral(root string) error {
	transactional, err := acquireV04Victim(root, policy.TaskTransactional, 21_001)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, transactional) }()
	service, err := acquireV04Victim(root, policy.TaskService, 21_002)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, service) }()
	older, err := acquireV04Victim(root, policy.TaskEphemeral, 21_003)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, older) }()
	newer, err := acquireV04Victim(root, policy.TaskEphemeral, 21_004)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, newer) }()
	victim, selected, err := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
	if err != nil || !selected || victim.Token != newer.Token {
		return fmt.Errorf("newest ephemeral was not selected first: victim=%+v selected=%v error=%w", victim, selected, err)
	}

	return nil
}

func requireV04NewestService(root string) error {
	older, err := acquireV04Victim(root, policy.TaskService, 22_001)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, older) }()
	newer, err := acquireV04Victim(root, policy.TaskService, 22_002)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, newer) }()
	victim, selected, err := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
	if err != nil || !selected || victim.Token != newer.Token {
		return fmt.Errorf("newest service was not selected: victim=%+v selected=%v error=%w", victim, selected, err)
	}

	return nil
}

func requireV04TransactionalProtection(root string) error {
	full := v04Plan(4, policy.GiB)
	transactional, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskTransactional, profileBalanced, "", full, 20, 0,
	)
	if err != nil || transactional == nil {
		return fmt.Errorf("create protected transaction: %w", err)
	}
	defer func() { _ = guard.ReleaseReservation(root, transactional) }()
	if err = guard.ActivateReservation(root, transactional, 23_001); err != nil {
		return err
	}
	if _, selected, selectionError := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode); selectionError != nil || selected {
		return fmt.Errorf("transactional owner was selected: selected=%v error=%w", selected, selectionError)
	}
	other, acquireError := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "", v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if other != nil || !errors.Is(acquireError, guard.ErrReservationDeferred) {
		return fmt.Errorf("admission bypassed protected transaction: session=%+v error=%w", other, acquireError)
	}

	return nil
}

func requireV04Bridge(root string) error {
	exclusive, err := guard.AcquireSession(context.Background(), root, "", policy.TaskService, 0)
	if err != nil || exclusive == nil {
		return fmt.Errorf("create compatibility owner: %w", err)
	}
	defer guard.ReleaseSession(root, exclusive) //nolint:errcheck // The asserted operation precedes cleanup.

	reserved, reserveError := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if reserved != nil || reserveError == nil {
		return errors.New("reservation mode took over a live exclusive compatibility owner")
	}

	return nil
}

func (driver *Driver) malformedCompatibilityV04() error {
	if err := driver.preparePendingV04(); err != nil {
		return err
	}
	heavyPath := filepath.Join(driver.evidenceRoot, "heavy.lock")
	if err := os.MkdirAll(heavyPath, 0o700); err != nil {
		return err
	}
	driver.v04State = []byte("malformed-heavy-owner\n")

	return os.WriteFile(filepath.Join(heavyPath, "owner.json"), driver.v04State, 0o600)
}

func (driver *Driver) inspectMalformedCompatibilityV04() error {
	driver.v04Session, driver.v04Error = guard.AcquireReservation(
		context.Background(), driver.evidenceRoot, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)

	return nil
}

func (driver *Driver) requireMalformedCompatibilityDeferredV04() error {
	if driver.v04Session != nil || !guard.IsCoordinationDeferred(driver.v04Error) {
		return fmt.Errorf("malformed compatibility state did not fail closed: session=%+v error=%w", driver.v04Session, driver.v04Error)
	}
	data, err := os.ReadFile(filepath.Join(driver.evidenceRoot, "heavy.lock", "owner.json"))
	if err != nil || !bytes.Equal(data, driver.v04State) {
		return fmt.Errorf("malformed compatibility state changed: %q: %w", data, err)
	}

	return nil
}

func (driver *Driver) unsupportedCompatibilityV04() error {
	if err := driver.preparePendingV04(); err != nil {
		return err
	}
	heavyPath := filepath.Join(driver.evidenceRoot, "heavy.lock")
	if err := os.MkdirAll(heavyPath, 0o700); err != nil {
		return err
	}
	driver.v04State = []byte(`{"schemaVersion":2,"pid":2147483647}`)

	return os.WriteFile(filepath.Join(heavyPath, "owner.json"), driver.v04State, 0o600)
}

func (driver *Driver) inspectUnsupportedCompatibilityV04() error {
	driver.v04Session, driver.v04Error = guard.AcquireSession(
		context.Background(), driver.evidenceRoot, "", policy.TaskEphemeral, 0,
	)
	driver.output = guard.DescribeHeavyLease(driver.evidenceRoot)

	return nil
}

func (driver *Driver) requireUnsupportedCompatibilityV04() error {
	if driver.v04Session != nil || driver.v04Error != nil {
		return fmt.Errorf("unsupported compatibility state did not defer: session=%+v error=%w", driver.v04Session, driver.v04Error)
	}
	data, err := os.ReadFile(filepath.Join(driver.evidenceRoot, "heavy.lock", "owner.json"))
	if err != nil || !bytes.Equal(data, driver.v04State) {
		return fmt.Errorf("unsupported compatibility state changed: %q: %w", data, err)
	}
	if !strings.Contains(driver.output, "cannot be verified") || !strings.Contains(driver.output, "inspect") ||
		strings.Contains(driver.output, driver.evidenceRoot) {
		return fmt.Errorf("unsupported compatibility diagnostic was not private and actionable: %q", driver.output)
	}

	return nil
}

func requireV04SummaryAndRetention(root string) error {
	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(2, 512*policy.MiB), 20, 0,
	)
	if err != nil || session == nil {
		return fmt.Errorf("create summary reservation: %w", err)
	}
	defer func() { _ = guard.ReleaseReservation(root, session) }()
	totals, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || totals.SchemaVersion != 4 || totals.ActiveOwners != 1 || totals.Allocated != session.Allocation {
		return fmt.Errorf("schema 4 reservation status: %+v: %w", totals, err)
	}
	writer, err := guard.NewEvidenceWriter(root, "reservation-summary", evidence.Limits{})
	if err != nil {
		return err
	}
	writer.SetContext(policy.Resolution{RequestedProfile: profileBalanced, ResolvedProfile: profileBalanced, Concurrency: 2}, "")
	writer.SetReservationContext(session, totals.ActiveOwners, "admitted")
	if err = writer.Append(healthySample(time.Now())); err != nil {
		return err
	}
	summary, err := writer.Finalize(policy.TaskEphemeral, "passed", 0)
	if err != nil || summary.SchemaVersion != 4 || summary.AllocatedCPU != 2 || summary.PeakOwnerCount != 1 {
		return fmt.Errorf("schema 4 development summary: %+v: %w", summary, err)
	}
	encoded, err := json.Marshal(struct {
		Status  guard.ReservationTotals `json:"status"`
		Summary guard.EvidenceSummary   `json:"summary"`
	}{totals, summary})
	if err != nil || bytes.Contains(encoded, []byte("command")) || bytes.Contains(encoded, []byte("repository")) || bytes.Contains(encoded, []byte(root)) {
		return fmt.Errorf("schema 4 reservation evidence exposed private inputs: %s: %w", encoded, err)
	}

	return nil
}

func requireV04Retention(root string) error {
	var err error
	protocolPath := filepath.Join(root, "reservations.json")
	if err = os.WriteFile(protocolPath, []byte("protocol"), 0o600); err != nil {
		return err
	}
	for _, name := range []string{"coordination.lock", "coordination-mode.json"} {
		if err = os.WriteFile(filepath.Join(root, name), []byte("protocol"), 0o600); err != nil {
			return err
		}
	}
	expired := filepath.Join(root, "expired.jsonl")
	if err = os.WriteFile(expired, []byte("evidence"), 0o600); err != nil {
		return err
	}
	old := time.Now().Add(-40 * 24 * time.Hour)
	for _, path := range []string{protocolPath, filepath.Join(root, "coordination.lock"), filepath.Join(root, "coordination-mode.json"), expired} {
		if err = os.Chtimes(path, old, old); err != nil {
			return err
		}
	}
	if err = evidence.Cleanup(root, time.Now()); err != nil {
		return err
	}
	if _, err = os.Stat(protocolPath); err != nil {
		return errors.New("reservation protocol was removed by evidence retention")
	}
	if _, err = os.Stat(expired); !errors.Is(err, os.ErrNotExist) {
		return errors.New("expired evidence survived reservation protocol retention")
	}

	return nil
}

func (driver *Driver) prepareReservationScenarioV04(name string, action func(string) error) error {
	if err := driver.preparePendingV04(); err != nil {
		return err
	}
	driver.v04Scenario, driver.v04Action, driver.v04Error = name, action, nil

	return nil
}

func (driver *Driver) exerciseReservationScenarioV04() error {
	if driver.v04Action == nil {
		return errors.New("reservation scenario has no production action")
	}
	driver.v04Error = driver.v04Action(driver.evidenceRoot)

	return nil
}

func (driver *Driver) requireReservationScenarioV04(name string) error {
	if driver.v04Scenario != name {
		return fmt.Errorf("reservation outcome %q belongs to scenario %q", name, driver.v04Scenario)
	}

	return driver.v04Error
}

func (driver *Driver) requireCompiledConfigurationV04(root string) error {
	if driver.binary == "" {
		return errors.New("HIPPO_BIN is required for compiled configuration conformance")
	}
	// A liveness maximum for a real compiled run, not the property under test. The
	// guarded binary samples the host and clears admission before it launches
	// anything, and deferring under pressure is what it is for: a trivial "exit 0"
	// payload measured 2.2 s idle and 17.0 s under a load of 16 on the same
	// machine. A 15 s ceiling killed the binary mid-admission and reported the
	// product as broken, which is how both macos-15 e2e timeouts happened. A
	// healthy run spends a couple of seconds of this.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	for _, testCase := range []struct {
		name, document, mode string
	}{
		{"schema-one", schemaOneDocument, exclusiveMode},
		{"schema-two", `{"schemaVersion":2,"coordination":{"maxActiveOwners":4}}`, reservationMode},
	} {
		configPath := filepath.Join(root, testCase.name+".json")
		if err := os.WriteFile(configPath, []byte(testCase.document), 0o600); err != nil {
			return err
		}
		status := exec.CommandContext(ctx, driver.binary, statusCommandName, jsonFlag, configFlag, configPath)
		status.Env = append(os.Environ(), "HIPPO_ROOT="+filepath.Join(root, testCase.name+"-shared"))
		statusOutput, err := status.Output()
		if err != nil {
			return fmt.Errorf("compiled %s status failed: %w", testCase.name, err)
		}
		var payload struct {
			SchemaVersion int `json:"schemaVersion"`
			Coordination  struct {
				SchemaVersion int    `json:"schemaVersion"`
				Mode          string `json:"mode"`
			} `json:"coordination"`
		}
		if err = json.Unmarshal(statusOutput, &payload); err != nil || payload.SchemaVersion != 4 ||
			payload.Coordination.SchemaVersion != 4 || payload.Coordination.Mode != testCase.mode {
			return fmt.Errorf("compiled %s compatibility status: %s: %w", testCase.name, statusOutput, err)
		}
	}

	return nil
}

func (driver *Driver) requireCompiledSummaryV04(root string) error {
	if driver.binary == "" {
		return errors.New("HIPPO_BIN is required for compiled summary conformance")
	}
	configPath := filepath.Join(root, "hippo.json")
	if err := os.WriteFile(configPath, []byte(`{"schemaVersion":2,"coordination":{"maxActiveOwners":4}}`), 0o600); err != nil {
		return err
	}
	sharedRoot := filepath.Join(root, "shared")
	environment := append(os.Environ(), "HIPPO_ROOT="+sharedRoot)
	// A liveness maximum for a real compiled run, not the property under test. The
	// guarded binary samples the host and clears admission before it launches
	// anything, and deferring under pressure is what it is for: a trivial "exit 0"
	// payload measured 2.2 s idle and 17.0 s under a load of 16 on the same
	// machine. A 15 s ceiling killed the binary mid-admission and reported the
	// product as broken, which is how both macos-15 e2e timeouts happened. A
	// healthy run spends a couple of seconds of this.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	// A saturated host defers admission, and a deferral is not a failure: the
	// loaded gate reproduced exactly that and this fixture read it as one. Waiting
	// is the documented response, and it is the behaviour every consumer should
	// copy from here.
	run := exec.CommandContext(
		ctx, driver.binary, "run", configFlag, configPath,
		"--wait-for-admission", "60s",
		"--reserve-cpu", "1", "--reserve-memory-mib", "256", "--", shellPath, "-c", "exit 0",
	)
	run.Env = environment
	if output, err := run.CombinedOutput(); err != nil {
		if saturatedDeferralV04(output, err, false) {
			return nil
		}

		return fmt.Errorf("compiled schema 4 summary run failed: %s: %w", output, err)
	}
	entries, err := os.ReadDir(sharedRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".summary.json") {
			continue
		}
		data, readError := os.ReadFile(filepath.Join(sharedRoot, entry.Name()))
		if readError != nil {
			return readError
		}
		var summary guard.EvidenceSummary
		if decodeError := json.Unmarshal(data, &summary); decodeError != nil {
			return decodeError
		}
		if summary.SchemaVersion != 4 || summary.RequestedCPU != 1 || summary.AllocatedCPU != 1 ||
			summary.RequestedMemoryBytes != 256*policy.MiB || summary.AllocatedMemoryBytes != 256*policy.MiB {
			return fmt.Errorf("compiled schema 4 reservation summary: %+v", summary)
		}

		return nil
	}

	return errors.New("compiled schema 4 reservation summary was not written")
}

func (driver *Driver) requirePendingTerminalV04() error {
	if driver.mode == e2eMode {
		return driver.requireCompiledPTYV04()
	}

	root, err := os.MkdirTemp("", "hippo-v04-terminal-")
	if err != nil {
		return err
	}
	driver.temporaryPaths = append(driver.temporaryPaths, root)
	collector := &sequenceCollector{samples: []policy.Sample{
		healthySample(time.Now()), healthySample(time.Now()), healthySample(time.Now()),
	}}
	output := &bytes.Buffer{}
	exitCode, err := guard.Run(context.Background(), guard.RunConfig{
		Command: shellPath, Arguments: []string{"-c", "printf terminal-safe"}, TaskClass: policy.TaskEphemeral,
		EvidenceRoot: root, Collector: collector, Policy: v04FastPolicy(),
		Resolution: policy.Resolution{RequestedProfile: profileBalanced, ResolvedProfile: profileBalanced, Concurrency: 4},
		ChildStdin: bytes.NewBuffer(nil), ChildStdout: output, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
	})
	if err != nil || exitCode != 0 || output.String() != "terminal-safe" {
		return fmt.Errorf("non-controlling input path changed: exit=%d output=%q error=%w", exitCode, output.String(), err)
	}

	return nil
}

func v04ScriptArguments(command string, arguments ...string) ([]string, error) {
	switch runtime.GOOS {
	case "darwin", "freebsd", "openbsd", "netbsd":
		return append([]string{"-q", "/dev/null", command}, arguments...), nil
	case "linux":
		parts := append([]string{command}, arguments...)
		for index := range parts {
			parts[index] = "'" + strings.ReplaceAll(parts[index], "'", "'\"'\"'") + "'"
		}

		return []string{"-q", "-e", "-c", strings.Join(parts, " "), "/dev/null"}, nil
	default:
		return nil, fmt.Errorf("compiled PTY conformance is unsupported on %s", runtime.GOOS)
	}
}

func (driver *Driver) requireCompiledPTYV04() error {
	if driver.binary == "" {
		return errors.New("HIPPO_BIN is required for compiled PTY conformance")
	}
	scriptPath, err := exec.LookPath("script")
	if err != nil {
		return err
	}
	root, err := os.MkdirTemp("", "hippo-v04-e2e-pty-")
	if err != nil {
		return err
	}
	driver.temporaryPaths = append(driver.temporaryPaths, root)
	resultPath := filepath.Join(root, "result")
	readyPath := filepath.Join(root, "ready")
	childScript := "printf ready > \"$HIPPO_PTY_READY\"; value=; while [ -z \"$value\" ]; do IFS= read -r value || :; done; printf '%s' \"$value\" > \"$HIPPO_PTY_RESULT\""
	arguments, err := v04ScriptArguments(
		driver.binary,
		"run", "--class", "ephemeral", "--wait-for-admission", "60s", "--", shellPath, "-c", childScript,
	)
	if err != nil {
		return err
	}
	// A liveness maximum for a real compiled run, not the property under test. The
	// guarded binary samples the host and clears admission before it launches
	// anything, and deferring under pressure is what it is for: a trivial "exit 0"
	// payload measured 2.2 s idle and 17.0 s under a load of 16 on the same
	// machine. A 15 s ceiling killed the binary mid-admission and reported the
	// product as broken, which is how both macos-15 e2e timeouts happened. A
	// healthy run spends a couple of seconds of this.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, scriptPath, arguments...)
	command.Env = append(os.Environ(), "HIPPO_ROOT="+root, "HIPPO_PTY_RESULT="+resultPath, "HIPPO_PTY_READY="+readyPath)
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	command.Stdin = reader
	go func() {
		// Synchronise on the child rather than on the clock. A constant cannot
		// promise the compiled binary has launched, cleared admission, and reached
		// its read, and the child's loop treats a failed read as "keep waiting", so
		// any EOF that lands before the first read spins until the deadline. The
		// in-process sibling of this scenario already publishes HIPPO_PTY_READY for
		// exactly this reason. Holding the terminal open until the result is
		// recorded keeps EOF strictly behind the read it must follow.
		//
		// This removes an unsynchronised fixture, which is a defect on its own
		// terms. It is not a proven fix for the one CI timeout seen on macOS: that
		// failure did not reproduce locally (0/30 at load 16), and its captured echo
		// showed the input had been delivered, so its mechanism is still unknown.
		defer func() { _ = writer.Close() }()
		if !awaitMarkerFileContext(ctx, readyPath) {
			return
		}
		_, _ = writer.WriteString("compiled-terminal-input\n")
		_ = awaitMarkerFileContext(ctx, resultPath)
	}()
	started := time.Now()
	output, runError := command.CombinedOutput()
	if ctx.Err() != nil {
		// Name what was actually observed rather than guessing at a cause. The one
		// CI occurrence of this timeout blamed SIGTTIN while its own captured echo
		// showed the input had been delivered, which sent the investigation the
		// wrong way; readiness and result state distinguish a child that never
		// started from one that started and never read.
		_, readyErr := os.Stat(readyPath)
		_, resultErr := os.Stat(resultPath)

		return fmt.Errorf(
			"compiled PTY child did not finish within %s: child signalled readiness=%t, wrote a result=%t: %s",
			time.Since(started).Round(time.Millisecond), readyErr == nil, resultErr == nil, output,
		)
	}
	if runError != nil {
		// A deferral on the saturated gate is accepted only for a child that never
		// started: readiness is the child's first act, so its absence proves the
		// refusal came before launch rather than from a child that ran and failed.
		_, readyErr := os.Stat(readyPath)
		if saturatedDeferralV04(output, runError, !errors.Is(readyErr, os.ErrNotExist)) {
			return nil
		}

		return fmt.Errorf("compiled PTY guard failed: %s: %w", output, runError)
	}
	result, err := os.ReadFile(resultPath)
	if err != nil || string(result) != "compiled-terminal-input" {
		return fmt.Errorf("compiled PTY child did not read input: result=%q output=%s: %w", result, output, err)
	}

	return nil
}

// saturatedDeferralV04 reports whether a compiled run ended in HIPPO's documented
// refusal on a host that scripts/test-loaded.sh saturates by design. That gate
// keeps every core busy for its whole run, and admission needs consecutive CPU
// samples under the profile ceiling, so no guarded child can clear it there:
// deferring is the product working, and the only correct answer a child can get.
// Anywhere the flag is unset, a deferral stays the failure it is.
func saturatedDeferralV04(output []byte, err error, childStarted bool) bool {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return false
	}

	return acceptsSaturatedDeferralV04(
		os.Getenv(loadSaturatedVariable) == "1", exitError.ExitCode(), output, childStarted,
	)
}

func (driver *Driver) requirePendingConformanceV04() error {
	root, err := os.MkdirTemp("", "hippo-v04-conformance-")
	if err != nil {
		return err
	}
	driver.temporaryPaths = append(driver.temporaryPaths, root)
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	binaryData, err := os.ReadFile(binary)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(binaryData)
	manifest := conformance.Manifest{
		SchemaVersion: 1,
		HIPPOBinary:   binary,
		HIPPOSHA256:   hex.EncodeToString(digest[:]),
		SharedRoot:    filepath.Join(root, "shared"),
	}
	for index := range 4 {
		name := fmt.Sprintf("consumer-%d", index+1)
		path := filepath.Join(root, name)
		if err = os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		if err = initializeFixtureCheckout(path); err != nil {
			return fmt.Errorf("create temporary consumer %q: %w", name, err)
		}
		manifest.Consumers = append(manifest.Consumers, conformance.Consumer{
			Name:      name,
			Path:      path,
			Bootstrap: []conformance.Command{{Arguments: []string{"git", "diff", "--quiet"}}},
			Gates:     []conformance.Command{{Arguments: []string{shellPath, "-c", "test -n \"$HIPPO_BIN\" && test -n \"$HIPPO_ROOT\""}}},
		})
	}
	manifest.CoordinationChecks = []conformance.Check{
		{
			Consumer: manifest.Consumers[0].Name,
			Command:  conformance.Command{Arguments: []string{shellPath, "-c", "test -x \"$HIPPO_BIN\""}},
		},
		{
			Consumer:          manifest.Consumers[1].Name,
			Command:           conformance.Command{Arguments: []string{shellPath, "-c", "exit 75"}},
			AllowCapacitySkip: true,
		},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(root, "manifest.json")
	if err = os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		return err
	}

	return conformance.Run(context.Background(), manifestPath, &bytes.Buffer{})
}
