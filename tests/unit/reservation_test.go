package unit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/support/runtimewiring"

	coordination "github.com/wahidyankf/hippo/internal/domain/coordination"

	guard "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
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

	session, err := runtimewiring.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
		fixedPlan(1, 256*policy.MiB, 4, policy.GiB), 20, 0,
	)
	if session != nil || err == nil || guard.IsCoordinationDeferred(err) || guard.IsCoordinationProtocolMismatch(err) {
		t.Fatalf("malformed compatibility session did not fail as corrupt state: session=%+v error=%v", session, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, state) {
		t.Fatalf("malformed compatibility session changed: %q error=%v", after, err)
	}
}

func TestUnsupportedReservationLedgerSchemaIsProtocolMismatchWithoutMutation(t *testing.T) {
	root := t.TempDir()
	marker := []byte("{\"schemaVersion\":1,\"mode\":\"reservation\"}\n")
	ledger := []byte("{\"schemaVersion\":3,\"capacity\":{\"cpu\":0,\"memoryBytes\":0},\"nextSequence\":0,\"owners\":[],\"waiters\":[]}\n")
	if err := os.WriteFile(filepath.Join(root, "coordination-mode.json"), marker, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "reservations.json")
	if err := os.WriteFile(path, ledger, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := guard.ReservationStatus(context.Background(), root)
	if !guard.IsCoordinationProtocolMismatch(err) {
		t.Fatalf("future ledger schema returned %v, want protocol mismatch", err)
	}
	after, readError := os.ReadFile(path)
	if readError != nil || !bytes.Equal(after, ledger) {
		t.Fatalf("future ledger changed: %q error=%v", after, readError)
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
		// 74 is the status a shed returns inside the process; a ledger records 73 or 75.
		"shedding exit 74":         strings.TrimSuffix(validOwner, "}") + `,"processGroup":123,"shedding":true,"sheddingExitCode":74}`,
		"shedding without a cause": strings.TrimSuffix(validOwner, "}") + `,"processGroup":123,"shedding":true}`,
		"a cause without shedding": strings.TrimSuffix(validOwner, "}") + `,"processGroup":123,"sheddingExitCode":73}`,
		"duplicate field":          `{"schemaVersion":2,"schemaVersion":2,"capacity":{"cpu":0,"memoryBytes":0},"nextSequence":0,"owners":[],"waiters":[]}`,
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

// TestReservationLedgerClassIsRefusedWhereItIsDecoded pins where a class is refused. A class this version has no
// member for fails at decode, naming it, because the decoder knows the closed set; one it does have a member for but a
// ledger never holds, release, decodes and is refused afterwards by validation, which only calls it invalid. A class
// that is absent or null never reaches the decoder, which only reads text that is present, so it arrives at validation
// as the empty class and only validation refuses it.
func TestReservationLedgerClassIsRefusedWhereItIsDecoded(t *testing.T) {
	const (
		decodeFailure     = "decode reservation ledger"
		validationFailure = "class is invalid"
	)
	owner := `{"token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","pid":1,"class":"batch","profile":"balanced","requested":{"cpu":1,"memoryBytes":268435456},"allocated":{"cpu":1,"memoryBytes":268435456},"sequence":1,"maxActiveOwners":20}`
	waiter := `{"token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","pid":1,"class":"batch","profile":"balanced","requested":{"cpu":1,"memoryBytes":268435456},"sequence":1,"maxActiveOwners":20}`
	ledger := func(owners, waiters string) string {
		return `{"schemaVersion":2,"capacity":{"cpu":4,"memoryBytes":1073741824},"nextSequence":1,"owners":[` + owners +
			`],"waiters":[` + waiters + `]}`
	}
	for name, test := range map[string]struct {
		document string
		wants    []string
		refuses  string
	}{
		"owner":                    {ledger(owner, ""), []string{decodeFailure, `"batch"`}, validationFailure},
		"waiter":                   {ledger("", waiter), []string{decodeFailure, `"batch"`}, validationFailure},
		"release owner":            {ledger(strings.Replace(owner, `"batch"`, `"release"`, 1), ""), []string{"owner " + validationFailure}, decodeFailure},
		"release waiter":           {ledger("", strings.Replace(waiter, `"batch"`, `"release"`, 1)), []string{"waiter " + validationFailure}, decodeFailure},
		"owner without a class":    {ledger(strings.Replace(owner, `"class":"batch",`, "", 1), ""), []string{"owner " + validationFailure}, decodeFailure},
		"waiter without a class":   {ledger("", strings.Replace(waiter, `"class":"batch",`, "", 1)), []string{"waiter " + validationFailure}, decodeFailure},
		"owner with a null class":  {ledger(strings.Replace(owner, `"batch"`, "null", 1), ""), []string{"owner " + validationFailure}, decodeFailure},
		"waiter with a null class": {ledger("", strings.Replace(waiter, `"batch"`, "null", 1)), []string{"waiter " + validationFailure}, decodeFailure},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "coordination-mode.json"), []byte("{\"schemaVersion\":1,\"mode\":\"reservation\"}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "reservations.json")
			state := []byte(test.document + "\n")
			if err := os.WriteFile(path, state, 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := guard.ReservationStatus(context.Background(), root)
			if err == nil {
				t.Fatal("a ledger holding a class no reservation may hold was accepted")
			}
			for _, want := range test.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if strings.Contains(err.Error(), test.refuses) {
				t.Errorf("error %q contains %q, so the class was refused at the other stage", err, test.refuses)
			}
			after, readError := os.ReadFile(path)
			if readError != nil || !bytes.Equal(after, state) {
				t.Errorf("refused ledger changed: %q error=%v", after, readError)
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

func reservationResolution(profile policy.ProfileName) policy.Resolution {
	return policy.Resolution{ResolvedProfile: profile, MemoryReserve: 4 * policy.GiB}
}

func reservationPolicy() guard.ReservationPolicy {
	return guard.ReservationPolicy{
		Enabled:         true,
		MaxActiveOwners: 20,
		OwnerShares: map[policy.ProfileName]int{
			"balanced": 4, "constrained": 2, "minimal": 1,
		},
	}
}

func TestAutomaticAndExplicitReservationPlanning(t *testing.T) {
	for profile, expectedCPU := range map[policy.ProfileName]int{"balanced": 2, "constrained": 4, "minimal": 8} {
		plan, err := coordination.PlanReservation(reservationSample(), reservationResolution(profile), reservationPolicy(), 0, 0)
		if err != nil {
			t.Fatalf("%s planning failed: %v", profile, err)
		}
		if plan.Requested.CPU != expectedCPU || plan.Requested.MemoryBytes != (28*policy.GiB)/int64(reservationPolicy().OwnerShares[profile]) {
			t.Fatalf("%s automatic vector = %+v", profile, plan.Requested)
		}
	}

	plan, err := coordination.PlanReservation(reservationSample(), reservationResolution("balanced"), reservationPolicy(), 1, 256*policy.MiB)
	if err != nil || plan.Requested.CPU != 1 || plan.Requested.MemoryBytes != 256*policy.MiB {
		t.Fatalf("explicit floor vector = %+v, %v", plan, err)
	}
	for _, request := range []struct {
		cpu    int
		memory int64
	}{{-1, 256 * policy.MiB}, {1, 255 * policy.MiB}, {9, policy.GiB}, {1, 29 * policy.GiB}} {
		if _, planError := coordination.PlanReservation(reservationSample(), reservationResolution("balanced"), reservationPolicy(), request.cpu, request.memory); !errors.Is(planError, guard.ErrReservationReplan) {
			t.Fatalf("unsafe vector (%d,%d) returned %v", request.cpu, request.memory, planError)
		}
	}
}

func TestTierReservationPlanningAndIdleBurst(t *testing.T) {
	tiers := coordination.DefaultResourceTiers()
	settings := reservationPolicy()
	settings.MaxCPU = 8
	settings.MaxMemoryBytes = 16 * policy.GiB
	settings.Tiers = tiers
	sample := reservationSample()
	resolution := reservationResolution("balanced")

	for name, expected := range map[string]struct {
		minimum guard.ReservationVector
		maximum guard.ReservationVector
		wait    time.Duration
	}{
		"light": {
			minimum: guard.ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
			maximum: guard.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB},
			wait:    30 * time.Minute,
		},
		"standard": {
			minimum: guard.ReservationVector{CPU: 2, MemoryBytes: 3 * policy.GiB},
			maximum: guard.ReservationVector{CPU: 4, MemoryBytes: 6 * policy.GiB},
			wait:    90 * time.Minute,
		},
		"heavy": {
			minimum: guard.ReservationVector{CPU: 4, MemoryBytes: 8 * policy.GiB},
			maximum: guard.ReservationVector{CPU: 8, MemoryBytes: 16 * policy.GiB},
			wait:    4 * time.Hour,
		},
	} {
		plan, wait, err := coordination.PlanTierReservation(sample, resolution, settings, name, 0, 0)
		if err != nil || plan.Minimum != expected.minimum || plan.Maximum != expected.maximum || wait != expected.wait {
			t.Fatalf("%s plan = %+v wait=%s error=%v", name, plan, wait, err)
		}
	}

	plan, _, err := coordination.PlanTierReservation(sample, resolution, settings, "heavy", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	session, err := runtimewiring.AcquireReservation(context.Background(), root, "", policy.TaskEphemeral, "balanced", "", plan, 2, 0)
	if err != nil || session.Allocation != (guard.ReservationVector{CPU: 8, MemoryBytes: 16 * policy.GiB}) ||
		session.Requested != (guard.ReservationVector{CPU: 4, MemoryBytes: 8 * policy.GiB}) {
		t.Fatalf("idle heavy allocation = %+v error=%v", session, err)
	}
	// Metadata-backed status preserves both the tier bounds and the actual
	// immutable ledger allocation.
	if err = guard.ReleaseReservation(root, session); err != nil {
		t.Fatal(err)
	}
	session, err = runtimewiring.AcquireReservationWithOptions(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "", plan, 2, 0,
		guard.ReservationAdmissionOptions{Metadata: guard.ReservationMetadata{Source: "hippo", Tier: "heavy"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	totals, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || len(totals.Owners) != 1 || totals.Owners[0].Minimum != session.Requested ||
		totals.Owners[0].Maximum != plan.Maximum || totals.Owners[0].Allocated != session.Allocation {
		t.Fatalf("idle heavy status = %+v error=%v", totals, err)
	}
	if err = guard.ReleaseReservation(root, session); err != nil {
		t.Fatal(err)
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
	session, err := runtimewiring.AcquireReservation(context.Background(), root, "", class, "balanced", "", plan, 20, 0)
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
	inherited, err := runtimewiring.AcquireReservation(context.Background(), root, sessions[1].Token, policy.TaskEphemeral, "minimal", "", fixedPlan(4, policy.GiB, 4, policy.GiB), 20, 0)
	if err != nil || inherited == nil || !inherited.Inherited || inherited.Allocation != plan.Allocated {
		t.Fatalf("inheritance expanded or failed: %+v error=%v", inherited, err)
	}
	after, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || after.ActiveOwners != 3 || after.Allocated != totals.Allocated {
		t.Fatalf("inheritance double-reserved: before=%+v after=%+v error=%v", totals, after, err)
	}
}

func TestSchemaThreeMetadataSurvivesSchemaTwoLedgerRewrite(t *testing.T) {
	root := t.TempDir()
	plan := guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: 8, MemoryBytes: 16 * policy.GiB},
		Requested: guard.ReservationVector{CPU: 2, MemoryBytes: 3 * policy.GiB},
		Allocated: guard.ReservationVector{CPU: 2, MemoryBytes: 3 * policy.GiB},
		Minimum:   guard.ReservationVector{CPU: 2, MemoryBytes: 3 * policy.GiB},
		Maximum:   guard.ReservationVector{CPU: 4, MemoryBytes: 6 * policy.GiB},
		Tier:      "standard",
	}
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	session, err := runtimewiring.AcquireReservationWithOptions(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "hash", plan, 2, 90*time.Minute,
		guard.ReservationAdmissionOptions{
			Metadata: guard.ReservationMetadata{
				Source: "ose-public", Tags: map[string]string{"checkout": "worktree"}, Tier: "standard",
			},
			Now: func() time.Time { return now },
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = guard.ReleaseReservation(root, session) }()

	ledgerPath := filepath.Join(root, "reservations.json")
	ledger, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // The path is rooted in t.TempDir and never includes caller input.
	if err = os.WriteFile(ledgerPath, ledger, 0o600); err != nil {
		t.Fatal(err)
	}

	totals, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || totals.LegacyEntries != 0 || len(totals.Owners) != 1 ||
		totals.Owners[0].Source != "ose-public" || totals.Owners[0].Tier != "standard" ||
		totals.Owners[0].Tags["checkout"] != "worktree" {
		t.Fatalf("metadata after schema-2 rewrite: %+v error=%v", totals, err)
	}
}

func TestAdmissionDeadlineWritesNeverStartedReceipt(t *testing.T) {
	root := t.TempDir()
	plan := fixedPlan(1, 256*policy.MiB, 1, 256*policy.MiB)
	owner := acquireReservation(t, root, policy.TaskEphemeral, plan)
	defer func() { _ = guard.ReleaseReservation(root, owner) }()
	_, err := runtimewiring.AcquireReservationWithOptions(
		context.Background(), root, "", policy.TaskTransactional, "balanced", "", plan, 2, 0,
		guard.ReservationAdmissionOptions{Metadata: guard.ReservationMetadata{
			Source: "hippo", Tags: map[string]string{"checkout": "worktree"}, Tier: "light",
		}},
	)
	if !errors.Is(err, guard.ErrReservationDeferred) {
		t.Fatalf("deadline error=%v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "receipts"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("receipt entries=%d error=%v", len(entries), err)
	}
	data, err := os.ReadFile(filepath.Join(root, "receipts", entries[0].Name()))
	if err != nil || !strings.Contains(string(data), `"state":"never-started"`) ||
		!strings.Contains(string(data), `"reason":"admission-deadline"`) {
		t.Fatalf("receipt=%s error=%v", data, err)
	}
}

// The bounded wait's last passes reach the coordination lock with only a
// sliver of budget left. An uncontended root must still defer on capacity with
// a never-started receipt, never as a coordination deferral that writes none.
// The injected clock parks the loop one nanosecond short of its deadline for
// twenty passes, so a lock that can lose to its own expired timer fails here.
func TestBoundedWaitDefersOnCapacityWhenBudgetIsNearlySpent(t *testing.T) {
	root := t.TempDir()
	owner := acquireReservation(t, root, policy.TaskService, fixedPlan(4, policy.GiB, 4, policy.GiB))
	defer func() { _ = guard.ReleaseReservation(root, owner) }()

	const wait = 30 * time.Millisecond
	started := time.Now()
	deadline := started.Add(wait)
	clock, parked := started, 0
	_, err := runtimewiring.AcquireReservationWithOptions(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
		fixedPlan(1, 256*policy.MiB, 4, policy.GiB), 20, wait,
		guard.ReservationAdmissionOptions{
			Metadata: guard.ReservationMetadata{Source: "fixture"},
			Now:      func() time.Time { return clock },
			Pause: func(context.Context, time.Duration) error {
				if parked < 20 {
					parked++
					clock = deadline.Add(-time.Nanosecond)
				} else {
					clock = deadline
				}

				return nil
			},
		},
	)
	if !errors.Is(err, guard.ErrReservationDeferred) || guard.IsCoordinationDeferred(err) {
		t.Fatalf("nearly spent bounded wait returned %v, want a capacity deferral", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "receipts"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("receipt entries=%d error=%v, want one never-started receipt", len(entries), err)
	}
}

// A waiter that stops at a held coordination lock owes the same receipt as one
// whose retry wait ran out, and a receipt it cannot write outranks the
// deferral, so a caller never requeues on a receipt that is missing.
func TestHeldCoordinationLockWithRefusedReceiptReportsTheRefusal(t *testing.T) {
	root := t.TempDir()
	lock, err := os.OpenFile(filepath.Join(root, "coordination.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	receipts := filepath.Join(root, "receipts")
	if err = os.Mkdir(receipts, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(receipts, 0o700) }()

	session, err := runtimewiring.AcquireReservationWithOptions(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
		fixedPlan(1, 256*policy.MiB, 4, policy.GiB), 20, 20*time.Millisecond,
		guard.ReservationAdmissionOptions{Metadata: guard.ReservationMetadata{Source: "fixture"}},
	)
	if session != nil {
		t.Fatal("a held coordination lock admitted the waiter")
	}
	failure, classified := errors.AsType[status.Failure](err)
	if !classified || failure.Code != status.CodeEvidenceUnwritable || guard.IsCoordinationDeferred(err) {
		t.Fatalf("a refused receipt at a held lock returned %v, want only %s", err, status.CodeEvidenceUnwritable)
	}
}

func TestReservationAdmissionIsAtomicAndFIFO(t *testing.T) {
	root := t.TempDir()
	owner := acquireReservation(t, root, policy.TaskService, fixedPlan(2, 512*policy.MiB, 3, 768*policy.MiB))
	largeResult := make(chan *guard.Session, 1)
	largeError := make(chan error, 1)
	go func() {
		session, err := runtimewiring.AcquireReservation(
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

	small, err := runtimewiring.AcquireReservation(
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
	candidate, err := runtimewiring.AcquireReservation(
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
		session, err := runtimewiring.AcquireReservation(
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
	loose, err := runtimewiring.AcquireReservation(
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
	loose, err = runtimewiring.AcquireReservation(
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
			other, err := runtimewiring.AcquireReservation(
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
	for _, cause := range []guard.ShedCause{guard.ShedCauseNone, guard.ShedCause(200)} {
		if _, selected, err := guard.SelectPressureVictim(root, cause); err == nil || selected {
			t.Fatalf("shed cause %d was accepted: selected=%v error=%v", cause, selected, err)
		}
	}
	victim, selected, err := guard.SelectPressureVictim(root, guard.ShedCausePressure)
	if err != nil || !selected || victim.Token != ephemeral.Token {
		t.Fatalf("first victim=%+v selected=%v error=%v", victim, selected, err)
	}
	if err = guard.ReleaseReservation(root, ephemeral); err != nil {
		t.Fatal(err)
	}
	victim, selected, err = guard.SelectPressureVictim(root, guard.ShedCausePressure)
	if err != nil || !selected || victim.Token != service.Token {
		t.Fatalf("second victim=%+v selected=%v error=%v", victim, selected, err)
	}
	if err = guard.ReleaseReservation(root, service); err != nil {
		t.Fatal(err)
	}
	if _, selected, err = guard.SelectPressureVictim(root, guard.ShedCausePressure); err != nil || selected {
		t.Fatalf("transactional owner was selected: selected=%v error=%v", selected, err)
	}
}

// legacyShedCauses is every shed cause with the integer v0.8.4 wrote for it in
// the reservation ledger's sheddingExitCode: 73 for storage and 75 for any other
// pressure, which is the integer the capacity deferral also carried.
var legacyShedCauses = []struct {
	cause  guard.ShedCause
	code   int
	reason policy.Reason
}{
	{guard.ShedCauseNone, 0, policy.ReasonNone},
	{guard.ShedCauseStorage, 73, policy.ReasonStorageBlocked},
	{guard.ShedCausePressure, 75, policy.ReasonPressureShed},
}

func TestShedCauseKeepsItsV084LedgerCode(t *testing.T) {
	for _, row := range legacyShedCauses {
		encoded, err := json.Marshal(row.cause)
		if err != nil || string(encoded) != strconv.Itoa(row.code) {
			t.Errorf("shed cause %d encodes as %s (%v), want %d", row.cause, encoded, err, row.code)
		}
		var decoded guard.ShedCause
		if err = json.Unmarshal([]byte(strconv.Itoa(row.code)), &decoded); err != nil || decoded != row.cause {
			t.Errorf("%d decodes as shed cause %d (%v), want %d", row.code, decoded, err, row.cause)
		}
	}
	if encoded, err := json.Marshal(guard.ShedCause(200)); err == nil {
		t.Errorf("a shed cause that is no member encoded as %s, want a refusal", encoded)
	}
	// 74 is the pressure-shed status inside the process, which a ledger never
	// records; 1 and 72 are neither cause, and the rest are not integers.
	for _, text := range []string{"1", "72", "74", "76", "78", "-73", "73.5", `"73"`, "true", "[]"} {
		decoded := guard.ShedCausePressure
		if err := json.Unmarshal([]byte(text), &decoded); err == nil || decoded != guard.ShedCausePressure {
			t.Errorf("%s decodes as shed cause %d (%v), want a refusal that leaves the cause as it was", text, decoded, err)
		}
	}
}

func TestShedCauseNamesTheReasonItsOwnerStopsFor(t *testing.T) {
	for _, row := range legacyShedCauses {
		if reason := row.cause.Reason(); reason != row.reason {
			t.Errorf("shed cause %d stops for reason %d, want %d", row.cause, reason, row.reason)
		}
	}
	if reason := guard.ShedCause(200).Reason(); reason != policy.ReasonNone {
		t.Errorf("a shed cause that is no member stops for reason %d, want none", reason)
	}
}

func TestEachShedWritesItsV084LedgerCode(t *testing.T) {
	for _, row := range legacyShedCauses[1:] {
		t.Run("sheddingExitCode "+strconv.Itoa(row.code), func(t *testing.T) {
			root := t.TempDir()
			session := acquireReservation(t, root, policy.TaskEphemeral, fixedPlan(1, 256*policy.MiB, 4, policy.GiB))
			defer func() { _ = guard.ReleaseReservation(root, session) }()
			if err := guard.ActivateReservation(root, session, 10_000); err != nil {
				t.Fatal(err)
			}
			victim, selected, err := guard.SelectPressureVictim(root, row.cause)
			if err != nil || !selected || victim.SheddingCause != row.cause {
				t.Fatalf("victim=%+v selected=%v error=%v, want it shed for cause %d", victim, selected, err, row.cause)
			}

			data, err := os.ReadFile(filepath.Join(root, "reservations.json"))
			if err != nil {
				t.Fatal(err)
			}
			var ledger struct {
				Owners []struct {
					Shedding bool `json:"shedding"`
					Code     int  `json:"sheddingExitCode"`
				} `json:"owners"`
			}
			if err = json.Unmarshal(data, &ledger); err != nil || len(ledger.Owners) != 1 ||
				!ledger.Owners[0].Shedding || ledger.Owners[0].Code != row.code {
				t.Fatalf("ledger %s (%v), want one shedding owner recording sheddingExitCode %d", data, err, row.code)
			}
		})
	}
}

func TestALedgerShedCauseOutsideStorageAndPressureFailsAdmissionClosed(t *testing.T) {
	root := t.TempDir()
	marker := []byte("{\"schemaVersion\":1,\"mode\":\"reservation\"}\n")
	if err := os.WriteFile(filepath.Join(root, "coordination-mode.json"), marker, 0o600); err != nil {
		t.Fatal(err)
	}
	owner := `{"token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","pid":1,"class":"ephemeral","profile":"balanced",` +
		`"requested":{"cpu":1,"memoryBytes":268435456},"allocated":{"cpu":1,"memoryBytes":268435456},"sequence":1,` +
		`"processGroup":123,"shedding":true,"sheddingExitCode":74,"maxActiveOwners":20}`
	ledger := []byte(`{"schemaVersion":2,"capacity":{"cpu":4,"memoryBytes":1073741824},"nextSequence":1,"owners":[` +
		owner + `],"waiters":[]}` + "\n")
	path := filepath.Join(root, "reservations.json")
	if err := os.WriteFile(path, ledger, 0o600); err != nil {
		t.Fatal(err)
	}

	session, err := runtimewiring.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
		fixedPlan(1, 256*policy.MiB, 4, policy.GiB), 20, 0,
	)
	if session != nil || err == nil || guard.IsCoordinationDeferred(err) {
		t.Fatalf("admission over a ledger recording sheddingExitCode 74: session=%+v error=%v, want it to fail closed", session, err)
	}
	after, readError := os.ReadFile(path)
	if readError != nil || !bytes.Equal(after, ledger) {
		t.Fatalf("the refused ledger changed: %q error=%v", after, readError)
	}
}

func TestEmergencyPressureSelectsTransactionalLast(t *testing.T) {
	root := t.TempDir()
	transactional := acquireReservation(
		t, root, policy.TaskTransactional, fixedPlan(1, 256*policy.MiB, 4, policy.GiB),
	)
	defer func() { _ = guard.ReleaseReservation(root, transactional) }()
	if err := guard.ActivateReservation(root, transactional, 12_345); err != nil {
		t.Fatal(err)
	}
	if _, selected, err := guard.SelectPressureVictim(root, guard.ShedCausePressure); err != nil || selected {
		t.Fatalf("ordinary pressure selected transactional: selected=%v error=%v", selected, err)
	}
	victim, selected, err := guard.SelectEmergencyPressureVictim(root, guard.ShedCausePressure)
	if err != nil || !selected || victim.Token != transactional.Token || victim.Class != policy.TaskTransactional {
		t.Fatalf("emergency victim=%+v selected=%v error=%v", victim, selected, err)
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

// TestAutomaticOwnerSharesDefaultByLineage holds the share a profile gets when
// nothing configures one to its lineage, not to its name.
func TestAutomaticOwnerSharesDefaultByLineage(t *testing.T) {
	for _, row := range []struct {
		lineage policy.Lineage
		shares  int
	}{
		{policy.LineageBalanced, 4},
		{policy.LineageConstrained, 2},
		{policy.LineageMinimal, 1},
		{policy.LineageUnset, 1},
	} {
		// A name the old switch never knew, and one it knew for another lineage.
		for _, name := range []policy.ProfileName{"local-profile", "balanced"} {
			resolution := policy.Resolution{ResolvedProfile: name, Lineage: row.lineage, MemoryReserve: 4 * policy.GiB}
			settings := guard.ReservationPolicy{Enabled: true, MaxActiveOwners: 20, OwnerShares: map[policy.ProfileName]int{}}
			plan, err := coordination.PlanReservation(reservationSample(), resolution, settings, 0, 0)
			wantCPU := (8 + row.shares - 1) / row.shares
			if err != nil || plan.Requested.CPU != wantCPU ||
				plan.Requested.MemoryBytes != (28*policy.GiB+int64(row.shares)-1)/int64(row.shares) {
				t.Errorf("profile %q of lineage %d planned %+v (%v), want %d owner shares", name, row.lineage, plan.Requested, err, row.shares)
			}
		}
	}
}

func TestAConfiguredOwnerShareOutranksTheLineageDefault(t *testing.T) {
	resolution := policy.Resolution{ResolvedProfile: "local-profile", Lineage: policy.LineageBalanced, MemoryReserve: 4 * policy.GiB}
	settings := guard.ReservationPolicy{Enabled: true, MaxActiveOwners: 20, OwnerShares: map[policy.ProfileName]int{"local-profile": 2}}
	plan, err := coordination.PlanReservation(reservationSample(), resolution, settings, 0, 0)
	if err != nil || plan.Requested.CPU != 4 {
		t.Errorf("a profile configured for two shares planned %+v (%v), want four CPUs", plan.Requested, err)
	}
}
