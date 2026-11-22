package conformance //nolint:testpackage // Host-seam tests drive the unexported lifecycle functions that take the system port.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/application"
)

const (
	redManifestPath = "/hippo/manifest.json"
	redBinaryPath   = "/hippo/bin/hippo"
	redSharedRoot   = "/hippo/shared"
	redReceipts     = redSharedRoot + "/receipts"
	// The double numbers every MkdirTemp call across patterns, so a root's name depends on the calls before it.
	redBinaryRoot   = "hippo-conformance-bin.1"
	redCommandRoot  = "hippo-conformance-command.1"
	redDeferralRoot = "hippo-deferral-probe-2"
)

var redStart = time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)

// replacedDirectory reports the directory's name and size, but an identity that differs from the captured one.
type replacedDirectory struct{ fs.FileInfo }

func (replacedDirectory) Sys() any { return "replacement" }

func redHost() *systemtest.Host { return systemtest.NewHost(redStart) }

func redSum(content []byte) string {
	digest := sha256.Sum256(content)

	return hex.EncodeToString(digest[:])
}

func redConsumers(count int) []Consumer {
	consumers := make([]Consumer, 0, count)
	for index := range count {
		consumers = append(consumers, Consumer{
			Name:  fmt.Sprintf("consumer-%d", index+1),
			Path:  fmt.Sprintf("/hippo/consumer-%d", index+1),
			Gates: []Command{{Arguments: []string{"true"}}},
		})
	}

	return consumers
}

// redFixture registers a valid binary and four checkouts in the host, and returns the manifest that names them.
func redFixture(host *systemtest.Host) Manifest {
	content := []byte("hippo-binary")
	host.Files.Dirs["/hippo"] = true
	host.Files.Dirs["/hippo/bin"] = true
	host.Files.Contents[redBinaryPath] = content
	host.Files.Modes[redBinaryPath] = 0o700
	manifest := Manifest{SchemaVersion: 1, HIPPOBinary: redBinaryPath, HIPPOSHA256: redSum(content), SharedRoot: redSharedRoot}
	for _, consumer := range redConsumers(4) {
		host.Files.Dirs[consumer.Path] = true
		manifest.Consumers = append(manifest.Consumers, consumer)
	}

	return manifest
}

func redWriteManifest(t *testing.T, host *systemtest.Host, manifest Manifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	host.Files.Contents[redManifestPath] = data
}

// redScriptedAfter makes successive timers fire or stay silent in the order given. Calls past the script stay silent.
func redScriptedAfter(fires ...bool) func(time.Duration) <-chan time.Time {
	calls := 0

	return func(time.Duration) <-chan time.Time {
		channel := make(chan time.Time, 1)
		if calls < len(fires) && fires[calls] {
			channel <- redStart
		}
		calls++

		return channel
	}
}

func TestConsumeManifestJSONRejectsTruncatedInput(t *testing.T) {
	for _, input := range []string{`{"a":`, `{"a":1,`, `{"a":[`, `[{`} {
		if err := consumeManifestJSON(json.NewDecoder(strings.NewReader(input))); err == nil {
			t.Fatalf("truncated manifest %q was accepted", input)
		}
	}
}

func TestDecodeManifestRejectsTrailingValue(t *testing.T) {
	if _, err := decodeManifest([]byte(`{} {}`)); err == nil {
		t.Fatal("a second JSON value after the manifest was accepted")
	}
}

func TestValidateRawManifestRequiresBinaryChecksumAndSharedRoot(t *testing.T) {
	if err := validateRawManifest(Manifest{SchemaVersion: 1, SharedRoot: redSharedRoot}); err == nil {
		t.Fatal("a manifest without a binary and checksum was accepted")
	}
}

func TestCanonicalPathJoinsRelativeValueToBase(t *testing.T) {
	host := redHost()
	host.Files.Dirs["/hippo/base/rel"] = true
	got, err := canonicalPath(host.Port(), "/hippo/base", "rel", true)
	if err != nil || got != "/hippo/base/rel" {
		t.Fatalf("relative path resolved to %q with %v, want /hippo/base/rel", got, err)
	}
}

func TestCanonicalPathReportsWorkingDirectoryFailure(t *testing.T) {
	port := redHost().Port()
	port.Environment.Getwd = func() (string, error) { return "", errors.New("working directory removed") }
	if _, err := canonicalPath(port, "", "relative", true); err == nil {
		t.Fatal("a relative path resolved without a working directory")
	}
}

func TestCanonicalPathReportsStatFailureOnAncestor(t *testing.T) {
	port := redHost().Port()
	port.Files.Lstat = func(name string) (fs.FileInfo, error) {
		if name == "/hippo/missing" {
			return nil, fs.ErrNotExist
		}

		return nil, errors.New("permission denied")
	}
	if _, err := canonicalPath(port, "/hippo", "missing", false); err == nil {
		t.Fatal("an unreadable ancestor was treated as absent")
	}
}

func TestCanonicalPathReportsMissingChainToRoot(t *testing.T) {
	_, err := canonicalPath(redHost().Port(), "/", "/missing/leaf", false)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a path with no existing ancestor returned %v, want not-exist", err)
	}
}

func TestCanonicalPathReportsUnreadableSymlinkAncestor(t *testing.T) {
	host := redHost()
	host.Files.Contents["/hippo/link"] = []byte("/hippo/target")
	host.Files.Modes["/hippo/link"] = fs.ModeSymlink | 0o777
	if _, err := canonicalPath(host.Port(), "/hippo/link", "leaf", false); err == nil {
		t.Fatal("a symlinked ancestor that cannot be read was resolved")
	}
}

func TestCanonicalizeManifestReportsMissingBinary(t *testing.T) {
	manifest := Manifest{HIPPOBinary: "/hippo/bin/missing", SharedRoot: redSharedRoot}
	_, err := canonicalizeManifest(redHost().Port(), manifest, "/hippo")
	if err == nil || !strings.Contains(err.Error(), "HIPPO binary identity is unavailable") {
		t.Fatalf("a missing binary returned %v", err)
	}
}

func TestCanonicalizeManifestReportsUnavailableSharedRoot(t *testing.T) {
	host := redHost()
	host.Files.Contents[redBinaryPath] = []byte("hippo-binary")
	port := host.Port()
	lstat := port.Files.Lstat
	port.Files.Lstat = func(name string) (fs.FileInfo, error) {
		if name == redSharedRoot {
			return nil, errors.New("permission denied")
		}

		return lstat(name)
	}
	_, err := canonicalizeManifest(port, Manifest{HIPPOBinary: redBinaryPath, SharedRoot: redSharedRoot}, "/hippo")
	if err == nil || !strings.Contains(err.Error(), "shared root identity is unavailable") {
		t.Fatalf("an unreadable shared root returned %v", err)
	}
}

func TestValidateManifestRejectsUnsupportedSchema(t *testing.T) {
	if err := validateManifest(redHost().Port(), Manifest{SchemaVersion: 2}); err == nil {
		t.Fatal("an unsupported manifest schema was accepted")
	}
}

func TestValidateManifestRequiresBinaryFields(t *testing.T) {
	manifest := Manifest{SchemaVersion: 1, SharedRoot: redSharedRoot, Consumers: redConsumers(4)}
	if err := validateManifest(redHost().Port(), manifest); err == nil {
		t.Fatal("a manifest without a binary and checksum was accepted")
	}
}

func TestValidateManifestRejectsUnavailableCheckouts(t *testing.T) {
	missing := redHost()
	missingManifest := redFixture(missing)
	delete(missing.Files.Dirs, "/hippo/consumer-1")
	if err := validateManifest(missing.Port(), missingManifest); err == nil {
		t.Fatal("a missing checkout was accepted")
	}

	regular := redHost()
	regularManifest := redFixture(regular)
	delete(regular.Files.Dirs, "/hippo/consumer-1")
	regular.Files.Contents["/hippo/consumer-1"] = []byte("not a directory")
	if err := validateManifest(regular.Port(), regularManifest); err == nil {
		t.Fatal("a checkout path that is a file was accepted")
	}
}

func TestValidateManifestRequiresGatePerConsumer(t *testing.T) {
	host := redHost()
	manifest := redFixture(host)
	manifest.Consumers[0].Gates = nil
	if err := validateManifest(host.Port(), manifest); err == nil {
		t.Fatal("a consumer without a gate was accepted")
	}
}

func TestValidateManifestRejectsInvalidCoordinationCheck(t *testing.T) {
	host := redHost()
	manifest := redFixture(host)
	manifest.CoordinationChecks = []Check{{Consumer: "consumer-1"}}
	if err := validateManifest(host.Port(), manifest); err == nil {
		t.Fatal("a coordination check without an executable was accepted")
	}
}

func TestCaptureCheckoutIdentitiesRejectsMissingCheckout(t *testing.T) {
	if _, err := captureCheckoutIdentities(redHost().Port(), redConsumers(1)); err == nil {
		t.Fatal("a missing checkout was given an identity")
	}
}

func TestCheckoutIdentityTracksTheCapturedDirectory(t *testing.T) {
	host := redHost()
	consumer := redConsumers(1)[0]
	host.Files.Dirs[consumer.Path] = true
	port := host.Port()
	identities, err := captureCheckoutIdentities(port, []Consumer{consumer})
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyCheckoutIdentity(port, consumer, identities[consumer.Name]); err != nil {
		t.Fatalf("an unchanged checkout was rejected: %v", err)
	}
	stat := port.Files.Stat
	port.Files.Stat = func(name string) (fs.FileInfo, error) {
		info, statError := stat(name)
		if statError != nil {
			return nil, statError
		}

		return replacedDirectory{FileInfo: info}, nil
	}
	if err = verifyCheckoutIdentity(port, consumer, identities[consumer.Name]); err == nil {
		t.Fatal("a replaced checkout was accepted")
	}
}

func TestVerifyCheckoutIdentityRejectsMissingCheckout(t *testing.T) {
	if err := verifyCheckoutIdentity(redHost().Port(), redConsumers(1)[0], checkoutIdentity{}); err == nil {
		t.Fatal("a missing checkout verified against an identity")
	}
}

func TestSharedRootIdentityTracksTheCapturedRoot(t *testing.T) {
	host := redHost()
	if _, err := captureSharedRootIdentity(host.Port(), redSharedRoot); err == nil {
		t.Fatal("a missing shared root was given an identity")
	}
	host.Files.Dirs[redSharedRoot] = true
	port := host.Port()
	identity, err := captureSharedRootIdentity(port, redSharedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = verifySharedRootIdentity(port, redSharedRoot, identity); err != nil {
		t.Fatalf("an unchanged shared root was rejected: %v", err)
	}
	stat := port.Files.Stat
	port.Files.Stat = func(name string) (fs.FileInfo, error) {
		info, statError := stat(name)
		if statError != nil {
			return nil, statError
		}

		return replacedDirectory{FileInfo: info}, nil
	}
	if err = verifySharedRootIdentity(port, redSharedRoot, identity); err == nil {
		t.Fatal("a replaced shared root was accepted")
	}
	delete(host.Files.Dirs, redSharedRoot)
	if err = verifySharedRootIdentity(host.Port(), redSharedRoot, identity); err == nil {
		t.Fatal("a removed shared root was accepted")
	}
}

func TestOpenBinaryObjectRejectsObjectsThatAreNotExecutableFiles(t *testing.T) {
	host := redHost()
	host.Files.Dirs["/hippo/bin"] = true
	host.Files.Contents["/hippo/plain"] = []byte("data")
	host.Files.Modes["/hippo/plain"] = 0o600
	host.Files.Contents[redBinaryPath] = []byte("bin")
	host.Files.Modes[redBinaryPath] = 0o700
	port := host.Port()
	for _, path := range []string{"/hippo/bin", "/hippo/plain", "/hippo/missing"} {
		if _, _, err := openBinaryObject(port, path); err == nil {
			t.Fatalf("%s was opened as a verified binary", path)
		}
	}
	failing := host.Port()
	failing.Files.ReadFile = func(string) ([]byte, error) { return nil, errors.New("permission denied") }
	if _, _, err := openBinaryObject(failing, redBinaryPath); err == nil {
		t.Fatal("an unreadable binary was opened")
	}
	data, info, err := openBinaryObject(port, redBinaryPath)
	if err != nil || string(data) != "bin" || info == nil {
		t.Fatalf("an executable regular file returned %q, %v, %v", data, info, err)
	}
}

func TestCaptureBinaryIdentityRejectsMalformedChecksum(t *testing.T) {
	host := redHost()
	manifest := redFixture(host)
	port := host.Port()
	for _, sum := range []string{"not-hex", strings.ToUpper(manifest.HIPPOSHA256)} {
		manifest.HIPPOSHA256 = sum
		if _, err := captureBinaryIdentity(port, manifest); err == nil {
			t.Fatalf("checksum %q was accepted", sum)
		}
	}
}

// redExpectBinaryCaptureFailure captures the verified binary on a failing host. The failure must be reported, and
// the temporary root must not survive it.
func redExpectBinaryCaptureFailure(t *testing.T, corrupt func(*Manifest), mutate func(*system.System)) {
	t.Helper()
	host := redHost()
	manifest := redFixture(host)
	if corrupt != nil {
		corrupt(&manifest)
	}
	port := host.Port()
	if mutate != nil {
		mutate(&port)
	}
	if _, err := captureBinaryIdentity(port, manifest); err == nil {
		t.Fatal("a verified binary was captured despite the failure")
	}
	if host.Files.Dirs[redBinaryRoot] {
		t.Fatalf("temporary root %s survived the failed capture", redBinaryRoot)
	}
}

func TestCaptureBinaryIdentityReportsEachFailureAndCleansUp(t *testing.T) {
	cases := []struct {
		name    string
		corrupt func(*Manifest)
		mutate  func(*system.System)
	}{
		{name: "binary unreadable", mutate: func(port *system.System) {
			port.Files.ReadFile = func(string) ([]byte, error) { return nil, errors.New("permission denied") }
		}},
		{name: "storage unavailable", mutate: func(port *system.System) {
			port.Files.MkdirTemp = func(string, string) (string, error) { return "", errors.New("no space") }
		}},
		{name: "copy cannot be written", mutate: func(port *system.System) {
			port.Files.WriteFile = func(string, []byte, fs.FileMode) error { return errors.New("disk full") }
		}},
		{name: "checksum mismatch", corrupt: func(manifest *Manifest) {
			manifest.HIPPOSHA256 = redSum([]byte("other binary"))
		}},
		{name: "copy cannot be statted", mutate: func(port *system.System) {
			stat := port.Files.Stat
			port.Files.Stat = func(name string) (fs.FileInfo, error) {
				if name == redBinaryRoot+"/hippo" {
					return nil, errors.New("gone")
				}

				return stat(name)
			}
		}},
		{name: "copy cannot be made executable", mutate: func(port *system.System) {
			port.Files.Chmod = func(string, fs.FileMode) error { return errors.New("read-only") }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			redExpectBinaryCaptureFailure(t, tc.corrupt, tc.mutate)
		})
	}
}

func TestCaptureBinaryIdentityVerifiesAndCleansUpCopy(t *testing.T) {
	host := redHost()
	manifest := redFixture(host)
	port := host.Port()
	identity, err := captureBinaryIdentity(port, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.verify(port); err != nil {
		t.Fatalf("a fresh verified binary was rejected: %v", err)
	}
	if err = identity.cleanup(port); err != nil {
		t.Fatalf("cleaning the verified copy failed: %v", err)
	}
	if host.Files.Dirs[redBinaryRoot] {
		t.Fatal("cleanup left the verified copy behind")
	}
	if err = (binaryIdentity{}).cleanup(port); err != nil {
		t.Fatalf("cleaning an empty identity reported %v", err)
	}
}

func TestVerifyBinaryObjectRejectsReadFailureAndTampering(t *testing.T) {
	host := redHost()
	manifest := redFixture(host)
	port := host.Port()
	info, err := port.Files.Stat(redBinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	failing := host.Port()
	failing.Files.ReadFile = func(string) ([]byte, error) { return nil, errors.New("permission denied") }
	if err = verifyBinaryObject(failing, redBinaryPath, info, manifest.HIPPOSHA256); err == nil {
		t.Fatal("an unreadable binary verified")
	}
	host.Files.Contents[redBinaryPath] = []byte("tampered")
	if err = verifyBinaryObject(host.Port(), redBinaryPath, info, manifest.HIPPOSHA256); err == nil {
		t.Fatal("a tampered binary verified against its checksum")
	}
}

func TestMaterializeCommandBinaryReportsEachFailureAndCleansUp(t *testing.T) {
	identity := binaryIdentity{checksum: redSum([]byte("hippo")), content: []byte("hippo")}
	cases := []struct {
		name   string
		mutate func(*system.System)
	}{
		{name: "storage unavailable", mutate: func(port *system.System) {
			port.Files.MkdirTemp = func(string, string) (string, error) { return "", errors.New("no space") }
		}},
		{name: "copy cannot be written", mutate: func(port *system.System) {
			port.Files.WriteFile = func(string, []byte, fs.FileMode) error { return errors.New("disk full") }
		}},
		{name: "copy cannot be statted", mutate: func(port *system.System) {
			port.Files.Stat = func(string) (fs.FileInfo, error) { return nil, errors.New("gone") }
		}},
		{name: "copy cannot be made executable", mutate: func(port *system.System) {
			port.Files.Chmod = func(string, fs.FileMode) error { return errors.New("read-only") }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := redHost()
			port := host.Port()
			tc.mutate(&port)
			if _, err := identity.materializeCommandBinary(port); err == nil {
				t.Fatal("a command binary was materialized despite the failure")
			}
			if host.Files.Dirs[redCommandRoot] {
				t.Fatalf("temporary root %s survived the failed materialization", redCommandRoot)
			}
		})
	}
}

func TestExecuteVerifiedCommandReportsMaterializationFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.MkdirTemp = func(string, string) (string, error) { return "", errors.New("no space") }
	identity := binaryIdentity{checksum: redSum([]byte("hippo")), content: []byte("hippo")}
	err := executeVerifiedCommand(port, context.Background(), ".", Command{Arguments: []string{"true"}}, nil, &bytes.Buffer{}, identity)
	if err == nil {
		t.Fatal("a command ran without its verified binary")
	}
}

func TestExecuteVerifiedCommandReportsCleanupFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.RemoveAll = func(string) error { return errors.New("busy") }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	identity := binaryIdentity{checksum: redSum([]byte("hippo")), content: []byte("hippo")}
	err := executeVerifiedCommand(port, ctx, ".", Command{Arguments: []string{"true"}}, nil, &bytes.Buffer{}, identity)
	if err == nil || !strings.Contains(err.Error(), "verified HIPPO binary cleanup failed") {
		t.Fatalf("a failed cleanup returned %v", err)
	}
}

func TestSignalProcessGroupTargetsTheHostGroup(t *testing.T) {
	host := redHost()
	port := host.Port()
	if err := signalProcessGroup(port, nil, syscall.SIGTERM); err != nil {
		t.Fatalf("signalling no process reported %v", err)
	}
	if len(host.Processes.Sent) != 0 {
		t.Fatalf("signalling no process reached the host: %v", host.Processes.Sent)
	}
	group := &os.Process{Pid: 42}
	if err := signalProcessGroup(port, group, syscall.SIGTERM); err != nil {
		t.Fatalf("a group that already exited reported %v", err)
	}
	want := systemtest.SentSignal{Pid: -42, Signal: syscall.SIGTERM}
	if len(host.Processes.Sent) != 1 || host.Processes.Sent[0] != want {
		t.Fatalf("the group signal reached the host as %v, want %v", host.Processes.Sent, want)
	}
}

func TestProcessGroupAliveProbesTheHostGroup(t *testing.T) {
	host := redHost()
	port := host.Port()
	if alive, err := processGroupAlive(port, nil); err != nil || alive {
		t.Fatalf("no process reported alive=%t error=%v", alive, err)
	}
	group := &os.Process{Pid: 42}
	host.Processes.Alive[-42] = true
	if alive, err := processGroupAlive(port, group); err != nil || !alive {
		t.Fatalf("a live group reported alive=%t error=%v", alive, err)
	}
	delete(host.Processes.Alive, -42)
	if alive, err := processGroupAlive(port, group); err != nil || alive {
		t.Fatalf("an exited group reported alive=%t error=%v", alive, err)
	}
	port.Processes.Kill = func(int, syscall.Signal) error { return syscall.EINVAL }
	if _, err := processGroupAlive(port, group); err == nil {
		t.Fatal("a failed liveness probe was reported as no error")
	}
}

func TestObserveCommandGroupRetirementUsesTheHostClock(t *testing.T) {
	host := redHost()
	port := host.Port()
	if retired, err := observeCommandGroupRetirement(port, commandRuntime{}, nil, time.Second); err != nil || !retired {
		t.Fatalf("a runtime without a liveness probe reported retired=%t error=%v", retired, err)
	}
	stubborn := commandRuntime{alive: func(*os.Process) (bool, error) { return true, nil }}
	retired, err := observeCommandGroupRetirement(port, stubborn, nil, 12*time.Millisecond)
	if err != nil || retired {
		t.Fatalf("a live group reported retired=%t error=%v before its window closed", retired, err)
	}
	if elapsed := host.Clock.Current.Sub(redStart); elapsed != 12*time.Millisecond {
		t.Fatalf("observation advanced the host clock by %s, want the full 12ms window", elapsed)
	}
	var slept time.Duration
	for _, duration := range host.Clock.Slept {
		slept += duration
	}
	if slept != 12*time.Millisecond {
		t.Fatalf("observation slept %s on the host clock, want 12ms", slept)
	}
	exited := commandRuntime{alive: func(*os.Process) (bool, error) { return false, nil }}
	before := host.Clock.Current
	if retired, err = observeCommandGroupRetirement(port, exited, nil, time.Second); err != nil || !retired {
		t.Fatalf("an exited group reported retired=%t error=%v", retired, err)
	}
	if host.Clock.Current != before {
		t.Fatal("an exited group still advanced the host clock")
	}
	failing := commandRuntime{alive: func(*os.Process) (bool, error) { return false, errors.New("probe failed") }}
	if _, err = observeCommandGroupRetirement(port, failing, nil, time.Second); err == nil {
		t.Fatal("a failed liveness probe was not reported")
	}
}

func TestRetireCompletedCommandGroupFollowsTheStopSequence(t *testing.T) {
	host := redHost()
	port := host.Port()
	if err := retireCompletedCommandGroup(port, commandRuntime{}, nil); err != nil {
		t.Fatalf("a runtime without a liveness probe reported %v", err)
	}
	failing := commandRuntime{alive: func(*os.Process) (bool, error) { return false, errors.New("probe failed") }}
	if err := retireCompletedCommandGroup(port, failing, nil); err == nil {
		t.Fatal("a failed liveness probe was not reported")
	}
	var sent []syscall.Signal
	probes := 0
	termRetires := commandRuntime{
		alive: func(*os.Process) (bool, error) {
			probes++

			return probes == 1, nil
		},
		signal: func(_ *os.Process, signal syscall.Signal) error {
			sent = append(sent, signal)

			return nil
		},
	}
	if err := retireCompletedCommandGroup(port, termRetires, nil); err != nil {
		t.Fatalf("a group that stopped on TERM reported %v", err)
	}
	if len(sent) != 1 || sent[0] != syscall.SIGTERM {
		t.Fatalf("a group that stopped on TERM received %v", sent)
	}
	sent = nil
	stubborn := commandRuntime{
		alive: func(*os.Process) (bool, error) { return true, nil },
		signal: func(_ *os.Process, signal syscall.Signal) error {
			sent = append(sent, signal)

			return nil
		},
	}
	before := host.Clock.Current
	if err := retireCompletedCommandGroup(port, stubborn, nil); err == nil {
		t.Fatal("a group that never stopped was reported as retired")
	}
	if len(sent) != 2 || sent[0] != syscall.SIGTERM || sent[1] != syscall.SIGKILL {
		t.Fatalf("a group that never stopped received %v, want TERM then KILL", sent)
	}
	if waited := host.Clock.Current.Sub(before); waited != commandStopGrace+commandReapGrace {
		t.Fatalf("retirement waited %s on the host clock, want %s", waited, commandStopGrace+commandReapGrace)
	}
}

func TestCommandResultReportsWaitFailure(t *testing.T) {
	var failure *commandError
	if err := commandResult(errors.New("wait broke")); !errors.As(err, &failure) || failure.category != "wait failed" {
		t.Fatalf("a wait failure returned %v", err)
	}
}

func TestReceiptSnapshotReportsInventoryFailure(t *testing.T) {
	host := redHost()
	port := host.Port()
	names, err := receiptSnapshot(port, redSharedRoot)
	if err != nil || len(names) != 0 {
		t.Fatalf("a missing receipts directory returned %v with %d names", err, len(names))
	}
	denied := port
	denied.Files.ReadDir = func(string) ([]fs.DirEntry, error) { return nil, errors.New("permission denied") }
	if _, err = receiptSnapshot(denied, redSharedRoot); err == nil {
		t.Fatal("an unreadable receipts directory was inventoried")
	}
	host.Files.Dirs[redReceipts] = true
	host.Files.Contents[redReceipts+"/a.json"] = []byte("{}")
	host.Files.Contents[redReceipts+"/b.txt"] = []byte("{}")
	if names, err = receiptSnapshot(host.Port(), redSharedRoot); err != nil {
		t.Fatal(err)
	}
	if _, ok := names["a.json"]; !ok || len(names) != 1 {
		t.Fatalf("the inventory held %v, want only a.json", names)
	}
}

func TestNewNeverStartedReceiptValidatesNewReceipts(t *testing.T) {
	neverStarted := []byte(`{"schemaVersion":1,"state":"never-started"}`)
	oldReceipt := map[string]struct{}{"old.json": {}}
	cases := []struct {
		name     string
		setup    func(host *systemtest.Host, port *system.System)
		before   map[string]struct{}
		verified bool
		fails    bool
	}{
		{name: "no receipts directory"},
		{name: "unreadable inventory", setup: func(host *systemtest.Host, port *system.System) {
			host.Files.Dirs[redReceipts] = true
			port.Files.ReadDir = func(string) ([]fs.DirEntry, error) { return nil, errors.New("denied") }
		}, fails: true},
		{name: "known and foreign entries are skipped", before: oldReceipt, setup: func(host *systemtest.Host, _ *system.System) {
			host.Files.Dirs[redReceipts] = true
			host.Files.Contents[redReceipts+"/old.json"] = neverStarted
			host.Files.Contents[redReceipts+"/notes.txt"] = []byte("note")
		}},
		{name: "directory posing as a receipt", setup: func(host *systemtest.Host, _ *system.System) {
			host.Files.Dirs[redReceipts] = true
			host.Files.Dirs[redReceipts+"/new.json"] = true
		}, fails: true},
		{name: "receipt cannot be inspected", setup: func(host *systemtest.Host, port *system.System) {
			host.Files.Dirs[redReceipts] = true
			host.Files.Contents[redReceipts+"/new.json"] = neverStarted
			lstat := port.Files.Lstat
			port.Files.Lstat = func(name string) (fs.FileInfo, error) {
				if name == redReceipts+"/new.json" {
					return nil, errors.New("denied")
				}

				return lstat(name)
			}
		}, fails: true},
		{name: "receipt cannot be opened", setup: func(host *systemtest.Host, port *system.System) {
			host.Files.Dirs[redReceipts] = true
			host.Files.Contents[redReceipts+"/new.json"] = neverStarted
			read := port.Files.ReadFile
			port.Files.ReadFile = func(name string) ([]byte, error) {
				if name == redReceipts+"/new.json" {
					return nil, errors.New("denied")
				}

				return read(name)
			}
		}, fails: true},
		{name: "receipt too large", setup: func(host *systemtest.Host, _ *system.System) {
			host.Files.Dirs[redReceipts] = true
			host.Files.Contents[redReceipts+"/new.json"] = make([]byte, maximumReceiptBytes+1)
		}, fails: true},
		{name: "receipt is not JSON", setup: func(host *systemtest.Host, _ *system.System) {
			host.Files.Dirs[redReceipts] = true
			host.Files.Contents[redReceipts+"/new.json"] = []byte("not json")
		}, fails: true},
		{name: "receipt state is not never-started", setup: func(host *systemtest.Host, _ *system.System) {
			host.Files.Dirs[redReceipts] = true
			host.Files.Contents[redReceipts+"/new.json"] = []byte(`{"schemaVersion":1,"state":"started-safety-stop"}`)
		}, fails: true},
		{name: "data after the receipt", setup: func(host *systemtest.Host, _ *system.System) {
			host.Files.Dirs[redReceipts] = true
			host.Files.Contents[redReceipts+"/new.json"] = append(bytes.Clone(neverStarted), []byte(" trailing")...)
		}, fails: true},
		{name: "new never-started receipt", setup: func(host *systemtest.Host, _ *system.System) {
			host.Files.Dirs[redReceipts] = true
			host.Files.Contents[redReceipts+"/new.json"] = neverStarted
		}, verified: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := redHost()
			port := host.Port()
			if tc.setup != nil {
				tc.setup(host, &port)
			}
			before := tc.before
			if before == nil {
				before = map[string]struct{}{}
			}
			verified, err := newNeverStartedReceipt(port, redSharedRoot, before)
			if tc.fails != (err != nil) {
				t.Fatalf("error = %v, want failure %t", err, tc.fails)
			}
			if verified != tc.verified {
				t.Fatalf("verified = %t, want %t", verified, tc.verified)
			}
		})
	}
}

func TestExecuteWithRuntimeReportsStartAndRetirementFailures(t *testing.T) {
	command := Command{Arguments: []string{"true"}}
	start := commandRuntime{
		start: func(*exec.Cmd) error { return errors.New("no fork") },
		wait:  func(*exec.Cmd) error { return nil },
	}
	if err := executeWithRuntime(redHost().Port(), context.Background(), ".", command, nil, &bytes.Buffer{}, start); err == nil || !strings.Contains(err.Error(), "start failed") {
		t.Fatalf("a command that failed to start returned %v", err)
	}
	probe := commandRuntime{
		start:  func(*exec.Cmd) error { return nil },
		wait:   func(*exec.Cmd) error { return nil },
		signal: func(*os.Process, syscall.Signal) error { return nil },
		alive:  func(*os.Process) (bool, error) { return false, errors.New("probe failed") },
	}
	if err := executeWithRuntime(redHost().Port(), context.Background(), ".", command, nil, &bytes.Buffer{}, probe); err == nil || !strings.Contains(err.Error(), "group retirement failed") {
		t.Fatalf("a failed retirement probe returned %v", err)
	}
}

// redStopRun runs executeWithRuntime with a context cancelled as the command starts. The leader exits once releaseOn
// is signalled, the group stops being alive once retireOn is signalled, and KILL returns killError. The host timers
// fire in the order given. It returns the signals sent, in order, and the run's error.
func redStopRun(t *testing.T, fires []bool, releaseOn, retireOn syscall.Signal, killError error) ([]syscall.Signal, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	released, retired := false, false
	var sent []syscall.Signal
	runtime := commandRuntime{
		start: func(*exec.Cmd) error {
			cancel()

			return nil
		},
		wait: func(*exec.Cmd) error {
			<-release

			return nil
		},
		signal: func(_ *os.Process, signal syscall.Signal) error {
			sent = append(sent, signal)
			if signal == releaseOn && !released {
				released = true
				close(release)
			}
			if signal == retireOn {
				retired = true
			}
			if signal == syscall.SIGKILL {
				return killError
			}

			return nil
		},
		alive: func(*os.Process) (bool, error) { return !retired, nil },
	}
	port := redHost().Port()
	port.Clock.After = redScriptedAfter(fires...)
	err := executeWithRuntime(port, ctx, ".", Command{Arguments: []string{"true"}}, nil, &bytes.Buffer{}, runtime)
	if !released {
		close(release)
	}

	return sent, err
}

func TestExecuteWithRuntimeEscalatesThroughTheStopSequence(t *testing.T) {
	stopFailed := errors.New("kill refused")
	cases := []struct {
		name      string
		fires     []bool
		releaseOn syscall.Signal
		retireOn  syscall.Signal
		killError error
		want      string
	}{
		{
			name: "TERM completes the leader without retiring the group", releaseOn: syscall.SIGTERM,
			want: "force-stop reap timed out",
		},
		{
			name: "KILL fails after the group is retired", releaseOn: syscall.SIGTERM, retireOn: syscall.SIGKILL,
			killError: stopFailed, want: "force-stop signal failed",
		},
		{
			name: "KILL retires the group", releaseOn: syscall.SIGTERM, retireOn: syscall.SIGKILL,
			want: "cancelled",
		},
		{
			name: "TERM retires the group", releaseOn: syscall.SIGTERM, retireOn: syscall.SIGTERM,
			want: "cancelled",
		},
		{
			name: "grace timer fires and the leader is never observed", fires: []bool{true, false},
			releaseOn: syscall.SIGKILL, want: "force-stop reap timed out",
		},
		{
			name: "grace timer fires and KILL fails", fires: []bool{true, false}, releaseOn: syscall.SIGKILL,
			retireOn: syscall.SIGKILL, killError: stopFailed, want: "force-stop signal failed",
		},
		{
			name: "grace timer fires and KILL retires the group", fires: []bool{true, false},
			releaseOn: syscall.SIGKILL, retireOn: syscall.SIGKILL, want: "cancelled",
		},
		{
			name: "reap timer fires before the leader exits", fires: []bool{true, true},
			want: "force-stop reap timed out",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sent, err := redStopRun(t, tc.fires, tc.releaseOn, tc.retireOn, tc.killError)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("the run returned %v, want %q", err, tc.want)
			}
			if len(sent) == 0 || sent[0] != syscall.SIGTERM {
				t.Fatalf("the run sent %v, want TERM first", sent)
			}
		})
	}
}

func TestGitOutputReportsEachFailure(t *testing.T) {
	host := redHost()
	port := host.Port()
	dir := redConsumers(1)[0].Path
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	host.Runner.Failures["git"] = errors.New("no repository")
	if _, err := gitOutput(port, cancelled, dir, "rev-parse", "HEAD"); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled git call returned %v, want context.Canceled", err)
	}
	host.Runner.Failures["git"] = &exec.ExitError{}
	if _, err := gitOutput(port, context.Background(), dir, "status"); err == nil || !strings.Contains(err.Error(), "exited with status") {
		t.Fatalf("a failed git call returned %v", err)
	}
	host.Runner.Failures["git"] = errors.New("no git")
	if _, err := gitOutput(port, context.Background(), dir, "status"); err == nil || !strings.Contains(err.Error(), "start failed") {
		t.Fatalf("a git call that did not start returned %v", err)
	}
	delete(host.Runner.Failures, "git")
	host.Runner.Stdout["git"] = []byte("abc123\n")
	head, err := gitOutput(port, context.Background(), dir, "rev-parse", "HEAD")
	if err != nil || head != "abc123" {
		t.Fatalf("git output was %q with %v, want abc123", head, err)
	}
}

// redSnapshotSetup registers a checkout, captures its identity, and returns the host, its port, and the identity.
func redSnapshotSetup(t *testing.T, consumer Consumer) (*systemtest.Host, system.System, checkoutIdentity) {
	t.Helper()
	host := redHost()
	host.Files.Dirs[consumer.Path] = true
	host.Runner.Stdout["git"] = []byte("abc123\n")
	port := host.Port()
	identities, err := captureCheckoutIdentities(port, []Consumer{consumer})
	if err != nil {
		t.Fatal(err)
	}

	return host, port, identities[consumer.Name]
}

func TestSnapshotReportsEachCheckpointFailure(t *testing.T) {
	consumer := redConsumers(1)[0]
	ctx := context.Background()

	if _, err := snapshot(redHost().Port(), ctx, consumer, checkoutIdentity{}); err == nil {
		t.Fatal("a checkout without an identity was snapshotted")
	}

	host, port, identity := redSnapshotSetup(t, consumer)
	host.Runner.Failures["git"] = errors.New("no repository")
	if _, err := snapshot(port, ctx, consumer, identity); err == nil || !strings.Contains(err.Error(), "HEAD snapshot") {
		t.Fatalf("a failed HEAD read returned %v", err)
	}

	_, port, identity = redSnapshotSetup(t, consumer)
	stats := 0
	stat := port.Files.Stat
	port.Files.Stat = func(name string) (fs.FileInfo, error) {
		if name == consumer.Path {
			stats++
			if stats == 2 {
				return nil, errors.New("checkout replaced")
			}
		}

		return stat(name)
	}
	if _, err := snapshot(port, ctx, consumer, identity); err == nil {
		t.Fatal("a checkout replaced after the HEAD read was snapshotted")
	}

	_, port, identity = redSnapshotSetup(t, consumer)
	port.Runner.CaptureOutput = func(_ context.Context, _ string, arguments ...string) ([]byte, error) {
		if slices.Contains(arguments, "status") {
			return nil, errors.New("status failed")
		}

		return []byte("abc123\n"), nil
	}
	if _, err := snapshot(port, ctx, consumer, identity); err == nil || !strings.Contains(err.Error(), "dirty snapshot") {
		t.Fatalf("a failed status read returned %v", err)
	}

	_, port, identity = redSnapshotSetup(t, consumer)
	stats = 0
	stat = port.Files.Stat
	port.Files.Stat = func(name string) (fs.FileInfo, error) {
		if name == consumer.Path {
			stats++
			if stats == 3 {
				return nil, errors.New("checkout replaced")
			}
		}

		return stat(name)
	}
	if _, err := snapshot(port, ctx, consumer, identity); err == nil {
		t.Fatal("a checkout replaced after the status read was snapshotted")
	}

	_, port, identity = redSnapshotSetup(t, consumer)
	port.Runner.CaptureOutput = func(_ context.Context, _ string, arguments ...string) ([]byte, error) {
		if slices.Contains(arguments, "status") {
			return []byte("?? new.txt\n"), nil
		}

		return []byte("abc123\n"), nil
	}
	state, err := snapshot(port, ctx, consumer, identity)
	if err != nil || state.head != "abc123" || state.dirty != "?? new.txt" {
		t.Fatalf("a clean snapshot returned %+v with %v", state, err)
	}
}

func TestEnvironmentOpenReportsSetupFailures(t *testing.T) {
	cases := []struct {
		name    string
		corrupt func(*Manifest)
		mutate  func(*system.System)
	}{
		{name: "binary missing from manifest", corrupt: func(manifest *Manifest) { manifest.HIPPOBinary = "" }},
		{name: "consumer checkout moves after validation", mutate: func(port *system.System) {
			stat := port.Files.Stat
			checks := 0
			port.Files.Stat = func(name string) (fs.FileInfo, error) {
				if strings.HasPrefix(name, "/hippo/consumer-") {
					checks++
					if checks > 4 {
						return nil, errors.New("checkout moved")
					}
				}

				return stat(name)
			}
		}},
		{name: "shared root cannot be created", mutate: func(port *system.System) {
			port.Files.MkdirAll = func(string, fs.FileMode) error { return errors.New("read-only") }
		}},
		{name: "shared root cannot be identified", mutate: func(port *system.System) {
			stat := port.Files.Stat
			port.Files.Stat = func(name string) (fs.FileInfo, error) {
				if name == redSharedRoot {
					return nil, errors.New("gone")
				}

				return stat(name)
			}
		}},
		{name: "binary cannot be read", mutate: func(port *system.System) {
			read := port.Files.ReadFile
			port.Files.ReadFile = func(name string) ([]byte, error) {
				if name == redBinaryPath {
					return nil, errors.New("permission denied")
				}

				return read(name)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := redHost()
			manifest := redFixture(host)
			if tc.corrupt != nil {
				tc.corrupt(&manifest)
			}
			redWriteManifest(t, host, manifest)
			port := host.Port()
			if tc.mutate != nil {
				tc.mutate(&port)
			}
			if _, err := (Environment{Host: port}).Open(context.Background(), redManifestPath); err == nil {
				t.Fatal("a session opened despite the setup failure")
			}
		})
	}
}

func TestEnvironmentOpenReportsMissingManifest(t *testing.T) {
	host := redHost()
	if _, err := (Environment{Host: host.Port()}).Open(context.Background(), redManifestPath); err == nil {
		t.Fatal("a missing manifest opened a session")
	}
}

// redOpenSession opens a session over a valid fixture, after the optional mutation of the port.
func redOpenSession(t *testing.T, mutate func(*system.System)) (*systemtest.Host, application.ConformanceSession) {
	t.Helper()
	host := redHost()
	redWriteManifest(t, host, redFixture(host))
	port := host.Port()
	if mutate != nil {
		mutate(&port)
	}
	session, err := (Environment{Host: port}).Open(context.Background(), redManifestPath)
	if err != nil {
		t.Fatal(err)
	}

	return host, session
}

func TestSessionVerifiesAndClosesOverTheHost(t *testing.T) {
	host, session := redOpenSession(t, nil)
	if got := session.Manifest().SharedRoot; got != redSharedRoot {
		t.Fatalf("the session reports shared root %q, want %q", got, redSharedRoot)
	}
	if err := session.Verify("consumer-1"); err != nil {
		t.Fatalf("an unchanged checkout failed verification: %v", err)
	}
	if err := session.VerifyBinary(); err != nil {
		t.Fatalf("an unchanged binary failed verification: %v", err)
	}
	if receipts, err := session.Receipts(); err != nil || len(receipts) != 0 {
		t.Fatalf("an empty shared root reported %d receipts with %v", len(receipts), err)
	}
	if err := session.Verify("unknown"); err == nil {
		t.Fatal("an unknown consumer verified")
	}
	if err := session.Close(); err != nil {
		t.Fatalf("closing the session failed: %v", err)
	}
	if host.Files.Dirs[redBinaryRoot] {
		t.Fatal("closing the session left the verified binary behind")
	}
}

func TestSessionVerifyDetectsReplacedSharedRoot(t *testing.T) {
	replaced := false
	_, session := redOpenSession(t, func(port *system.System) {
		stat := port.Files.Stat
		port.Files.Stat = func(name string) (fs.FileInfo, error) {
			if replaced && name == redSharedRoot {
				return nil, errors.New("replaced")
			}

			return stat(name)
		}
	})
	replaced = true
	if err := session.Verify(""); err == nil {
		t.Fatal("a shared root replaced after opening verified")
	}
}

func TestSessionCloseReportsBinaryCleanupFailure(t *testing.T) {
	failing := false
	_, session := redOpenSession(t, func(port *system.System) {
		remove := port.Files.RemoveAll
		port.Files.RemoveAll = func(path string) error {
			if failing {
				return errors.New("busy")
			}

			return remove(path)
		}
	})
	failing = true
	if err := session.Close(); err == nil || !strings.Contains(err.Error(), "verified HIPPO binary cleanup failed") {
		t.Fatalf("a failed binary cleanup returned %v", err)
	}
}

func TestDeferralFixtureReportsEachFailure(t *testing.T) {
	cases := []struct {
		name   string
		stage  string
		mutate func(*system.System)
	}{
		{name: "probe root cannot be created", stage: "setup", mutate: func(port *system.System) {
			mkdirTemp := port.Files.MkdirTemp
			port.Files.MkdirTemp = func(dir, pattern string) (string, error) {
				if pattern == "hippo-deferral-probe-" {
					return "", errors.New("no space")
				}

				return mkdirTemp(dir, pattern)
			}
		}},
		{name: "capacity hold cannot be written", stage: "setup", mutate: func(port *system.System) {
			writeFile := port.Files.WriteFile
			port.Files.WriteFile = func(name string, data []byte, perm fs.FileMode) error {
				if strings.HasSuffix(name, capacityHoldName) {
					return errors.New("disk full")
				}

				return writeFile(name, data, perm)
			}
		}},
		{name: "receipt cannot be written", stage: "setup", mutate: func(port *system.System) {
			writeFile := port.Files.WriteFile
			port.Files.WriteFile = func(name string, data []byte, perm fs.FileMode) error {
				if strings.HasSuffix(name, neverStartedReceiptName) {
					return errors.New("disk full")
				}

				return writeFile(name, data, perm)
			}
		}},
		{name: "capacity release fails", stage: "release", mutate: func(port *system.System) {
			remove := port.Files.Remove
			port.Files.Remove = func(name string) error {
				if strings.HasSuffix(name, capacityHoldName) {
					return errors.New("busy")
				}

				return remove(name)
			}
		}},
		{name: "probe cleanup fails", stage: "close", mutate: func(port *system.System) {
			removeAll := port.Files.RemoveAll
			port.Files.RemoveAll = func(path string) error {
				if strings.HasPrefix(path, "hippo-deferral-probe-") {
					return errors.New("busy")
				}

				return removeAll(path)
			}
		}},
		{name: "probe is usable", stage: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, session := redOpenSession(t, tc.mutate)
			fixture, err := session.DeferralFixture()
			if tc.stage == "setup" {
				if err == nil {
					t.Fatal("a deferral probe was built despite the failure")
				}
				if host.Files.Dirs[redDeferralRoot] {
					t.Fatalf("probe root %s survived the failed setup", redDeferralRoot)
				}

				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fixture.Root() != redDeferralRoot {
				t.Fatalf("probe root is %q, want %q", fixture.Root(), redDeferralRoot)
			}
			releaseErr := fixture.Release()
			if (tc.stage == "release") != (releaseErr != nil) {
				t.Fatalf("release returned %v, want failure %t", releaseErr, tc.stage == "release")
			}
			closeErr := fixture.Close()
			if (tc.stage == "close") != (closeErr != nil) {
				t.Fatalf("close returned %v, want failure %t", closeErr, tc.stage == "close")
			}
		})
	}
}
