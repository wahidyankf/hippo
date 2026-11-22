package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

func TestFloorRetainWaiterMonitorRetriesFailedRemoval(t *testing.T) {
	root := t.TempDir()
	floorWriteLedger(t, root, floorEmptyLedger())
	identity, err := openReservationIdentity(system.Production, root, floorToken)
	if err != nil {
		t.Fatal(err)
	}
	// No defer identity.Close here: the handle below is stolen by the waiter
	// monitor, which closes it once the deferred removal lands.
	held, err := acquireCoordinationLock(system.Production, context.Background(), root, 0)
	if err != nil {
		t.Fatal(err)
	}
	retainReservationWaiterUntilCleanup(system.Production, root, floorToken, identity)
	// The first deferred removal spends its whole lifecycle wait against the
	// held coordination lock and retries, so the monitor outlives this sleep
	// and finishes only after the test hands the lock back.
	time.Sleep(250 * time.Millisecond)
	if err := releaseCoordinationLock(system.Production, held); err != nil {
		t.Fatal(err)
	}
	floorPollForRemoval(t, filepath.Join(root, "reservation-identities", floorToken+".lock"))
}
