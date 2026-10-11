package support

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/tests/contract"
	"golang.org/x/sys/unix"
)

// NativeTUITeaHelper is the sole selected integration helper in this increment.
const NativeTUITeaHelper = "TestBDDNativeTeaTransportHelper"

const nativeTUICaptureLimit = 8 * 1024 * 1024

type nativeTUIWindowSize = unix.Winsize

func nativeTUISignedDevice(device int32) uint64 {
	// Keep the low 31 bits in their proven nonnegative range, then explicitly
	// extend the sign bit through the uint64 identity, matching the original cast.
	lowBits := uint64(device & math.MaxInt32)
	if device < 0 {
		return lowBits | ^uint64(math.MaxInt32)
	}
	return lowBits
}

func nativeTUIDescriptor(descriptor uintptr) (int, error) {
	if descriptor > math.MaxInt {
		return 0, errors.New("native TUI descriptor exceeds bounds")
	}
	return int(descriptor), nil
}

func nativeTUISize(width, height int) (nativeTUIWindowSize, error) {
	if width < 0 || width > math.MaxUint16 || height < 0 || height > math.MaxUint16 {
		return nativeTUIWindowSize{}, errors.New("native TUI window size exceeds bounds")
	}
	return nativeTUIWindowSize{Col: uint16(width), Row: uint16(height)}, nil
}

func setNativeTUISize(descriptor uintptr, width, height int) error {
	value, err := nativeTUISize(width, height)
	if err != nil {
		return err
	}
	descriptorNumber, err := nativeTUIDescriptor(descriptor)
	if err != nil {
		return err
	}
	return unix.IoctlSetWinsize(descriptorNumber, unix.TIOCSWINSZ, &value)
}

func getNativeTUISize(descriptor uintptr) (nativeTUIWindowSize, error) {
	descriptorNumber, err := nativeTUIDescriptor(descriptor)
	if err != nil {
		return nativeTUIWindowSize{}, err
	}
	size, err := unix.IoctlGetWinsize(descriptorNumber, unix.TIOCGWINSZ)
	if err != nil {
		return nativeTUIWindowSize{}, err
	}
	if size == nil {
		return nativeTUIWindowSize{}, errors.New("native TUI window size unavailable")
	}
	return *size, nil
}

func nativeTUIForeground(descriptor uintptr) (int, error) {
	descriptorNumber, err := nativeTUIDescriptor(descriptor)
	if err != nil {
		return 0, err
	}
	return unix.IoctlGetInt(descriptorNumber, unix.TIOCGPGRP)
}

// NativeTUIStart selects the current test executable, never a public candidate.
// Callbacks belong to the TUI-local decoder; support does not interpret output.
type NativeTUIStart struct {
	Mode, Executable, Helper string
	Environment              []string
	Width, Height            int
	Feed                     func([]byte) error
	ResizeScreen             func(int, int)
	Finish                   func() error
}

// NativeTUIIdentity records the exact child, controlling slave and executable.
type NativeTUIIdentity struct {
	PID        int    `json:"PID"`
	Session    int    `json:"Session"`
	Group      int    `json:"Group"`
	Device     uint64 `json:"Device"`
	Inode      uint64 `json:"Inode"`
	Executable string `json:"Executable"`
	SHA256     string `json:"SHA256"`
	Nonce      string `json:"Nonce"`
}

// NativeTUIReady is produced inside the child from its own controlling slave.
type NativeTUIReady struct {
	Identity   NativeTUIIdentity `json:"Identity"`
	Foreground int               `json:"Foreground"`
	Helper     string            `json:"Helper"`
	Width      int               `json:"Width"`
	Height     int               `json:"Height"`
	RawInput   bool              `json:"RawInput"`
}

// NativeTUIResult keeps output channels separate and exposes unavailable joins.
type NativeTUIResult struct {
	Identity NativeTUIIdentity

	Stdout, Stderr                        []byte
	ExitCode                              int
	Reaped, OutputEOF, ErrorEOF, ReadyEOF bool
}

// Joined requires process reaping and actual completion of every owned reader.
func (result NativeTUIResult) Joined() bool {
	return result.Reaped && result.OutputEOF && result.ErrorEOF && result.ReadyEOF
}

// NativeTUITransport owns one session child, master, and three joined readers.
// Its lock orders the sole output producer against native resize and snapshots.
type NativeTUITransport struct {
	mu                                sync.Mutex
	command                           *exec.Cmd
	master, readyReader, stderrReader *os.File
	identity                          NativeTUIIdentity

	ready                                           NativeTUIReady
	width, height                                   int
	feed                                            func([]byte) error
	resizeScreen                                    func(int, int)
	finish                                          func() error
	closeDescriptors                                func() error
	stdout, stderr                                  []byte
	readyDone, outputDone, errorDone, waitDone      chan struct{}
	readyError, outputError, errorError, waitError  error
	readyEOF, outputEOF, errorEOF, reaped, accepted bool
	exitCode                                        int
}

func validateNativeTUIStart(options NativeTUIStart) error {
	if options.Mode != contract.Integration {
		return errors.New("native TUI helper requires integration mode")
	}
	if options.Executable != os.Args[0] || !filepath.IsAbs(options.Executable) {
		return errors.New("native TUI helper refuses a foreign candidate")
	}
	if options.Helper != NativeTUITeaHelper {
		return errors.New("native TUI helper selection is not owned")
	}
	if options.Width < 1 || options.Width > 1024 || options.Height < 1 || options.Height > 256 {
		return errors.New("native TUI viewport is unknown or unsupported")
	}
	if options.Feed == nil || options.ResizeScreen == nil || options.Finish == nil {
		return errors.New("native TUI decoder callbacks are incomplete")
	}
	return nil
}

// StartNativeTUI starts exactly the selected test helper with a controlling slave.
// The returned transport refuses input, resize and signal until AwaitReady passes.
func StartNativeTUI(ctx context.Context, options NativeTUIStart) (*NativeTUITransport, error) {
	if err := validateNativeTUIStart(options); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	executable, err := os.ReadFile(options.Executable)
	if err != nil {
		return nil, fmt.Errorf("read native helper identity: %w", err)
	}
	digest := sha256.Sum256(executable)
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("create native helper nonce: %w", err)
	}
	master, slave, err := openNativeTUIPTY()
	if err != nil {
		return nil, err
	}
	opened := []*os.File{master, slave}
	closeOpened := func(cause error) (*NativeTUITransport, error) {
		for _, file := range opened {
			cause = errors.Join(cause, file.Close())
		}
		return nil, cause
	}
	if err := setNativeTUISize(master.Fd(), options.Width, options.Height); err != nil {
		return closeOpened(err)
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(slave.Fd()), &stat); err != nil {
		return closeOpened(err)
	}
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		return closeOpened(err)
	}
	opened = append(opened, readyReader, readyWriter)
	errorReader, errorWriter, err := os.Pipe()
	if err != nil {
		return closeOpened(err)
	}
	opened = append(opened, errorReader, errorWriter)
	identity := NativeTUIIdentity{Device: nativeTUIDevice(stat), Inode: stat.Ino, Executable: options.Executable, SHA256: hex.EncodeToString(digest[:]), Nonce: hex.EncodeToString(nonce)}
	command := exec.Command(options.Executable, "-test.run=^"+options.Helper+"$", "-test.count=1", "-test.timeout=20s")
	command.Env = nativeTUIEnvironment(options.Environment, identity.Nonce)
	// The fixed test child can reopen only this newly allocated slave; it must
	// still compare the reopened device and terminal state with its owned fd0.
	command.Env = append(command.Env, "HIPPO_TEST_NATIVE_TUI_SLAVE="+slave.Name())
	command.Stdin, command.Stdout, command.Stderr = slave, slave, errorWriter
	command.ExtraFiles = []*os.File{readyWriter}
	// Ctty is child descriptor zero, after Stdin is installed. Setsid and
	// Setctty make this exact child the new session and foreground owner.
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := command.Start(); err != nil {
		return closeOpened(err)
	}
	identity.PID, identity.Session, identity.Group = command.Process.Pid, command.Process.Pid, command.Process.Pid
	transport := &NativeTUITransport{command: command, master: master, readyReader: readyReader, stderrReader: errorReader, identity: identity, width: options.Width, height: options.Height, feed: options.Feed, resizeScreen: options.ResizeScreen, finish: options.Finish, readyDone: make(chan struct{}), outputDone: make(chan struct{}), errorDone: make(chan struct{}), waitDone: make(chan struct{}), exitCode: -1}
	// Keep descriptor closure connected to the actual files while allowing
	// pure lifetime controls to publish a failure at the forced-close boundary.
	transport.closeDescriptors = func() error {
		return errors.Join(master.Close(), readyReader.Close(), errorReader.Close())
	}
	// Parent slave/writer copies must close before any EOF can be observed.
	closeError := errors.Join(slave.Close(), readyWriter.Close(), errorWriter.Close())
	go transport.readReady()
	go transport.readOutput()
	go transport.readErrors()
	go transport.waitChild()
	if closeError != nil {
		cleanupContext, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return nil, errors.Join(closeError, transport.Close(cleanupContext))
	}
	return transport, nil
}

func nativeTUIEnvironment(environment []string, nonce string) []string {
	result := make([]string, 0, len(environment)+4)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if name != bddSuiteVariable && name != "HIPPO_TEST_NATIVE_TUI_NONCE" && name != "HIPPO_TEST_NATIVE_TUI_SLAVE" && name != "TERM" {
			result = append(result, entry)
		}
	}
	return append(result, bddSuiteVariable+"="+contract.Integration, "HIPPO_TEST_NATIVE_TUI_NONCE="+nonce, "TERM=xterm-ghostty")
}

func validateNativeTUIReady(expected NativeTUIIdentity, ready NativeTUIReady, width, height int) error {
	if ready.Identity != expected || expected.PID <= 0 || expected.Session != expected.PID || expected.Group != expected.PID || ready.Foreground != expected.PID || ready.Helper != NativeTUITeaHelper || ready.Width != width || ready.Height != height || !ready.RawInput {
		return errors.New("native TUI readiness does not match the owned child and slave")
	}
	return nil
}

func (transport *NativeTUITransport) readReady() {
	defer close(transport.readyDone)
	content, err := io.ReadAll(io.LimitReader(transport.readyReader, 4097))
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if err != nil {
		transport.readyError = err
		return
	}
	if len(content) > 4096 {
		transport.readyError = errors.New("native TUI readiness exceeds bounds")
		return
	}
	transport.readyEOF = true
	if err := json.Unmarshal(content, &transport.ready); err != nil {
		transport.readyError = fmt.Errorf("decode native TUI readiness: %w", err)
		return
	}
	transport.readyError = validateNativeTUIReady(transport.identity, transport.ready, transport.width, transport.height)
}

// AwaitReady waits for EOF of the dedicated report and checks live ownership.
func (transport *NativeTUITransport) AwaitReady(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-transport.readyDone:
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.readyError != nil {
		return transport.readyError
	}
	if err := transport.ownedChild(); err != nil {
		return err
	}
	transport.accepted = true
	return nil
}

func (transport *NativeTUITransport) ownedChild() error {
	if transport.command == nil || transport.command.Process == nil || transport.reaped || transport.identity.PID != transport.command.Process.Pid {
		return errors.New("native TUI owned child is unavailable")
	}
	session, err := nativeTUISession(transport.identity.PID)
	if err != nil {
		return fmt.Errorf("query owned TUI session: %w", err)
	}
	group, err := syscall.Getpgid(transport.identity.PID)
	if err != nil {
		return fmt.Errorf("query owned TUI process group: %w", err)
	}
	if session != transport.identity.Session || group != transport.identity.Group {
		return errors.New("native TUI child lost its owned session")
	}
	return nil
}

// SendKey sends public input bytes only after readiness, with a finite deadline.
func (transport *NativeTUITransport) SendKey(ctx context.Context, key []byte) error {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if !transport.accepted {
		return errors.New("native TUI input precedes readiness")
	}
	if err := transport.ownedChild(); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return errors.New("native TUI input requires a finite deadline")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(key) == 0 || len(key) > 64 {
		return errors.New("native TUI key bytes exceed bounds")
	}
	if err := transport.master.SetWriteDeadline(deadline); err != nil {
		return err
	}
	n, err := transport.master.Write(key)
	if err == nil && n != len(key) {
		err = io.ErrShortWrite
	}
	return err
}

// Resize changes the actual owned terminal size and signals only its child.
func (transport *NativeTUITransport) Resize(ctx context.Context, width, height int) error {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if !transport.accepted {
		return errors.New("native TUI resize precedes readiness")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if width < 1 || width > 1024 || height < 1 || height > 256 {
		return errors.New("native TUI resize has unknown size")
	}
	if err := transport.ownedChild(); err != nil {
		return err
	}
	if err := setNativeTUISize(transport.master.Fd(), width, height); err != nil {
		return err
	}
	transport.resizeScreen(width, height)
	transport.width, transport.height = width, height
	return transport.command.Process.Signal(syscall.SIGWINCH)
}

// Signal refuses every target except the currently validated owned child PID.
func (transport *NativeTUITransport) Signal(pid int, signal os.Signal) error {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if !transport.accepted || pid != transport.identity.PID {
		return errors.New("native TUI signal target is not owned and ready")
	}
	if err := transport.ownedChild(); err != nil {
		return err
	}
	nativeSignal, ok := signal.(syscall.Signal)
	if !ok || nativeSignal != syscall.SIGINT && nativeSignal != syscall.SIGTERM {
		return errors.New("native TUI signal is unsupported")
	}
	return transport.command.Process.Signal(signal)
}

func appendNativeTUICapture(capture, chunk []byte) ([]byte, error) {
	remaining := nativeTUICaptureLimit - len(capture)
	if len(chunk) > remaining {
		return append(capture, chunk[:remaining]...), errors.New("native TUI capture exceeds bounds")
	}
	return append(capture, chunk...), nil
}

func (transport *NativeTUITransport) readOutput() {
	defer close(transport.outputDone)
	buffer := make([]byte, 4096)
	for {
		readCount, err := transport.master.Read(buffer)
		transport.mu.Lock()
		if readCount > 0 {
			var captureError error
			transport.stdout, captureError = appendNativeTUICapture(transport.stdout, buffer[:readCount])
			if transport.outputError == nil {
				transport.outputError = captureError
			}
			if transport.outputError == nil {
				transport.outputError = transport.feed(buffer[:readCount])
			}
		}
		if err != nil {
			// A PTY master reports EIO when its last slave closes (Linux, and
			// some Darwin versions). This is actual slave hangup, not reaping.
			if errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) {
				transport.outputEOF = true
				transport.outputError = errors.Join(transport.outputError, transport.finish())
			} else {
				transport.outputError = errors.Join(transport.outputError, err)
			}
			transport.mu.Unlock()
			return
		}
		transport.mu.Unlock()
	}
}

func (transport *NativeTUITransport) readErrors() {
	defer close(transport.errorDone)
	buffer := make([]byte, 4096)
	for {
		n, err := transport.stderrReader.Read(buffer)
		transport.mu.Lock()
		if n > 0 {
			var captureError error
			transport.stderr, captureError = appendNativeTUICapture(transport.stderr, buffer[:n])
			if transport.errorError == nil {
				transport.errorError = captureError
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				transport.errorEOF = true
			} else {
				transport.errorError = errors.Join(transport.errorError, err)
			}
			transport.mu.Unlock()
			return
		}
		transport.mu.Unlock()
	}
}

func (transport *NativeTUITransport) waitChild() {
	defer close(transport.waitDone)
	err := transport.command.Wait()
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.waitError, transport.reaped = err, true
	if transport.command.ProcessState != nil {
		transport.exitCode = transport.command.ProcessState.ExitCode()
	}
}

// OutputError returns the first decoder or drain failure before any predicate.
func (transport *NativeTUITransport) OutputError() error {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transport.outputError
}

// Result owns its raw byte slices; it cannot become joined merely by reaping.
func (transport *NativeTUITransport) Result() NativeTUIResult {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return NativeTUIResult{Identity: transport.identity, Stdout: slices.Clone(transport.stdout), Stderr: slices.Clone(transport.stderr), ExitCode: transport.exitCode, Reaped: transport.reaped, OutputEOF: transport.outputEOF, ErrorEOF: transport.errorEOF, ReadyEOF: transport.readyEOF}
}

// WaitAndDrain waits for all owned goroutines, preserving missing EOF evidence.
func (transport *NativeTUITransport) WaitAndDrain(ctx context.Context) (NativeTUIResult, error) {
	for _, done := range []chan struct{}{transport.waitDone, transport.outputDone, transport.errorDone, transport.readyDone} {
		select {
		case <-ctx.Done():
			return transport.Result(), errors.Join(transport.latchedError(), ctx.Err())
		case <-done:
		}
	}
	return transport.Result(), transport.latchedError()
}

func (transport *NativeTUITransport) latchedError() error {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return errors.Join(transport.waitError, transport.outputError, transport.errorError, transport.readyError)
}

// Close kills only the recorded session child when needed, joins readers, and
// closes descriptors. Forced descriptor closure never fabricates EOF evidence.
func (transport *NativeTUITransport) Close(ctx context.Context) error {
	transport.mu.Lock()
	var killError error
	if transport.command != nil && !transport.reaped {
		if err := transport.ownedChild(); err != nil {
			killError = err
		} else {
			killError = transport.command.Process.Kill()
		}
	}
	transport.mu.Unlock()
	_, waitError := transport.WaitAndDrain(ctx)
	closeError := transport.closeDescriptors()
	// Forced closure releases pollable readers after timeout. A separate finite
	// join window records failure instead of blocking forever or claiming EOF.
	joinContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	for _, done := range []chan struct{}{transport.outputDone, transport.errorDone, transport.readyDone, transport.waitDone} {
		select {
		case <-done:
		case <-joinContext.Done():
			return errors.Join(killError, waitError, closeError, transport.latchedError(), joinContext.Err())
		}
	}
	return errors.Join(killError, waitError, closeError, transport.latchedError())
}

// ReportNativeTUIReady queries only the caller's own terminal and session.
// The helper invokes it from the first real observation request after Tea starts.
func ReportNativeTUIReady(input, report *os.File) error {
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(input.Fd()), &stat); err != nil {
		return err
	}
	session, err := nativeTUISession(0)
	if err != nil {
		return err
	}
	group, err := syscall.Getpgid(0)
	if err != nil {
		return err
	}
	foreground, err := nativeTUIForeground(input.Fd())
	if err != nil {
		return err
	}
	size, err := getNativeTUISize(input.Fd())
	if err != nil {
		return err
	}
	raw, err := nativeTUIRawInput(int(input.Fd()))
	if err != nil {
		return err
	}
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}
	// Hash the running image selected by the OS while retaining the original
	// launch pathname in the reported identity checked by the parent.
	executable, err := os.ReadFile(executablePath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(executable)
	identity := NativeTUIIdentity{PID: os.Getpid(), Session: session, Group: group, Device: nativeTUIDevice(stat), Inode: stat.Ino, Executable: os.Args[0], SHA256: hex.EncodeToString(digest[:]), Nonce: os.Getenv("HIPPO_TEST_NATIVE_TUI_NONCE")}
	ready := NativeTUIReady{Identity: identity, Foreground: foreground, Helper: NativeTUITeaHelper, Width: int(size.Col), Height: int(size.Row), RawInput: raw}
	if identity.Nonce == "" || session != identity.PID || group != identity.PID || foreground != identity.PID || !raw {
		return errors.New("helper has no owned raw controlling foreground terminal")
	}
	return errors.Join(json.NewEncoder(report).Encode(ready), report.Close())
}

// PublicTUIStart selects only this driver's unchanged compiled E2E candidate.
// Preparation is explicit; neither a constructor nor binding discovery launches it.
type PublicTUIStart struct {
	Driver                 *Driver
	Mode, Executable       string
	Root, Directory        string
	Arguments, Environment []string
	Width, Height          int
	Feed                   func([]byte) error
	ResizeScreen           func(int, int)
	Finish                 func() error
}

// PublicTUITerminalState is an actual native observation, not a helper report.
type PublicTUITerminalState struct {
	Mode       unix.Termios
	Foreground int
	RawInput   bool
}

// PublicTUIIdentity identifies the launched public process and its owned slave.
type PublicTUIIdentity struct {
	NativeTUIIdentity

	Slave, Root, Version, Commit string
	Arguments                    []string
	UID                          uint32
}

// PublicTUIHolderIdentity names the persistent session/foreground owner.
type PublicTUIHolderIdentity struct {
	PID, Session, Group              int
	Device, Inode                    uint64
	UID                              uint32
	Executable, SHA256, Slave, Nonce string
}

// PublicTUIWaitFacts retain the candidate's actual WaitStatus separately.
type PublicTUIWaitFacts struct {
	PID, ExitCode, Signal    int
	RawStatus                int64
	Exited, Signaled, Forced bool
}

// PublicTUINativeFailure identifies an unavailable native getter without zero facts.
type PublicTUINativeFailure struct {
	Phase, Operation, Detail string
	Descriptor, Errno        int
}

// PublicTUIResult distinguishes candidate, holder, protocol and stream lifetimes.
type PublicTUIResult struct {
	Identity                                                                                                 PublicTUIIdentity
	Holder                                                                                                   PublicTUIHolderIdentity
	CandidateWait                                                                                            PublicTUIWaitFacts
	Stdout, Stderr, HolderStderr, Reports, Controls                                                          []byte
	HolderSignals                                                                                            []string
	ExitCode, HolderExitCode                                                                                 int
	Reaped, OutputEOF, ErrorEOF, ReadersJoined, Forced                                                       bool
	HolderReaped, HolderWaitJoined, HolderErrorEOF, HolderErrorJoined, ReportEOF, ReportJoined, HolderForced bool
	TerminalAllocation, TerminalBefore, TerminalReady, TerminalAfter                                         PublicTUITerminalState
	TerminalReadyAvailable, TerminalAfterAvailable, HolderAliveAtPostChild                                   bool
	PostChildAcknowledged                                                                                    bool
	NativeFailure                                                                                            *PublicTUINativeFailure
}

// Joined requires candidate reap, holder reap and every actual reader EOF/join.
func (result PublicTUIResult) Joined() bool {
	return result.Reaped && result.OutputEOF && result.ErrorEOF && result.ReadersJoined && result.HolderReaped && result.HolderWaitJoined && result.HolderErrorEOF && result.HolderErrorJoined && result.ReportEOF && result.ReportJoined
}

// NaturalExit refers only to the actual candidate, never the holder's status.
func (result PublicTUIResult) NaturalExit() bool {
	return result.Reaped && result.CandidateWait.PID == result.Identity.PID && validPublicTUIWait(result.CandidateWait) && !result.Forced && !result.CandidateWait.Forced
}

type publicHolderSpec struct {
	Version                                                      int
	Nonce, Candidate, CandidateSHA256, BuildVersion, BuildCommit string
	HolderExecutable, HolderSHA256, Root, Directory, Kind, Slave string
	Arguments, LaunchArguments, Environment, OwnedPaths          []string
	Width, Height                                                int
	Device, Inode                                                uint64
	UID                                                          uint32
	Deadline                                                     time.Time
}

type publicHolderControl struct {
	Version, Sequence int
	Nonce, Action     string
	Accepted          bool
	Spec              *publicHolderSpec
}

type publicHolderReport struct {
	Version, Sequence int
	Nonce, Phase      string
	Holder            PublicTUIHolderIdentity
	Candidate         PublicTUIIdentity
	Wait              PublicTUIWaitFacts
	Terminal          PublicTUITerminalState
	NativeFailure     *PublicTUINativeFailure
	Width, Height     int
	Accepted          bool
	Signals           []string
}

const (
	publicHolderProtocolVersion = 1
	publicHolderFrameLimit      = 64 * 1024
)

// PublicTUITransport owns the real holder and separately observed public child.
// No private exec.Cmd or helper-ready channel stands in for the candidate.
type PublicTUITransport struct {
	mu, controlMu                                                                                                                       sync.Mutex
	holderCommand                                                                                                                       *exec.Cmd
	master, observer, controlWriter, reportReader, stderrReader, holderErrorReader                                                      *os.File
	identity, expected                                                                                                                  PublicTUIIdentity
	holder                                                                                                                              PublicTUIHolderIdentity
	kind                                                                                                                                string
	deadline                                                                                                                            time.Time
	width, height                                                                                                                       int
	feed                                                                                                                                func([]byte) error
	resizeScreen                                                                                                                        func(int, int)
	finish                                                                                                                              func() error
	allocation, before, ready, after                                                                                                    PublicTUITerminalState
	readyAvailable, afterAvailable, accepted, reaped, forced, holderAliveAtPost, postAcknowledged, postAuthorized                       bool
	candidateWait                                                                                                                       PublicTUIWaitFacts
	nativeFailure                                                                                                                       *PublicTUINativeFailure
	stdout, stderr, holderStderr, reports, controls                                                                                     []byte
	outputEOF, errorEOF, holderErrorEOF, reportEOF, holderReaped, holderForced                                                          bool
	holderSignals                                                                                                                       []string
	exitCode, holderExitCode, reportSequence, controlSequence                                                                           int
	outputError, errorError, holderError, reportError, holderWaitError                                                                  error
	baselineDone, launchDone, candidateDone, postDone, teardownDone, reportDone, outputDone, errorDone, holderErrorDone, holderWaitDone chan struct{}
	observerOnce, closeOnce                                                                                                             sync.Once
	observerError, closeError                                                                                                           error
}

func publicTUIPathOwned(driver *Driver, path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	for _, directory := range driver.temporaryPaths {
		relative, err := filepath.Rel(directory, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func validatePublicTUIStart(options PublicTUIStart, selected string) error {
	if options.Mode != contract.E2E || options.Driver == nil || options.Driver.mode != contract.E2E {
		return errors.New("public TUI transport requires the E2E boundary")
	}
	if selected == "" || options.Executable != selected || options.Driver.binary != selected || !filepath.IsAbs(selected) {
		return errors.New("public TUI transport refuses a foreign candidate")
	}
	if !filepath.IsAbs(options.Directory) || filepath.Clean(options.Directory) != options.Directory || !slices.Contains(options.Driver.temporaryPaths, options.Directory) || options.Root != filepath.Join(options.Directory, "public-state") {
		return errors.New("public TUI transport requires this driver's owned root")
	}
	if options.Width < 1 || options.Width > 1024 || options.Height < 1 || options.Height > 256 {
		return errors.New("public TUI viewport is unknown or unsupported")
	}
	if options.Feed == nil || options.ResizeScreen == nil || options.Finish == nil {
		return errors.New("public TUI decoder callbacks are incomplete")
	}
	if err := validatePublicTUIArguments(options.Arguments); err != nil {
		return err
	}
	return validatePublicTUIEnvironment(options)
}

// PreparePublicTUIRoot explicitly acquires a root registered to this E2E driver.
// Its owning temporary directory is registered before the next fallible effect.
func (driver *Driver) PreparePublicTUIRoot() (string, string, error) {
	if driver.mode != contract.E2E {
		return "", "", errors.New("public TUI root requires the E2E boundary")
	}
	directory, err := driver.temporaryRoot()
	if err != nil {
		return "", "", err
	}
	root := filepath.Join(directory, "public-state")
	if err = os.Mkdir(root, 0o700); err != nil {
		return "", "", err
	}
	return root, directory, nil
}

func publicTUIOwnedDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return errors.New("public TUI directory is not private and owned")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	owner := os.Getuid()
	if !ok || owner < 0 || owner > math.MaxUint32 || stat.Uid != uint32(owner) {
		return errors.New("public TUI directory has a foreign owner")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if resolved != path {
		return errors.New("public TUI directory contains a symlink")
	}
	return nil
}

// Parent termios stays on the retained slave. Foreground corroboration uses the
// retained master because the parent is outside H's controlling session. The
// holder's baseline/post-child restoration witness still queries its SAME slave.
func publicTUIState(file, master *os.File, phase string) (PublicTUITerminalState, error) {
	descriptorNumber, err := nativeTUIDescriptor(file.Fd())
	if err != nil {
		return PublicTUITerminalState{}, publicTUIParentNativeError(phase, "descriptor", file.Fd(), err)
	}
	mode, err := publicTUITermios(descriptorNumber)
	if err != nil {
		return PublicTUITerminalState{}, publicTUIParentNativeError(phase, "termios", file.Fd(), err)
	}
	if mode == nil {
		return PublicTUITerminalState{}, publicTUIParentNativeError(phase, "termios", file.Fd(), errors.New("public TUI terminal state is unavailable"))
	}
	foreground, err := nativeTUIForeground(master.Fd())
	if err != nil {
		return PublicTUITerminalState{}, publicTUIParentNativeError(phase, "foreground-master", master.Fd(), err)
	}
	return PublicTUITerminalState{Mode: *mode, Foreground: foreground, RawInput: mode.Lflag&(syscall.ICANON|syscall.ECHO|syscall.ISIG) == 0}, nil
}

func publicTUIParentNativeError(phase, getter string, descriptor uintptr, cause error) error {
	var errno syscall.Errno
	errors.As(cause, &errno)
	return fmt.Errorf("public parent native stage=%s getter=%s fd=%d errno=%d: %w", phase, getter, descriptor, int(errno), cause)
}

// Startup observations are diagnostic only. Unavailable native identities and a
// missing validated candidate receipt stay explicitly unavailable, never zero facts.
type publicTUIStartupEvidence struct {
	HolderPID, ObservedHolderSession, ObservedHolderGroup                     int
	HolderSessionAvailable, HolderGroupAvailable, CandidateReceiptAvailable   bool
	HolderSessionError, HolderGroupError                                      error
	ValidatedCandidatePID, ValidatedCandidateSession, ValidatedCandidateGroup int
	ValidatedReportSequence                                                   int
	BaselineAvailable                                                         bool
	Baseline                                                                  PublicTUITerminalState
}

func (transport *PublicTUITransport) publicStartupEvidence() publicTUIStartupEvidence {
	transport.mu.Lock()
	evidence := publicTUIStartupEvidence{
		HolderPID:             transport.holder.PID,
		ValidatedCandidatePID: transport.identity.PID, ValidatedCandidateSession: transport.identity.Session, ValidatedCandidateGroup: transport.identity.Group,
		CandidateReceiptAvailable: transport.identity.PID > 0, ValidatedReportSequence: transport.reportSequence, BaselineAvailable: publicTUIChannelJoined(transport.baselineDone), Baseline: transport.before,
	}
	transport.mu.Unlock()
	if evidence.HolderPID > 0 {
		evidence.ObservedHolderSession, evidence.HolderSessionError = nativeTUISession(evidence.HolderPID)
		evidence.ObservedHolderGroup, evidence.HolderGroupError = syscall.Getpgid(evidence.HolderPID)
		evidence.HolderSessionAvailable = evidence.HolderSessionError == nil
		evidence.HolderGroupAvailable = evidence.HolderGroupError == nil
	}
	return evidence
}

// Retain small suffixes of the actual bounded captures after cleanup even when
// Start returns nil and the caller never installs its successful-transport logger.
func publicTUIStartupCapture(capture []byte) []byte {
	if len(capture) > 1024 {
		capture = capture[len(capture)-1024:]
	}
	return slices.Clone(capture)
}

func (transport *PublicTUITransport) publicStartupDiagnostics() string {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return fmt.Sprintf("finalValidatedReportSequence=%d holderStderrSuffix=%q reportsSuffix=%q controlsSuffix=%q candidateStderrSuffix=%q",
		transport.reportSequence, publicTUIStartupCapture(transport.holderStderr), publicTUIStartupCapture(transport.reports), publicTUIStartupCapture(transport.controls), publicTUIStartupCapture(transport.stderr))
}

func publicTUIStartupFailure(stage string, cause error, evidence publicTUIStartupEvidence, diagnostics string) error {
	return fmt.Errorf("public TUI startup stage=%s observations=%+v diagnostics=%s: %w", stage, evidence, diagnostics, cause)
}

func publicTUIStamp(ctx context.Context, driver *Driver) (string, string, error) {
	command, err := driver.NativeAnnotationCommand(ctx, "version", "--json")
	if err != nil {
		return "", "", err
	}
	output, err := command.Output()
	if err != nil {
		return "", "", fmt.Errorf("query public TUI candidate identity: %w", err)
	}
	var stamp struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal(output, &stamp); err != nil {
		return "", "", err
	}
	if stamp.Version != testBuildVersion || stamp.Commit != strings.Repeat("0", 40) {
		return "", "", errors.New("public TUI candidate lacks the selected test stamp")
	}
	return stamp.Version, stamp.Commit, nil
}

// publicHolderPacket uses bounded length framing; partial/malformed frames never
// become readiness, wait, or EOF witnesses.
func publicHolderPacket(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	length := len(body)
	if length < 1 || length > math.MaxUint32 || length > publicHolderFrameLimit {
		return nil, errors.New("public holder frame exceeds bounds")
	}
	packet := make([]byte, 4, 4+len(body))
	binary.BigEndian.PutUint32(packet, uint32(length))
	return append(packet, body...), nil
}

// Duplicate keys are ambiguous receipts even when encoding/json would overwrite.
func validatePublicHolderJSON(body []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	var consume func(int) error
	consume = func(depth int) error {
		if depth > 64 {
			return errors.New("public holder JSON nesting exceeds bounds")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, nested := token.(json.Delim)
		if !nested {
			return nil
		}
		switch delimiter {
		case '{':
			return consumePublicHolderObject(decoder, depth, consume)
		case '[':
			return consumePublicHolderArray(decoder, depth, consume)
		default:
			return errors.New("public holder JSON unexpected delimiter")
		}
	}
	if err := consume(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("public holder JSON trailing value")
	}
	return nil
}

func readPublicHolderPacket(reader io.Reader, destination any) error {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return err
	}
	n := int(header[0])<<24 | int(header[1])<<16 | int(header[2])<<8 | int(header[3])
	if n < 1 || n > publicHolderFrameLimit {
		return errors.New("public holder frame exceeds bounds")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(reader, body); err != nil {
		return err
	}
	if err := validatePublicHolderJSON(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("public holder frame has trailing JSON")
	}
	return nil
}

func writePublicHolderPacket(file *os.File, deadline time.Time, value any) ([]byte, error) {
	packet, err := publicHolderPacket(value)
	if err != nil {
		return nil, err
	}
	if err := file.SetWriteDeadline(deadline); err != nil {
		return nil, err
	}
	n, err := file.Write(packet)
	if err == nil && n != len(packet) {
		err = io.ErrShortWrite
	}
	return packet[:n], err
}

func publicTUIFileDigest(path string) (string, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	image, readError := root.ReadFile(filepath.Base(path))
	if err := errors.Join(readError, root.Close()); err != nil {
		return "", err
	}
	digest := sha256.Sum256(image)
	return hex.EncodeToString(digest[:]), nil
}

func validatePublicTUISlave(identity PublicTUIIdentity, stat syscall.Stat_t) error {
	if nativeTUIDevice(stat) != identity.Device || stat.Ino != identity.Inode || stat.Uid != identity.UID {
		return errors.New("public TUI controlling slave identity changed")
	}
	return nil
}

func publicHolderNativeState(slave *os.File, phase string) (PublicTUITerminalState, *PublicTUINativeFailure) {
	descriptorNumber, err := nativeTUIDescriptor(slave.Fd())
	failure := func(operation string, cause error) (PublicTUITerminalState, *PublicTUINativeFailure) {
		var errno syscall.Errno
		errors.As(cause, &errno)
		return PublicTUITerminalState{}, &PublicTUINativeFailure{Phase: phase, Operation: operation, Descriptor: descriptorNumber, Errno: int(errno), Detail: cause.Error()}
	}
	if err != nil {
		return failure("descriptor", err)
	}
	mode, err := publicTUITermios(descriptorNumber)
	if err != nil {
		return failure("termios", err)
	}
	if mode == nil {
		return failure("termios", errors.New("native termios unavailable"))
	}
	foreground, err := nativeTUIForeground(slave.Fd())
	if err != nil {
		return failure("foreground", err)
	}
	return PublicTUITerminalState{Mode: *mode, Foreground: foreground, RawInput: mode.Lflag&(syscall.ICANON|syscall.ECHO|syscall.ISIG) == 0}, nil
}

func validatePublicTUIRestoration(before, after PublicTUITerminalState) error {
	if before.Mode != after.Mode || before.Foreground != after.Foreground || before.RawInput != after.RawInput {
		return errors.New("public candidate did not restore the exact native baseline")
	}
	return nil
}

func validatePublicHolderControl(value publicHolderControl, nonce string, sequence int, action string) error {
	if value.Version != publicHolderProtocolVersion || value.Nonce != nonce || value.Sequence != sequence || value.Action != action || value.Spec != nil {
		return errors.New("public holder control identity or phase mismatch")
	}
	return nil
}

func validatePublicHolderReport(value publicHolderReport, holder PublicTUIHolderIdentity, sequence int) error {
	phases := []string{"baseline", "launch", "child-wait", publicHolderPostChildPhase, "teardown"}
	if sequence < 1 || sequence > len(phases) || value.Version != publicHolderProtocolVersion || value.Sequence != sequence || value.Nonce != holder.Nonce || value.Phase != phases[sequence-1] || value.Holder != holder {
		return errors.New("public holder report identity or phase mismatch")
	}
	return nil
}

// StartPublicTUI acquires a fixed session holder; the selected public binary is
// still the sole product subject. Constructor validation itself remains passive.
func StartPublicTUI(ctx context.Context, options PublicTUIStart) (*PublicTUITransport, error) {
	return startPublicTUI(ctx, options, tuiCommandName, options.Arguments)
}

// StartPublicTUIWatchSignalControl is a fixed cooked-kernel signal control. Its
// normal tui options still pass the ordinary validator; arbitrary argv is absent.
func StartPublicTUIWatchSignalControl(ctx context.Context, options PublicTUIStart) (*PublicTUITransport, error) {
	return startPublicTUI(ctx, options, "watch-control", []string{watchCommandName, intervalFlagName, "10s"})
}

// StartPublicTUIReadOnlyOutput launches the unchanged public CLI with stdout
// reopened readonly on its own slave. It waits for actual cold-failure exit;
// neither raw readiness nor a successfully rendered dashboard is claimed.
func StartPublicTUIReadOnlyOutput(ctx context.Context, options PublicTUIStart) (*PublicTUITransport, error) {
	return startPublicTUI(ctx, options, publicHolderReadOnlyOutputKind, options.Arguments)
}

func startPublicTUI(ctx context.Context, options PublicTUIStart, kind string, arguments []string) (*PublicTUITransport, error) {
	launch, err := preparePublicTUILaunch(ctx, options)
	if err != nil {
		return nil, err
	}
	deadline, candidateDigest := launch.deadline, launch.candidateDigest
	version, commit := launch.version, launch.commit
	holderPath, holderDigest, nonce := launch.holderPath, launch.holderDigest, launch.nonce

	acquired := make([]*os.File, 0, 2)
	fail := func(cause error) (*PublicTUITransport, error) {
		for _, file := range acquired {
			cause = errors.Join(cause, file.Close())
		}
		return nil, cause
	}
	master, slave, err := openNativeTUIPTY()
	if err != nil {
		return nil, err
	}
	acquired = append(acquired, master, slave)
	if err := setNativeTUISize(master.Fd(), options.Width, options.Height); err != nil {
		return fail(err)
	}
	allocation, err := publicTUIState(slave, master, "allocation")
	if err != nil {
		return fail(err)
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(slave.Fd()), &stat); err != nil {
		return fail(err)
	}
	pipes, err := acquirePublicHolderPipes(&acquired)
	if err != nil {
		return fail(err)
	}
	controlReader, controlWriter := pipes.controlReader, pipes.controlWriter
	reportReader, reportWriter := pipes.reportReader, pipes.reportWriter
	stderrReader, stderrWriter := pipes.stderrReader, pipes.stderrWriter
	holderErrorReader, holderErrorWriter := pipes.holderErrorReader, pipes.holderErrorWriter

	command := exec.Command(holderPath, "-test.run=^"+PublicTUISessionHolderHelper+"$", "-test.timeout="+time.Until(deadline).String())
	command.Dir = options.Directory
	command.Env = publicTUIHolderEnvironment()
	command.Stdin, command.Stdout, command.Stderr = controlReader, reportWriter, holderErrorWriter
	command.ExtraFiles = []*os.File{slave, stderrWriter}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 3}
	if err := command.Start(); err != nil {
		return fail(err)
	}
	holder := PublicTUIHolderIdentity{PID: command.Process.Pid, Session: command.Process.Pid, Group: command.Process.Pid, Device: nativeTUIDevice(stat), Inode: stat.Ino, UID: stat.Uid, Executable: holderPath, SHA256: holderDigest, Slave: slave.Name(), Nonce: nonce}
	transport := &PublicTUITransport{holderCommand: command, master: master, observer: slave, controlWriter: controlWriter, reportReader: reportReader, stderrReader: stderrReader, holderErrorReader: holderErrorReader, holder: holder, expected: PublicTUIIdentity{NativeTUIIdentity: NativeTUIIdentity{Device: holder.Device, Inode: holder.Inode, Executable: options.Executable, SHA256: candidateDigest}, Slave: holder.Slave, Root: options.Root, Version: version, Commit: commit, Arguments: slices.Clone(arguments), UID: holder.UID}, kind: kind, deadline: deadline, width: options.Width, height: options.Height, feed: options.Feed, resizeScreen: options.ResizeScreen, finish: options.Finish, allocation: allocation, exitCode: -1, holderExitCode: -1, baselineDone: make(chan struct{}), launchDone: make(chan struct{}), candidateDone: make(chan struct{}), postDone: make(chan struct{}), teardownDone: make(chan struct{}), reportDone: make(chan struct{}), outputDone: make(chan struct{}), errorDone: make(chan struct{}), holderErrorDone: make(chan struct{}), holderWaitDone: make(chan struct{})}
	// All process, descriptor and join handles exist before another fallible close.
	go transport.readPublicOutput()
	go transport.readPublicErrors(false)
	go transport.readPublicErrors(true)
	go transport.readPublicReports()
	go transport.waitPublicHolder()
	var closeError error
	for _, file := range []*os.File{controlReader, reportWriter, stderrWriter, holderErrorWriter} {
		closeError = errors.Join(closeError, file.Close())
	}
	failStarted := func(stage string, cause error) (*PublicTUITransport, error) {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		evidence := transport.publicStartupEvidence()
		cleanupError := transport.Close(cleanup)
		return nil, publicTUIStartupFailure(stage, errors.Join(cause, cleanupError), evidence, transport.publicStartupDiagnostics())
	}
	if closeError != nil {
		return failStarted("parent-duplicate-close", closeError)
	}
	spec := &publicHolderSpec{Version: publicHolderProtocolVersion, Nonce: nonce, Candidate: options.Executable, CandidateSHA256: candidateDigest, BuildVersion: version, BuildCommit: commit, HolderExecutable: holderPath, HolderSHA256: holderDigest, Slave: slave.Name(), Root: options.Root, Directory: options.Directory, Kind: kind, Arguments: slices.Clone(options.Arguments), LaunchArguments: slices.Clone(arguments), Environment: slices.Clone(options.Environment), OwnedPaths: slices.Clone(options.Driver.temporaryPaths), Width: options.Width, Height: options.Height, Device: holder.Device, Inode: holder.Inode, UID: holder.UID, Deadline: deadline}
	if err := transport.sendPublicControl(ctx, "spec", false, spec); err != nil {
		return failStarted("send-spec", err)
	}
	if err := transport.awaitPublicPhase(ctx, transport.baselineDone); err != nil {
		return failStarted("await-baseline", err)
	}
	transport.mu.Lock()
	before := transport.before
	transport.mu.Unlock()
	if before.Foreground != holder.Group {
		return failStarted("baseline-foreground", errors.New("holder baseline foreground is not its persistent group"))
	}
	if err := transport.ownedPublicHolder(); err != nil {
		return failStarted("baseline-holder-lifetime", err)
	}
	actual, err := publicTUIState(slave, master, "baseline-readback")
	if err != nil || actual != before {
		return failStarted("baseline-readback", errors.Join(err, errors.New("holder baseline differs from actual parent native readback")))
	}
	if err := transport.sendPublicControl(ctx, "launch", true, nil); err != nil {
		return failStarted("send-launch", err)
	}
	if err := transport.awaitPublicPhase(ctx, transport.launchDone); err != nil {
		return failStarted("await-launch", err)
	}
	if stage, err := transport.qualifyLaunchedPublicCandidate(ctx); err != nil {
		return failStarted(stage, err)
	}
	return transport, nil
}

func (transport *PublicTUITransport) sendPublicControl(ctx context.Context, action string, accepted bool, spec *publicHolderSpec) error {
	deadline, bounded := ctx.Deadline()
	if !bounded {
		return errors.New("public holder control requires finite deadline")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	transport.controlMu.Lock()
	defer transport.controlMu.Unlock()
	value := publicHolderControl{Version: publicHolderProtocolVersion, Sequence: transport.controlSequence, Nonce: transport.holder.Nonce, Action: action, Accepted: accepted, Spec: spec}
	packet, err := writePublicHolderPacket(transport.controlWriter, deadline, value)
	transport.mu.Lock()
	var captureError error
	transport.controls, captureError = appendNativeTUICapture(transport.controls, packet)
	transport.mu.Unlock()
	if err == nil {
		transport.controlSequence++
	}
	return errors.Join(err, captureError)
}

func (transport *PublicTUITransport) awaitPublicPhase(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return transport.OutputError()
	default:
	}
	select {
	case <-done:
		return transport.OutputError()
	case <-transport.reportDone:
		return errors.Join(transport.OutputError(), errors.New("public holder ended before required phase"))
	case <-ctx.Done():
		return errors.Join(transport.OutputError(), ctx.Err())
	}
}

func (transport *PublicTUITransport) ownedPublicHolder() error {
	return transport.ownedPublicHolderLifetime(true)
}

func (transport *PublicTUITransport) ownedPublicHolderLifetime(requireForeground bool) error {
	if transport.holderCommand == nil || transport.holderCommand.Process == nil || transport.holderCommand.Process.Pid != transport.holder.PID || transport.holder.PID <= 0 || transport.holder.PID == os.Getpid() {
		return errors.New("public holder identity unavailable")
	}
	if err := syscall.Kill(transport.holder.PID, 0); err != nil {
		return err
	}
	session, sessionError := nativeTUISession(transport.holder.PID)
	group, groupError := syscall.Getpgid(transport.holder.PID)
	if sessionError != nil || groupError != nil {
		return errors.Join(sessionError, groupError)
	}
	if session != transport.holder.PID || group != transport.holder.PID {
		return errors.New("public holder lost owned session/group")
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(transport.observer.Fd()), &stat); err != nil {
		return err
	}
	if nativeTUIDevice(stat) != transport.holder.Device || stat.Ino != transport.holder.Inode || stat.Uid != transport.holder.UID {
		return errors.New("public holder slave identity changed")
	}
	if requireForeground {
		// H owns the slave's controlling session; parent corroboration must use
		// its retained master, never replace H's same-slave restoration witness.
		foreground, err := nativeTUIForeground(transport.master.Fd())
		if err != nil {
			return publicTUIParentNativeError("holder-lifetime", "foreground-master", transport.master.Fd(), err)
		}
		if foreground != transport.holder.Group {
			return errors.New("public holder foreground changed")
		}
	}
	return nil
}

func (transport *PublicTUITransport) ownedPublicChild() error {
	transport.mu.Lock()
	identity, reaped := transport.identity, transport.reaped
	transport.mu.Unlock()
	if identity.PID <= 0 || identity.PID == transport.holder.PID || identity.PID == os.Getpid() || reaped {
		return errors.New("public candidate is not live and distinct")
	}
	if err := transport.ownedPublicHolder(); err != nil {
		return err
	}
	session, sessionError := nativeTUISession(identity.PID)
	group, groupError := syscall.Getpgid(identity.PID)
	if sessionError != nil || groupError != nil {
		return errors.Join(sessionError, groupError)
	}
	if session != transport.holder.Session || group != transport.holder.Group || identity.Session != session || identity.Group != group {
		return errors.New("public candidate lost shared holder ownership")
	}
	digest, err := publicTUIFileDigest(identity.Executable)
	if err != nil || digest != identity.SHA256 {
		return errors.Join(err, errors.New("public candidate digest changed"))
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(transport.observer.Fd()), &stat); err != nil {
		return err
	}
	return validatePublicTUISlave(identity, stat)
}

func (transport *PublicTUITransport) ownedPublicCleanupGroup() error {
	transport.mu.Lock()
	holderReaped, identity := transport.holderReaped, transport.identity
	transport.mu.Unlock()
	if !holderReaped {
		return transport.ownedPublicHolderLifetime(false)
	}
	if transport.holderCommand == nil || transport.holderCommand.Process == nil || transport.holderCommand.ProcessState == nil || transport.holderCommand.Process.Pid != transport.holder.PID || identity.PID <= 0 || identity.PID == transport.holder.PID || identity.PID == os.Getpid() {
		return errors.New("public cleanup cohort has no actual holder-reap/candidate anchor")
	}
	session, sessionError := nativeTUISession(identity.PID)
	group, groupError := syscall.Getpgid(identity.PID)
	if sessionError != nil || groupError != nil || session != transport.holder.Session || group != transport.holder.Group {
		return errors.Join(sessionError, groupError, errors.New("public cleanup refuses stale candidate cohort"))
	}
	digest, err := publicTUIFileDigest(identity.Executable)
	if err != nil || digest != identity.SHA256 {
		return errors.Join(err, errors.New("public cleanup selected candidate changed"))
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(transport.observer.Fd()), &stat); err != nil {
		return err
	}
	return validatePublicTUISlave(identity, stat)
}

func (transport *PublicTUITransport) ownedPublicInput() error {
	transport.mu.Lock()
	accepted := transport.accepted
	transport.mu.Unlock()
	if !accepted {
		return errors.New("public TUI input precedes readiness")
	}
	return transport.ownedPublicChild()
}

// ReadinessEvidence is a bounded read-only failure witness. It does not qualify
// readiness, change ownership or terminal modes, or consume another allowance.
func (transport *PublicTUITransport) ReadinessEvidence() string {
	result := transport.Result()
	state, stateError := publicTUIState(transport.observer, transport.master, "readiness-failure")
	size, sizeError := getNativeTUISize(transport.observer.Fd())
	stdout := result.Stdout[max(0, len(result.Stdout)-1024):]
	stderr := result.Stderr[max(0, len(result.Stderr)-1024):]
	return fmt.Sprintf("identity=%+v accepted=%t reaped=%t state=%+v size=%+v nativeError=%v stdoutTail=%q stderrTail=%q", result.Identity, result.TerminalReadyAvailable, result.Reaped, state, size, errors.Join(stateError, sizeError), stdout, stderr)
}

// AwaitReady qualifies only actual current frames plus native ownership/raw mode.
func (transport *PublicTUITransport) AwaitReady(ctx context.Context, frame func() (bool, error)) error {
	if frame == nil {
		return errors.New("public TUI semantic readiness callback is missing")
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return errors.New("public TUI readiness requires a finite deadline")
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := transport.OutputError(); err != nil {
			return err
		}
		ready, err := frame()
		if err != nil {
			return err
		}
		if err := transport.OutputError(); err != nil {
			return err
		}
		if ready {
			qualified, err := transport.qualifyPublicReadiness()
			if err != nil {
				return err
			}
			if qualified {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-transport.candidateDone:
			return errors.New("public candidate exited before readiness")
		case <-ticker.C:
		}
	}
}

// SendKey writes a key only to the owned live candidate.
func (transport *PublicTUITransport) SendKey(ctx context.Context, key []byte) error {
	if err := transport.ownedPublicInput(); err != nil {
		return err
	}
	deadline, bounded := ctx.Deadline()
	if !bounded {
		return errors.New("public TUI input requires a finite deadline")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(key) == 0 || len(key) > 64 {
		return errors.New("public TUI key bytes exceed bounds")
	}
	if err := transport.master.SetWriteDeadline(deadline); err != nil {
		return err
	}
	n, err := transport.master.Write(key)
	if err == nil && n != len(key) {
		err = io.ErrShortWrite
	}
	return err
}

// Resize resizes the owned terminal and decoder together.
func (transport *PublicTUITransport) Resize(ctx context.Context, width, height int) error {
	if _, bounded := ctx.Deadline(); !bounded {
		return errors.New("public TUI resize requires finite deadline")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if width < 1 || width > 1024 || height < 1 || height > 256 {
		return errors.New("public TUI resize has unknown size")
	}
	if err := transport.ownedPublicInput(); err != nil {
		return err
	}
	if err := setNativeTUISize(transport.master.Fd(), width, height); err != nil {
		return err
	}
	transport.mu.Lock()
	transport.resizeScreen(width, height)
	transport.width, transport.height = width, height
	identity := transport.identity
	transport.mu.Unlock()
	return syscall.Kill(identity.PID, syscall.SIGWINCH)
}

// Signal signals only the authenticated candidate session.
func (transport *PublicTUITransport) Signal(pid int, signal os.Signal) error {
	transport.mu.Lock()
	identity := transport.identity
	transport.mu.Unlock()
	if pid != identity.PID {
		return errors.New("public TUI signal target is not owned")
	}
	if err := transport.ownedPublicInput(); err != nil {
		return err
	}
	nativeSignal, ok := signal.(syscall.Signal)
	if !ok || nativeSignal != syscall.SIGINT && nativeSignal != syscall.SIGTERM {
		return errors.New("public TUI signal is unsupported")
	}
	return syscall.Kill(pid, nativeSignal)
}

// OutputError returns all retained output and report failures.
func (transport *PublicTUITransport) OutputError() error {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return errors.Join(transport.outputError, transport.errorError, transport.holderError, transport.reportError)
}

func publicTUIChannelJoined(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// Result returns an owned snapshot of observed transport facts.
func (transport *PublicTUITransport) Result() PublicTUIResult {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	identity := transport.identity
	identity.Arguments = slices.Clone(identity.Arguments)
	var failure *PublicTUINativeFailure
	if transport.nativeFailure != nil {
		retained := *transport.nativeFailure
		failure = &retained
	}
	return PublicTUIResult{Identity: identity, Holder: transport.holder, CandidateWait: transport.candidateWait, Stdout: slices.Clone(transport.stdout), Stderr: slices.Clone(transport.stderr), HolderStderr: slices.Clone(transport.holderStderr), Reports: slices.Clone(transport.reports), Controls: slices.Clone(transport.controls), ExitCode: transport.exitCode, HolderExitCode: transport.holderExitCode, Reaped: transport.reaped, OutputEOF: transport.outputEOF, ErrorEOF: transport.errorEOF, ReadersJoined: publicTUIChannelJoined(transport.outputDone) && publicTUIChannelJoined(transport.errorDone), Forced: transport.forced, HolderReaped: transport.holderReaped, HolderForced: transport.holderForced, HolderSignals: slices.Clone(transport.holderSignals), HolderWaitJoined: publicTUIChannelJoined(transport.holderWaitDone), HolderErrorEOF: transport.holderErrorEOF, HolderErrorJoined: publicTUIChannelJoined(transport.holderErrorDone), ReportEOF: transport.reportEOF, ReportJoined: publicTUIChannelJoined(transport.reportDone), TerminalAllocation: transport.allocation, TerminalBefore: transport.before, TerminalReady: transport.ready, TerminalReadyAvailable: transport.readyAvailable, TerminalAfter: transport.after, TerminalAfterAvailable: transport.afterAvailable, HolderAliveAtPostChild: transport.holderAliveAtPost, PostChildAcknowledged: transport.postAcknowledged, NativeFailure: failure}
}

func (transport *PublicTUITransport) readPublicOutput() {
	defer close(transport.outputDone)
	buffer := make([]byte, 4096)
	for {
		count, err := transport.master.Read(buffer)
		transport.mu.Lock()
		if count > 0 {
			var captureError error
			transport.stdout, captureError = appendNativeTUICapture(transport.stdout, buffer[:count])
			transport.outputError = errors.Join(transport.outputError, captureError)
			if transport.outputError == nil {
				transport.outputError = transport.feed(buffer[:count])
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) {
				transport.outputEOF = true
				transport.outputError = errors.Join(transport.outputError, transport.finish())
			} else {
				transport.outputError = errors.Join(transport.outputError, err)
			}
			transport.mu.Unlock()
			return
		}
		transport.mu.Unlock()
	}
}

func (transport *PublicTUITransport) readPublicErrors(holder bool) {
	done, reader := transport.errorDone, transport.stderrReader
	if holder {
		done, reader = transport.holderErrorDone, transport.holderErrorReader
	}
	defer close(done)
	buffer := make([]byte, 4096)
	for {
		count, err := reader.Read(buffer)
		transport.mu.Lock()
		capture, failure := &transport.stderr, &transport.errorError
		if holder {
			capture, failure = &transport.holderStderr, &transport.holderError
		}
		if count > 0 {
			var captureError error
			*capture, captureError = appendNativeTUICapture(*capture, buffer[:count])
			*failure = errors.Join(*failure, captureError)
		}
		if err != nil {
			transport.acceptPublicErrorEndLocked(holder, failure, err)
			transport.mu.Unlock()
			return
		}
		transport.mu.Unlock()
	}
}

type publicHolderReportReader struct{ transport *PublicTUITransport }

func (reader publicHolderReportReader) Read(buffer []byte) (int, error) {
	count, err := reader.transport.reportReader.Read(buffer)
	reader.transport.mu.Lock()
	var captureError error
	reader.transport.reports, captureError = appendNativeTUICapture(reader.transport.reports, buffer[:count])
	reader.transport.reportError = errors.Join(reader.transport.reportError, captureError)
	reader.transport.mu.Unlock()
	if captureError != nil {
		return count, errors.Join(err, captureError)
	}
	return count, err
}

func (transport *PublicTUITransport) readPublicReports() {
	defer close(transport.reportDone)
	reader := publicHolderReportReader{transport: transport}
	for sequence := 1; ; sequence++ {
		var value publicHolderReport
		err := readPublicHolderPacket(reader, &value)
		transport.mu.Lock()
		if err != nil {
			if errors.Is(err, io.EOF) && sequence == 6 {
				transport.reportEOF = true
				transport.mu.Unlock()
				return
			}
			transport.reportError = errors.Join(transport.reportError, err, errors.New("public holder missing or invalid receipt"))
			transport.mu.Unlock()
			transport.drainRejectedPublicReports(reader)
			return
		}
		if err := validatePublicHolderReport(value, transport.holder, sequence); err != nil {
			transport.reportError = errors.Join(transport.reportError, err)
			transport.mu.Unlock()
			transport.drainRejectedPublicReports(reader)
			return
		}
		transport.reportSequence = sequence
		transport.acceptPublicReportLocked(sequence, value)
		failed := transport.reportError != nil
		transport.mu.Unlock()
		if failed {
			transport.drainRejectedPublicReports(reader)
			return
		}
	}
}

func (transport *PublicTUITransport) drainRejectedPublicReports(reader io.Reader) {
	buffer := make([]byte, 4096)
	for {
		_, err := reader.Read(buffer)
		if err != nil {
			transport.mu.Lock()
			if errors.Is(err, io.EOF) {
				transport.reportEOF = true
			} else {
				transport.reportError = errors.Join(transport.reportError, err)
			}
			transport.mu.Unlock()
			return
		}
	}
}

func validPublicTUIWait(facts PublicTUIWaitFacts) bool {
	if facts.PID <= 0 || facts.RawStatus < 0 || facts.RawStatus > math.MaxUint32 {
		return false
	}
	status := syscall.WaitStatus(facts.RawStatus)
	exit := status.ExitStatus()
	if status.Signaled() {
		exit = 128 + int(status.Signal())
	}
	return facts.Exited == status.Exited() && facts.Signaled == status.Signaled() && facts.Signal == int(status.Signal()) && facts.ExitCode == exit && (facts.Exited != facts.Signaled)
}

func (transport *PublicTUITransport) waitPublicHolder() {
	defer close(transport.holderWaitDone)
	err := transport.holderCommand.Wait()
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.holderReaped = transport.holderCommand.ProcessState != nil
	if transport.holderReaped {
		transport.holderExitCode = transport.holderCommand.ProcessState.ExitCode()
	}
	// Candidate nonzero is an ordinary result; holder failure is fixture failure.
	transport.holderWaitError = err
}

func (transport *PublicTUITransport) closeObserver() error {
	transport.observerOnce.Do(func() { transport.observerError = transport.observer.Close() })
	return transport.observerError
}

func (transport *PublicTUITransport) lifetimeError() error {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return errors.Join(transport.holderWaitError, transport.outputError, transport.errorError, transport.holderError, transport.reportError)
}

// WaitAndDrain joins the holder and readers after the restoration witness.
func (transport *PublicTUITransport) WaitAndDrain(ctx context.Context) (PublicTUIResult, error) {
	if _, bounded := ctx.Deadline(); !bounded {
		return transport.Result(), errors.New("public TUI drain requires finite deadline")
	}
	if !transport.Result().PostChildAcknowledged {
		if err := transport.acknowledgePublicPostChild(ctx); err != nil {
			return transport.Result(), err
		}
	}
	if err := transport.awaitPublicPhase(ctx, transport.teardownDone); err != nil {
		return transport.Result(), err
	}
	if err := transport.closeObserver(); err != nil {
		return transport.Result(), err
	}
	for _, done := range []chan struct{}{transport.holderWaitDone, transport.outputDone, transport.errorDone, transport.holderErrorDone, transport.reportDone} {
		select {
		case <-done:
		case <-ctx.Done():
			return transport.Result(), errors.Join(transport.lifetimeError(), ctx.Err())
		}
	}
	return transport.Result(), transport.lifetimeError()
}

// Close keeps the original one cleanup budget. Forced descriptor closure never
// sets EOF flags; missing receipts or reader joins remain failures.
func (transport *PublicTUITransport) Close(ctx context.Context) error {
	if _, bounded := ctx.Deadline(); !bounded {
		return errors.New("public TUI cleanup requires finite deadline")
	}
	result := transport.Result()
	var abortError error
	if !result.Reaped {
		if err := transport.ownedPublicHolderLifetime(false); err != nil {
			abortError = err
		} else {
			transport.mu.Lock()
			transport.forced = true
			transport.mu.Unlock()
			abortError = transport.sendPublicControl(ctx, "abort", false, nil)
		}
	}
	cooperative := ctx
	var stop context.CancelFunc
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) > 500*time.Millisecond {
		cooperative, stop = context.WithDeadline(ctx, deadline.Add(-500*time.Millisecond))
		defer stop()
	}
	_, drainError := transport.WaitAndDrain(cooperative)
	if drainError != nil && (!publicTUIChannelJoined(transport.holderWaitDone) || !transport.Result().Reaped) {
		if err := transport.ownedPublicCleanupGroup(); err != nil {
			abortError = errors.Join(abortError, err)
		} else {
			transport.mu.Lock()
			transport.holderForced = true
			if !transport.reaped {
				transport.forced = true
			}
			transport.mu.Unlock()
			abortError = errors.Join(abortError, syscall.Kill(-transport.holder.Group, syscall.SIGKILL))
		}
	}
	transport.closeOnce.Do(func() {
		transport.closeError = transport.closeObserver()
		for _, file := range []*os.File{transport.controlWriter, transport.master, transport.stderrReader, transport.reportReader, transport.holderErrorReader} {
			transport.closeError = errors.Join(transport.closeError, file.Close())
		}
	})
	for _, done := range []chan struct{}{transport.holderWaitDone, transport.outputDone, transport.errorDone, transport.reportDone, transport.holderErrorDone} {
		select {
		case <-done:
			continue
		default:
		}
		select {
		case <-done:
		case <-ctx.Done():
			return errors.Join(abortError, drainError, transport.closeError, transport.lifetimeError(), ctx.Err())
		}
	}
	return errors.Join(abortError, drainError, transport.closeError, transport.lifetimeError())
}

func publicHolderPolledFile(descriptor uintptr, name string) (*os.File, error) {
	descriptorNumber, err := nativeTUIDescriptor(descriptor)
	if err != nil {
		return nil, err
	}
	if err := unix.SetNonblock(descriptorNumber, true); err != nil {
		return nil, err
	}
	unix.CloseOnExec(descriptorNumber)
	return os.NewFile(descriptor, name), nil
}

func validatePublicHolderSpec(spec publicHolderSpec) error {
	if spec.Version != publicHolderProtocolVersion || len(spec.Nonce) != 32 || spec.Deadline.IsZero() || !spec.Deadline.After(time.Now()) {
		return errors.New("public holder launch specification identity/deadline invalid")
	}
	if _, err := hex.DecodeString(spec.Nonce); err != nil {
		return err
	}
	if spec.HolderExecutable != os.Args[0] || !filepath.IsAbs(spec.HolderExecutable) || spec.Candidate != os.Getenv("HIPPO_BIN") {
		return errors.New("public holder specification executable mismatch")
	}
	for path, expected := range map[string]string{spec.Candidate: spec.CandidateSHA256, spec.HolderExecutable: spec.HolderSHA256} {
		digest, err := publicTUIFileDigest(path)
		if err != nil || digest != expected {
			return errors.Join(err, errors.New("public holder specification digest mismatch"))
		}
	}
	if spec.BuildVersion != testBuildVersion || spec.BuildCommit != strings.Repeat("0", 40) {
		return errors.New("public holder selected candidate stamp invalid")
	}
	driver := &Driver{mode: contract.E2E, binary: spec.Candidate, temporaryPaths: slices.Clone(spec.OwnedPaths)}
	options := PublicTUIStart{Driver: driver, Mode: contract.E2E, Executable: spec.Candidate, Root: spec.Root, Directory: spec.Directory, Arguments: spec.Arguments, Environment: spec.Environment, Width: spec.Width, Height: spec.Height, Feed: func([]byte) error { return nil }, ResizeScreen: func(int, int) {}, Finish: func() error { return nil }}
	if err := validatePublicTUIStart(options, spec.Candidate); err != nil {
		return err
	}
	for _, path := range []string{spec.Root, spec.Directory} {
		if err := publicTUIOwnedDirectory(path); err != nil {
			return err
		}
	}
	switch spec.Kind {
	case tuiCommandName, publicHolderReadOnlyOutputKind:
		if !slices.Equal(spec.LaunchArguments, spec.Arguments) {
			return errors.New("public holder altered normal candidate arguments")
		}
	case "watch-control":
		if !slices.Equal(spec.LaunchArguments, []string{watchCommandName, intervalFlagName, "10s"}) {
			return errors.New("public holder cooked control is not the fixed public watch command")
		}
	default:
		return errors.New("public holder unknown fixed launch kind")
	}
	return nil
}

func publicHolderWaitFacts(command *exec.Cmd, forced bool) (PublicTUIWaitFacts, error) {
	if command.Process == nil || command.ProcessState == nil {
		return PublicTUIWaitFacts{}, errors.New("public candidate actual wait state unavailable")
	}
	status, ok := command.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		return PublicTUIWaitFacts{}, errors.New("public candidate native WaitStatus unavailable")
	}
	exit := status.ExitStatus()
	if status.Signaled() {
		exit = 128 + int(status.Signal())
	}
	return PublicTUIWaitFacts{PID: command.Process.Pid, ExitCode: exit, Signal: int(status.Signal()), RawStatus: int64(status), Exited: status.Exited(), Signaled: status.Signaled(), Forced: forced}, nil
}

type publicHolderControlRead struct {
	value publicHolderControl
	err   error
}

// publicHolderCloseResult retains the first actual Close result. Later cleanup
// still attempts Close at its original point, without turning an expected
// duplicate descriptor close into a fresh fixture failure.
type publicHolderCloseResult struct {
	handle    string
	attempted bool
	first     error
}

func (result *publicHolderCloseResult) close(closeHandle func() error) error {
	actual := closeHandle()
	if !result.attempted {
		result.attempted = true
		if actual != nil {
			result.first = fmt.Errorf("close public holder %s: %w", result.handle, actual)
		}
	}
	return result.first
}

func retainPublicHolderCloseError(previous, closeError error) error {
	if closeError == nil || errors.Is(previous, closeError) {
		return previous
	}
	return errors.Join(previous, closeError)
}

// validatePublicReadOnlyOutput checks the actual open descriptor. A path name
// alone cannot substitute for the owned slave's UID/device/inode and native TTY.
func validatePublicReadOnlyOutput(output *os.File, expected syscall.Stat_t) error {
	var actual syscall.Stat_t
	if err := syscall.Fstat(int(output.Fd()), &actual); err != nil {
		return err
	}
	if nativeTUIDevice(actual) != nativeTUIDevice(expected) || actual.Ino != expected.Ino || actual.Uid != expected.Uid {
		return errors.New("readonly output is not the owned PTY slave")
	}
	flags, err := unix.FcntlInt(output.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY {
		return errors.Join(err, errors.New("candidate stdout is not actually readonly"))
	}
	_, err = publicTUITermios(int(output.Fd()))
	return err
}

// RunPublicTUISessionHolder serves only the fixed re-executed test entry. After
// session setup it never reads the PTY, sets termios/foreground, or flushes it.
func RunPublicTUISessionHolder(ctx context.Context) error {
	closeResults := make([]*publicHolderCloseResult, 0, 4)
	operationError := runPublicHolderOperation(ctx, &closeResults)
	// Deferred closes have now all run at their original points; their first
	// actual outcomes remain available even after an early protocol failure.
	for _, result := range closeResults {
		operationError = retainPublicHolderCloseError(operationError, result.first)
	}
	return operationError
}

// AwaitCandidateExit lets a scenario verify actual probe cancellation/completion
// before authorizing post-child teardown. It does not ACK or close the holder.
func (transport *PublicTUITransport) AwaitCandidateExit(ctx context.Context) (PublicTUIResult, error) {
	if _, bounded := ctx.Deadline(); !bounded {
		return transport.Result(), errors.New("public candidate wait requires finite deadline")
	}
	err := transport.awaitPublicPhase(ctx, transport.candidateDone)
	return transport.Result(), err
}

// retain records close failures in the same owner inspected at holder return.
func (result *publicHolderCloseResult) retain(closeHandle func() error) {
	if err := result.close(closeHandle); err != nil {
		return
	}
}

func (transport *PublicTUITransport) qualifyPublicReadiness() (bool, error) {
	if err := transport.ownedPublicChild(); err != nil {
		return false, err
	}
	state, stateError := publicTUIState(transport.observer, transport.master, "readiness")
	size, sizeError := getNativeTUISize(transport.observer.Fd())
	if stateError != nil || sizeError != nil {
		return false, errors.Join(stateError, sizeError)
	}
	transport.mu.Lock()
	if err := errors.Join(transport.outputError, transport.errorError, transport.reportError); err != nil {
		transport.mu.Unlock()
		return false, err
	}
	modeReady := state.RawInput
	if transport.kind == "watch-control" {
		modeReady = !state.RawInput && state.Mode.Lflag&syscall.ISIG != 0
	}
	if modeReady && state.Foreground == transport.holder.Group && int(size.Col) == transport.width && int(size.Row) == transport.height {
		transport.ready, transport.readyAvailable, transport.accepted = state, true, true
		transport.mu.Unlock()
		return true, nil
	}
	transport.mu.Unlock()
	return false, nil
}

func (transport *PublicTUITransport) acknowledgePublicPostChild(ctx context.Context) error {
	if err := transport.awaitPublicPhase(ctx, transport.candidateDone); err != nil {
		return err
	}
	if err := transport.awaitPublicPhase(ctx, transport.postDone); err != nil {
		return err
	}
	result := transport.Result()
	if !result.TerminalAfterAvailable {
		return fmt.Errorf("public post-child native getter unavailable: %+v", result.NativeFailure)
	}
	if err := transport.ownedPublicHolder(); err != nil {
		return err
	}
	if err := validatePublicTUIRestoration(result.TerminalBefore, result.TerminalAfter); err != nil {
		return err
	}
	transport.mu.Lock()
	transport.holderAliveAtPost, transport.postAuthorized = true, true
	transport.mu.Unlock()
	if err := transport.sendPublicControl(ctx, "teardown", true, nil); err != nil {
		return err
	}
	return nil
}

type publicHolderSetup struct {
	spec     publicHolderSpec
	deadline time.Time
	holder   PublicTUIHolderIdentity
	baseline PublicTUITerminalState
	stat     syscall.Stat_t
}

func preparePublicHolder(control, slave *os.File, initialDeadline time.Time) (publicHolderSetup, error) {
	var initial publicHolderControl
	if err := readPublicHolderPacket(control, &initial); err != nil {
		return publicHolderSetup{}, err
	}
	if initial.Version != publicHolderProtocolVersion || initial.Sequence != 0 || initial.Action != "spec" || initial.Spec == nil || initial.Nonce != initial.Spec.Nonce || initial.Accepted {
		return publicHolderSetup{}, errors.New("public holder missing authenticated initial specification")
	}
	spec := *initial.Spec
	if err := validatePublicHolderSpec(spec); err != nil {
		return publicHolderSetup{}, err
	}
	deadline := spec.Deadline
	if initialDeadline.Before(deadline) {
		deadline = initialDeadline
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(3, &stat); err != nil {
		return publicHolderSetup{}, err
	}
	if nativeTUIDevice(stat) != spec.Device || stat.Ino != spec.Inode || stat.Uid != spec.UID {
		return publicHolderSetup{}, errors.New("public holder inherited a foreign controlling slave")
	}
	pid := os.Getpid()
	session, sessionError := nativeTUISession(pid)
	group, groupError := syscall.Getpgid(pid)
	if sessionError != nil || groupError != nil || session != pid || group != pid {
		return publicHolderSetup{}, errors.Join(sessionError, groupError, errors.New("public holder is not the stable owned session leader"))
	}
	size, err := getNativeTUISize(slave.Fd())
	if err != nil || int(size.Col) != spec.Width || int(size.Row) != spec.Height {
		return publicHolderSetup{}, errors.Join(err, errors.New("public holder inherited foreign geometry"))
	}
	// The exact slave name comes from the parent's descriptor, not fd3's label.
	holder := PublicTUIHolderIdentity{PID: pid, Session: session, Group: group, Device: spec.Device, Inode: spec.Inode, UID: spec.UID, Executable: spec.HolderExecutable, SHA256: spec.HolderSHA256, Slave: spec.Slave, Nonce: spec.Nonce}
	baseline, failure := publicHolderNativeState(slave, "baseline")
	if failure != nil || baseline.Foreground != pid {
		return publicHolderSetup{}, fmt.Errorf("public holder baseline failed: %+v", failure)
	}
	return publicHolderSetup{spec: spec, deadline: deadline, holder: holder, baseline: baseline, stat: stat}, nil
}

func forcePublicHolderCandidate(candidate PublicTUIIdentity, pid int, forced *bool) error {
	*forced = true
	actualSession, sessionError := nativeTUISession(candidate.PID)
	actualGroup, groupError := syscall.Getpgid(candidate.PID)
	if sessionError != nil || groupError != nil || actualSession != pid || actualGroup != pid {
		return errors.Join(sessionError, groupError, errors.New("holder abort refuses stale candidate ownership"))
	}
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func expirePublicHolder(ctx context.Context, pid int, controlError error) error {
	actualSession, sessionError := nativeTUISession(pid)
	actualGroup, groupError := syscall.Getpgid(pid)
	if sessionError != nil || groupError != nil || actualSession != pid || actualGroup != pid {
		return errors.Join(controlError, ctx.Err(), sessionError, groupError, errors.New("expired holder refuses foreign cleanup group"))
	}
	fmt.Fprintln(os.Stderr, "public holder expired before candidate wait; forced owned-group teardown")
	return errors.Join(controlError, ctx.Err(), syscall.Kill(-pid, syscall.SIGKILL))
}

func runPublicHolderOperation(ctx context.Context, closeResults *[]*publicHolderCloseResult) error {
	if os.Getenv(PublicTUISessionHolderEnvironment) != "1" {
		return errors.New("public holder helper marker absent")
	}
	initialDeadline, bounded := ctx.Deadline()
	if !bounded {
		return errors.New("public holder helper requires finite deadline")
	}
	control, err := publicHolderPolledFile(0, "public-holder-control")
	if err != nil {
		return err
	}
	controlClose := &publicHolderCloseResult{handle: "control descriptor"}
	*closeResults = append(*closeResults, controlClose)
	defer controlClose.retain(control.Close)
	report, err := publicHolderPolledFile(1, "public-holder-report")
	if err != nil {
		return err
	}
	reportClose := &publicHolderCloseResult{handle: "report descriptor"}
	*closeResults = append(*closeResults, reportClose)
	defer reportClose.retain(report.Close)
	slave := os.NewFile(3, "public-holder-slave")
	slaveClose := &publicHolderCloseResult{handle: "controlling slave descriptor"}
	*closeResults = append(*closeResults, slaveClose)
	defer slaveClose.retain(slave.Close)
	candidateErrors := os.NewFile(4, "public-holder-candidate-stderr")
	candidateErrorClose := &publicHolderCloseResult{handle: "candidate stderr descriptor"}
	*closeResults = append(*closeResults, candidateErrorClose)
	defer candidateErrorClose.retain(candidateErrors.Close)
	unix.CloseOnExec(3)
	unix.CloseOnExec(4)
	signals := startPublicHolderSignals(syscall.SIGINT, syscall.SIGTERM)
	defer signals.close()
	if err := control.SetReadDeadline(initialDeadline); err != nil {
		return err
	}
	setup, err := preparePublicHolder(control, slave, initialDeadline)
	if err != nil {
		return err
	}
	files := publicHolderExecutionFiles{control: control, report: report, slave: slave, candidateErrors: candidateErrors, controlClose: controlClose, candidateErrorClose: candidateErrorClose}
	return executePublicHolderCandidate(ctx, setup, files, signals, closeResults)
}

func consumePublicHolderObject(decoder *json.Decoder, depth int, consume func(int) error) error {
	seen := make(map[string]bool)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return errors.New("public holder JSON duplicate or invalid key")
		}
		seen[name] = true
		if err := consume(depth + 1); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return errors.Join(err, errors.New("public holder JSON unclosed object"))
	}
	return nil
}

func consumePublicHolderArray(decoder *json.Decoder, depth int, consume func(int) error) error {
	for decoder.More() {
		if err := consume(depth + 1); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim(']') {
		return errors.Join(err, errors.New("public holder JSON unclosed array"))
	}
	return nil
}

func (transport *PublicTUITransport) acceptPublicReportLocked(sequence int, value publicHolderReport) {
	switch sequence {
	case 1:
		if value.NativeFailure != nil || value.Width != transport.width || value.Height != transport.height || value.Terminal.Foreground != transport.holder.Group {
			transport.reportError = errors.New("public holder baseline unavailable or foreign")
		} else {
			transport.before = value.Terminal
			close(transport.baselineDone)
		}
	case 2:
		candidate := value.Candidate
		if transport.rejectsPublicCandidate(candidate) {
			transport.reportError = errors.New("public holder launched foreign candidate identity")
		} else {
			transport.identity = candidate
			close(transport.launchDone)
		}
	case 3:
		if value.Candidate.PID != transport.identity.PID || value.Wait.PID != transport.identity.PID || !validPublicTUIWait(value.Wait) {
			transport.reportError = errors.New("public holder candidate wait identity/status mismatch")
		} else {
			transport.candidateWait = value.Wait
			transport.exitCode = value.Wait.ExitCode
			transport.reaped = true
			transport.forced = transport.forced || value.Wait.Forced
			close(transport.candidateDone)
		}
	case 4:
		if !transport.reaped || value.Candidate.PID != transport.identity.PID || value.Wait != transport.candidateWait {
			transport.reportError = errors.New("public holder post-child lacks actual candidate wait")
		} else {
			transport.after, transport.nativeFailure = value.Terminal, value.NativeFailure
			transport.afterAvailable = value.NativeFailure == nil
			close(transport.postDone)
		}
	case 5:
		transport.holderSignals = slices.Clone(value.Signals)
		if !transport.postAuthorized || !value.Accepted {
			transport.reportError = errors.New("public holder teardown lacks accepted post-child acknowledgment")
		} else {
			transport.postAcknowledged = true
		}
		close(transport.teardownDone)
	}
}

func (transport *PublicTUITransport) rejectsPublicCandidate(candidate PublicTUIIdentity) bool {
	return candidate.PID <= 0 || candidate.PID == transport.holder.PID || candidate.Session != transport.holder.Session || candidate.Group != transport.holder.Group || candidate.Device != transport.holder.Device || candidate.Inode != transport.holder.Inode || candidate.UID != transport.holder.UID || candidate.Slave != transport.holder.Slave || candidate.Executable != os.Getenv("HIPPO_BIN") || candidate.SHA256 != transport.expected.SHA256 || candidate.Root != transport.expected.Root || !slices.Equal(candidate.Arguments, transport.expected.Arguments) || candidate.Version != transport.expected.Version || candidate.Commit != transport.expected.Commit
}

func validatePublicTUIEnvironment(options PublicTUIStart) error {
	roots := 0
	for _, entry := range options.Environment {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			return errors.New("public TUI environment entry has no value separator")
		}
		switch {
		case name == "HIPPO_ROOT":
			roots++
			if value != options.Root {
				return errors.New("public TUI environment has a foreign root")
			}
		case name == linuxEvidenceRootEnvironment:
			if !publicTUIPathOwned(options.Driver, value) {
				return errors.New("public TUI host evidence root is not owned")
			}
		case strings.HasPrefix(name, "HIPPO_") || name == PublicTUISessionHolderEnvironment || name == PublicTUIHostProbeHelperEnvironment:
			return errors.New("public TUI environment contains a private harness input")
		}
	}
	if roots != 1 {
		return errors.New("public TUI environment requires exactly one owned root")
	}
	return nil
}

func (transport *PublicTUITransport) acceptPublicErrorEndLocked(holder bool, failure *error, err error) {
	if errors.Is(err, io.EOF) {
		if holder {
			transport.holderErrorEOF = true
		} else {
			transport.errorEOF = true
		}
	} else {
		*failure = errors.Join(*failure, err)
	}
}

type publicTUILaunchIdentity struct {
	deadline                                                          time.Time
	candidateDigest, version, commit, holderPath, holderDigest, nonce string
}

func preparePublicTUILaunch(ctx context.Context, options PublicTUIStart) (publicTUILaunchIdentity, error) {
	if err := validatePublicTUIStart(options, os.Getenv("HIPPO_BIN")); err != nil {
		return publicTUILaunchIdentity{}, err
	}
	deadline, bounded := ctx.Deadline()
	if !bounded {
		return publicTUILaunchIdentity{}, errors.New("public TUI launch requires a finite deadline")
	}
	if err := ctx.Err(); err != nil {
		return publicTUILaunchIdentity{}, err
	}
	for _, path := range []string{options.Directory, options.Root} {
		if err := publicTUIOwnedDirectory(path); err != nil {
			return publicTUILaunchIdentity{}, err
		}
	}
	candidateDigest, err := publicTUIFileDigest(options.Executable)
	if err != nil {
		return publicTUILaunchIdentity{}, err
	}
	version, commit, err := publicTUIStamp(ctx, options.Driver)
	if err != nil {
		return publicTUILaunchIdentity{}, err
	}
	holderPath, err := os.Executable()
	if err != nil {
		return publicTUILaunchIdentity{}, err
	}
	if holderPath != os.Args[0] || !filepath.IsAbs(holderPath) || filepath.Clean(holderPath) != holderPath {
		return publicTUILaunchIdentity{}, errors.New("public holder test executable is not absolute")
	}
	holderDigest, err := publicTUIFileDigest(holderPath)
	if err != nil {
		return publicTUILaunchIdentity{}, err
	}
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return publicTUILaunchIdentity{}, err
	}
	nonce := hex.EncodeToString(nonceBytes)
	return publicTUILaunchIdentity{deadline: deadline, candidateDigest: candidateDigest, version: version, commit: commit, holderPath: holderPath, holderDigest: holderDigest, nonce: nonce}, nil
}

type publicHolderAwaitedCandidate struct {
	forced, controlPending  bool
	waitError, controlError error
	early                   *publicHolderControlRead
}

func awaitPublicHolderCandidate(ctx context.Context, pid int, candidate PublicTUIIdentity, spec publicHolderSpec, waited <-chan error, next <-chan publicHolderControlRead, closeError, launchError error) (publicHolderAwaitedCandidate, error) {
	outcome := publicHolderAwaitedCandidate{controlPending: true}
	forced := false
	var waitError, controlError error
	var early *publicHolderControlRead
	force := func() error { return forcePublicHolderCandidate(candidate, pid, &forced) }
	if closeError != nil || launchError != nil {
		controlError = errors.Join(closeError, launchError, force())
	}
	select {
	case waitError = <-waited:
	case received := <-next:
		outcome.controlPending = false
		early = &received
		if received.err != nil {
			controlError = errors.Join(controlError, received.err)
		} else {
			controlError = errors.Join(controlError, validatePublicHolderControl(received.value, spec.Nonce, 2, "abort"))
		}
		controlError = errors.Join(controlError, force())
		select {
		case waitError = <-waited:
		case <-ctx.Done():
			// The already-expired journey has no renewed cleanup allowance. Retire only
			// the verified new group, including this holder; status/witness remain
			// unavailable and the parent records a forced fixture failure.
			return publicHolderAwaitedCandidate{forced: forced, waitError: waitError, controlError: controlError, early: early, controlPending: outcome.controlPending}, expirePublicHolder(ctx, pid, controlError)
		}
	case <-ctx.Done():
		controlError = errors.Join(controlError, ctx.Err(), force())
		select {
		case waitError = <-waited:
		case <-ctx.Done():
			// The already-expired journey has no renewed cleanup allowance. Retire only
			// the verified new group, including this holder; status/witness remain
			// unavailable and the parent records a forced fixture failure.
			return publicHolderAwaitedCandidate{forced: forced, waitError: waitError, controlError: controlError, early: early, controlPending: outcome.controlPending}, expirePublicHolder(ctx, pid, controlError)
		}
	}
	return publicHolderAwaitedCandidate{forced: forced, waitError: waitError, controlError: controlError, early: early, controlPending: outcome.controlPending}, nil
}

func validatePublicTUIArguments(arguments []string) error {
	if len(arguments) >= 3 && arguments[0] == outputFlag && arguments[1] == outputJSONValue {
		arguments = arguments[2:]
	}
	if len(arguments) == 0 || arguments[0] != tuiCommandName {
		return errors.New("public TUI transport requires a normal tui command")
	}
	for _, argument := range arguments {
		if strings.HasPrefix(argument, "-test.") {
			return errors.New("public TUI transport refuses test helper arguments")
		}
	}
	return nil
}

func publicTUIHolderEnvironment() []string {
	var environment []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name != PublicTUISessionHolderEnvironment && name != "HIPPO_BDD_SUITE" {
			environment = append(environment, entry)
		}
	}
	environment = append(environment, PublicTUISessionHolderEnvironment+"=1", "HIPPO_BDD_SUITE="+contract.Unit)
	return environment
}

func publicHolderPostChildState(slave *os.File, spec publicHolderSpec) (PublicTUITerminalState, *PublicTUINativeFailure) {
	after, nativeFailure := publicHolderNativeState(slave, publicHolderPostChildPhase)
	var afterStat syscall.Stat_t
	if statError := syscall.Fstat(3, &afterStat); statError != nil || nativeTUIDevice(afterStat) != spec.Device || afterStat.Ino != spec.Inode || afterStat.Uid != spec.UID {
		if nativeFailure == nil {
			nativeFailure = &PublicTUINativeFailure{Phase: publicHolderPostChildPhase, Operation: "slave-identity", Descriptor: 3, Detail: fmt.Sprint(statError)}
		}
	}
	return after, nativeFailure
}

func receivePublicHolderAcknowledgment(control *os.File, next <-chan publicHolderControlRead, controlPending *bool, readNext func(time.Time), early *publicHolderControlRead, cleanupDeadline time.Time, nonce string, controlError *error) (publicHolderControlRead, error) {
	ackSequence := 2
	if early != nil {
		ackSequence = 3
		readNext(cleanupDeadline)
		*controlPending = true
	} else if *controlPending {
		*controlError = errors.Join(*controlError, control.SetReadDeadline(cleanupDeadline))
	}
	var acknowledgment publicHolderControlRead
	if *controlPending {
		acknowledgment = <-next
		*controlPending = false
	}
	ackError := acknowledgment.err
	if ackError == nil {
		ackError = validatePublicHolderControl(acknowledgment.value, nonce, ackSequence, "teardown")
	}
	return acknowledgment, ackError
}

func (transport *PublicTUITransport) qualifyLaunchedPublicCandidate(ctx context.Context) (string, error) {
	if transport.kind == publicHolderReadOnlyOutputKind {
		// An early failing C can already have been reaped by H. Authenticate its
		// actual Wait via the existing protocol, rather than querying a reused PID.
		if _, err := transport.AwaitCandidateExit(ctx); err != nil {
			return "readonly-output-candidate-wait", err
		}
	} else if err := transport.ownedPublicChild(); err != nil {
		return "candidate-lifetime", err
	}
	return "", nil
}

func sendPublicHolderPostChildReports(send func(publicHolderReport, time.Time) error, cleanupDeadline time.Time, candidate PublicTUIIdentity, facts PublicTUIWaitFacts, after PublicTUITerminalState, nativeFailure *PublicTUINativeFailure, controlError error) error {
	if err := send(publicHolderReport{Phase: "child-wait", Candidate: candidate, Wait: facts}, cleanupDeadline); err != nil {
		controlError = errors.Join(controlError, err)
	}
	if err := send(publicHolderReport{Phase: publicHolderPostChildPhase, Candidate: candidate, Wait: facts, Terminal: after, NativeFailure: nativeFailure}, cleanupDeadline); err != nil {
		controlError = errors.Join(controlError, err)
	}
	return controlError
}

type publicHolderPipes struct {
	controlReader, controlWriter         *os.File
	reportReader, reportWriter           *os.File
	stderrReader, stderrWriter           *os.File
	holderErrorReader, holderErrorWriter *os.File
}

func publicTUIOwnedPipe(acquired *[]*os.File) (*os.File, *os.File, error) {
	reader, writer, err := os.Pipe()
	if err == nil {
		*acquired = append(*acquired, reader, writer)
	}
	return reader, writer, err
}

func acquirePublicHolderPipes(acquired *[]*os.File) (publicHolderPipes, error) {
	var pipes publicHolderPipes
	var err error
	pipes.controlReader, pipes.controlWriter, err = publicTUIOwnedPipe(acquired)
	if err != nil {
		return publicHolderPipes{}, err
	}
	pipes.reportReader, pipes.reportWriter, err = publicTUIOwnedPipe(acquired)
	if err != nil {
		return publicHolderPipes{}, err
	}
	pipes.stderrReader, pipes.stderrWriter, err = publicTUIOwnedPipe(acquired)
	if err != nil {
		return publicHolderPipes{}, err
	}
	pipes.holderErrorReader, pipes.holderErrorWriter, err = publicTUIOwnedPipe(acquired)
	if err != nil {
		return publicHolderPipes{}, err
	}
	return pipes, nil
}

type publicHolderCompletionState struct {
	candidate                           PublicTUIIdentity
	facts                               PublicTUIWaitFacts
	baseline, after                     PublicTUITerminalState
	nativeFailure                       *PublicTUINativeFailure
	waitError, factsError, controlError error
	early                               *publicHolderControlRead
}

func finishPublicHolderCompletion(control *os.File, next <-chan publicHolderControlRead, controlPending *bool, readNext func(time.Time), send func(publicHolderReport, time.Time) error, signals *publicHolderSignals, deadline time.Time, nonce string, completion publicHolderCompletionState) error {
	candidate, facts := completion.candidate, completion.facts
	baseline, after, nativeFailure := completion.baseline, completion.after, completion.nativeFailure
	waitError, factsError, controlError, early := completion.waitError, completion.factsError, completion.controlError, completion.early
	cleanupDeadline := time.Now().Add(3 * time.Second)
	if deadline.Before(cleanupDeadline) {
		cleanupDeadline = deadline
	}
	controlError = sendPublicHolderPostChildReports(send, cleanupDeadline, candidate, facts, after, nativeFailure, controlError)
	acknowledgment, ackError := receivePublicHolderAcknowledgment(control, next, controlPending, readNext, early, cleanupDeadline, nonce, &controlError)
	controlError = errors.Join(controlError, signals.captureError())
	// A cleanup ACK proves the actual authenticated Wait and restoration even
	// after an owned abort. The forced Wait fact remains separate; it can
	// never satisfy the original natural-q acceptance oracles.
	accepted := ackError == nil && acknowledgment.value.Accepted && factsError == nil && waitError == nil && nativeFailure == nil && validatePublicTUIRestoration(baseline, after) == nil && controlError == nil
	if !accepted {
		ackError = errors.Join(ackError, errors.New("holder post-child receipt was not accepted"))
	}
	if err := send(publicHolderReport{Phase: "teardown", Candidate: candidate, Wait: facts, Accepted: accepted, Signals: signals.snapshot()}, cleanupDeadline); err != nil {
		ackError = errors.Join(ackError, err)
	}
	// The parent has retained its receipt and ACK; only now may H close the slave.
	return errors.Join(waitError, factsError, controlError, ackError)
}

func authorizePublicHolderLaunch(control *os.File, spec publicHolderSpec, baseline PublicTUITerminalState, send func(publicHolderReport, time.Time) error, deadline time.Time) error {
	if err := send(publicHolderReport{Phase: "baseline", Terminal: baseline, Width: spec.Width, Height: spec.Height}, deadline); err != nil {
		return err
	}
	if err := control.SetReadDeadline(deadline); err != nil {
		return err
	}
	var launch publicHolderControl
	if err := readPublicHolderPacket(control, &launch); err != nil {
		return err
	}
	if err := validatePublicHolderControl(launch, spec.Nonce, 1, "launch"); err != nil || !launch.Accepted {
		return errors.Join(err, errors.New("public holder launch was not acknowledged"))
	}
	return nil
}

func readPublicHolderControlAt(control *os.File, bound time.Time, next chan<- publicHolderControlRead) {
	err := control.SetReadDeadline(bound)
	var value publicHolderControl
	if err == nil {
		err = readPublicHolderPacket(control, &value)
	}
	next <- publicHolderControlRead{value: value, err: err}
}

// publicHolderExecutionFiles borrows the descriptors owned by runPublicHolderOperation.
// Candidate execution closes its pending reader and readonly output before that
// caller stops signals and closes the remaining protocol and slave descriptors.
type publicHolderExecutionFiles struct {
	control, report, slave, candidateErrors *os.File
	controlClose, candidateErrorClose       *publicHolderCloseResult
}

func executePublicHolderCandidate(ctx context.Context, setup publicHolderSetup, files publicHolderExecutionFiles, signals *publicHolderSignals, closeResults *[]*publicHolderCloseResult) error {
	control, report, slave, candidateErrors := files.control, files.report, files.slave, files.candidateErrors
	controlClose, candidateErrorClose := files.controlClose, files.candidateErrorClose
	spec, deadline, holder, baseline := setup.spec, setup.deadline, setup.holder, setup.baseline
	pid, session, group := holder.PID, holder.Session, holder.Group
	stat := setup.stat

	sequence := 0
	send := func(value publicHolderReport, bound time.Time) error {
		sequence++
		value.Version, value.Sequence, value.Nonce, value.Holder = publicHolderProtocolVersion, sequence, spec.Nonce, holder
		_, err := writePublicHolderPacket(report, bound, value)
		return err
	}
	if err := authorizePublicHolderLaunch(control, spec, baseline, send, deadline); err != nil {
		return err
	}
	driver := &Driver{mode: contract.E2E, binary: spec.Candidate}
	command, err := driver.NativeAnnotationCommand(context.WithoutCancel(ctx), spec.LaunchArguments...)
	if err != nil {
		return err
	}
	command.Dir, command.Env = spec.Directory, slices.Clone(spec.Environment)
	command.Stdin, command.Stdout, command.Stderr = slave, slave, candidateErrors
	if spec.Kind == publicHolderReadOnlyOutputKind {
		output, err := os.OpenFile(spec.Slave, os.O_RDONLY|syscall.O_NOCTTY|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		outputClose := &publicHolderCloseResult{handle: "readonly candidate stdout"}
		*closeResults = append(*closeResults, outputClose)
		defer outputClose.retain(output.Close)
		if err := validatePublicReadOnlyOutput(output, stat); err != nil {
			return err
		}
		command.Stdout = output
	}
	// No SysProcAttr or ExtraFiles: C inherits H's session/group, and exec closes
	// every holder protocol descriptor. Candidate fd0/1/2 are explicit product IO.
	if err := command.Start(); err != nil {
		return err
	}
	// Actual Wait is installed before the next fallible operation.
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	candidate := PublicTUIIdentity{NativeTUIIdentity: NativeTUIIdentity{PID: command.Process.Pid, Session: session, Group: group, Device: spec.Device, Inode: spec.Inode, Executable: spec.Candidate, SHA256: spec.CandidateSHA256}, Slave: spec.Slave, Root: spec.Root, Version: spec.BuildVersion, Commit: spec.BuildCommit, Arguments: slices.Clone(spec.LaunchArguments), UID: spec.UID}
	// H's duplicate must not postpone the actual candidate/probe stderr EOF.
	closeError := candidateErrorClose.close(candidateErrors.Close)
	launchError := send(publicHolderReport{Phase: "launch", Candidate: candidate}, deadline)
	// This reader owns only one next packet at a time, and always has a finite
	// deadline. At final teardown the caller closes/joins it before returning.
	next := make(chan publicHolderControlRead, 1)
	readNext := func(bound time.Time) {
		go readPublicHolderControlAt(control, bound, next)
	}
	readNext(deadline)
	controlPending := true
	defer func() {
		if controlPending {
			controlClose.retain(control.Close)
			<-next
		}
	}()
	awaited, awaitError := awaitPublicHolderCandidate(ctx, pid, candidate, spec, waited, next, closeError, launchError)
	controlPending = awaited.controlPending
	if awaitError != nil {
		return awaitError
	}
	forced, waitError, controlError, early := awaited.forced, awaited.waitError, awaited.controlError, awaited.early

	facts, factsError := publicHolderWaitFacts(command, forced)
	if _, ok := errors.AsType[*exec.ExitError](waitError); ok {
		waitError = nil
	}
	// First effect after actual Wait is the retained slave's read-only witness.
	// No close, setting, read, flush, or holder exit intervenes.
	after, nativeFailure := publicHolderPostChildState(slave, spec)
	completion := publicHolderCompletionState{candidate: candidate, facts: facts, baseline: baseline, after: after, nativeFailure: nativeFailure, waitError: waitError, factsError: factsError, controlError: controlError, early: early}
	return finishPublicHolderCompletion(control, next, &controlPending, readNext, send, signals, deadline, spec.Nonce, completion)
}
