package support

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This portable lifecycle control owns a genuine held FIFO; only the Linux
// writer-side kernel observation is injected on other platforms.
func TestPublicHostProbeHeldFIFOReaderRetirementOwnership(t *testing.T) {
	probe := publicHostProtocolFixture()
	probe.platform, probe.origin = linuxPlatform, time.Now()
	probe.directory = t.TempDir()
	path := filepath.Join(probe.directory, "meminfo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	probe.fifoPaths = []string{path}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	reading, err := probe.HoldNextReading()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	readerClosed := false
	t.Cleanup(func() {
		if !readerClosed {
			if err := reader.Close(); err != nil {
				t.Error(err)
			}
		}
		if !probe.Trace().Closed {
			cleanup, stop := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
			defer stop()
			if err := probe.Close(cleanup); err != nil {
				t.Error(err)
			}
		}
	})
	observations := 0
	probe.fifoObserver = func(original context.Context, descriptor uintptr) (publicHostFIFOObservation, error) {
		observations++
		if original != ctx {
			t.Error("retirement renewed the original cleanup context")
		}
		if descriptor == reader.Fd() {
			t.Error("retirement inspected the reader instead of the original writer")
		}
		return publicHostFIFOObservation{descriptor: int(descriptor), events: 0x0008}, nil
	}
	done := make(chan struct{})
	probe.done = append(probe.done, done)
	go probe.serveFIFO(path, linuxMeminfoName, done)
	if err := probe.WaitForReading(ctx, reading); err != nil {
		t.Fatal(err)
	}
	before := probe.Trace()
	if before.Active != 1 || before.Readings != 1 || before.MaximumActive != 1 {
		t.Fatalf("held reading not acknowledged: %+v", before)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	readerClosed = true
	if err := probe.Close(ctx); err != nil {
		t.Fatal(err)
	}
	after := probe.Trace()
	if after.Active != 0 || after.Readings != 1 || after.MaximumActive != 1 || observations != 1 || len(probe.releaseReaders) != 0 {
		t.Fatalf("retired reader lost original writer ownership: active=%d readings=%d maximum=%d observations=%d synthetic=%d", after.Active, after.Readings, after.MaximumActive, observations, len(probe.releaseReaders))
	}
	select {
	case <-done:
	default:
		t.Fatal("owned FIFO worker was not joined")
	}
	if _, err := os.Stat(probe.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned fixture cleanup unavailable: %v", err)
	}
}

func TestPublicHostProbeDecodeFailureRetainsBoundedPhaseMetadata(t *testing.T) {
	for _, test := range []struct {
		name, body string
		phase      string
		accepted   bool
		cause      error
	}{
		{"pre-request whitespace", " ", "pre-request", false, io.EOF},
		{"pre-request truncated", `{"Nonce":`, "pre-request", false, io.ErrUnexpectedEOF},
		{"accepted ACK truncated", `"output-`, "accepted-ack", true, io.ErrUnexpectedEOF},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := &publicHostRequestInput{reader: strings.NewReader(test.body)}
			decoder := json.NewDecoder(input)
			metadata := publicHostDecodeMetadata{accepted: test.accepted}
			if test.accepted {
				metadata.reading, metadata.pid, metadata.parent = 1, 17, 23
			}
			metadata, beforeError := preparePublicHostDecodeMetadata(decoder, input, metadata)
			var decodeError error
			if test.accepted {
				var message string
				decodeError = decoder.Decode(&message)
				decodeError = validatePublicHostAcknowledgement(message, decodeError)
			} else {
				var request publicHostRequest
				decodeError = decoder.Decode(&request)
			}
			result := finishPublicHostDecodeMetadata(decoder, input, metadata, errors.Join(beforeError, decodeError))
			err := publicHostRetainedDecodeError(result)
			if !errors.Is(err, test.cause) {
				t.Fatal("original decode identity lost", err)
			}
			if !strings.Contains(err.Error(), "phase="+test.phase) || !strings.Contains(err.Error(), "wire-before=") || !strings.Contains(err.Error(), "buffered-before=") || !strings.Contains(err.Error(), "offset-after=") {
				t.Fatalf("retained decoder error lacks bounded phase/count metadata: %v", err)
			}
		})
	}
}

func TestPublicHostFIFOReaderAbsentMasks(t *testing.T) {
	for _, test := range []struct {
		name    string
		events  int16
		absent  bool
		failure bool
	}{
		{"none", 0, false, false},
		{"writable", 0x0004, false, false},
		{"hangup", 0x0010, false, false},
		{"error", 0x0008, true, false},
		{"error and hangup", 0x0018, true, false},
		{"invalid", 0x0020, false, true},
		{"invalid and error", 0x0028, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			absent, err := publicHostFIFOReaderAbsent(test.events)
			if absent != test.absent || (err != nil) != test.failure {
				t.Fatalf("events=%#x absent=%t error=%v", test.events, absent, err)
			}
			if test.failure && !errors.Is(err, syscall.EBADF) {
				t.Fatal("invalid original writer lost error identity", err)
			}
		})
	}
}

func TestPublicHostFIFOReaderAbsentRetryAndContext(t *testing.T) {
	calls := 0
	observation, err := waitOriginalFIFOReaderAbsent(t.Context(), 42, func(descriptor, timeout int) (int16, error) {
		calls++
		if descriptor != 42 || timeout < 0 || timeout > 10 {
			t.Errorf("foreign descriptor or unbounded poll: %d/%d", descriptor, timeout)
		}
		switch calls {
		case 1:
			return 0, syscall.EINTR
		case 2:
			return 0x0004, nil
		default:
			return 0x0008, nil
		}
	})
	if err != nil || observation.descriptor != 42 || observation.events != 0x0008 || calls != 3 {
		t.Fatalf("retry proof=%+v error=%v calls=%d", observation, err, calls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = waitOriginalFIFOReaderAbsent(ctx, 42, func(int, int) (int16, error) { t.Error("expired caller acquired a poll"); return 0x0008, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal("caller expiry lost", err)
	}
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	_, err = waitOriginalFIFOReaderAbsent(ctx, 42, func(int, int) (int16, error) { cancel(); return 0, syscall.EIO })
	if !errors.Is(err, context.Canceled) || !errors.Is(err, syscall.EIO) {
		t.Fatal("expiry hid original syscall failure", err)
	}
}

func TestPublicHostProbeFIFOClosingWinsBeforeLogicalAcquisition(t *testing.T) {
	probe := publicHostProtocolFixture()
	probe.directory = t.TempDir()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := errors.Join(reader.Close(), writer.Close()); err != nil {
			t.Error(err)
		}
	}()
	if err := probe.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	probe.mu.Lock()
	owner, gate, registerError := probe.registerFIFOWriterLocked(writer, "owned-path", linuxMeminfoName)
	probe.mu.Unlock()
	if owner != nil || gate != nil || registerError == nil || probe.Trace().Readings != 0 || len(probe.fifoWriters) != 0 {
		t.Fatal("physical acquisition after Close invented a logical reading or writer registration")
	}
}

func TestPublicHostProbeNormalWriterCloseClaimWinsBeforeShutdown(t *testing.T) {
	probe := publicHostProtocolFixture()
	probe.directory = t.TempDir()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	}()
	probe.mu.Lock()
	owner, _, err := probe.registerFIFOWriterLocked(writer, "owned-path", linuxMeminfoName)
	probe.mu.Unlock()
	if err != nil || owner == nil {
		t.Fatal("original writer registration unavailable", err)
	}
	if probe.claimFIFOWriterClose(owner) {
		t.Fatal("normal close was incorrectly shutdown-claimed")
	}
	if err := probe.finishFIFOWriterClose(owner); err != nil {
		t.Fatal(err)
	}
	probe.fifoObserver = func(context.Context, uintptr) (publicHostFIFOObservation, error) {
		t.Error("Close inspected normally retired descriptor")
		return publicHostFIFOObservation{}, syscall.EBADF
	}
	if err := probe.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if owner.shutdownClaimed || len(probe.fifoWriters) != 0 {
		t.Fatal("Close reclaimed a normal-close winner")
	}
}

func newHeldFIFOProbeControl(ctx context.Context, t *testing.T) (*PublicTUIHostProbe, func() error) {
	t.Helper()
	probe := publicHostProtocolFixture()
	probe.platform, probe.origin = linuxPlatform, time.Now()
	probe.directory = t.TempDir()
	path := filepath.Join(probe.directory, "meminfo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	probe.fifoPaths = []string{path}
	reading, err := probe.HoldNextReading()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	closeReader := func() error {
		if closed {
			return nil
		}
		closed = true
		return reader.Close()
	}
	t.Cleanup(func() {
		if err := closeReader(); err != nil {
			t.Error(err)
		}
		if !probe.Trace().Closed {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			if err := probe.Close(cleanup); err != nil {
				t.Log("retained original failed fixture cleanup", err)
			}
		}
	})
	done := make(chan struct{})
	probe.done = append(probe.done, done)
	go probe.serveFIFO(path, linuxMeminfoName, done)
	if err := probe.WaitForReading(ctx, reading); err != nil {
		t.Fatal(err)
	}
	return probe, closeReader
}

func TestPublicHostProbeFailedFIFOWitnessPreservesActiveAndJoins(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(map[bool]string{false: "syscall failure", true: "original context expiry"}[expire], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			probe, closeReader := newHeldFIFOProbeControl(ctx, t)
			calls := 0
			probe.fifoObserver = func(original context.Context, descriptor uintptr) (publicHostFIFOObservation, error) {
				calls++
				if original != ctx {
					t.Error("witness renewed caller context")
				}
				number, err := nativeTUIDescriptor(descriptor)
				if err != nil {
					return publicHostFIFOObservation{}, err
				}
				var info syscall.Stat_t
				if err := syscall.Fstat(number, &info); err != nil {
					t.Error("original writer closed before failed witness", err)
				}
				if expire {
					cancel()
					return publicHostFIFOObservation{}, errors.Join(ctx.Err(), syscall.EIO)
				}
				return publicHostFIFOObservation{}, syscall.EIO
			}
			if err := closeReader(); err != nil {
				t.Fatal(err)
			}
			closeError := probe.Close(ctx)
			if !errors.Is(closeError, syscall.EIO) || expire && !errors.Is(closeError, context.Canceled) {
				t.Fatal("failed witness cause disappeared", closeError)
			}
			assertFailedFIFOProbeCleanup(ctx, t, probe, calls)
		})
	}
}

func assertFailedFIFOProbeCleanup(ctx context.Context, t *testing.T, probe *PublicTUIHostProbe, calls int) {
	t.Helper()
	// Failure-only cleanup can join the already-decided worker; it cannot rerun
	// the original witness or qualify successful cancellation.
	cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	select {
	case <-probe.done[0]:
	case <-cleanup.Done():
		t.Fatal("failed witness stranded original writer worker")
	}
	if err := probe.Close(cleanup); !errors.Is(err, syscall.EIO) {
		t.Fatal("later cleanup lost original failure", err)
	}
	trace := probe.Trace()
	if trace.Active != 1 || trace.Readings != 1 || trace.MaximumActive != 1 || !trace.Closed || calls != 1 || len(trace.Events) != 1 || len(probe.fifoWriters) != 0 {
		t.Fatalf("failed proof fabricated retirement or leaked ownership: %+v calls=%d writers=%d", trace, calls, len(probe.fifoWriters))
	}
}

func TestPublicHostProbeFIFOWitnessRejectsChangedOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	probe, closeReader := newHeldFIFOProbeControl(ctx, t)
	probe.fifoObserver = func(context.Context, uintptr) (publicHostFIFOObservation, error) {
		probe.mu.Lock()
		defer probe.mu.Unlock()
		for _, owner := range probe.fifoWriters {
			probe.reading++
			number, err := nativeTUIDescriptor(owner.writer.Fd())
			return publicHostFIFOObservation{descriptor: number, events: 0x0008}, err
		}
		return publicHostFIFOObservation{}, syscall.EBADF
	}
	if err := closeReader(); err != nil {
		t.Fatal(err)
	}
	if err := probe.Close(ctx); err == nil {
		t.Fatal("changed reading ownership accepted original writer proof")
	}
	if probe.Trace().Active != 1 || len(probe.Trace().Events) != 1 {
		t.Fatal("foreign proof retired the original reading")
	}
}

func TestPublicHostProbeShutdownClaimKeepsWriterUntilDecision(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	probe, closeReader := newHeldFIFOProbeControl(ctx, t)
	entered := make(chan int, 1)
	release := make(chan struct{})
	probe.fifoObserver = func(original context.Context, descriptor uintptr) (publicHostFIFOObservation, error) {
		number, err := nativeTUIDescriptor(descriptor)
		if err != nil {
			return publicHostFIFOObservation{}, err
		}
		entered <- number
		select {
		case <-release:
		case <-original.Done():
			return publicHostFIFOObservation{}, original.Err()
		}
		return publicHostFIFOObservation{descriptor: number, events: 0x0008}, nil
	}
	if err := closeReader(); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 1)
	done := make(chan struct{})
	go func() { results <- probe.Close(ctx); close(done) }()
	defer func() { close(release); <-done }()
	var descriptor int
	select {
	case descriptor = <-entered:
	case <-ctx.Done():
		t.Fatal("Close did not reach original writer observation")
	}
	var info syscall.Stat_t
	if err := syscall.Fstat(descriptor, &info); err != nil {
		t.Fatal("worker closed shutdown-claimed writer during witness", err)
	}
	before := probe.Trace()
	if before.Active != 1 || len(before.Events) != 1 {
		t.Fatal("pending proof prematurely retired reading", before)
	}
	select {
	case <-probe.done[0]:
		t.Fatal("worker returned before shutdown decision")
	default:
	}
	// The observer remains blocked until this release; use a separate signal so
	// the deferred release cannot double-close the witness channel.
	release <- struct{}{}
	select {
	case err := <-results:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("proof decision did not join original worker")
	}
	after := probe.Trace()
	if after.Active != 0 || len(after.Events) != 2 || after.Events[1].Phase != "canceled-reader-absent" || after.Events[1].Descriptor != descriptor || after.Events[1].PollEvents != 0x0008 {
		t.Fatalf("actual descriptor/mask retirement witness absent: %+v", after)
	}
}

type publicHostResetReader struct{ cause error }

func (reader publicHostResetReader) Read([]byte) (int, error) { return 0, reader.cause }

func TestPublicHostProbeResetMetadataRemainsFatalInBothPhases(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		input := &publicHostRequestInput{reader: publicHostResetReader{cause: syscall.ECONNRESET}}
		decoder := json.NewDecoder(input)
		metadata, beforeError := preparePublicHostDecodeMetadata(decoder, input, publicHostDecodeMetadata{accepted: accepted, reading: 1, pid: 17, parent: 23})
		var message string
		decodeError := decoder.Decode(&message)
		result := finishPublicHostDecodeMetadata(decoder, input, metadata, errors.Join(beforeError, decodeError))
		err := publicHostRetainedDecodeError(result)
		if err == nil {
			t.Fatal("zero-byte reset became successful")
		}
		if !errors.Is(err, syscall.ECONNRESET) {
			t.Fatal("zero-byte reset was exempted or lost identity", err)
		}
		phase := "pre-request"
		identity := "reading=0 pid=0 parent=0"
		if accepted {
			phase = "accepted-ack"
			identity = "reading=1 pid=17 parent=23"
		}
		if !strings.Contains(err.Error(), "phase="+phase) || !strings.Contains(err.Error(), identity) || !strings.Contains(err.Error(), "wire-before=0 wire-after=0") {
			t.Fatal("reset metadata did not retain fixed phase and validated identity", err)
		}
	}
}

func TestPublicHostProbeACKPrefetchRetainsBufferedEvidence(t *testing.T) {
	for _, valid := range []bool{false, true} {
		request := publicHostProtocolRequest()
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		acknowledgement := `"invalid-ack"`
		if valid {
			acknowledgement = `"output-written"`
		}
		input := &publicHostRequestInput{reader: io.LimitReader(strings.NewReader(string(encoded)+"\n"+acknowledgement), 8192)}
		decoder := json.NewDecoder(input)
		decoder.DisallowUnknownFields()
		var received publicHostRequest
		if err := decoder.Decode(&received); err != nil {
			t.Fatal(err)
		}
		probe := publicHostProtocolFixture()
		reading, _, err := probe.acceptDarwinLocked(received)
		if err != nil {
			t.Fatal("prefetched request did not pass original ownership validator", err)
		}
		metadata, beforeError := preparePublicHostDecodeMetadata(decoder, input, publicHostDecodeMetadata{accepted: true, reading: reading, pid: received.PID, parent: received.ParentPID})
		results := make(chan publicHostDecodeResult, 1)
		done := make(chan struct{})
		go func() {
			defer close(done)
			var message string
			decodeError := decoder.Decode(&message)
			results <- finishPublicHostDecodeMetadata(decoder, input, metadata, errors.Join(beforeError, validatePublicHostAcknowledgement(message, decodeError)))
		}()
		result := <-results
		<-done
		snapshot := result.metadata
		if snapshot.before.buffered <= 0 || snapshot.before.received != snapshot.after.received || snapshot.after.offset <= snapshot.before.offset {
			t.Fatalf("zero new socket bytes hid prefetched ACK: before=%+v after=%+v", snapshot.before, snapshot.after)
		}
		retained := publicHostRetainedDecodeError(result)
		if valid {
			if retained != nil {
				t.Fatal("valid prefetched ACK rejected", retained)
			}
		} else {
			if retained == nil || !errors.Is(retained, result.err) {
				t.Fatal("invalid prefetched ACK was exempted", retained)
			}
			for _, private := range []string{request.Nonce, request.Executable, "invalid-ack", "output-written"} {
				if strings.Contains(retained.Error(), private) {
					t.Fatal("decoder metadata disclosed request or ACK payload")
				}
			}
		}
	}
}

func TestPublicHostProbeConcurrentCloseHonorsEachCallerContext(t *testing.T) {
	first, cancelFirst := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancelFirst()
	probe, closeReader := newHeldFIFOProbeControl(first, t)
	entered := make(chan struct{})
	probe.fifoObserver = func(ctx context.Context, _ uintptr) (publicHostFIFOObservation, error) {
		close(entered)
		<-ctx.Done()
		return publicHostFIFOObservation{}, errors.Join(ctx.Err(), syscall.EIO)
	}
	if err := closeReader(); err != nil {
		t.Fatal(err)
	}
	firstResult := make(chan error, 1)
	firstDone := make(chan struct{})
	go func() { firstResult <- probe.Close(first); close(firstDone) }()
	defer func() { cancelFirst(); <-firstDone }()
	select {
	case <-entered:
	case <-first.Done():
		t.Fatal("first caller did not own witness")
	}
	second, cancelSecond := context.WithCancel(t.Context())
	cancelSecond()
	secondResult := make(chan error, 1)
	secondDone := make(chan struct{})
	go func() { secondResult <- probe.Close(second); close(secondDone) }()
	select {
	case err := <-secondResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("second caller lost its original context", err)
		}
	case <-time.After(250 * time.Millisecond):
		cancelFirst()
		<-secondDone
		t.Fatal("second caller blocked behind first witness context")
	}
	<-secondDone
	select {
	case <-firstDone:
		t.Fatal("second caller prematurely completed the first witness")
	default:
	}
	cancelFirst()
	if err := <-firstResult; !errors.Is(err, syscall.EIO) || !errors.Is(err, context.Canceled) {
		t.Fatal("first original error vanished", err)
	}
	cleanup, stop := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
	defer stop()
	if err := probe.Close(cleanup); !errors.Is(err, syscall.EIO) {
		t.Fatal("later cleanup lost original failure", err)
	}
	if probe.Trace().Active != 1 {
		t.Fatal("failed original witness invented successful retirement")
	}
}

func TestPublicHostProbeDarwinRejectsLinuxWriterWitness(t *testing.T) {
	if runtime.GOOS != darwinPlatform {
		return
	}
	observation, err := observePublicHostFIFOReaderAbsent(t.Context(), 42)
	if err == nil || observation != (publicHostFIFOObservation{}) {
		t.Fatal("Darwin fabricated a Linux reader-absence proof", observation, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := observePublicHostFIFOReaderAbsent(ctx, 42); !errors.Is(err, context.Canceled) {
		t.Fatal("Darwin witness lost original expiry", err)
	}
}

func TestPublicHostProbeFIFOWitnessRejectsUnprovenMaskOrDescriptor(t *testing.T) {
	for _, test := range []struct {
		name    string
		events  int16
		foreign bool
	}{
		{"no error bit", 0, false}, {"invalid error combination", 0x0028, false}, {"foreign descriptor", 0x0008, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			probe, closeReader := newHeldFIFOProbeControl(ctx, t)
			probe.fifoObserver = func(_ context.Context, descriptor uintptr) (publicHostFIFOObservation, error) {
				number, err := nativeTUIDescriptor(descriptor)
				if test.foreign {
					number++
				}
				return publicHostFIFOObservation{descriptor: number, events: test.events}, err
			}
			if err := closeReader(); err != nil {
				t.Fatal(err)
			}
			closeError := probe.Close(ctx)
			if closeError == nil || probe.Trace().Active != 1 || !probe.Trace().Closed || len(probe.Trace().Events) != 1 {
				t.Fatal("unproven reader absence accepted", closeError, probe.Trace())
			}
			if test.events&0x0020 != 0 && !errors.Is(closeError, syscall.EBADF) {
				t.Fatal("NVAL witness lost failure identity", closeError)
			}
		})
	}
}
