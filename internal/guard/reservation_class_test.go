package guard //nolint:testpackage // validReservationClass is unexported, and the set it admits is pinned here directly.

import (
	"testing"

	"github.com/wahidyankf/hippo/internal/policy"
)

// TestAReservationHoldsExactlyTheClassesThatReserve holds validation, the step after decoding, to the classes a
// ledger may hold: ephemeral, service, and transactional. Release is a member that never reserves, and a class with no
// member never does either. Decoding cannot stand in for the member half: an owner or waiter whose class is absent or
// null never reaches the decoder and arrives here as the empty class, so only this check refuses it. A class added to
// TaskClasses must be decided here rather than admitted for being a member: the map must name every member, as the
// exhaustive linter requires of a map keyed by an enumerated type, and a member it leaves out fails the test.
func TestAReservationHoldsExactlyTheClassesThatReserve(t *testing.T) {
	reserves := map[policy.TaskClass]bool{
		policy.TaskEphemeral:     true,
		policy.TaskService:       true,
		policy.TaskTransactional: true,
		policy.TaskRelease:       false,
	}
	for _, class := range policy.TaskClasses() {
		want, decided := reserves[class]
		if !decided {
			t.Errorf("TaskClasses lists %q, and this test records no decision for whether a reservation may hold it", class)
		}
		if got := validReservationClass(class); got != want {
			t.Errorf("validReservationClass(%q) = %t, want %t", class, got, want)
		}
	}
	for _, class := range []policy.TaskClass{"", "batch", "heavy", "Ephemeral", "service "} {
		if validReservationClass(class) {
			t.Errorf("validReservationClass(%q) accepted a class with no member", class)
		}
	}
}
