package coordination_test

import (
	"testing"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestFIFOAndVictimTransitions(t *testing.T) {
	ledger := coordination.Ledger{Capacity: coordination.ReservationVector{CPU: 4, MemoryBytes: 4 * policy.GiB}}
	for _, id := range []string{"first", "second"} {
		if err := coordination.Enqueue(&ledger, coordination.Waiter{Token: id, Class: policy.TaskEphemeral, MaxOwners: 2}); err != nil {
			t.Fatal(err)
		}
	}
	if len(ledger.Waiters) != 2 || ledger.Waiters[0].Sequence != 1 || ledger.Waiters[1].Sequence != 2 {
		t.Fatalf("FIFO was not registered: %+v", ledger)
	}
	minimum := coordination.ReservationVector{CPU: 1, MemoryBytes: policy.GiB}
	maximum := coordination.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB}
	if _, admitted, err := coordination.AdmitHead(&ledger, "second", minimum, maximum, 2); admitted || err != nil {
		t.Fatalf("later waiter bypassed head: %t %v", admitted, err)
	}
	allocation, admitted, err := coordination.AdmitHead(&ledger, "first", minimum, maximum, 2)
	if !admitted || err != nil || allocation != maximum || len(ledger.Owners) != 1 || len(ledger.Waiters) != 1 {
		t.Fatalf("head allocation=%+v admitted=%t err=%v ledger=%+v", allocation, admitted, err, ledger)
	}
	ledger.Owners[0].ProcessGroup = 42
	victim, selected, err := coordination.ElectVictim(&ledger, coordination.ShedCausePressure, false)
	if !selected || err != nil || victim.Token != "first" || !ledger.Owners[0].Shedding {
		t.Fatalf("victim=%+v selected=%t err=%v", victim, selected, err)
	}
	if _, selected, err = coordination.ElectVictim(&ledger, coordination.ShedCausePressure, false); selected || err != nil {
		t.Fatalf("second victim elected: %t %v", selected, err)
	}
}
