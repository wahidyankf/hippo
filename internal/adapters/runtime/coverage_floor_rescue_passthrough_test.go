package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

func TestFloorRescueMonitorPassthroughReturnsLockError(t *testing.T) {
	root := t.TempDir()
	floorRescueOwnerLedger(t, root)
	lockPath := filepath.Join(root, "reservation-identities", floorToken+".lock")
	identity, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	// No defer identity.Close here: the handle below is stolen by the rescue
	// monitor, which closes it when the deferred release lands. Closing it
	// here would race that monitor shutdown.
	session := &Session{Token: floorToken, identityLock: identity}
	boom := errors.New("floor: reconcile probe refused")
	err = retainReleaseForReconciliation(system.Production, context.Background(), root, session, boom)
	if err != boom {
		t.Fatalf("retainReleaseForReconciliation on a foreign failure = %v, want the failure itself", err)
	}
	if session.identityLock != nil {
		t.Fatal("rescue left the stolen identity handle referenced by the session")
	}
	floorPollForRemoval(t, lockPath, reservationLedgerPath(root))
}
