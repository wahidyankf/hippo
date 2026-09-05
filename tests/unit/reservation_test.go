package unit_test

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestMalformedCompatibilitySessionFailsClosedWithoutMutation(t *testing.T) {
	root := t.TempDir()
	sessions := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessions, strings.Repeat("a", 32)+".json")
	state := []byte("{\"schemaVersion\":1,\"pid\":")
	if err := os.WriteFile(path, state, 0o600); err != nil {
		t.Fatal(err)
	}

	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
		fixedPlan(1, 256*policy.MiB, 4, policy.GiB), 20, 0,
	)
	if session != nil || !guard.IsCoordinationDeferred(err) {
		t.Fatalf("malformed compatibility session did not defer: session=%+v error=%v", session, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, state) {
		t.Fatalf("malformed compatibility session changed: %q error=%v", after, err)
	}
}

func TestReservationLedgerCorruptionFailsClosedWithoutMutation(t *testing.T) {
	validOwner := `{"token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","pid":1,"class":"ephemeral","profile":"balanced","requested":{"cpu":1,"memoryBytes":268435456},"allocated":{"cpu":1,"memoryBytes":268435456},"sequence":1,"maxActiveOwners":20}`
	validWaiter := `{"token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","pid":1,"class":"ephemeral","profile":"balanced","requested":{"cpu":1,"memoryBytes":268435456},"sequence":1,"maxActiveOwners":20}`
	//nolint:gosec // Synthetic repeated tokens are corruption fixtures, not credentials.
	for name, document := range map[string]string{
		"invalid token":         `{"schemaVersion":2,"capacity":{"cpu":4,"memoryBytes":1073741824},"nextSequence":1,"owners":[{"token":"invalid","pid":1,"class":"ephemeral","profile":"balanced","requested":{"cpu":1,"memoryBytes":268435456},"allocated":{"cpu":1,"memoryBytes":268435456},"sequence":1,"maxActiveOwners":20}],"waiters":[]}`,
		"duplicate token":       `{"schemaVersion":2,"capacity":{"cpu":4,"memoryBytes":1073741824},"nextSequence":2,"owners":[` + validOwner + `],"waiters":[{"token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","pid":1,"class":"service","profile":"balanced","requested":{"cpu":1,"memoryBytes":268435456},"sequence":2,"maxActiveOwners":20}]}`,
		"invalid class":         strings.Replace(validOwner, `"class":"ephemeral"`, `"class":"unknown"`, 1),
		"invalid sequence":      `{"schemaVersion":2,"capacity":{"cpu":4,"memoryBytes":1073741824},"nextSequence":2,"owners":[],"waiters":[` + strings.Replace(validWaiter, `"sequence":1`, `"sequence":2`, 1) + `,` + strings.ReplaceAll(validWaiter, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb") + `]}`,
		"negative vector":       strings.Replace(validOwner, `"cpu":1`, `"cpu":-1`, 1),
		"excess totals":         `{"schemaVersion":2,"capacity":{"cpu":1,"memoryBytes":268435456},"nextSequence":2,"owners":[` + validOwner + `,` + strings.Replace(strings.ReplaceAll(validOwner, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), `"sequence":1`, `"sequence":2`, 1) + `],"waiters":[]}`,
		"overflow totals":       `{"schemaVersion":2,"capacity":{"cpu":9223372036854775807,"memoryBytes":9223372036854775807},"nextSequence":2,"owners":[{"token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","pid":1,"class":"ephemeral","profile":"balanced","requested":{"cpu":9223372036854775807,"memoryBytes":9223372036854775807},"allocated":{"cpu":9223372036854775807,"memoryBytes":9223372036854775807},"sequence":1,"maxActiveOwners":20},{"token":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","pid":1,"class":"service","profile":"balanced","requested":{"cpu":1,"memoryBytes":268435456},"allocated":{"cpu":1,"memoryBytes":268435456},"sequence":2,"maxActiveOwners":20}],"waiters":[]}`,
		"impossible structure":  strings.Replace(validOwner, `"allocated":{"cpu":1`, `"allocated":{"cpu":2`, 1),
		"invalid shedding exit": strings.TrimSuffix(validOwner, "}") + `,"processGroup":123,"shedding":true,"sheddingExitCode":1}`,
		"duplicate field":       `{"schemaVersion":2,"schemaVersion":2,"capacity":{"cpu":0,"memoryBytes":0},"nextSequence":0,"owners":[],"waiters":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "coordination-mode.json"), []byte("{\"schemaVersion\":1,\"mode\":\"reservation\"}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(document, `"schemaVersion"`) || !strings.Contains(document, `"owners"`) {
				document = `{"schemaVersion":2,"capacity":{"cpu":4,"memoryBytes":1073741824},"nextSequence":1,"owners":[` + document + `],"waiters":[]}`
			}
			path := filepath.Join(root, "reservations.json")
			state := []byte(document + "\n")
			if err := os.WriteFile(path, state, 0o600); err != nil {
				t.Fatal(err)
			}

			if _, err := guard.ReservationStatus(context.Background(), root); err == nil {
				t.Fatal("corrupt ledger was accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, state) {
				t.Fatalf("corrupt ledger changed: %q error=%v", after, err)
			}
		})
	}
}

func reservationSample() policy.Sample {
	return policy.Sample{
		EffectiveMemoryLimitBytes: 32 * policy.GiB,
		PhysicalMemoryBytes:       32 * policy.GiB,
		AvailableParallelism:      9,
	}
}

func reservationResolution(profile string) policy.Resolution {
	return policy.Resolution{ResolvedProfile: profile, MemoryReserve: 4 * policy.GiB}
}

func reservationPolicy() guard.ReservationPolicy {
	return guard.ReservationPolicy{
		Enabled:         true,
		MaxActiveOwners: 20,
		OwnerShares: map[string]int{
			"balanced": 4, "constrained": 2, "minimal": 1,
		},
	}
}

func TestAutomaticAndExplicitReservationPlanning(t *testing.T) {
	for profile, expectedCPU := range map[string]int{"balanced": 2, "constrained": 4, "minimal": 8} {
		plan, err := guard.PlanReservation(reservationSample(), reservationResolution(profile), reservationPolicy(), 0, 0)
		if err != nil {
			t.Fatalf("%s planning failed: %v", profile, err)
		}
		if plan.Requested.CPU != expectedCPU || plan.Requested.MemoryBytes != (28*policy.GiB)/int64(reservationPolicy().OwnerShares[profile]) {
			t.Fatalf("%s automatic vector = %+v", profile, plan.Requested)
		}
	}

	plan, err := guard.PlanReservation(reservationSample(), reservationResolution("balanced"), reservationPolicy(), 1, 256*policy.MiB)
	if err != nil || plan.Requested.CPU != 1 || plan.Requested.MemoryBytes != 256*policy.MiB {
		t.Fatalf("explicit floor vector = %+v, %v", plan, err)
	}
	for _, request := range []struct {
		cpu    int
		memory int64
	}{{-1, 256 * policy.MiB}, {1, 255 * policy.MiB}, {9, policy.GiB}, {1, 29 * policy.GiB}} {
		if _, planError := guard.PlanReservation(reservationSample(), reservationResolution("balanced"), reservationPolicy(), request.cpu, request.memory); !errors.Is(planError, guard.ErrReservationReplan) {
			t.Fatalf("unsafe vector (%d,%d) returned %v", request.cpu, request.memory, planError)
		}
	}
}

func fixedPlan(cpu int, memory int64, capacityCPU int, capacityMemory int64) guard.ReservationPlan {
	vector := guard.ReservationVector{CPU: cpu, MemoryBytes: memory}

	return guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: capacityCPU, MemoryBytes: capacityMemory},
		Requested: vector,
		Allocated: vector,
	}
}

func acquireReservation(t *testing.T, root string, class policy.TaskClass, plan guard.ReservationPlan) *guard.Session {
	t.Helper()
	session, err := guard.AcquireReservation(context.Background(), root, "", class, "balanced", "", plan, 20, 0)
	if err != nil || session == nil {
		t.Fatalf("acquire reservation: session=%+v error=%v", session, err)
	}

	return session
}

func TestReservationLedgerCountsEveryClassAndReusesInheritance(t *testing.T) {
	root := t.TempDir()
	plan := fixedPlan(1, 256*policy.MiB, 4, policy.GiB)
	sessions := []*guard.Session{
		acquireReservation(t, root, policy.TaskService, plan),
		acquireReservation(t, root, policy.TaskEphemeral, plan),
		acquireReservation(t, root, policy.TaskTransactional, plan),
	}
	defer func() {
		for _, session := range sessions {
			if err := guard.ReleaseReservation(root, session); err != nil {
				t.Errorf("release: %v", err)
			}
		}
	}()

	totals, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || totals.ActiveOwners != 3 || totals.Service != 1 || totals.Ephemeral != 1 || totals.Transactional != 1 || totals.Allocated.CPU != 3 {
		t.Fatalf("unexpected totals %+v error=%v", totals, err)
	}
	inherited, err := guard.AcquireReservation(context.Background(), root, sessions[1].Token, policy.TaskEphemeral, "minimal", "", fixedPlan(4, policy.GiB, 4, policy.GiB), 20, 0)
	if err != nil || inherited == nil || !inherited.Inherited || inherited.Allocation != plan.Allocated {
		t.Fatalf("inheritance expanded or failed: %+v error=%v", inherited, err)
	}
	after, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || after.ActiveOwners != 3 || after.Allocated != totals.Allocated {
		t.Fatalf("inheritance double-reserved: before=%+v after=%+v error=%v", totals, after, err)
	}
}

func TestReservationAdmissionIsAtomicAndFIFO(t *testing.T) {
	root := t.TempDir()
	owner := acquireReservation(t, root, policy.TaskService, fixedPlan(2, 512*policy.MiB, 3, 768*policy.MiB))
	largeResult := make(chan *guard.Session, 1)
	largeError := make(chan error, 1)
	go func() {
		session, err := guard.AcquireReservation(
			context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
			fixedPlan(2, 512*policy.MiB, 3, 768*policy.MiB), 20, 500*time.Millisecond,
		)
		largeResult <- session
		largeError <- err
	}()

	deadline := time.Now().Add(time.Second)
	for {
		totals, err := guard.ReservationStatus(context.Background(), root)
		if err == nil && totals.WaitingOwners == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("large FIFO head did not enqueue")
		}
		time.Sleep(time.Millisecond)
	}

	small, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, "balanced", "",
		fixedPlan(1, 256*policy.MiB, 3, 768*policy.MiB), 20, 0,
	)
	if !errors.Is(err, guard.ErrReservationDeferred) || small != nil {
		t.Fatalf("smaller waiter bypassed FIFO head: session=%+v error=%v", small, err)
	}
	before, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || before.Allocated.CPU != 2 || before.Allocated.MemoryBytes != 512*policy.MiB {
		t.Fatalf("failed vector admission partially mutated totals: %+v error=%v", before, err)
	}
	if err = guard.ReleaseReservation(root, owner); err != nil {
		t.Fatal(err)
	}
	large := <-largeResult
	if err = <-largeError; err != nil || large == nil {
		t.Fatalf("FIFO head was not admitted after capacity release: session=%+v error=%v", large, err)
	}
	if err = guard.ReleaseReservation(root, large); err != nil {
		t.Fatal(err)
	}
}

func TestReservationAdmissionArithmeticCannotOverflow(t *testing.T) {
	root := t.TempDir()
	maximum := guard.ReservationVector{CPU: math.MaxInt, MemoryBytes: math.MaxInt64}
	owner := acquireReservation(t, root, policy.TaskService, guard.ReservationPlan{
		Capacity: maximum, Requested: maximum, Allocated: maximum,
	})
	defer func() { _ = guard.ReleaseReservation(root, owner) }()
	minimum := guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
	candidate, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
		guard.ReservationPlan{Capacity: maximum, Requested: minimum, Allocated: minimum}, 20, 0,
	)
	if candidate != nil || !errors.Is(err, guard.ErrReservationDeferred) {
		t.Fatalf("maximum-width vector admission wrapped: session=%+v error=%v", candidate, err)
	}
	totals, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || totals.Allocated != maximum {
		t.Fatalf("maximum-width totals changed: %+v error=%v", totals, err)
	}
}

func TestReservationOwnerLimitIsConservativeAcrossLiveOwnersAndWaiters(t *testing.T) {
	root := t.TempDir()
	plan := fixedPlan(1, 256*policy.MiB, 4, policy.GiB)
	owner := acquireReservation(t, root, policy.TaskService, plan)
	strictResult := make(chan *guard.Session, 1)
	strictError := make(chan error, 1)
	go func() {
		session, err := guard.AcquireReservation(
			context.Background(), root, "", policy.TaskEphemeral, "balanced", "", plan, 1, time.Second,
		)
		strictResult <- session
		strictError <- err
	}()

	deadline := time.Now().Add(time.Second)
	for {
		totals, err := guard.ReservationStatus(context.Background(), root)
		if err == nil && totals.WaitingOwners == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("strict shared-root waiter did not enqueue")
		}
		time.Sleep(time.Millisecond)
	}
	loose, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskTransactional, "balanced", "", plan, 20, 0,
	)
	if loose != nil || !errors.Is(err, guard.ErrReservationDeferred) {
		t.Fatalf("loose configuration bypassed strict waiter: session=%+v error=%v", loose, err)
	}
	if err = guard.ReleaseReservation(root, owner); err != nil {
		t.Fatal(err)
	}
	strict := <-strictResult
	if err = <-strictError; err != nil || strict == nil {
		t.Fatalf("strict waiter was not admitted: session=%+v error=%v", strict, err)
	}
	defer func() { _ = guard.ReleaseReservation(root, strict) }()
	loose, err = guard.AcquireReservation(
		context.Background(), root, "", policy.TaskTransactional, "balanced", "", plan, 20, 0,
	)
	if loose != nil || !errors.Is(err, guard.ErrReservationDeferred) {
		t.Fatalf("loose configuration bypassed strict live owner: session=%+v error=%v", loose, err)
	}
}

func TestCPUAndMemoryExhaustionNeverPartiallyAllocate(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		owner guard.ReservationPlan
		other guard.ReservationPlan
	}{
		{
			name:  "cpu-only",
			owner: fixedPlan(4, 256*policy.MiB, 4, policy.GiB),
			other: fixedPlan(1, 256*policy.MiB, 4, policy.GiB),
		},
		{
			name:  "memory-only",
			owner: fixedPlan(1, policy.GiB, 4, policy.GiB),
			other: fixedPlan(1, 256*policy.MiB, 4, policy.GiB),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			owner := acquireReservation(t, root, policy.TaskService, testCase.owner)
			defer func() { _ = guard.ReleaseReservation(root, owner) }()
			other, err := guard.AcquireReservation(
				context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
				testCase.other, 20, 0,
			)
			if other != nil || !errors.Is(err, guard.ErrReservationDeferred) {
				t.Fatalf("exhausted dimension admitted: session=%+v error=%v", other, err)
			}
			totals, err := guard.ReservationStatus(context.Background(), root)
			if err != nil || totals.Allocated != testCase.owner.Allocated {
				t.Fatalf("failed vector partially allocated: totals=%+v error=%v", totals, err)
			}
		})
	}
}

func TestPressureVictimOrderingNeverSelectsTransactional(t *testing.T) {
	root := t.TempDir()
	plan := fixedPlan(1, 256*policy.MiB, 4, policy.GiB)
	transactional := acquireReservation(t, root, policy.TaskTransactional, plan)
	service := acquireReservation(t, root, policy.TaskService, plan)
	ephemeral := acquireReservation(t, root, policy.TaskEphemeral, plan)
	defer func() {
		for _, session := range []*guard.Session{transactional, service, ephemeral} {
			_ = guard.ReleaseReservation(root, session)
		}
	}()
	for index, session := range []*guard.Session{transactional, service, ephemeral} {
		if err := guard.ActivateReservation(root, session, 10_000+index); err != nil {
			t.Fatal(err)
		}
	}
	if _, selected, err := guard.SelectPressureVictim(root, 1); err == nil || selected {
		t.Fatalf("invalid shedding exit was accepted: selected=%v error=%v", selected, err)
	}
	victim, selected, err := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
	if err != nil || !selected || victim.Token != ephemeral.Token {
		t.Fatalf("first victim=%+v selected=%v error=%v", victim, selected, err)
	}
	if err = guard.ReleaseReservation(root, ephemeral); err != nil {
		t.Fatal(err)
	}
	victim, selected, err = guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
	if err != nil || !selected || victim.Token != service.Token {
		t.Fatalf("second victim=%+v selected=%v error=%v", victim, selected, err)
	}
	if err = guard.ReleaseReservation(root, service); err != nil {
		t.Fatal(err)
	}
	if _, selected, err = guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode); err != nil || selected {
		t.Fatalf("transactional owner was selected: selected=%v error=%v", selected, err)
	}
}

func TestReservationEnvironmentIsFixedAndClamped(t *testing.T) {
	environment, err := guard.ReservationEnvironment(
		[]string{"LOWER=1", "HIGHER=8"},
		policy.Resolution{ResolvedProfile: "constrained"},
		guard.ReservationVector{CPU: 2, MemoryBytes: 512 * policy.MiB},
		[]string{"MISSING", "LOWER", "HIGHER"},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"HIPPO_PROFILE=constrained", "HIPPO_CONCURRENCY=2", "HIPPO_RESERVED_MEMORY_BYTES=536870912",
		"MISSING=2", "LOWER=1", "HIGHER=2",
	} {
		if !slices.Contains(environment, expected) {
			t.Fatalf("missing %q from %v", expected, environment)
		}
	}
	for _, invalid := range []string{"0", "-1", "many"} {
		if _, mapError := guard.ReservationEnvironment(
			[]string{"WORKERS=" + invalid}, policy.Resolution{ResolvedProfile: "balanced"},
			guard.ReservationVector{CPU: 2, MemoryBytes: 512 * policy.MiB}, []string{"WORKERS"},
		); mapError == nil {
			t.Fatalf("invalid mapping %q was accepted", invalid)
		}
	}
}
