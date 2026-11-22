package evidence //nolint:testpackage // Host-seam tests drive the unexported display functions that take the system port.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/identity"
)

const (
	redDisplayDir = redRoot + "/run-display"
	redToken      = "0123456789abcdef0123456789abcdef"
	redOtherToken = "ffffffffffffffffffffffffffffffff"
	// redTempName is the temporary a write creates when the random source yields 1 through 8.
	redTempName = ".run-display-" + redToken + "-0102030405060708.tmp"
)

// redDisplayHost returns a host whose evidence root holds an empty run-display directory.
func redDisplayHost() *systemtest.Host {
	host := redHost()
	redDirs(host, redRoot, redDisplayDir)
	host.Random.Bytes = []byte{1, 2, 3, 4, 5, 6, 7, 8}

	return host
}

// redDisplayRecord is the annotation file a run publishes for its token.
func redDisplayRecord(t *testing.T, runID, label string) []byte {
	t.Helper()
	data, err := json.Marshal(displayRecord{SchemaVersion: 1, RunID: runID, Label: label})
	if err != nil {
		t.Fatal(err)
	}

	return data
}

// redOpenDisplay returns the run-display directory of the host, carrying the faults.
func redOpenDisplay(t *testing.T, host *systemtest.Host, faults redDirFaults) system.Directory {
	t.Helper()
	dir, err := openDisplayDirectory(host.Port(), redRoot, false)
	if err != nil {
		t.Fatal(err)
	}

	return redDirectory{Directory: dir, faults: faults}
}

// redDirInfo is the information the double reports for a directory.
func redDirInfo(t *testing.T, host *systemtest.Host) fs.FileInfo {
	t.Helper()
	info, err := host.Port().Files.Stat(redDisplayDir)
	if err != nil {
		t.Fatal(err)
	}

	return info
}

func TestOpenDisplayDirectoryReportsARootFailure(t *testing.T) {
	port := redDisplayHost().Port()
	port.Files.OpenDirectory = func(string) (system.Directory, error) { return nil, errRed }
	if _, err := openDisplayDirectory(port, redRoot, false); !errors.Is(err, errRed) {
		t.Fatalf("root open error = %v, want the injected failure", err)
	}
}

func TestOpenDisplayDirectoryReportsACreationFailure(t *testing.T) {
	host := redHost()
	redDirs(host, redRoot)
	port := host.Port()
	redWrapOpenDirectory(&port, redDirFaults{mkdir: errRed})
	if _, err := openDisplayDirectory(port, redRoot, true); !errors.Is(err, errRed) {
		t.Fatalf("creation error = %v, want the injected failure", err)
	}
}

func TestOpenDisplayDirectoryTolerantlyReusesAnExistingDirectory(t *testing.T) {
	host := redDisplayHost()
	dir, err := openDisplayDirectory(host.Port(), redRoot, true)
	if err != nil {
		t.Fatalf("existing directory refused: %v", err)
	}
	if err = dir.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenDisplayDirectoryReportsAFailedOpenOfTheDisplayDirectory(t *testing.T) {
	host := redDisplayHost()
	port := host.Port()
	redWrapOpenDirectory(&port, redDirFaults{only: "run-display", openDir: errRed})
	if _, err := openDisplayDirectory(port, redRoot, false); !errors.Is(err, errRed) {
		t.Fatalf("display open error = %v, want the injected failure", err)
	}
}

func TestOpenDisplayDirectoryReportsAFailedChmodOfANewDirectory(t *testing.T) {
	host := redDisplayHost()
	port := host.Port()
	redWrapOpenDirectory(&port, redDirFaults{chmod: errRed})
	if _, err := openDisplayDirectory(port, redRoot, true); !errors.Is(err, errRed) {
		t.Fatalf("chmod error = %v, want the injected failure", err)
	}
}

func TestReadDisplayTreatsAMissingFileAsMissing(t *testing.T) {
	host := redDisplayHost()
	dir := redOpenDisplay(t, host, redDirFaults{})
	if got := readDisplay(dir, redToken); got.Availability != application.DisplayMissing {
		t.Fatalf("availability = %v, want missing", got.Availability)
	}
}

func TestReadDisplayReportsAnOpenFailureAsUnreadable(t *testing.T) {
	host := redDisplayHost()
	dir := redOpenDisplay(t, host, redDirFaults{openFile: errRed})
	if got := readDisplay(dir, redToken); got.Availability != application.DisplayUnreadable {
		t.Fatalf("availability = %v, want unreadable", got.Availability)
	}
}

func TestReadDisplayRefusesAFileThatCannotBeStatted(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redToken+".json"] = redDisplayRecord(t, redToken, "build")
	dir := redOpenDisplay(t, host, redDirFaults{file: redFileFaults{stat: errRed}})
	if got := readDisplay(dir, redToken); got.Availability != application.DisplayUnreadable {
		t.Fatalf("availability = %v, want unreadable after a stat failure", got.Availability)
	}
}

func TestReadDisplayRefusesAFileThatIsNotRegular(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redToken+".json"] = redDisplayRecord(t, redToken, "build")
	dir := redOpenDisplay(t, host, redDirFaults{file: redFileFaults{info: redDirInfo(t, host)}})
	if got := readDisplay(dir, redToken); got.Availability != application.DisplayUnreadable {
		t.Fatalf("availability = %v, want unreadable for a directory", got.Availability)
	}
}

func TestReadDisplayReportsAReadFailureAsUnreadable(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redToken+".json"] = redDisplayRecord(t, redToken, "build")
	dir := redOpenDisplay(t, host, redDirFaults{file: redFileFaults{read: errRed}})
	if got := readDisplay(dir, redToken); got.Availability != application.DisplayUnreadable {
		t.Fatalf("availability = %v, want unreadable after a read failure", got.Availability)
	}
}

func TestReadDisplayReportsACloseFailureAsUnreadable(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redToken+".json"] = redDisplayRecord(t, redToken, "build")
	dir := redOpenDisplay(t, host, redDirFaults{file: redFileFaults{closeErr: errRed}})
	if got := readDisplay(dir, redToken); got.Availability != application.DisplayUnreadable {
		t.Fatalf("availability = %v, want unreadable after a close failure", got.Availability)
	}
}

func TestReadDisplayRefusesAnOversizeAnnotation(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redToken+".json"] = bytes.Repeat([]byte(" "), maximumDisplayBytes+1)
	dir := redOpenDisplay(t, host, redDirFaults{})
	if got := readDisplay(dir, redToken); got.Availability != application.DisplayInvalid {
		t.Fatalf("availability = %v, want invalid for an oversize annotation", got.Availability)
	}
}

func TestReadDisplayReadsAValidAnnotation(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redToken+".json"] = redDisplayRecord(t, redToken, "build")
	dir := redOpenDisplay(t, host, redDirFaults{})
	got := readDisplay(dir, redToken)
	if got.Availability != application.DisplayAvailable || got.Label.String() != "build" {
		t.Fatalf("annotation = %+v, want the available build label", got)
	}
}

func TestDecodeDisplayClassifiesEveryRecordShape(t *testing.T) {
	tests := []struct {
		name string
		data string
		want application.DisplayAvailability
	}{
		{name: "valid", data: `{"schemaVersion":1,"runId":"TOKEN","label":"build"}`, want: application.DisplayAvailable},
		{name: "not an object", data: `[]`, want: application.DisplayInvalid},
		{name: "duplicated field", data: `{"schemaVersion":1,"schemaVersion":1,"runId":"TOKEN","label":"build"}`, want: application.DisplayInvalid},
		{name: "unknown field", data: `{"schemaVersion":1,"runId":"TOKEN","label":"build","extra":true}`, want: application.DisplayInvalid},
		{name: "wrong value type", data: `{"schemaVersion":"one","runId":"TOKEN","label":"build"}`, want: application.DisplayInvalid},
		{name: "truncated", data: `{"schemaVersion":1,"runId":"TOKEN","label":"build"`, want: application.DisplayInvalid},
		{name: "trailing value", data: `{"schemaVersion":1,"runId":"TOKEN","label":"build"} {}`, want: application.DisplayInvalid},
		{name: "missing field", data: `{"schemaVersion":1,"runId":"TOKEN"}`, want: application.DisplayInvalid},
		{name: "zero schema", data: `{"schemaVersion":0,"runId":"TOKEN","label":"build"}`, want: application.DisplayInvalid},
		{name: "other run", data: `{"schemaVersion":1,"runId":"other","label":"build"}`, want: application.DisplayInvalid},
		{name: "unsupported schema", data: `{"schemaVersion":2,"runId":"TOKEN","label":"build"}`, want: application.DisplayUnsupported},
		{name: "path-like label", data: `{"schemaVersion":1,"runId":"TOKEN","label":"../etc"}`, want: application.DisplayInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := strings.ReplaceAll(test.data, "TOKEN", redToken)
			if got := decodeDisplay([]byte(data), redToken); got.Availability != test.want {
				t.Fatalf("availability = %v, want %v", got.Availability, test.want)
			}
		})
	}
}

func TestReadDisplaysStopsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readDisplays(redDisplayHost().Port(), ctx, redRoot, []string{redToken})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read error = %v, want context.Canceled", err)
	}
}

func TestReadDisplaysRefusesAMalformedRunIdentifier(t *testing.T) {
	result, err := readDisplays(redDisplayHost().Port(), context.Background(), redRoot, []string{"../escape"})
	if err != nil || result["../escape"].Availability != application.DisplayInvalid {
		t.Fatalf("malformed identifier result = %+v with %v, want invalid", result, err)
	}
}

func TestReadDisplaysReportsAnAbsentDirectoryAsMissing(t *testing.T) {
	result, err := readDisplays(redHost().Port(), context.Background(), redRoot, []string{redToken})
	if err != nil || result[redToken].Availability != application.DisplayMissing {
		t.Fatalf("absent directory result = %+v with %v, want missing", result, err)
	}
}

func TestReadDisplaysReportsAnUnopenableDirectoryAsUnreadable(t *testing.T) {
	port := redDisplayHost().Port()
	port.Files.OpenDirectory = func(string) (system.Directory, error) { return nil, errRed }
	result, err := readDisplays(port, context.Background(), redRoot, []string{redToken})
	if !errors.Is(err, errRed) || result[redToken].Availability != application.DisplayUnreadable {
		t.Fatalf("unopenable directory result = %+v with %v, want unreadable and the failure", result, err)
	}
}

func TestReadDisplaysReadsEachPublishedAnnotation(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redToken+".json"] = redDisplayRecord(t, redToken, "build")
	result, err := readDisplays(host.Port(), context.Background(), redRoot, []string{redToken})
	if err != nil || result[redToken].Availability != application.DisplayAvailable || result[redToken].Label.String() != "build" {
		t.Fatalf("display result = %+v with %v, want the published label", result, err)
	}
}

func TestWriteDisplayRefusesAMalformedRunIdentifier(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	if err := writeDisplay(redDisplayHost().Port(), redRoot, "../escape", label); err == nil {
		t.Fatal("a malformed run identifier was written")
	}
}

func TestWriteDisplayRefusesAnEmptyLabel(t *testing.T) {
	if err := writeDisplay(redDisplayHost().Port(), redRoot, redToken, identity.DisplayLabel{}); err == nil {
		t.Fatal("an empty label was written")
	}
}

func TestWriteDisplayReportsADirectoryFailure(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	port := redDisplayHost().Port()
	port.Files.OpenDirectory = func(string) (system.Directory, error) { return nil, errRed }
	if err := writeDisplay(port, redRoot, redToken, label); !errors.Is(err, errRed) {
		t.Fatalf("directory error = %v, want the injected failure", err)
	}
}

func TestWriteDisplayReportsAFailedRandomSource(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	port := redDisplayHost().Port()
	port.Random.Read = func([]byte) (int, error) { return 0, errRed }
	if err := writeDisplay(port, redRoot, redToken, label); !errors.Is(err, errRed) {
		t.Fatalf("random error = %v, want the injected failure", err)
	}
}

func TestWriteDisplayReportsATemporaryCreationFailure(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	port := redDisplayHost().Port()
	redWrapOpenDirectory(&port, redDirFaults{openFile: errRed})
	if err := writeDisplay(port, redRoot, redToken, label); !errors.Is(err, errRed) {
		t.Fatalf("temporary error = %v, want the injected failure", err)
	}
}

func TestWriteDisplayReportsAFailureAndRemovesItsTemporary(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	host := redDisplayHost()
	port := host.Port()
	redWrapOpenDirectory(&port, redDirFaults{file: redFileFaults{chmod: errRed}})
	if err := writeDisplay(port, redRoot, redToken, label); !errors.Is(err, errRed) {
		t.Fatalf("chmod error = %v, want the injected failure", err)
	}
	if _, survived := host.Files.Contents[redDisplayDir+"/"+redTempName]; survived {
		t.Fatal("the temporary annotation survived the failed write")
	}
}

func TestWriteDisplayReportsAWriteFailure(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	port := redDisplayHost().Port()
	redWrapOpenDirectory(&port, redDirFaults{file: redFileFaults{writeFrom: 1}})
	if err := writeDisplay(port, redRoot, redToken, label); !errors.Is(err, errRed) {
		t.Fatalf("encode error = %v, want the injected failure", err)
	}
}

func TestWriteDisplayReportsASyncFailure(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	port := redDisplayHost().Port()
	redWrapOpenDirectory(&port, redDirFaults{file: redFileFaults{sync: errRed}})
	if err := writeDisplay(port, redRoot, redToken, label); !errors.Is(err, errRed) {
		t.Fatalf("sync error = %v, want the injected failure", err)
	}
}

func TestWriteDisplayReportsACloseFailure(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	port := redDisplayHost().Port()
	redWrapOpenDirectory(&port, redDirFaults{file: redFileFaults{closeErr: errRed}})
	if err := writeDisplay(port, redRoot, redToken, label); !errors.Is(err, errRed) {
		t.Fatalf("close error = %v, want the injected failure", err)
	}
}

func TestWriteDisplayJoinsATemporaryRemovalFailure(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	port := redDisplayHost().Port()
	removeErr := errors.New("temporary removal refused")
	redWrapOpenDirectory(&port, redDirFaults{file: redFileFaults{chmod: errRed}, remove: removeErr})
	err := writeDisplay(port, redRoot, redToken, label)
	if !errors.Is(err, errRed) || !errors.Is(err, removeErr) {
		t.Fatalf("write error = %v, want both the chmod and the removal failures", err)
	}
}

func TestWriteDisplayReportsARenameFailure(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	port := redDisplayHost().Port()
	redWrapOpenDirectory(&port, redDirFaults{rename: errRed})
	if err := writeDisplay(port, redRoot, redToken, label); !errors.Is(err, errRed) {
		t.Fatalf("rename error = %v, want the injected failure", err)
	}
}

func TestWriteDisplayPublishesTheAnnotation(t *testing.T) {
	label, _ := identity.ParseDisplayLabel("build")
	host := redDisplayHost()
	if err := writeDisplay(host.Port(), redRoot, redToken, label); err != nil {
		t.Fatal(err)
	}
	published, exists := host.Files.Contents[redDisplayDir+"/"+redToken+".json"]
	if !exists || !bytes.Contains(published, []byte(`"label":"build"`)) {
		t.Fatalf("published annotation = %q, want the build label", published)
	}
}

func TestRemoveDisplayRefusesAMalformedRunIdentifier(t *testing.T) {
	if err := removeDisplay(redDisplayHost().Port(), redRoot, "../escape"); err == nil {
		t.Fatal("a malformed run identifier was removed")
	}
}

func TestRemoveDisplayIgnoresAnAbsentRoot(t *testing.T) {
	if err := removeDisplay(redHost().Port(), redRoot, redToken); err != nil {
		t.Fatalf("absent root removal error = %v", err)
	}
}

func TestRemoveDisplayReportsAnOpenFailure(t *testing.T) {
	port := redDisplayHost().Port()
	port.Files.OpenDirectory = func(string) (system.Directory, error) { return nil, errRed }
	if err := removeDisplay(port, redRoot, redToken); !errors.Is(err, errRed) {
		t.Fatalf("open error = %v, want the injected failure", err)
	}
}

func TestRemoveDisplayRemovesOnlyTheOwnedAnnotation(t *testing.T) {
	host := redDisplayHost()
	owned := redDisplayDir + "/" + redToken + ".json"
	other := redDisplayDir + "/" + redOtherToken + ".json"
	host.Files.Contents[owned] = redDisplayRecord(t, redToken, "build")
	host.Files.Contents[other] = redDisplayRecord(t, redOtherToken, "test")
	if err := removeDisplay(host.Port(), redRoot, redToken); err != nil {
		t.Fatal(err)
	}
	if _, survived := host.Files.Contents[owned]; survived {
		t.Fatal("the owned annotation survived removal")
	}
	if _, kept := host.Files.Contents[other]; !kept {
		t.Fatal("another run's annotation was removed")
	}
}

func TestRemoveDisplayAtIgnoresAnAbsentEntry(t *testing.T) {
	host := redDisplayHost()
	if err := removeDisplayAt(redOpenDisplay(t, host, redDirFaults{}), redToken+".json"); err != nil {
		t.Fatalf("absent entry removal error = %v", err)
	}
}

func TestRemoveDisplayAtReportsAStatFailure(t *testing.T) {
	host := redDisplayHost()
	dir := redOpenDisplay(t, host, redDirFaults{stat: func(string, redStatFunc) (fs.FileInfo, error) { return nil, errRed }})
	if err := removeDisplayAt(dir, redToken+".json"); !errors.Is(err, errRed) {
		t.Fatalf("stat error = %v, want the injected failure", err)
	}
}

func TestRemoveDisplayAtRefusesANonRegularEntry(t *testing.T) {
	host := redDisplayHost()
	dir := redOpenDisplay(t, host, redDirFaults{stat: func(string, redStatFunc) (fs.FileInfo, error) {
		return redDirInfo(t, host), nil
	}})
	if err := removeDisplayAt(dir, redToken+".json"); err == nil {
		t.Fatal("a directory was removed as an annotation")
	}
}

func TestRemoveDisplayAtIgnoresAnEntryThatVanishedBeforeRemoval(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redToken+".json"] = redDisplayRecord(t, redToken, "build")
	dir := redOpenDisplay(t, host, redDirFaults{remove: fs.ErrNotExist})
	if err := removeDisplayAt(dir, redToken+".json"); err != nil {
		t.Fatalf("vanished entry removal error = %v", err)
	}
}

func TestRemoveDisplayAtReportsARemovalFailure(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redToken+".json"] = redDisplayRecord(t, redToken, "build")
	dir := redOpenDisplay(t, host, redDirFaults{remove: errRed})
	if err := removeDisplayAt(dir, redToken+".json"); !errors.Is(err, errRed) {
		t.Fatalf("removal error = %v, want the injected failure", err)
	}
}

func TestDisplayTokensReturnsNoTokensWhenTheRootIsMissing(t *testing.T) {
	tokens, err := displayTokens(redHost().Port(), redRoot)
	if err != nil || tokens == nil || len(tokens) != 0 {
		t.Fatalf("missing root tokens = %#v with %v, want an empty list", tokens, err)
	}
}

func TestDisplayTokensReportsAnOpenFailure(t *testing.T) {
	port := redDisplayHost().Port()
	port.Files.OpenDirectory = func(string) (system.Directory, error) { return nil, errRed }
	if _, err := displayTokens(port, redRoot); !errors.Is(err, errRed) {
		t.Fatalf("open error = %v, want the injected failure", err)
	}
}

func TestDisplayTokensReportsAListingFailure(t *testing.T) {
	port := redDisplayHost().Port()
	redWrapOpenDirectory(&port, redDirFaults{readDir: errRed})
	if _, err := displayTokens(port, redRoot); !errors.Is(err, errRed) {
		t.Fatalf("listing error = %v, want the injected failure", err)
	}
}

func TestDisplayTokensListsOnlyValidRegularAnnotations(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redToken+".json"] = redDisplayRecord(t, redToken, "build")
	host.Files.Contents[redDisplayDir+"/"+redOtherToken+".json"] = []byte("{")
	host.Files.Contents[redDisplayDir+"/bad-name.json"] = redDisplayRecord(t, redToken, "build")
	host.Files.Contents[redDisplayDir+"/notes.txt"] = []byte("not an annotation")
	host.Files.Dirs[redDisplayDir+"/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee.json"] = true
	tokens, err := displayTokens(host.Port(), redRoot)
	if err != nil || len(tokens) != 1 || tokens[0] != redToken {
		t.Fatalf("tokens = %v with %v, want only the valid published token", tokens, err)
	}
}

func TestPruneDisplayIgnoresAnAbsentRoot(t *testing.T) {
	if err := pruneDisplay(redHost().Port(), redRoot, nil, redStart); err != nil {
		t.Fatalf("absent root prune error = %v", err)
	}
}

func TestPruneDisplayReportsAnOpenFailure(t *testing.T) {
	port := redDisplayHost().Port()
	port.Files.OpenDirectory = func(string) (system.Directory, error) { return nil, errRed }
	if err := pruneDisplay(port, redRoot, nil, redStart); !errors.Is(err, errRed) {
		t.Fatalf("open error = %v, want the injected failure", err)
	}
}

func TestPruneDisplayRefusesAMalformedStaleIdentifier(t *testing.T) {
	if err := pruneDisplay(redDisplayHost().Port(), redRoot, []string{"../escape"}, redStart); err == nil {
		t.Fatal("a malformed stale identifier was pruned")
	}
}

func TestPruneDisplayRemovesProvenStaleAnnotations(t *testing.T) {
	host := redDisplayHost()
	annotation := redDisplayDir + "/" + redToken + ".json"
	host.Files.Contents[annotation] = redDisplayRecord(t, redToken, "build")
	if err := pruneDisplay(host.Port(), redRoot, []string{redToken}, redStart); err != nil {
		t.Fatal(err)
	}
	if _, survived := host.Files.Contents[annotation]; survived {
		t.Fatal("a proven stale annotation survived pruning")
	}
}

func TestPruneDisplayReportsAListingFailure(t *testing.T) {
	port := redDisplayHost().Port()
	redWrapOpenDirectory(&port, redDirFaults{readDir: errRed})
	if err := pruneDisplay(port, redRoot, nil, redStart); !errors.Is(err, errRed) {
		t.Fatalf("listing error = %v, want the injected failure", err)
	}
}

func TestPruneDisplaySkipsATemporaryThatVanishedAfterListing(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redTempName] = []byte("x")
	port := host.Port()
	redWrapOpenDirectory(&port, redDirFaults{stat: func(name string, real redStatFunc) (fs.FileInfo, error) {
		if name == redTempName {
			return nil, fs.ErrNotExist
		}

		return real(name)
	}})
	if err := pruneDisplay(port, redRoot, nil, redStart); err != nil {
		t.Fatalf("vanished temporary prune error = %v", err)
	}
}

func TestPruneDisplayReportsAFailedTemporaryStat(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redTempName] = []byte("x")
	port := host.Port()
	redWrapOpenDirectory(&port, redDirFaults{stat: func(name string, real redStatFunc) (fs.FileInfo, error) {
		if name == redTempName {
			return nil, errRed
		}

		return real(name)
	}})
	if err := pruneDisplay(port, redRoot, nil, redStart); !errors.Is(err, errRed) {
		t.Fatalf("temporary stat error = %v, want the injected failure", err)
	}
}

func TestPruneDisplaySkipsATemporaryThatIsNoLongerRegular(t *testing.T) {
	host := redDisplayHost()
	host.Files.Contents[redDisplayDir+"/"+redTempName] = []byte("x")
	port := host.Port()
	redWrapOpenDirectory(&port, redDirFaults{stat: func(name string, real redStatFunc) (fs.FileInfo, error) {
		if name == redTempName {
			return redDirInfo(t, host), nil
		}

		return real(name)
	}})
	if err := pruneDisplay(port, redRoot, nil, redStart); err != nil {
		t.Fatalf("non-regular temporary prune error = %v", err)
	}
	if _, kept := host.Files.Contents[redDisplayDir+"/"+redTempName]; !kept {
		t.Fatal("a temporary that is no longer regular was removed")
	}
}

func TestPruneDisplayRemovesOnlyExpiredTemporaries(t *testing.T) {
	host := redDisplayHost()
	fresh := redDisplayDir + "/.run-display-" + redToken + "-fedcba9876543210.tmp"
	redPlace(host, redDisplayDir+"/"+redTempName, []byte("x"), redStart.Add(-2*time.Hour))
	redPlace(host, fresh, []byte("x"), redStart.Add(-time.Minute))
	if err := pruneDisplay(host.Port(), redRoot, nil, redStart); err != nil {
		t.Fatal(err)
	}
	if _, stale := host.Files.Contents[redDisplayDir+"/"+redTempName]; stale {
		t.Fatal("an expired temporary survived pruning")
	}
	if _, kept := host.Files.Contents[fresh]; !kept {
		t.Fatal("a fresh temporary was pruned")
	}
}

func TestPruneDisplayReportsAFailedTemporaryRemoval(t *testing.T) {
	host := redDisplayHost()
	redPlace(host, redDisplayDir+"/"+redTempName, []byte("x"), redStart.Add(-2*time.Hour))
	port := host.Port()
	redWrapOpenDirectory(&port, redDirFaults{remove: errRed})
	if err := pruneDisplay(port, redRoot, nil, redStart); !errors.Is(err, errRed) {
		t.Fatalf("temporary removal error = %v, want the injected failure", err)
	}
}

func TestDisplayTemporaryRecognizesOnlyWellFormedNames(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: redTempName, want: true},
		{name: ".run-display-" + redToken + "-zzzzzzzzzzzzzzzz.tmp", want: false},
		{name: ".run-display-short.tmp", want: false},
		{name: "run-display-" + redToken + "-0123456789abcdef.tmp", want: false},
		{name: ".run-display-" + redToken + "-0123456789abcdef.txt", want: false},
	}
	for _, test := range tests {
		if got := displayTemporary(test.name); got != test.want {
			t.Fatalf("displayTemporary(%q) = %t, want %t", test.name, got, test.want)
		}
	}
}
