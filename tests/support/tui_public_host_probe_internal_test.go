package support

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/contract"
)

func TestPublicHostProbeRefusesForeignPreparationBeforeEffects(t *testing.T) {
	for _, test := range []struct {
		name      string
		driver    *Driver
		directory string
		reply     PublicTUIHostReply
	}{
		{"nil-driver", nil, "/owned/probe", PublicTUIHostReply{}},
		{"unit", &Driver{mode: contract.Unit}, "/owned/probe", PublicTUIHostReply{}},
		{"integration", &Driver{mode: contract.Integration}, "/owned/probe", PublicTUIHostReply{}},
		{"foreign-directory", &Driver{mode: contract.E2E, temporaryPaths: []string{"/owned/probe"}}, "/foreign/probe", PublicTUIHostReply{}},
		{"relative-directory", &Driver{mode: contract.E2E, temporaryPaths: []string{"relative"}}, "relative", PublicTUIHostReply{}},
		{"traversal-directory", &Driver{mode: contract.E2E, temporaryPaths: []string{"/owned/../foreign"}}, "/owned/../foreign", PublicTUIHostReply{}},
		{"negative-CPU", &Driver{mode: contract.E2E, temporaryPaths: []string{"/owned/probe"}}, "/owned/probe", PublicTUIHostReply{CPUPercent: -1}},
		{"excess-CPU", &Driver{mode: contract.E2E, temporaryPaths: []string{"/owned/probe"}}, "/owned/probe", PublicTUIHostReply{CPUPercent: 101}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, err := test.driver.NewPublicTUIHostProbe(t.Context(), test.directory, test.reply)
			if fixture != nil || err == nil {
				t.Fatal("foreign preparation acquired a public collector fixture")
			}
		})
	}
}

func publicHostProtocolFixture() *PublicTUIHostProbe {
	return &PublicTUIHostProbe{
		executable: "/owned/test-helper", nonce: "owned-protocol-nonce",
		gates: map[int]*publicHostGate{}, changed: make(chan struct{}), stopping: make(chan struct{}),
	}
}

func publicHostProtocolRequest() publicHostRequest {
	return publicHostRequest{
		Nonce: "owned-protocol-nonce", Executable: "/owned/test-helper",
		PID: 17, ParentPID: 23, Name: "ps", Arguments: []string{"-A", "-o", "%cpu="},
	}
}

// A helper may be cancelled after connect and before its first request. Only
// zero received bytes are a clean disconnect; received invalid data stays fatal.
func TestPublicHostProbePreRequestDisconnect(t *testing.T) {
	for _, test := range []struct {
		name, input string
		wantError   error
		wantSyntax  bool
	}{
		{name: "zero-byte"},
		{name: "whitespace", input: " \t\n", wantError: io.EOF},
		{name: "truncated", input: `{"Nonce":`, wantError: io.ErrUnexpectedEOF},
		{name: "malformed", input: "}", wantSyntax: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe, owner := newPublicHostPreRequestControl(t)
			closeError, ownerError := disconnectPublicHostPreRequest(t, probe, owner, test.input)
			requirePublicHostPreRequestJoined(t, probe)
			if test.input == "" {
				if err := errors.Join(closeError, ownerError); err != nil {
					t.Fatal("zero-byte peer disconnect poisoned joined retirement", err)
				}
				if len(owner.sets) != 0 || probe.wrappers != nil {
					t.Fatal("joined zero-byte handler retained its wrapper borrower")
				}
				if _, err := os.Stat(probe.wrapperDirectory); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("joined wrapper code survived owner retirement: %v", err)
				}
				return
			}
			requirePublicHostPreRequestFailure(t, closeError, test.wantError, test.wantSyntax)
			if ownerError == nil || probe.wrappers == nil || probe.wrappers.borrowers != 1 {
				t.Fatal("received invalid data bypassed failed borrower retirement")
			}
			if _, err := os.Stat(probe.wrapperDirectory); err != nil {
				t.Fatal("failed borrower retirement lost its owned wrapper code", err)
			}
			t.Logf("received data preserved decode failure and borrower: %v / %v", closeError, ownerError)
		})
	}
}

func disconnectPublicHostPreRequest(t *testing.T, probe *PublicTUIHostProbe, owner *publicTUIHostWrapperOwner, input string) (error, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "tcp4", probe.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server, err := probe.listener.Accept()
	if err != nil {
		t.Fatal(errors.Join(err, connection.Close()))
	}
	probe.connections[server] = 0
	done := make(chan struct{})
	probe.done = append(probe.done, done)
	go func() { probe.handleDarwin(server); close(done) }()
	if _, err := io.WriteString(connection, input); err != nil {
		t.Fatal(errors.Join(err, connection.Close()))
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("real pre-request handler did not join", ctx.Err())
	}
	closeError := probe.Close(ctx)
	return closeError, owner.close()
}

func requirePublicHostPreRequestJoined(t *testing.T, probe *PublicTUIHostProbe) {
	t.Helper()
	trace := probe.Trace()
	if !trace.Closed || trace.Readings != 0 || trace.Active != 0 || trace.MaximumActive != 0 || len(trace.Events) != 0 ||
		len(probe.connections) != 0 || probe.darwinPending != nil || probe.darwinParent != 0 || len(probe.darwinHelpers) != 0 {
		t.Fatalf("unaccepted connection invented a reading or retained a handler: %+v", trace)
	}
	if _, err := os.Stat(probe.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("joined probe directory survived: %v", err)
	}
}

func requirePublicHostPreRequestFailure(t *testing.T, err, expected error, syntax bool) {
	t.Helper()
	if syntax {
		if _, ok := errors.AsType[*json.SyntaxError](err); !ok {
			t.Fatal("malformed request lost its syntax error", err)
		}
		return
	}
	if !errors.Is(err, expected) {
		t.Fatal("received request lost its decode error", err)
	}
}

func newPublicHostPreRequestControl(t *testing.T) (*PublicTUIHostProbe, *publicTUIHostWrapperOwner) {
	t.Helper()
	owner, err := newPublicTUIHostWrapperOwner(publicHostPreRequestDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	set, err := owner.acquire(executable, publicHostDarwinScripts(executable))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	probe := publicHostProtocolFixture()
	probe.directory = publicHostPreRequestDirectory(t)
	probe.executable, probe.platform, probe.origin = executable, darwinPlatform, time.Now()
	probe.wrapperOwner, probe.wrappers, probe.wrapperDirectory = owner, set, set.directory
	probe.listener, probe.connections = listener, map[net.Conn]int{}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
		defer cancel()
		if err := probe.Close(ctx); err != nil {
			t.Log("retained original probe failure at final owned cleanup", err)
		}
		if err := owner.close(); err != nil {
			t.Log("retained original borrower failure; private TempDir removes only joined test data", err)
		}
	})
	return probe, owner
}

func publicHostPreRequestDirectory(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestPublicHostProbeExactProtocolRejectsForeignAndOutOfOrder(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*publicHostRequest)
	}{
		{"foreign-nonce", func(request *publicHostRequest) { request.Nonce = "foreign" }},
		{"foreign-helper", func(request *publicHostRequest) { request.Executable = "/foreign/helper" }},
		{"missing-PID", func(request *publicHostRequest) { request.PID = 0 }},
		{"missing-parent", func(request *publicHostRequest) { request.ParentPID = 0 }},
		{"foreign-query", func(request *publicHostRequest) { request.Name = "foreign" }},
		{"wrong-argv", func(request *publicHostRequest) { request.Arguments = []string{"-A"} }},
		{"out-of-order-query", func(request *publicHostRequest) { request.Name = "memory_pressure"; request.Arguments = []string{"-Q"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := publicHostProtocolFixture()
			request := publicHostProtocolRequest()
			test.change(&request)
			if _, _, err := probe.acceptDarwinLocked(request); err == nil {
				t.Fatal("foreign or out-of-order protocol query was accepted")
			}
			if probe.trace.Readings != 0 || probe.trace.Active != 0 {
				t.Fatal("rejected query invented an acknowledged reading")
			}
		})
	}
}

func TestPublicHostProbeProtocolDetectsOverlapWithoutReplacingActiveReading(t *testing.T) {
	probe := publicHostProtocolFixture()
	request := publicHostProtocolRequest()
	if reading, _, err := probe.acceptDarwinLocked(request); err != nil || reading != 1 {
		t.Fatalf("first protocol query: %d %v", reading, err)
	}
	if _, _, err := probe.acceptDarwinLocked(request); err == nil {
		t.Fatal("overlapping protocol query was accepted")
	}
	if probe.trace.MaximumActive != 2 || probe.trace.Readings != 2 || probe.trace.Active != 1 || probe.reading != 1 {
		t.Fatalf("overlap was hidden or replaced the acknowledged reading: %+v", probe.trace)
	}
	if probe.trace.Events[len(probe.trace.Events)-1].Phase != "overlap-refused" {
		t.Fatal("overlap refusal is absent from retained events")
	}
}

func TestPublicHostProbePinnedHelperRefusesInvalidSelectionBeforeEffects(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "lean-helper")
	original := []byte("owned executable fixture\n")
	if err := os.WriteFile(executable, original, 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(original)
	valid := PublicTUIHostHelper{Executable: executable, SHA256: hex.EncodeToString(digest[:])}
	if err := validatePublicTUIHostHelper(valid); err != nil {
		t.Fatal("valid pinned artifact refused", err)
	}
	link := filepath.Join(directory, "symlink")
	if err := os.Symlink(executable, link); err != nil {
		t.Fatal(err)
	}
	nonExecutable := filepath.Join(directory, "not-executable")
	if err := os.WriteFile(nonExecutable, original, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		helper PublicTUIHostHelper
	}{
		{"missing-input", PublicTUIHostHelper{}},
		{"missing-file", PublicTUIHostHelper{Executable: filepath.Join(directory, "missing"), SHA256: valid.SHA256}},
		{"relative", PublicTUIHostHelper{Executable: "lean-helper", SHA256: valid.SHA256}},
		{"unclean", PublicTUIHostHelper{Executable: directory + "/../" + filepath.Base(directory) + "/lean-helper", SHA256: valid.SHA256}},
		{"directory", PublicTUIHostHelper{Executable: directory, SHA256: valid.SHA256}},
		{"symlink", PublicTUIHostHelper{Executable: link, SHA256: valid.SHA256}},
		{"non-executable", PublicTUIHostHelper{Executable: nonExecutable, SHA256: valid.SHA256}},
		{"bad-digest", PublicTUIHostHelper{Executable: executable, SHA256: "not-hex"}},
		{"short-digest", PublicTUIHostHelper{Executable: executable, SHA256: "aa"}},
		{"mismatched", PublicTUIHostHelper{Executable: executable, SHA256: strings.Repeat("0", 64)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validatePublicTUIHostHelper(test.helper); err == nil {
				t.Fatal("invalid helper selected")
			}
			if runtime.GOOS != darwinPlatform {
				return
			} // Linux does not consume a Darwin helper.
			before, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			driver := &Driver{mode: contract.E2E, temporaryPaths: []string{directory}}
			probe, err := driver.NewPublicTUIHostProbeWithHelper(t.Context(), directory, PublicTUIHostReply{}, test.helper)
			after, readError := os.ReadDir(directory)
			if readError != nil {
				t.Fatal(readError)
			}
			names := func(entries []os.DirEntry) []string {
				result := make([]string, len(entries))
				for i, entry := range entries {
					result[i] = entry.Name()
				}
				return result
			}
			if err == nil || probe != nil || len(driver.stops) != 0 || !slices.Equal(names(before), names(after)) {
				t.Fatalf("refusal acquired resources: %v probe=%v stops=%d", err, probe, len(driver.stops))
			}
		})
	}
	if err := os.WriteFile(executable, []byte("mutated helper\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validatePublicTUIHostHelper(valid); err == nil {
		t.Fatal("artifact mutation retained stale SHA selection")
	}
}

// These controls exercise the selected externally prepared executable through
// the actual native wrapper and nonce-bound server. No product is substituted.
func TestPublicHostProbeLeanWrapperNaturalExitAndCancellation(t *testing.T) {
	if runtime.GOOS != darwinPlatform {
		return
	} // Darwin wrapper controls; Linux retains its FIFO controls.
	helper := PublicTUIHostHelper{Executable: os.Getenv("HIPPO_PUBLIC_HOST_HELPER"), SHA256: os.Getenv("HIPPO_PUBLIC_HOST_HELPER_SHA256")}
	for _, canceled := range []bool{false, true} {
		name := "natural-exit"
		if canceled {
			name = "canceled-real-reader"
		}
		t.Run(name, func(t *testing.T) {
			probe := newPublicHostProbeControl(t, helper)
			var err error
			reading := 0
			if canceled {
				reading, err = probe.HoldNextReading()
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, filepath.Join(probe.wrapperDirectory, "ps"), "-A", "-o", "%cpu=")
			command.Env = probe.Environment([]string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "GOMAXPROCS=2"})
			command.WaitDelay = 3 * time.Second
			var output, diagnostic bytes.Buffer
			command.Stdout = &output
			command.Stderr = &diagnostic
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			pid := command.Process.Pid
			var observedError error
			if canceled {
				observedError = probe.WaitForReading(ctx, reading)
				cancel()
			}
			waitError := command.Wait()
			before := probe.Trace()
			cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
			closeError := probe.Close(cleanup)
			stop()
			if err := errors.Join(observedError, closeError); err != nil {
				t.Fatal(err)
			}
			if canceled {
				requireCanceledPublicProbe(t, probe, waitError, &output, pid)
			} else {
				requireNaturalPublicProbe(t, command, waitError, &output, &diagnostic, before, pid)
			}
			if err := validatePublicTUIHostHelper(helper); err != nil {
				t.Fatal("selected helper changed during actual control", err)
			}
			if _, err := os.Stat(probe.directory); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("owned probe directory survived: %v", err)
			}
		})
	}
}

func requireCanceledPublicProbe(t *testing.T, probe *PublicTUIHostProbe, waitError error, output *bytes.Buffer, pid int) {
	t.Helper()
	if waitError == nil || output.Len() != 0 {
		t.Fatalf("canceled helper fabricated successful output/exit: %q %v", output.String(), waitError)
	}
	trace := probe.Trace()
	if trace.Active != 0 || !trace.Closed || !slices.ContainsFunc(trace.Events, func(event PublicTUIHostProbeEvent) bool {
		return event.Phase == "canceled-eof" && event.PID == pid && event.ParentPID == os.Getpid()
	}) {
		t.Fatalf("actual cancellation/EOF absent: %+v", trace)
	}
}

func requireNaturalPublicProbe(t *testing.T, command *exec.Cmd, waitError error, output, diagnostic *bytes.Buffer, before PublicTUIHostProbeTrace, pid int) {
	t.Helper()

	want := strconv.Itoa(20*runtime.NumCPU()) + ".0\n"
	if waitError != nil || command.ProcessState.ExitCode() != 0 || output.String() != want || diagnostic.Len() != 0 {
		t.Fatalf("natural query output/Wait: %q %q %v", output.String(), diagnostic.String(), waitError)
	}
	if before.Readings != 1 || before.Active != 1 || before.MaximumActive != 1 || len(before.Events) != 2 || before.Events[0].Phase != "start" || before.Events[1].Phase != "output-ack" {
		t.Fatalf("single query invented a completed cohort: %+v", before)
	}
	for _, event := range before.Events {
		if event.PID != pid || event.ParentPID != os.Getpid() {
			t.Fatalf("real helper identity mismatch: %+v", event)
		}
	}
}

func newPublicHostProbeControl(t *testing.T, helper PublicTUIHostHelper) *PublicTUIHostProbe {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	driver := &Driver{mode: contract.E2E, temporaryPaths: []string{directory}}
	probe, err := driver.NewPublicTUIHostProbeWithHelper(t.Context(), directory, PublicTUIHostReply{CPUPercent: 20}, helper)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := probe.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return probe
}

// The package TestMain calls RunIsolated, the suite lifetime that must own
// reusable wrapper code. These are real pinned-helper probes, not timing mocks.
func TestPublicHostProbeSuiteReusesWrappersAcrossClosedProbes(t *testing.T) {
	if runtime.GOOS != darwinPlatform {
		return
	} // Darwin executable wrappers; Linux retains its existing FIFO route.
	if os.Getenv(isolatedRootVariable) == "" {
		t.Fatal("wrapper lifetime control requires the actual RunIsolated suite")
	}
	helper := PublicTUIHostHelper{Executable: os.Getenv("HIPPO_PUBLIC_HOST_HELPER"), SHA256: os.Getenv("HIPPO_PUBLIC_HOST_HELPER_SHA256")}
	first := newPublicHostProbeControl(t, helper)
	firstWrappers := publicHostWrapperIdentity(t, first)
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
	closeError := first.Close(cleanup)
	cancel()
	if closeError != nil || !first.Trace().Closed {
		t.Fatalf("first real probe did not join: %v %+v", closeError, first.Trace())
	}
	if _, err := os.Stat(first.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first probe root survived joined Close: %v", err)
	}

	second := newPublicHostProbeControl(t, helper)
	secondWrappers := publicHostWrapperIdentity(t, second)
	if first.directory == second.directory || first.nonce == second.nonce || first.listener == second.listener {
		t.Fatal("sequential probes reused authentication state")
	}
	for index, previous := range firstWrappers {
		current := secondWrappers[index]
		if previous.path != current.path {
			t.Fatalf("suite wrapper %s recreated at a new path after first Close: %s -> %s", filepath.Base(previous.path), previous.path, current.path)
		}
		if !os.SameFile(previous.info, current.info) || previous.digest != current.digest {
			t.Fatalf("suite wrapper %s changed inode or bytes between probes", current.path)
		}
		info, err := os.Lstat(previous.path)
		if err != nil || !os.SameFile(previous.info, info) {
			t.Fatalf("first Close retired borrowed suite code: %v", err)
		}
	}
	cleanup, cancel = context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
	closeError = second.Close(cleanup)
	cancel()
	if closeError != nil || !second.Trace().Closed {
		t.Fatalf("second real probe did not join: %v %+v", closeError, second.Trace())
	}
	if err := validatePublicTUIHostHelper(helper); err != nil {
		t.Fatal("selected helper changed during suite reuse control", err)
	}
}

type publicHostWrapperFileIdentity struct {
	path   string
	info   os.FileInfo
	digest [sha256.Size]byte
}

func publicHostWrapperIdentity(t *testing.T, probe *PublicTUIHostProbe) []publicHostWrapperFileIdentity {
	t.Helper()
	environment := probe.Environment([]string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"})
	var directory string
	for _, entry := range environment {
		if value, found := strings.CutPrefix(entry, "PATH="); found {
			directory = strings.SplitN(value, string(os.PathListSeparator), 2)[0]
		}
	}
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		t.Fatalf("probe wrapper directory is not canonical: %q", directory)
	}
	result := make([]publicHostWrapperFileIdentity, 0, 4)
	for _, name := range []string{"ps", darwinMemoryPressureTool, darwinVMStatTool, darwinSysctlTool} {
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o700 {
			t.Fatalf("wrapper is not a nonsymlink 0700 executable: %s %v", path, info.Mode())
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, publicHostWrapperFileIdentity{path: path, info: info, digest: sha256.Sum256(body)})
	}
	return result
}

func TestPublicHostProbeSharedOwnerRetainsLiveBorrowerAndFinalizes(t *testing.T) {
	if runtime.GOOS != darwinPlatform {
		return
	}
	owner := publicHostControlOwner(t)
	helper := PublicTUIHostHelper{Executable: os.Getenv("HIPPO_PUBLIC_HOST_HELPER"), SHA256: os.Getenv("HIPPO_PUBLIC_HOST_HELPER_SHA256")}
	probe := newPublicHostProbeControl(t, helper)
	wrapper := filepath.Join(probe.wrapperDirectory, "ps")
	if err := owner.close(); err == nil {
		t.Fatal("owner removed code with a live unjoined probe")
	}
	if _, err := os.Stat(wrapper); err != nil {
		t.Fatal("failed owner join removed live borrowed code", err)
	}
	if _, err := os.Stat(owner.root); err != nil {
		t.Fatal("failed owner join lost the enclosing suite root", err)
	}
	if _, err := owner.acquire(helper.Executable, publicHostDarwinScripts(helper.Executable)); err == nil {
		t.Fatal("retired owner admitted a new borrower")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
	defer cancel()
	if err := probe.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := probe.Close(ctx); err != nil {
		t.Fatal("idempotent probe Close", err)
	}
	if err := owner.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wrapper); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("final joined owner retained wrapper code: %v", err)
	}
	if err := owner.close(); err != nil {
		t.Fatal("idempotent owner Close", err)
	}
}

func publicHostControlOwner(t *testing.T) *publicTUIHostWrapperOwner {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	owner, err := newPublicTUIHostWrapperOwner(root)
	if err != nil {
		t.Fatal(err)
	}
	previous := publicTUISuiteWrapperOwner
	publicTUISuiteWrapperOwner = owner
	t.Cleanup(func() {
		publicTUISuiteWrapperOwner = previous
		if err := owner.close(); err != nil {
			t.Error(err)
		}
	})
	return owner
}

func TestPublicHostProbeSharedWrapperTamperRefused(t *testing.T) {
	if runtime.GOOS != darwinPlatform {
		return
	}
	owner := publicHostControlOwner(t)
	helper := PublicTUIHostHelper{Executable: os.Getenv("HIPPO_PUBLIC_HOST_HELPER"), SHA256: os.Getenv("HIPPO_PUBLIC_HOST_HELPER_SHA256")}
	first := newPublicHostProbeControl(t, helper)
	root, err := os.OpenRoot(first.wrapperDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error("close owned tamper root", err)
		}
	})
	body, err := root.ReadFile("ps")
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("ps", []byte("changed executable bytes\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.acquire(helper.Executable, publicHostDarwinScripts(helper.Executable)); err == nil {
		t.Fatal("modified shared wrapper was silently accepted or rewritten")
	}
	changed, err := root.ReadFile("ps")
	if err != nil || string(changed) != "changed executable bytes\n" {
		t.Fatalf("refusal rewrote the tampered fixture: %q %v", changed, err)
	}
	if err := root.WriteFile("ps", body, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestPublicHostProbeFreshEnvironmentRejectsCrossProbeNonce(t *testing.T) {
	if runtime.GOOS != darwinPlatform {
		return
	}
	helper := PublicTUIHostHelper{Executable: os.Getenv("HIPPO_PUBLIC_HOST_HELPER"), SHA256: os.Getenv("HIPPO_PUBLIC_HOST_HELPER_SHA256")}
	first := newPublicHostProbeControl(t, helper)
	second := newPublicHostProbeControl(t, helper)
	provided := []string{"PATH=/usr/bin:/bin", publicHostAddressVariable + "=foreign-address", publicHostNonceVariable + "=foreign-nonce", publicHostRootVariable + "=/foreign/root", isolatedRootVariable + "=/private/marker", "KEEP_ME=retained"}
	actual := second.Environment(provided)
	for _, name := range []string{publicHostAddressVariable, publicHostNonceVariable, publicHostRootVariable} {
		matches := 0
		for _, entry := range actual {
			if strings.HasPrefix(entry, name+"=") {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("conflicting inherited IPC key survived: %s %+v", name, actual)
		}
	}
	for _, expected := range []string{publicHostAddressVariable + "=" + second.listener.Addr().String(), publicHostNonceVariable + "=" + second.nonce, publicHostRootVariable + "=" + second.directory, "KEEP_ME=retained"} {
		if !slices.Contains(actual, expected) {
			t.Fatalf("fresh probe IPC value missing: %s", expected)
		}
	}
	request := publicHostProtocolRequest()
	request.Executable, request.Nonce = second.executable, first.nonce
	second.mu.Lock()
	_, _, err := second.acceptDarwinLocked(request)
	second.mu.Unlock()
	if err == nil || second.Trace().Readings != 0 {
		t.Fatal("first probe nonce authenticated to second listener state")
	}
	for _, wrapper := range publicHostWrapperIdentity(t, second) {
		body, err := os.ReadFile(wrapper.path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), first.nonce) || strings.Contains(string(body), second.nonce) || strings.Contains(string(body), second.directory) {
			t.Fatal("shared executable bytes retained per-probe authentication values")
		}
	}
}

func TestPublicHostProbeEnvironmentHonorsPublicPrivacyBoundary(t *testing.T) {
	if runtime.GOOS != darwinPlatform {
		return
	}
	helper := PublicTUIHostHelper{Executable: os.Getenv("HIPPO_PUBLIC_HOST_HELPER"), SHA256: os.Getenv("HIPPO_PUBLIC_HOST_HELPER_SHA256")}
	probe := newPublicHostProbeControl(t, helper)
	environment := probe.Environment([]string{"HIPPO_ROOT=" + probe.directory, "PATH=/usr/bin:/bin"})
	options := PublicTUIStart{Root: probe.directory, Environment: environment}
	if err := validatePublicTUIEnvironment(options); err != nil {
		t.Fatal("actual external probe environment violates public privacy boundary", err)
	}
	for _, name := range []string{isolatedRootVariable, PublicTUIHostProbeHelperEnvironment, "HIPPO_BDD_SUITE"} {
		options.Environment = append(slices.Clone(environment), name+"=private-marker")
		if err := validatePublicTUIEnvironment(options); err == nil {
			t.Fatalf("private helper marker admitted to product subject: %s", name)
		}
	}
}
