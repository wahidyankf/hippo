package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"os"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

func TestFloorRemoveReservationWaiterReportsUnwritableLedger(t *testing.T) {
	root := t.TempDir()
	floorWriteLedger(t, root, floorEmptyLedger())
	lock, err := openCoordinationLock(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = lock.Close()
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
	if err := removeReservationWaiterWithAdvisories(
		system.Production, root, floorToken, time.Second, nil,
	); err == nil {
		t.Fatal("removeReservationWaiterWithAdvisories under a read-only root = nil, want error")
	}
}
