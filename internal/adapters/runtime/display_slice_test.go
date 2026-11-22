package runtime //nolint:testpackage // Slice (display) doubles run inside the production package, as run_test.go does.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/application"
)

const (
	displaySliceRoot   = "/hippo/display"
	displaySliceToken  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	displaySliceTokenB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var errDisplaySlice = errors.New("display slice port failure")

func displaySliceSystem() (*systemtest.Host, system.System) {
	host := systemtest.NewHost(time.Unix(1700000000, 0).UTC())

	return host, host.Port()
}

func displaySliceLedger(t *testing.T, owners, waiters string) []byte {
	t.Helper()

	return []byte(`{"schemaVersion":2,"capacity":{"cpu":4,"memoryBytes":1073741824},` +
		`"nextSequence":3,"owners":[` + owners + `],"waiters":[` + waiters + `]}` + "\n")
}

func displaySliceOwner(token string) string {
	return `{"token":"` + token + `","pid":1,"class":"ephemeral","profile":"balanced",` +
		`"requested":{"cpu":1,"memoryBytes":268435456},"allocated":{"cpu":1,"memoryBytes":268435456},` +
		`"sequence":1,"maxActiveOwners":20}`
}

func TestDisplayRegularPathReportsAnUnclassifiedError(t *testing.T) {
	_, port := displaySliceSystem()
	port.Files.Lstat = func(string) (os.FileInfo, error) { return nil, errDisplaySlice }
	if _, err := displayRegularPath(port, displaySliceRoot+"/run-display/x.json"); err == nil {
		t.Fatal("display regular path error = nil, want the injected failure")
	}
}

func TestDisplayDirectoryEntriesReportsAnUnclassifiedError(t *testing.T) {
	_, port := displaySliceSystem()
	port.Files.Lstat = func(string) (os.FileInfo, error) { return nil, errDisplaySlice }
	if _, err := displayDirectoryEntries(port, displaySliceRoot+"/sessions"); err == nil {
		t.Fatal("display directory entries error = nil, want the injected failure")
	}
}

func TestReadDisplayCensusRecordReportsAReadError(t *testing.T) {
	host, port := displaySliceSystem()
	path := displaySliceRoot + "/reservations.json"
	host.Files.Contents[path] = []byte("{}\n")
	port.Files.ReadFile = func(string) ([]byte, error) { return nil, errDisplaySlice }
	var ledger reservationLedger
	if _, err := readDisplayCensusRecord(port, path, &ledger); err == nil {
		t.Fatal("display census read error = nil, want the injected failure")
	}
}

func TestDisplayReservationCensusRefusesAnInvalidLedger(t *testing.T) {
	host, port := displaySliceSystem()
	host.Files.Contents[displaySliceRoot+"/reservations.json"] = []byte("{\"schemaVersion\":99}\n")
	var live []application.DisplayLiveness
	if err := displayReservationCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display reservation census = nil, want the ledger refusal")
	}
}

func TestDisplayExclusiveCensusReportsAnInventoryError(t *testing.T) {
	host, port := displaySliceSystem()
	host.Files.Dirs[displaySliceRoot+"/sessions"] = true
	port.Files.ReadDir = func(string) ([]os.DirEntry, error) { return nil, errDisplaySlice }
	var live []application.DisplayLiveness
	if err := displayExclusiveCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display exclusive census = nil, want the inventory failure")
	}
}

func TestDisplayExclusiveCensusRefusesAnInvalidSessionEntry(t *testing.T) {
	host, port := displaySliceSystem()
	host.Files.Dirs[displaySliceRoot+"/sessions"] = true
	host.Files.Contents[displaySliceRoot+"/sessions/not-a-token.json"] = []byte("{}\n")
	var live []application.DisplayLiveness
	if err := displayExclusiveCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display exclusive census = nil, want the invalid entry refusal")
	}
}

func TestDisplayExclusiveCensusRefusesAnUnverifiableSessionRecord(t *testing.T) {
	host, port := displaySliceSystem()
	host.Files.Dirs[displaySliceRoot+"/sessions"] = true
	host.Files.Contents[displaySliceRoot+"/sessions/"+displaySliceToken+".json"] = []byte("{}\n")
	var live []application.DisplayLiveness
	if err := displayExclusiveCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display exclusive census = nil, want the unverifiable record refusal")
	}
}

func TestDisplayExclusiveCensusReportsAnUnverifiableOwner(t *testing.T) {
	host, port := displaySliceSystem()
	host.Files.Dirs[displaySliceRoot+"/sessions"] = true
	host.Files.Contents[displaySliceRoot+"/sessions/"+displaySliceToken+".json"] = []byte(
		`{"schemaVersion":1,"pid":99,"token":"` + displaySliceToken +
			`","class":"ephemeral","identityDevice":1}` + "\n")
	var live []application.DisplayLiveness
	if err := displayExclusiveCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display exclusive census = nil, want the unverifiable owner refusal")
	}
}

func TestDisplayExclusiveCensusReportsAHeavyInventoryError(t *testing.T) {
	host, port := displaySliceSystem()
	host.Files.Dirs[displaySliceRoot+"/heavy.lock"] = true
	port.Files.ReadDir = func(string) ([]os.DirEntry, error) { return nil, errDisplaySlice }
	var live []application.DisplayLiveness
	if err := displayExclusiveCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display exclusive census = nil, want the heavy inventory failure")
	}
}

func TestDisplayExclusiveCensusRefusesANonOwnerHeavyEntry(t *testing.T) {
	host, port := displaySliceSystem()
	host.Files.Dirs[displaySliceRoot+"/heavy.lock"] = true
	host.Files.Contents[displaySliceRoot+"/heavy.lock/other.json"] = []byte("{}\n")
	var live []application.DisplayLiveness
	if err := displayExclusiveCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display exclusive census = nil, want the non-owner entry refusal")
	}
}

func TestDisplayExclusiveCensusReportsAHeavyRecordError(t *testing.T) {
	host, port := displaySliceSystem()
	host.Files.Dirs[displaySliceRoot+"/sessions"] = true
	host.Files.Dirs[displaySliceRoot+"/heavy.lock"] = true
	host.Files.Contents[displaySliceRoot+"/heavy.lock/owner.json"] = []byte("{}\n")
	port.Files.ReadFile = func(string) ([]byte, error) { return nil, errDisplaySlice }
	var live []application.DisplayLiveness
	if err := displayExclusiveCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display exclusive census = nil, want the heavy record failure")
	}
}

func TestDisplayExclusiveCensusRefusesHeavyOwnershipWithoutARecord(t *testing.T) {
	host, port := displaySliceSystem()
	host.Files.Dirs[displaySliceRoot+"/heavy.lock"] = true
	var live []application.DisplayLiveness
	if err := displayExclusiveCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display exclusive census = nil, want the heavy ownership refusal")
	}
}

func TestDisplayExclusiveCensusReportsAnUnverifiableHeavyOwner(t *testing.T) {
	host, port := displaySliceSystem()
	host.Files.Dirs[displaySliceRoot+"/heavy.lock"] = true
	host.Files.Contents[displaySliceRoot+"/heavy.lock/owner.json"] = []byte(
		`{"schemaVersion":1,"pid":99,"token":"` + displaySliceToken +
			`","class":"ephemeral","identityDevice":1}` + "\n")
	var live []application.DisplayLiveness
	if err := displayExclusiveCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display exclusive census = nil, want the unverifiable heavy owner refusal")
	}
}

func TestDisplayIdentityCensusReportsAnInventoryError(t *testing.T) {
	host, port := displaySliceSystem()
	identities := displaySliceRoot + "/reservation-identities"
	host.Files.Dirs[identities] = true
	port.Files.ReadDir = func(string) ([]os.DirEntry, error) { return nil, errDisplaySlice }
	var live []application.DisplayLiveness
	if err := displayIdentityCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display identity census = nil, want the inventory failure")
	}
}

func TestDisplayIdentityCensusRefusesAnInvalidEntry(t *testing.T) {
	host, port := displaySliceSystem()
	identities := displaySliceRoot + "/reservation-identities"
	host.Files.Dirs[identities] = true
	host.Files.Contents[identities+"/not-a-token.lock"] = []byte("x")
	var live []application.DisplayLiveness
	if err := displayIdentityCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display identity census = nil, want the invalid entry refusal")
	}
}

func TestDisplayIdentityCensusRefusesAnUnknownExtension(t *testing.T) {
	host, port := displaySliceSystem()
	identities := displaySliceRoot + "/reservation-identities"
	host.Files.Dirs[identities] = true
	host.Files.Contents[identities+"/"+displaySliceToken+".bad"] = []byte("x")
	var live []application.DisplayLiveness
	if err := displayIdentityCensus(port, displaySliceRoot, &live); err == nil {
		t.Fatal("display identity census = nil, want the unknown extension refusal")
	}
}

func TestDisplayCensusReportsAMarkerError(t *testing.T) {
	_, port := displaySliceSystem()
	port.Files.Lstat = func(string) (os.FileInfo, error) { return nil, errDisplaySlice }
	if _, err := displayCensus(port, displaySliceRoot); err == nil {
		t.Fatal("display census = nil, want the marker failure")
	}
}

func displaySliceRealRoot(t *testing.T) string {
	t.Helper()

	return t.TempDir()
}

func TestDisplayCensusRefusesAnUnreadableMarker(t *testing.T) {
	root := displaySliceRealRoot(t)
	if err := os.WriteFile(filepath.Join(root, "coordination-mode.json"), []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := displayCensus(system.Production, root); err == nil {
		t.Fatal("display census = nil, want the marker refusal")
	}
}

func TestDisplayCensusReportsAReservationError(t *testing.T) {
	root := displaySliceRealRoot(t)
	ledger := displaySliceLedger(t, `{"token":"bad","pid":1,"class":"ephemeral","profile":"balanced",`+
		`"requested":{"cpu":1,"memoryBytes":268435456},"allocated":{"cpu":1,"memoryBytes":268435456},`+
		`"sequence":1,"maxActiveOwners":20}`, "")
	if err := os.WriteFile(filepath.Join(root, "reservations.json"), ledger, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := displayCensus(system.Production, root); err == nil {
		t.Fatal("display census = nil, want the reservation failure")
	}
}

func TestDisplayReservationCensusReportsAnOwnerIdentityError(t *testing.T) {
	root := displaySliceRealRoot(t)
	owner := displaySliceOwner(displaySliceToken)
	ledger := displaySliceLedger(t, owner, "")
	if err := os.WriteFile(filepath.Join(root, "reservations.json"), ledger, 0o600); err != nil {
		t.Fatal(err)
	}
	identities := filepath.Join(root, "reservation-identities")
	if err := os.MkdirAll(identities, 0o700); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(identities, displaySliceToken+".lock")
	if err := os.WriteFile(lock, []byte("lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(lock, filepath.Join(identities, displaySliceToken+".anchor")); err != nil {
		t.Fatal(err)
	}
	port := system.Production
	port.Files.Flock = func(system.File, int) error { return errDisplaySlice }
	var live []application.DisplayLiveness
	if err := displayReservationCensus(port, root, &live); err == nil {
		t.Fatal("display reservation census = nil, want the owner identity failure")
	}
}

func TestDisplayReservationCensusReportsAWaiterIdentityError(t *testing.T) {
	root := displaySliceRealRoot(t)
	owner := displaySliceOwner(displaySliceToken)
	waiter := `{"token":"` + displaySliceTokenB + `","pid":1,"class":"ephemeral","profile":"balanced",` +
		`"requested":{"cpu":1,"memoryBytes":268435456},"sequence":2,"maxActiveOwners":20}`
	ledger := displaySliceLedger(t, owner, waiter)
	if err := os.WriteFile(filepath.Join(root, "reservations.json"), ledger, 0o600); err != nil {
		t.Fatal(err)
	}
	identities := filepath.Join(root, "reservation-identities")
	if err := os.MkdirAll(identities, 0o700); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(identities, displaySliceTokenB+".lock")
	if err := os.WriteFile(lock, []byte("lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(lock, filepath.Join(identities, displaySliceTokenB+".anchor")); err != nil {
		t.Fatal(err)
	}
	port := system.Production
	port.Files.Flock = func(system.File, int) error { return errDisplaySlice }
	var live []application.DisplayLiveness
	if err := displayReservationCensus(port, root, &live); err == nil {
		t.Fatal("display reservation census = nil, want the waiter identity failure")
	}
}

func TestHousekeepDisplayLockedReportsACleanupError(t *testing.T) {
	root := displaySliceRealRoot(t)
	marker, err := os.Create(filepath.Join(root, "marker"))
	if err != nil {
		t.Fatal(err)
	}
	if err = marker.Close(); err != nil {
		t.Fatal(err)
	}
	port := system.Production
	port.Files.Lstat = func(string) (os.FileInfo, error) {
		t.Error("housekeep ran the census after the prune failed")
		return nil, errDisplaySlice
	}
	housekeepDisplayLocked(port, marker.Name(), time.Now())
}

func TestRetireUnregisteredDisplayReportsALockFailure(t *testing.T) {
	root := displaySliceRealRoot(t)
	record := filepath.Join(root, "run-display", displaySliceToken+".json")
	if err := os.MkdirAll(filepath.Dir(record), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	port := system.Production
	port.Files.Flock = func(system.File, int) error { return unix.EIO }
	advisories := &application.DisplayAdvisories{}
	retireUnregisteredDisplay(port, root, displaySliceToken, time.Second, advisories)
	if _, err := os.Stat(record); err != nil {
		t.Fatalf("retired record: %v, want the record kept when the lock fails", err)
	}
}

func TestRetireUnregisteredDisplayReportsAReleaseFailure(t *testing.T) {
	root := displaySliceRealRoot(t)
	record := filepath.Join(root, "run-display", displaySliceToken+".json")
	if err := os.MkdirAll(filepath.Dir(record), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	port := system.Production
	base := port.Files.Flock
	port.Files.Flock = func(file system.File, how int) error {
		if how == unix.LOCK_UN {
			return unix.EIO
		}

		return base(file, how)
	}
	advisories := &application.DisplayAdvisories{}
	retireUnregisteredDisplay(port, root, displaySliceToken, time.Second, advisories)
	if _, err := os.Stat(record); !os.IsNotExist(err) {
		t.Fatalf("retired record stat = %v, want the record removed before the release failed", err)
	}
}

func TestHousekeepIdleDisplaySkipsARootWithoutALock(t *testing.T) {
	root := displaySliceRealRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "run-display"), 0o700); err != nil {
		t.Fatal(err)
	}
	housekeepIdleDisplay(context.Background(), system.Production, root)
	if _, err := os.Stat(filepath.Join(root, "coordination.lock")); !os.IsNotExist(err) {
		t.Fatalf("coordination lock stat = %v, want no lock initialized to prune annotations", err)
	}
}

func TestHousekeepIdleDisplayReportsALockFailure(t *testing.T) {
	root := displaySliceRealRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "run-display"), 0o700); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(root, "coordination.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	port := system.Production
	port.Files.Flock = func(system.File, int) error { return unix.EIO }
	housekeepIdleDisplay(context.Background(), port, root)
}

func TestHousekeepIdleDisplayReportsAReleaseFailure(t *testing.T) {
	root := displaySliceRealRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "run-display"), 0o700); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(root, "coordination.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	port := system.Production
	base := port.Files.Flock
	port.Files.Flock = func(file system.File, how int) error {
		if how == unix.LOCK_UN {
			return unix.EIO
		}

		return base(file, how)
	}
	housekeepIdleDisplay(context.Background(), port, root)
}
