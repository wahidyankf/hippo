package support

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support/hostprobe"
)

// PublicTUIHostProbeHelperEnvironment marks only the external collector helper.
const PublicTUIHostProbeHelperEnvironment = hostprobe.HelperEnvironment

// PublicTUIHostProbeHelper names the collector fixture entry, never a TUI subject.
const PublicTUIHostProbeHelper = hostprobe.HelperEntry

// PublicTUIHostReply supplies external collector evidence, not application state.
type PublicTUIHostReply struct {
	CPUPercent    int
	InvalidMemory bool
}

// PublicTUIHostProbeEvent records a real acknowledged probe or FIFO operation.
// PID and ParentPID are unavailable for a FIFO reader and remain zero there.
type PublicTUIHostProbeEvent struct {
	Sequence, Reading  int
	Operation, Phase   string
	RaceOptions        string
	PID, ParentPID     int
	Arguments          []string
	ElapsedNanoseconds int64
}

// PublicTUIHostProbeTrace retains observed protocol events and lifetime state.
type PublicTUIHostProbeTrace struct {
	Events                          []PublicTUIHostProbeEvent
	Readings, Active, MaximumActive int
	Closed                          bool
}

type publicHostGate struct {
	release                chan struct{}
	acknowledged, released bool
}

type publicHostRequest = hostprobe.Request

type publicHostResponse = hostprobe.Response

var publicHostDarwinCommands = hostprobe.Commands()

// PublicTUIHostProbe owns one candidate's external collector fixture.
// Its events describe external reads; an output acknowledgement is not proof
// that production completed its collection, snapshot or rendered frame.
type PublicTUIHostProbe struct {
	mu                           sync.Mutex
	directory, executable, nonce string
	wrapperDirectory             string
	wrapperOwner                 *publicTUIHostWrapperOwner
	wrappers                     *publicTUIHostWrapperSet
	platform                     string
	origin                       time.Time
	reply, readingReply          PublicTUIHostReply
	trace                        PublicTUIHostProbeTrace
	reading, commandIndex        int
	darwinParent                 int
	darwinHelpers                map[int]struct{}
	darwinPending                *publicHostRequest
	gates                        map[int]*publicHostGate
	changed, stopping            chan struct{}
	done                         []chan struct{}
	listener                     net.Listener
	connections                  map[net.Conn]int
	releaseReaders               []*os.File
	fifoPaths                    []string
	closeOnce                    sync.Once
	closeError                   error
	closing                      bool
	cpuBusy, cpuIdle             uint64
}

// PublicTUIHostHelper identifies a separately prepared collector fixture.
// It is never a replacement for the selected HIPPO_BIN product subject.
type PublicTUIHostHelper struct{ Executable, SHA256 string }

// NewPublicTUIHostProbe retains the historical same-executable diagnostic route.
func (driver *Driver) NewPublicTUIHostProbe(ctx context.Context, directory string, reply PublicTUIHostReply) (*PublicTUIHostProbe, error) {
	if err := validatePublicHostPreparation(driver, directory, reply); err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == darwinPlatform {
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return nil, err
		}
	}
	return driver.newPublicTUIHostProbe(ctx, directory, reply, executable)
}

// NewPublicTUIHostProbeWithHelper refuses an unpinned Darwin helper before
// acquiring any directory, listener, wrapper, stop callback or other effect.
// Linux uses its genuine FIFO route and needs no unused Darwin executable.
func (driver *Driver) NewPublicTUIHostProbeWithHelper(ctx context.Context, directory string, reply PublicTUIHostReply, helper PublicTUIHostHelper) (*PublicTUIHostProbe, error) {
	if err := validatePublicHostPreparation(driver, directory, reply); err != nil {
		return nil, err
	}
	if runtime.GOOS == darwinPlatform {
		if err := validatePublicTUIHostHelper(helper); err != nil {
			return nil, err
		}
	}
	return driver.newPublicTUIHostProbe(ctx, directory, reply, helper.Executable)
}

func validatePublicTUIHostHelper(helper PublicTUIHostHelper) error {
	if !filepath.IsAbs(helper.Executable) || filepath.Clean(helper.Executable) != helper.Executable {
		return errors.New("public host helper requires a clean absolute executable path")
	}
	expected, err := hex.DecodeString(helper.SHA256)
	if err != nil || len(expected) != sha256.Size {
		return errors.New("public host helper expected SHA-256 is invalid")
	}
	info, err := os.Lstat(helper.Executable)
	if err != nil {
		return fmt.Errorf("inspect pinned public host helper: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("public host helper must be a regular executable, never a symlink")
	}
	file, err := os.Open(helper.Executable)
	if err != nil {
		return err
	}
	digest := sha256.New()
	_, readError := io.Copy(digest, file)
	if err := errors.Join(readError, file.Close()); err != nil {
		return fmt.Errorf("hash pinned public host helper: %w", err)
	}
	if !slices.Equal(digest.Sum(nil), expected) {
		return errors.New("public host helper SHA-256 does not match selected artifact")
	}
	return nil
}

func (driver *Driver) newPublicTUIHostProbe(ctx context.Context, directory string, reply PublicTUIHostReply, executable string) (*PublicTUIHostProbe, error) {
	if err := publicTUIOwnedDirectory(directory); err != nil {
		return nil, err
	}
	owned, err := os.MkdirTemp(directory, "host-probe-")
	if err != nil {
		return nil, err
	}
	probe := &PublicTUIHostProbe{
		directory: owned, executable: executable, nonce: rand.Text(),
		platform: runtime.GOOS, origin: time.Now(), reply: reply, gates: map[int]*publicHostGate{},
		changed: make(chan struct{}), stopping: make(chan struct{}), connections: map[net.Conn]int{}, cpuBusy: 1000, cpuIdle: 1000,
	}
	driver.stops = append(driver.stops, func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if closeError := probe.Close(cleanup); closeError != nil {
			fmt.Fprintf(os.Stderr, "join owned host probe: %v\n", closeError)
		}
	})
	if err = probe.prepare(); err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		return nil, errors.Join(err, probe.Close(cleanup))
	}
	return probe, nil
}

func validatePublicHostPreparation(driver *Driver, directory string, reply PublicTUIHostReply) error {
	if driver == nil || driver.mode != contract.E2E {
		return errors.New("public host probe requires the E2E boundary")
	}
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || !slices.Contains(driver.temporaryPaths, directory) {
		return errors.New("public host probe requires an owned directory")
	}
	if reply.CPUPercent < 0 || reply.CPUPercent > 100 {
		return errors.New("public host probe CPU percentage is outside bounds")
	}
	if runtime.GOOS != linuxPlatform && runtime.GOOS != darwinPlatform {
		return errors.New("public host probe platform is unsupported")
	}
	return nil
}

func (probe *PublicTUIHostProbe) prepare() error {
	if probe.platform == darwinPlatform {
		return probe.prepareDarwin()
	}
	for name, body := range quietLinuxEvidence {
		if name == "proc/meminfo" {
			continue
		}
		path := filepath.Join(probe.directory, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			return err
		}
	}
	for _, name := range []string{linuxMeminfoName, "stat"} {
		path := filepath.Join(probe.directory, "proc", name)
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			return err
		}
		probe.fifoPaths = append(probe.fifoPaths, path)
		done := make(chan struct{})
		probe.done = append(probe.done, done)
		go probe.serveFIFO(path, name, done)
	}
	return nil
}

// Environment returns this fixture's external probe environment.
func (probe *PublicTUIHostProbe) Environment(environment []string) []string {
	result := make([]string, 0, len(environment)+1)
	path := os.Getenv("PATH")
	for _, entry := range environment {
		if value, found := strings.CutPrefix(entry, "PATH="); found {
			path = value
			continue
		}
		if publicHostProbeEnvironmentKey(entry) {
			continue
		}
		result = append(result, entry)
	}
	if probe.platform == linuxPlatform {
		result = append(result, linuxEvidenceRootEnvironment+"="+probe.directory, "PATH="+path)
	} else {
		result = append(result, "PATH="+probe.wrapperDirectory+string(os.PathListSeparator)+path,
			publicHostAddressVariable+"="+probe.listener.Addr().String(),
			publicHostNonceVariable+"="+probe.nonce, publicHostRootVariable+"="+probe.directory)
	}
	return result
}

func (probe *PublicTUIHostProbe) notifyLocked() {
	close(probe.changed)
	probe.changed = make(chan struct{})
}

func (probe *PublicTUIHostProbe) eventLocked(reading int, operation, phase string, request publicHostRequest) {
	if len(probe.trace.Events) >= 8192 {
		probe.closeError = errors.Join(probe.closeError, errors.New("public host probe event capture exceeds bounds"))
		probe.notifyLocked()
		return
	}
	probe.trace.Events = append(probe.trace.Events, PublicTUIHostProbeEvent{
		Sequence: len(probe.trace.Events) + 1,
		Reading:  reading, Operation: operation, Phase: phase, PID: request.PID, ParentPID: request.ParentPID,
		RaceOptions: request.RaceOptions,
		Arguments:   slices.Clone(request.Arguments), ElapsedNanoseconds: time.Since(probe.origin).Nanoseconds(),
	})
	probe.notifyLocked()
}

func (probe *PublicTUIHostProbe) beginLocked(operation string, request publicHostRequest) (int, *publicHostGate, error) {
	if probe.closing {
		return 0, nil, errors.New("public host probe is closing")
	}
	probe.trace.Readings++
	reading := probe.trace.Readings
	probe.trace.MaximumActive = max(probe.trace.MaximumActive, probe.trace.Active+1)
	if probe.trace.Active != 0 {
		probe.eventLocked(reading, operation, "overlap-refused", request)
		return reading, nil, errors.New("public host readings overlap")
	}
	probe.reading = reading
	probe.readingReply = probe.reply
	probe.trace.Active = 1
	gate := probe.gates[reading]
	if gate != nil {
		gate.acknowledged = true
	}
	probe.eventLocked(reading, operation, tuiStartPhase, request)
	return reading, gate, nil
}

// HoldNextReading gates the next actual reading until its explicit release.
func (probe *PublicTUIHostProbe) HoldNextReading() (int, error) {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	reading := probe.trace.Readings + 1
	if probe.closing || probe.gates[reading] != nil {
		return 0, errors.New("next public host reading cannot be gated")
	}
	probe.gates[reading] = &publicHostGate{release: make(chan struct{})}
	return reading, nil
}

// WaitForReading waits for real read acknowledgement, never a readiness marker.
func (probe *PublicTUIHostProbe) WaitForReading(ctx context.Context, reading int) error {
	for {
		probe.mu.Lock()
		observed := false
		for _, event := range probe.trace.Events {
			observed = observed || event.Reading == reading && event.Phase == tuiStartPhase
		}
		changed, closing := probe.changed, probe.closing
		probe.mu.Unlock()
		if observed {
			return nil
		}
		if reading < 1 || closing {
			return errors.New("public host reading acknowledgement is unavailable")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

// ReleaseReading releases exactly the already acknowledged owned reading.
func (probe *PublicTUIHostProbe) ReleaseReading(reading int) error {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	gate := probe.gates[reading]
	if probe.closing || gate == nil || !gate.acknowledged || gate.released {
		return errors.New("public host reading release is not owned")
	}
	gate.released = true
	close(gate.release)
	return nil
}

// SetReply changes future external replies without changing a product snapshot.
func (probe *PublicTUIHostProbe) SetReply(reply PublicTUIHostReply) error {
	if reply.CPUPercent < 0 || reply.CPUPercent > 100 {
		return errors.New("public host probe CPU percentage is outside bounds")
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.closing {
		return errors.New("public host probe is closing")
	}
	probe.reply = reply
	return nil
}

// Trace returns independently owned event data.
func (probe *PublicTUIHostProbe) Trace() PublicTUIHostProbeTrace {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	result := probe.trace
	result.Events = slices.Clone(result.Events)
	for index := range result.Events {
		result.Events[index].Arguments = slices.Clone(result.Events[index].Arguments)
	}
	return result
}

func (probe *PublicTUIHostProbe) waitGate(gate *publicHostGate) bool {
	if gate == nil {
		return true
	}
	select {
	case <-probe.stopping:
		return false
	case <-gate.release:
		return true
	}
}

func (probe *PublicTUIHostProbe) serveFIFO(path, name string, done chan struct{}) {
	defer close(done)
	for {
		writer, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			probe.mu.Lock()
			closing := probe.closing
			probe.mu.Unlock()
			if !closing {
				probe.retainError(err)
			}
			return
		}
		probe.mu.Lock()
		if probe.closing {
			probe.mu.Unlock()
			probe.retainError(writer.Close())
			return
		}
		reading, gate, beginError := probe.fifoReadingLocked(name)
		probe.mu.Unlock()
		if beginError != nil || !probe.waitGate(gate) {
			probe.retainError(errors.Join(beginError, writer.Close()))
			continue
		}
		probe.mu.Lock()
		if probe.closing {
			probe.mu.Unlock()
			probe.retainError(writer.Close())
			return
		}
		output := probe.linuxReplyLocked(name)
		rotateError := probe.rotateFIFOLocked(path)
		probe.mu.Unlock()
		if rotateError != nil {
			probe.retainError(errors.Join(rotateError, writer.Close()))
			return
		}
		_, writeError := io.WriteString(writer, output)
		probe.mu.Lock()
		phase := "output"
		if writeError != nil {
			phase = "write-error"
		}
		probe.eventLocked(reading, name, phase, publicHostRequest{})
		if name == "stat" {
			probe.trace.Active = 0
			probe.commandIndex = 0
		}
		probe.mu.Unlock()
		probe.retainError(errors.Join(writeError, writer.Close()))
	}
}

// Rotate the pathname while the acknowledged reader keeps the old inode.
// A new open therefore cannot join that old read and append a second reply.
func (probe *PublicTUIHostProbe) rotateFIFOLocked(path string) error {
	if probe.closing {
		return errors.New("public host FIFO is closing")
	}
	next := path + ".next"
	if err := syscall.Mkfifo(next, 0o600); err != nil {
		return err
	}
	if err := os.Rename(next, path); err != nil {
		return errors.Join(err, os.Remove(next))
	}
	return nil
}

func (probe *PublicTUIHostProbe) fifoReadingLocked(name string) (int, *publicHostGate, error) {
	if name == linuxMeminfoName {
		reading, gate, err := probe.beginLocked(name, publicHostRequest{})
		if err == nil {
			probe.commandIndex = 1
		}
		return reading, gate, err
	}
	if probe.trace.Active != 1 || probe.commandIndex != 1 {
		return 0, nil, errors.New("public Linux CPU read is out of order")
	}
	probe.commandIndex = 2
	probe.eventLocked(probe.reading, name, tuiStartPhase, publicHostRequest{})
	return probe.reading, nil, nil
}

func (probe *PublicTUIHostProbe) linuxReplyLocked(name string) string {
	if name == linuxMeminfoName {
		if probe.readingReply.InvalidMemory {
			// The mandatory production parser fails before requesting CPU counters.
			probe.trace.Active = 0
			probe.commandIndex = 0
			return "invalid mandatory memory evidence\n"
		}
		return quietLinuxEvidence["proc/meminfo"]
	}
	cpu := probe.readingReply.CPUPercent
	if cpu < 0 || cpu > 100 {
		return "invalid mandatory CPU evidence\n"
	}
	probe.cpuBusy += uint64(cpu)
	probe.cpuIdle += uint64(100 - cpu)
	return fmt.Sprintf("cpu  %d 0 0 %d 0 0 0 0 0 0\n", probe.cpuBusy, probe.cpuIdle)
}

func (probe *PublicTUIHostProbe) retainError(err error) {
	if err == nil {
		return
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	probe.closeError = errors.Join(probe.closeError, err)
}

func publicHostShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (probe *PublicTUIHostProbe) prepareDarwin() error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	probe.listener = listener
	done := make(chan struct{})
	probe.done = append(probe.done, done)
	go probe.serveDarwin(done)
	scripts := publicHostDarwinScripts(probe.executable)
	probe.wrapperDirectory = probe.directory
	if publicTUISuiteWrapperOwner != nil {
		set, acquireError := publicTUISuiteWrapperOwner.acquire(probe.executable, scripts)
		if acquireError != nil {
			return acquireError
		}
		probe.wrapperOwner, probe.wrappers = publicTUISuiteWrapperOwner, set
		probe.wrapperDirectory = set.directory
		return nil
	}
	return writePublicHostWrappers(probe.directory, scripts)
}

func publicHostDarwinScripts(executable string) map[string]string {
	tools := map[string]string{"ps": "/bin/ps", darwinMemoryPressureTool: "/usr/bin/memory_pressure", darwinVMStatTool: "/usr/bin/vm_stat", darwinSysctlTool: "/usr/sbin/sysctl"}
	result := make(map[string]string, len(tools))
	for name, actual := range tools {
		var matches []string
		for _, command := range publicHostDarwinCommands {
			if command[0] != name {
				continue
			}
			terms := []string{fmt.Sprintf("[ \"$#\" -eq %d ]", len(command)-1)}
			for index, argument := range command[1:] {
				terms = append(terms, fmt.Sprintf("[ \"$%d\" = %s ]", index+1, publicHostShellQuote(argument)))
			}
			matches = append(matches, "( "+strings.Join(terms, " && ")+" )")
		}
		// Keep every inherited reporting flag and override only the marked
		// helper's exit delay before race runtime initialization. The pinned
		// sanitizer parser handles repeated options left to right, last wins.
		body := "#!/bin/sh\nset -eu\nif " + strings.Join(matches, " || ") + "; then\n" +
			"GORACE=\"${GORACE:+$GORACE }atexit_sleep_ms=0\" " +
			PublicTUIHostProbeHelperEnvironment + "=1 " + publicHostAddressVariable + "=\"${" + publicHostAddressVariable + ":?}\" " +
			publicHostNonceVariable + "=\"${" + publicHostNonceVariable + ":?}\" " + isolatedRootVariable + "=\"${" + publicHostRootVariable + ":?}\"" +
			" HIPPO_BDD_SUITE=unit exec " + publicHostShellQuote(executable) + " -test.run=" + publicHostShellQuote("^"+PublicTUIHostProbeHelper+"$") +
			" -test.count=1 -test.timeout=10s -- " + publicHostShellQuote(name) + " \"$@\"\nfi\nexec " + publicHostShellQuote(actual) + " \"$@\"\n"
		result[name] = body
	}
	return result
}

func (probe *PublicTUIHostProbe) serveDarwin(done chan struct{}) {
	var children sync.WaitGroup
	defer func() { children.Wait(); close(done) }()
	for {
		connection, err := probe.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				probe.retainError(err)
			}
			return
		}
		probe.mu.Lock()
		if probe.closing {
			probe.mu.Unlock()
			probe.retainError(publicHostCloseConnection(connection))
			continue
		}
		probe.connections[connection] = 0
		probe.mu.Unlock()
		children.Go(func() { probe.handleDarwin(connection) })
	}
}

func publicHostCommandIndex(request publicHostRequest) int {
	return hostprobe.CommandIndex(request)
}

func (probe *PublicTUIHostProbe) acceptDarwinLocked(request publicHostRequest) (int, *publicHostGate, error) {
	if request.Nonce != probe.nonce || request.Executable != probe.executable || request.PID < 1 || request.ParentPID < 1 {
		return 0, nil, errors.New("public host probe identity is not owned")
	}
	index := publicHostCommandIndex(request)
	if index < 0 {
		return 0, nil, errors.New("public host probe argv is not a production collector query")
	}
	if index == 0 {
		reading, gate, err := probe.beginLocked(request.Name, request)
		if err == nil {
			probe.commandIndex = 0
			probe.darwinParent = request.ParentPID
			probe.darwinHelpers = map[int]struct{}{}
			probe.retainDarwinRequestLocked(request)
		}
		return reading, gate, err
	}
	if probe.trace.Active != 1 || index != probe.commandIndex {
		return 0, nil, errors.New("public host probe command is out of order")
	}
	if request.ParentPID != probe.darwinParent || probe.darwinPending != nil {
		return 0, nil, errors.New("public host probe continuation is not owned")
	}
	probe.retainDarwinRequestLocked(request)
	probe.eventLocked(probe.reading, request.Name, tuiStartPhase, request)
	return probe.reading, nil, nil
}

func (probe *PublicTUIHostProbe) retainDarwinRequestLocked(request publicHostRequest) {
	request.Arguments = slices.Clone(request.Arguments)
	probe.darwinHelpers[request.PID] = struct{}{}
	probe.darwinPending = &request
}

func (probe *PublicTUIHostProbe) handleDarwin(connection net.Conn) {
	defer func() {
		probe.retainError(publicHostCloseConnection(connection))
		probe.mu.Lock()
		delete(probe.connections, connection)
		probe.mu.Unlock()
	}()
	if err := connection.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		probe.retainError(err)
		return
	}
	decoder := json.NewDecoder(io.LimitReader(connection, 8192))
	decoder.DisallowUnknownFields()
	var request publicHostRequest
	if err := decoder.Decode(&request); err != nil {
		probe.retainError(err)
		return
	}
	probe.mu.Lock()
	reading, gate, err := probe.acceptDarwinLocked(request)
	if err == nil {
		probe.connections[connection] = request.PID
	}
	probe.mu.Unlock()
	encoder := json.NewEncoder(connection)
	if err != nil {
		probe.retainError(encoder.Encode(publicHostResponse{Error: err.Error()}))
		return
	}
	acknowledgement := make(chan error, 1)
	acknowledgementDone := make(chan struct{})
	defer func() { <-acknowledgementDone }()
	go func() {
		defer close(acknowledgementDone)
		var message string
		decodeError := decoder.Decode(&message)
		if decodeError == nil && message != "output-written" {
			decodeError = errors.New("public host probe output acknowledgement is invalid")
		}
		acknowledgement <- decodeError
	}()
	if !probe.darwinGate(gate, acknowledgement, reading, request) {
		return
	}
	probe.mu.Lock()
	output := probe.darwinReplyLocked(request)
	probe.mu.Unlock()
	if err = encoder.Encode(publicHostResponse{Output: output}); err != nil {
		probe.retainError(publicHostCloseConnection(connection))
		<-acknowledgement
		probe.completeDarwin(reading, request, "write-error")
		probe.retainError(err)
		return
	}
	err = <-acknowledgement
	if err != nil {
		phase := "read-error"
		if errors.Is(err, io.EOF) {
			phase = tuiCanceledEOFPhase
		} else {
			probe.retainError(err)
		}
		probe.completeDarwin(reading, request, phase)
		return
	}
	probe.completeDarwin(reading, request, "output-ack")
	probe.retainError(encoder.Encode("committed"))
}

func (probe *PublicTUIHostProbe) darwinGate(gate *publicHostGate, acknowledgement <-chan error, reading int, request publicHostRequest) bool {
	if gate == nil {
		return true
	}
	select {
	case <-probe.stopping:
		err := <-acknowledgement
		phase := "teardown-eof"
		if errors.Is(err, io.EOF) {
			phase = tuiCanceledEOFPhase
		}
		probe.completeDarwin(reading, request, phase)
		return false
	case err := <-acknowledgement:
		phase := "protocol-error"
		if errors.Is(err, io.EOF) {
			phase = tuiCanceledEOFPhase
		} else {
			probe.retainError(err)
		}
		probe.completeDarwin(reading, request, phase)
		return false
	case <-gate.release:
		return true
	}
}

func (probe *PublicTUIHostProbe) completeDarwin(reading int, request publicHostRequest, phase string) {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if reading != probe.reading || probe.darwinPending == nil || !publicHostSameRequest(*probe.darwinPending, request) {
		return
	}
	probe.darwinPending = nil
	probe.eventLocked(reading, request.Name, phase, request)
	if phase != "output-ack" || probe.commandIndex == len(publicHostDarwinCommands)-1 {
		probe.trace.Active = 0
		probe.commandIndex = 0
	} else {
		probe.commandIndex++
	}
}

func publicHostSameRequest(first, second publicHostRequest) bool {
	return first.Nonce == second.Nonce && first.Executable == second.Executable &&
		first.PID == second.PID && first.ParentPID == second.ParentPID && first.Name == second.Name &&
		first.RaceOptions == second.RaceOptions && slices.Equal(first.Arguments, second.Arguments)
}

// RetireDarwinReading records an incomplete cohort only after the peer observes
// its original parent and every accepted helper absent. The controller must
// separately qualify the exact parent's real Wait before requesting retirement.
func (probe *PublicTUIHostProbe) RetireDarwinReading(ctx context.Context, reading, parentPID int) error {
	return probe.retireDarwinReading(ctx, reading, parentPID, func(pid int) error { return syscall.Kill(pid, 0) })
}

func (probe *PublicTUIHostProbe) retireDarwinReading(ctx context.Context, reading, parentPID int, inspect func(int) error) error {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if probe.platform != darwinPlatform || probe.closing || probe.trace.Active != 1 || reading != probe.reading ||
		parentPID < 1 || parentPID != probe.darwinParent || len(probe.darwinHelpers) == 0 || probe.darwinPending != nil ||
		len(probe.trace.Events) >= 8192 {
		return errors.New("incomplete public host cohort retirement is not owned")
	}
	for _, pid := range probe.connections {
		if _, accepted := probe.darwinHelpers[pid]; accepted {
			return errors.New("public host cohort handler is still pending")
		}
	}
	helpers := slices.Sorted(maps.Keys(probe.darwinHelpers))
	for _, pid := range append([]int{parentPID}, helpers...) {
		err := inspect(pid)
		if err == nil {
			return fmt.Errorf("public host cohort PID %d is still live", pid)
		}
		if !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("public host cohort PID %d absence is unproven: %w", pid, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	probe.eventLocked(reading, "cohort", "retired-incomplete", publicHostRequest{ParentPID: parentPID})
	probe.trace.Active = 0
	probe.commandIndex = 0
	return nil
}

func (probe *PublicTUIHostProbe) darwinReplyLocked(request publicHostRequest) string {
	switch request.Name {
	case "ps":
		return strconv.Itoa(probe.readingReply.CPUPercent*runtime.NumCPU()) + ".0\n"
	case darwinMemoryPressureTool:
		if probe.readingReply.InvalidMemory {
			return "invalid mandatory memory estimate\n"
		}
		return "System-wide memory free percentage: 80%\n"
	case darwinVMStatTool:
		return "Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages stored in compressor: 0.\nPages occupied by compressor: 0.\nSwapins: 0.\nSwapouts: 0.\n"
	case darwinSysctlTool:
		switch request.Arguments[1] {
		case "vm.swapusage":
			return "total = 1024.00M  used = 0.00M  free = 1024.00M  (encrypted)\n"
		case "kern.memorystatus_vm_pressure_level", "vm.compressor_available":
			return "1\n"
		case "vm.compressor_bytes_used":
			return "0\n"
		}
	}
	return ""
}

func publicHostCloseConnection(connection net.Conn) error {
	err := connection.Close()
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (probe *PublicTUIHostProbe) stopOwnedEffects() {
	probe.mu.Lock()
	probe.closing = true
	close(probe.stopping)
	probe.notifyLocked()
	connections := make(map[net.Conn]int, len(probe.connections))
	maps.Copy(connections, probe.connections)
	probe.mu.Unlock()
	if probe.listener != nil {
		probe.retainError(probe.listener.Close())
	}
	for connection, pid := range connections {
		// An already reaped helper must yield its actual kernel EOF. Closing the
		// server's endpoint first would replace that cancellation witness.
		if pid > 0 && errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			continue
		}
		probe.retainError(publicHostCloseConnection(connection))
	}
	for _, path := range probe.fifoPaths {
		release, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			probe.retainError(err)
			continue
		}
		probe.releaseReaders = append(probe.releaseReaders, release)
		probe.retainError(os.Remove(path))
	}
}

// Close joins owned probe effects within the caller's cleanup bound.
func (probe *PublicTUIHostProbe) Close(ctx context.Context) error {
	probe.closeOnce.Do(probe.stopOwnedEffects)
	for _, done := range probe.done {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if !probe.trace.Closed {
		for _, reader := range probe.releaseReaders {
			probe.closeError = errors.Join(probe.closeError, reader.Close())
		}
		probe.closeError = errors.Join(probe.closeError, os.RemoveAll(probe.directory))
		probe.trace.Closed = true
	}
	if probe.trace.Closed && probe.closeError == nil && probe.wrappers != nil {
		probe.wrapperOwner.release(probe.wrappers)
		probe.wrappers = nil
	}
	return probe.closeError
}

// RunPublicTUIHostProbeHelper preserves the historical same-executable route.
func RunPublicTUIHostProbeHelper(ctx context.Context) error {
	return hostprobe.Run(ctx)
}
