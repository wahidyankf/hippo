package coordination_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

func validCoordinationLedger() coordination.Ledger {
	vector := coordination.ReservationVector{CPU: 1, MemoryBytes: policy.GiB}
	return coordination.Ledger{
		SchemaVersion: coordination.LedgerSchemaVersion, NextSequence: 2,
		Capacity: coordination.ReservationVector{CPU: 4, MemoryBytes: 4 * policy.GiB},
		Owners: []coordination.ReservationOwner{{
			Token: strings.Repeat("a", 32), PID: 1, Class: policy.TaskEphemeral, Profile: "balanced",
			Requested: vector, Allocated: vector, Sequence: 1, MaxOwners: 2, PeakOwners: 1,
			IdentityDevice: 1, IdentityInode: 2, ProcessGroup: 1,
		}},
		Waiters: []coordination.Waiter{{
			Token: strings.Repeat("b", 32), PID: 2, Class: policy.TaskService, Profile: "balanced",
			Requested: vector, Sequence: 2, MaxOwners: 2, IdentityDevice: 1, IdentityInode: 3,
		}},
	}
}

func copyCoordinationLedger(ledger coordination.Ledger) coordination.Ledger {
	ledger.Owners = append([]coordination.ReservationOwner(nil), ledger.Owners...)
	ledger.Waiters = append([]coordination.Waiter(nil), ledger.Waiters...)
	return ledger
}

type invalidLedgerCase struct {
	name   string
	change func(*coordination.Ledger)
	want   string
}

func checkRejectedLedgers(t *testing.T, cases []invalidLedgerCase) {
	t.Helper()
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ledger := validCoordinationLedger()
			test.change(&ledger)
			before := copyCoordinationLedger(ledger)
			if err := coordination.ValidateLedger(ledger); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("invalid ledger error=%v, want %q", err, test.want)
			}
			if !reflect.DeepEqual(ledger, before) {
				t.Fatal("validation changed the rejected ledger")
			}
		})
	}
}

func TestLedgerValidationRejectsInvalidProtocolAndCapacity(t *testing.T) {
	checkRejectedLedgers(t, []invalidLedgerCase{
		{"unset schema", func(ledger *coordination.Ledger) { ledger.SchemaVersion = 0 }, "schema must be positive"},
		{"negative CPU capacity", func(ledger *coordination.Ledger) { ledger.Capacity.CPU = -1 }, "capacity must be nonnegative"},
		{"negative memory capacity", func(ledger *coordination.Ledger) { ledger.Capacity.MemoryBytes = -1 }, "capacity must be nonnegative"},
		{"active CPU below floor", func(ledger *coordination.Ledger) { ledger.Capacity.CPU = 0 }, "active capacity is below"},
		{"active memory below floor", func(ledger *coordination.Ledger) {
			ledger.Capacity.MemoryBytes = coordination.MinimumReservationMemoryBytes - 1
		}, "active capacity is below"},
	})
	ledger := validCoordinationLedger()
	ledger.SchemaVersion++
	if err := coordination.ValidateLedger(ledger); !errors.Is(err, coordination.ErrCoordinationProtocolMismatch) {
		t.Fatalf("unknown positive schema error=%v, want a protocol mismatch", err)
	}
	if err := coordination.ValidateLedger(validCoordinationLedger()); err != nil {
		t.Fatalf("valid owner and FIFO waiter rejected: %v", err)
	}
}

func TestLedgerValidationRejectsMalformedOwners(t *testing.T) {
	checkRejectedLedgers(t, []invalidLedgerCase{
		{"owner PID", func(ledger *coordination.Ledger) { ledger.Owners[0].PID = 0 }, "owner PID"},
		{"owner profile", func(ledger *coordination.Ledger) { ledger.Owners[0].Profile = "" }, "owner profile"},
		{"owner limit below floor", func(ledger *coordination.Ledger) { ledger.Owners[0].MaxOwners = 0 }, "owner maximum"},
		{"owner limit above ceiling", func(ledger *coordination.Ledger) { ledger.Owners[0].MaxOwners = coordination.MaximumOwners + 1 }, "owner maximum"},
		{"owner device missing", func(ledger *coordination.Ledger) { ledger.Owners[0].IdentityDevice = 0 }, "owner identity"},
		{"owner inode missing", func(ledger *coordination.Ledger) { ledger.Owners[0].IdentityInode = 0 }, "owner identity"},
		{"negative peak", func(ledger *coordination.Ledger) { ledger.Owners[0].PeakOwners = -1 }, "owner peak"},
		{"peak above ceiling", func(ledger *coordination.Ledger) { ledger.Owners[0].PeakOwners = coordination.MaximumOwners + 1 }, "owner peak"},
		{"unset owner sequence", func(ledger *coordination.Ledger) { ledger.Owners[0].Sequence = 0 }, "owner sequence"},
		{"owner sequence after ledger", func(ledger *coordination.Ledger) { ledger.Owners[0].Sequence = 3 }, "owner sequence"},
		{"allocation below floor", func(ledger *coordination.Ledger) { ledger.Owners[0].Allocated.MemoryBytes = 0 }, "owner allocation"},
		{"allocation exceeds capacity", func(ledger *coordination.Ledger) { ledger.Owners[0].Allocated.CPU = 5 }, "owner allocation exceeds"},
	})
}

func TestLedgerValidationRejectsMalformedWaiters(t *testing.T) {
	checkRejectedLedgers(t, []invalidLedgerCase{
		{"waiter PID", func(ledger *coordination.Ledger) { ledger.Waiters[0].PID = 0 }, "waiter PID"},
		{"waiter profile", func(ledger *coordination.Ledger) { ledger.Waiters[0].Profile = "" }, "waiter profile"},
		{"waiter limit below floor", func(ledger *coordination.Ledger) { ledger.Waiters[0].MaxOwners = 0 }, "waiter maximum"},
		{"waiter limit above ceiling", func(ledger *coordination.Ledger) { ledger.Waiters[0].MaxOwners = coordination.MaximumOwners + 1 }, "waiter maximum"},
		{"waiter device missing", func(ledger *coordination.Ledger) { ledger.Waiters[0].IdentityDevice = 0 }, "waiter identity"},
		{"waiter inode missing", func(ledger *coordination.Ledger) { ledger.Waiters[0].IdentityInode = 0 }, "waiter identity"},
		{"request below floor", func(ledger *coordination.Ledger) { ledger.Waiters[0].Requested.MemoryBytes = 0 }, "waiter request"},
		{"request exceeds capacity", func(ledger *coordination.Ledger) { ledger.Waiters[0].Requested.CPU = 5 }, "waiter request exceeds"},
	})
}
