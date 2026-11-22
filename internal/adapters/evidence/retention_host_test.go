package evidence //nolint:testpackage // Host-seam tests drive the unexported retention functions that take the system port.

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"
	"time"
)

// redInactiveCap is an entry size that fills most of the 50 MiB inactive cap, so that two of them exceed it.
const redInactiveCap = 40 << 20

func TestLivePIDRejectsANonPositiveIdentifierWithoutSignalling(t *testing.T) {
	host := redHost()
	if livePID(host.Port(), 0) {
		t.Fatal("pid zero was reported live")
	}
	if len(host.Processes.Sent) != 0 {
		t.Fatalf("signals sent for pid zero: %+v", host.Processes.Sent)
	}
}

func TestLivePIDTreatsAPermissionRefusalAsLive(t *testing.T) {
	port := redHost().Port()
	port.Processes.Kill = func(int, syscall.Signal) error { return syscall.EPERM }
	if !livePID(port, 42) {
		t.Fatal("a process the caller may not signal was reported dead")
	}
}

func TestLivePIDReportsAMissingProcessAsDeadAndALiveOneAsLive(t *testing.T) {
	host := redHost()
	if livePID(host.Port(), 42) {
		t.Fatal("a missing process was reported live")
	}
	host.Processes.Alive[42] = true
	if !livePID(host.Port(), 42) {
		t.Fatal("a live process was reported dead")
	}
}

func TestActivePrefixesSkipsAMarkerThatVanishedAfterListing(t *testing.T) {
	host := redHost()
	redDirs(host, redRoot)
	marker := redRoot + "/run.active.json"
	host.Files.Contents[marker] = []byte(`{"schemaVersion":1,"pid":5}`)
	port := host.Port()
	redReadFile(&port, marker, 0, nil, fs.ErrNotExist)
	entries, err := port.Files.ReadDir(redRoot)
	if err != nil {
		t.Fatal(err)
	}
	active, err := activePrefixes(port, entries, redRoot)
	if err != nil || len(active) != 0 {
		t.Fatalf("active writers = %v with %v, want none", active, err)
	}
}

func TestActivePrefixesReportsAnUnreadableMarker(t *testing.T) {
	host := redHost()
	redDirs(host, redRoot)
	marker := redRoot + "/run.active.json"
	host.Files.Contents[marker] = []byte(`{"schemaVersion":1,"pid":5}`)
	port := host.Port()
	redReadFile(&port, marker, 0, nil, errRed)
	entries, err := port.Files.ReadDir(redRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = activePrefixes(port, entries, redRoot); !errors.Is(err, errRed) {
		t.Fatalf("marker read error = %v, want the injected failure", err)
	}
}

func TestActivePrefixesKeepsALiveWriterAndRemovesAStaleMarker(t *testing.T) {
	host := redHost()
	redDirs(host, redRoot)
	host.Processes.Alive[5] = true
	live := redRoot + "/live.active.json"
	stale := redRoot + "/stale.active.json"
	host.Files.Contents[live] = []byte(`{"schemaVersion":1,"pid":5}`)
	host.Files.Contents[stale] = []byte(`{"schemaVersion":1,"pid":6}`)
	entries, err := host.Port().Files.ReadDir(redRoot)
	if err != nil {
		t.Fatal(err)
	}
	active, err := activePrefixes(host.Port(), entries, redRoot)
	if err != nil || !active[redRoot+"/live."] {
		t.Fatalf("active writers = %v with %v, want the live writer", active, err)
	}
	if _, survived := host.Files.Contents[stale]; survived {
		t.Fatal("a stale marker survived")
	}
}

func TestActivePrefixesReportsAFailedRemovalOfAnInvalidMarker(t *testing.T) {
	host := redHost()
	redDirs(host, redRoot)
	host.Files.Contents[redRoot+"/bad.active.json"] = []byte("{")
	port := host.Port()
	port.Files.Remove = func(string) error { return errRed }
	entries, err := port.Files.ReadDir(redRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = activePrefixes(port, entries, redRoot); !errors.Is(err, errRed) {
		t.Fatalf("marker removal error = %v, want the injected failure", err)
	}
}

func TestCleanupReportsARootListingFailure(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	redReadDirFails(&port, redRoot, errRed)
	if err := cleanupLocked(port, redRoot, redStart); !errors.Is(err, errRed) {
		t.Fatalf("listing error = %v, want the injected failure", err)
	}
}

func TestCleanupReportsAnUnreadableActiveMarker(t *testing.T) {
	host := redCompactionHost()
	marker := redRoot + "/run.active.json"
	host.Files.Contents[marker] = []byte(`{"schemaVersion":1,"pid":5}`)
	port := host.Port()
	redReadFile(&port, marker, 0, nil, errRed)
	if err := cleanupLocked(port, redRoot, redStart); !errors.Is(err, errRed) {
		t.Fatalf("active marker error = %v, want the injected failure", err)
	}
}

func TestCleanupReportsACompactionFailure(t *testing.T) {
	host := redCompactionHost()
	redPlace(host, redLooseRun, []byte("{"), redStart)
	if err := cleanupLocked(host.Port(), redRoot, redStart); err == nil {
		t.Fatal("cleanup continued past an uncompactable summary")
	}
}

func TestCleanupReportsASecondRootListingFailure(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	// The fourth listing of the root is the one cleanup makes after compaction.
	redListing(&port, redRoot, 4, func([]fs.DirEntry) ([]fs.DirEntry, error) { return nil, errRed })
	if err := cleanupLocked(port, redRoot, redStart); !errors.Is(err, errRed) {
		t.Fatalf("second listing error = %v, want the injected failure", err)
	}
}

func TestCleanupSkipsAnEntryThatVanishedAfterListing(t *testing.T) {
	host := redCompactionHost()
	gone := redRoot + "/gone.txt"
	redPlace(host, gone, []byte("x"), redStart)
	port := host.Port()
	redEntryInfo(&port, gone, func(fs.DirEntry) (fs.FileInfo, error) { return nil, fs.ErrNotExist })
	if err := cleanupLocked(port, redRoot, redStart); err != nil {
		t.Fatalf("vanished entry cleanup error = %v", err)
	}
}

func TestCleanupReportsAnEntryInfoFailure(t *testing.T) {
	host := redCompactionHost()
	notes := redRoot + "/notes.txt"
	redPlace(host, notes, []byte("x"), redStart)
	port := host.Port()
	redEntryInfo(&port, notes, func(fs.DirEntry) (fs.FileInfo, error) { return nil, errRed })
	if err := cleanupLocked(port, redRoot, redStart); !errors.Is(err, errRed) {
		t.Fatalf("entry info error = %v, want the injected failure", err)
	}
}

func TestCleanupReportsAFailedRemovalOfAnExpiredTemporary(t *testing.T) {
	host := redCompactionHost()
	temporary := redRoot + "/.summary-1.tmp"
	redPlace(host, temporary, []byte("x"), redStart.Add(-2*time.Hour))
	port := host.Port()
	port.Files.Remove = func(string) error { return errRed }
	if err := cleanupLocked(port, redRoot, redStart); !errors.Is(err, errRed) {
		t.Fatalf("temporary removal error = %v, want the injected failure", err)
	}
}

func TestCleanupReportsAFailedRemovalOfAnExpiredFile(t *testing.T) {
	host := redCompactionHost()
	expired := redRoot + "/old.txt"
	redPlace(host, expired, []byte("x"), redStart.Add(-(8 * 24 * time.Hour)))
	port := host.Port()
	port.Files.Remove = func(string) error { return errRed }
	if err := cleanupLocked(port, redRoot, redStart); !errors.Is(err, errRed) {
		t.Fatalf("expired removal error = %v, want the injected failure", err)
	}
}

func TestCleanupKeepsAPreservedFileOverTheInactiveCap(t *testing.T) {
	host := redCompactionHost()
	keep := redRoot + "/keep.txt"
	drop := redRoot + "/drop.txt"
	redPlace(host, keep, []byte("x"), redStart.Add(-2*time.Hour))
	redPlace(host, drop, []byte("x"), redStart.Add(-time.Hour))
	port := host.Port()
	redEntrySize(&port, keep, redInactiveCap)
	redEntrySize(&port, drop, redInactiveCap)
	if err := cleanupLocked(port, redRoot, redStart, keep); err != nil {
		t.Fatal(err)
	}
	if _, kept := host.Files.Contents[keep]; !kept {
		t.Fatal("the preserved file was removed over the cap")
	}
	if _, survived := host.Files.Contents[drop]; survived {
		t.Fatal("the unprotected file survived the cap")
	}
}

func TestCleanupReportsAFailedRemovalOverTheCap(t *testing.T) {
	host := redCompactionHost()
	drop := redRoot + "/drop.txt"
	redPlace(host, drop, []byte("x"), redStart.Add(-time.Hour))
	port := host.Port()
	redEntrySize(&port, drop, redInactiveCap*2)
	port.Files.Remove = func(string) error { return errRed }
	if err := cleanupLocked(port, redRoot, redStart); !errors.Is(err, errRed) {
		t.Fatalf("cap removal error = %v, want the injected failure", err)
	}
}

func TestCleanupRejectsAnEmptyRoot(t *testing.T) {
	if err := cleanup(redHost().Port(), "", redStart); err == nil {
		t.Fatal("an empty evidence root was cleaned")
	}
}

func TestCleanupReportsAFailedRootPermissionChange(t *testing.T) {
	port := redHost().Port()
	port.Files.Chmod = func(string, fs.FileMode) error { return errRed }
	if err := cleanup(port, redRoot, redStart); !errors.Is(err, errRed) {
		t.Fatalf("chmod error = %v, want the injected failure", err)
	}
}

func TestCleanupReportsAFailedRootCreation(t *testing.T) {
	port := redHost().Port()
	port.Files.MkdirAll = func(string, fs.FileMode) error { return errRed }
	if err := cleanup(port, redRoot, redStart); !errors.Is(err, errRed) {
		t.Fatalf("root creation error = %v, want the injected failure", err)
	}
}
