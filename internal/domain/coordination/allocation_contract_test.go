package coordination_test

import (
	"reflect"
	"testing"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestVectorFitRejectsInvalidAccountingWithoutOverflow(t *testing.T) {
	capacity := coordination.ReservationVector{CPU: 4, MemoryBytes: 4 * policy.GiB}
	unit := coordination.ReservationVector{CPU: 1, MemoryBytes: policy.GiB}
	cases := []struct {
		name      string
		used      coordination.ReservationVector
		requested coordination.ReservationVector
		capacity  coordination.ReservationVector
	}{
		{"negative used CPU", coordination.ReservationVector{CPU: -1}, unit, capacity},
		{"negative used memory", coordination.ReservationVector{MemoryBytes: -1}, unit, capacity},
		{"negative requested CPU", unit, coordination.ReservationVector{CPU: -1}, capacity},
		{"negative requested memory", unit, coordination.ReservationVector{MemoryBytes: -1}, capacity},
		{"negative capacity CPU", unit, unit, coordination.ReservationVector{CPU: -1}},
		{"negative capacity memory", unit, unit, coordination.ReservationVector{MemoryBytes: -1}},
		{"used CPU over capacity", coordination.ReservationVector{CPU: 5}, unit, capacity},
		{"used memory over capacity", coordination.ReservationVector{MemoryBytes: 5 * policy.GiB}, unit, capacity},
		{"request CPU over capacity", unit, coordination.ReservationVector{CPU: 5}, capacity},
		{"request memory over capacity", unit, coordination.ReservationVector{MemoryBytes: 5 * policy.GiB}, capacity},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if coordination.VectorFits(test.used, test.requested, test.capacity) {
				t.Fatal("invalid accounting was declared to fit")
			}
		})
	}
	maximum := coordination.ReservationVector{CPU: int(^uint(0) >> 1), MemoryBytes: int64(^uint64(0) >> 1)}
	if coordination.VectorFits(maximum, unit, maximum) {
		t.Fatal("full maximum-width capacity accepted another allocation")
	}
	if !coordination.VectorFits(coordination.ReservationVector{}, maximum, maximum) {
		t.Fatal("an exact maximum-width allocation did not fit an empty pool")
	}
}

func overflowingOwners() []coordination.ReservationOwner {
	return []coordination.ReservationOwner{
		{Allocated: coordination.ReservationVector{CPU: int(^uint(0) >> 1), MemoryBytes: policy.GiB}},
		{Allocated: coordination.ReservationVector{CPU: 1, MemoryBytes: policy.GiB}},
	}
}

func TestOverflowedOwnersCannotProduceTotalsOrAdmission(t *testing.T) {
	owners := overflowingOwners()
	total, err := coordination.SumOwners(owners)
	if err == nil || total != (coordination.ReservationVector{}) {
		t.Fatalf("overflowed owners returned total=%+v error=%v", total, err)
	}
	ledger := validCoordinationLedger()
	ledger.Owners = owners
	before := copyCoordinationLedger(ledger)
	allocation, admitted, err := coordination.AdmitHead(&ledger, ledger.Waiters[0].Token, ledger.Waiters[0].Requested, ledger.Waiters[0].Requested, 2)
	if err == nil || admitted || allocation != (coordination.ReservationVector{}) || !reflect.DeepEqual(ledger, before) {
		t.Fatalf("invalid totals admitted=%t allocation=%+v error=%v or changed ledger", admitted, allocation, err)
	}
}

func TestCapacityUpdatePreservesEpochWhenOwnersOrWaitersCannotFit(t *testing.T) {
	for _, reason := range []string{"owner overflow", "pending waiter", "active owner"} {
		t.Run(reason, func(t *testing.T) {
			ledger := validCoordinationLedger()
			capacity := coordination.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB}
			switch reason {
			case "owner overflow":
				ledger.Owners = overflowingOwners()
			case "pending waiter":
				ledger.Waiters[0].Requested = coordination.ReservationVector{CPU: 3, MemoryBytes: 3 * policy.GiB}
			case "active owner":
				ledger.Owners[0].Allocated = coordination.ReservationVector{CPU: 3, MemoryBytes: 3 * policy.GiB}
			}
			before := copyCoordinationLedger(ledger)
			if coordination.UpdateCapacity(&ledger, capacity) || !reflect.DeepEqual(ledger, before) {
				t.Fatal("capacity update abandoned a live or pending commitment")
			}
		})
	}
}

func TestSharedOwnerLimitKeepsTheConservativeLiveMinimum(t *testing.T) {
	for _, supplied := range []int{-1, 0, coordination.MaximumOwners + 1} {
		if got := coordination.NormalizedOwnerLimit(supplied); got != coordination.MaximumOwners {
			t.Errorf("unspecified owner limit %d normalized to %d", supplied, got)
		}
	}
	ledger := validCoordinationLedger()
	ledger.Waiters[0].MaxOwners = 1
	if got := coordination.SharedOwnerLimit(ledger, 3); got != 1 {
		t.Fatalf("shared owner limit=%d, want pending waiter's conservative limit 1", got)
	}
	if got := coordination.NormalizedOwnerLimit(2); got != 2 {
		t.Errorf("explicit owner limit changed to %d", got)
	}
}

func TestInvalidVictimCauseLeavesAllOwnersUntouched(t *testing.T) {
	ledger := validCoordinationLedger()
	before := copyCoordinationLedger(ledger)
	victim, selected, err := coordination.ElectVictim(&ledger, coordination.ShedCauseNone, true)
	if err == nil || selected || victim != (coordination.ReservationOwner{}) || !reflect.DeepEqual(ledger, before) {
		t.Fatalf("invalid victim cause returned victim=%+v selected=%t error=%v or changed ledger", victim, selected, err)
	}
}
