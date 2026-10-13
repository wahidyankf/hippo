package tui_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/tests/support"
)

// This entry is only the accepted transport/renderer building block. The
// controlled observer does not prove native application observation or BDD.
func TestBDDNativeTeaTransportCurrentFrameNavigationResize(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != "integration" {
		t.Skip("native transport requires explicit integration selection")
	}
	screen := tui.NewBDDNativeTransportScreen(39, 15)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transport, err := support.StartNativeTUI(ctx, support.NativeTUIStart{Mode: "integration", Executable: os.Args[0], Helper: support.NativeTUITeaHelper, Environment: os.Environ(), Width: 39, Height: 15, Feed: screen.Feed, ResizeScreen: screen.Resize, Finish: screen.Finish})
	if err != nil {
		t.Fatal(err)
	}
	registerBDDNativeTransportCleanup(t, transport)
	if err := transport.SendKey(ctx, []byte("q")); err == nil {
		t.Fatal("real child accepted input before readiness")
	}
	if err := transport.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	identity := transport.Result().Identity
	if identity.PID == os.Getpid() || identity.PID <= 0 || identity.Session != identity.PID || identity.Group != identity.PID {
		t.Fatal("readiness was not bound to the owned child session")
	}
	if err := transport.Signal(os.Getpid(), os.Interrupt); err == nil {
		t.Fatal("foreign parent signal was accepted")
	}
	if err := transport.Resize(ctx, 0, 16); err == nil {
		t.Fatal("unknown native viewport was accepted")
	}
	requireBDDNativeTransportFrame(ctx, t, transport, screen, 39, 15, "Terminal too small: need 40x16", "HIPPO | Running and Queue")
	if err := transport.Resize(ctx, 40, 16); err != nil {
		t.Fatal(err)
	}
	requireBDDNativeTransportFrame(ctx, t, transport, screen, 40, 16, "HIPPO | Running and Queue", "Terminal too small")
	if err := transport.SendKey(ctx, []byte("?")); err != nil {
		t.Fatal(err)
	}
	requireBDDNativeTransportFrame(ctx, t, transport, screen, 40, 16, "HIPPO | Help", "HIPPO | Running and Queue")
	// A deadline with an impossible current-view predicate must fail while the
	// process remains alive; scanning the transcript could find the old dashboard.
	deadline, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = awaitBDDNativeTransportFrame(deadline, bddNativeTransportCapture{transport: transport, screen: screen}, func(frame tui.BDDNativeTransportFrame) bool {
		return nativeTransportFrameHas(frame, "HIPPO | Running and Queue", "HIPPO | Help")
	})
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("obsolete dashboard predicate did not time out: %v", err)
	}
	if err := transport.SendKey(ctx, []byte("b")); err != nil {
		t.Fatal(err)
	}
	requireBDDNativeTransportFrame(ctx, t, transport, screen, 40, 16, "HIPPO | Running and Queue", "HIPPO | Help")
	if result := transport.Result(); result.Joined() || result.Reaped || result.OutputEOF || result.ErrorEOF {
		t.Fatal("running child was represented as joined or at EOF")
	}
	if !regexp.MustCompile("\x1b\\[[0-9;:]*m").Match(transport.Result().Stdout) {
		t.Fatal("styled real renderer emitted no SGR evidence")
	}
	if err := transport.SendKey(ctx, []byte("q")); err != nil {
		t.Fatal(err)
	}
	result, err := transport.WaitAndDrain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Joined() || result.ExitCode != 0 {
		t.Fatalf("quit did not reap and drain every owned stream: %+v", result)
	}
	if len(result.Stderr) != 0 {
		t.Fatalf("real helper stderr was not empty: %q", result.Stderr)
	}
}

func registerBDDNativeTransportCleanup(t *testing.T, transport *support.NativeTUITransport) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() {
			first := transport.Result()
			t.Logf("native first-failure identity=%+v reaped=%t stdoutEOF=%t stderrEOF=%t readyEOF=%t stdout=%q stderr=%q", first.Identity, first.Reaped, first.OutputEOF, first.ErrorEOF, first.ReadyEOF, first.Stdout, first.Stderr)
		}
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		closeError := transport.Close(cleanup)
		joined := transport.Result()
		t.Logf("native teardown pid=%d reaped=%t stdoutEOF=%t stderrEOF=%t readyEOF=%t joined=%t", joined.Identity.PID, joined.Reaped, joined.OutputEOF, joined.ErrorEOF, joined.ReadyEOF, joined.Joined())
		if !transport.Result().Joined() {
			t.Errorf("owned native transport did not join: %v; result=%+v", closeError, transport.Result())
		}
		// An earlier decoder error remains the primary failure. Cleanup still
		// joins its readers and preserves the raw output instead of replacing it.
		if closeError != nil && !t.Failed() {
			t.Errorf("close native transport: %v", closeError)
		}
	})
}

func nativeTransportFrameHas(frame tui.BDDNativeTransportFrame, present, absent string) bool {
	content := strings.Join(frame.Rows, "\n")
	if present == "HIPPO | Running and Queue" && (!strings.Contains(content, "Running 3 | Queue 1") || !strings.Contains(content, "LIVE")) {
		return false
	}
	return strings.Contains(content, present) && !strings.Contains(content, absent)
}

type bddNativeTransportObservation interface {
	OutputError() error
	Snapshot() tui.BDDNativeTransportFrame
	Result() support.NativeTUIResult
}

type bddNativeTransportCapture struct {
	transport *support.NativeTUITransport
	screen    *tui.BDDNativeTransportScreen
}

func (capture bddNativeTransportCapture) OutputError() error { return capture.transport.OutputError() }

func (capture bddNativeTransportCapture) Snapshot() tui.BDDNativeTransportFrame {
	return capture.screen.Snapshot()
}

func (capture bddNativeTransportCapture) Result() support.NativeTUIResult {
	return capture.transport.Result()
}

func awaitBDDNativeTransportFrame(ctx context.Context, observation bddNativeTransportObservation, predicate func(tui.BDDNativeTransportFrame) bool) (tui.BDDNativeTransportFrame, error) {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := observation.OutputError(); err != nil {
			return observation.Snapshot(), fmt.Errorf("decode actual renderer output: %w", err)
		}
		frame := observation.Snapshot()
		if err := observation.OutputError(); err != nil {
			return frame, fmt.Errorf("decode actual renderer output: %w", err)
		}
		if err := ctx.Err(); err != nil {
			// Sticky history alone never fails liveness: a transitional
			// write for a retired viewport must wait for the next repaint,
			// which clears the current viewport. Only an overflow that
			// survives until the deadline names the renderer.
			if frame.OverflowWrites != 0 {
				return frame, fmt.Errorf("actual renderer wrote outside the viewport: %w; rows=%q overflow=%s", err, frame.Rows, frame.OverflowEvidence)
			}
			return frame, err
		}
		if predicate(frame) {
			return frame, nil
		}
		if result := observation.Result(); result.OutputEOF {
			return frame, errors.New("output EOF before the current-frame predicate")
		}
		select {
		case <-ctx.Done():
			if frame.OverflowWrites != 0 {
				return frame, fmt.Errorf("actual renderer wrote outside the viewport: %w; rows=%q overflow=%s", ctx.Err(), frame.Rows, frame.OverflowEvidence)
			}
			return frame, ctx.Err()
		case <-ticker.C:
		}
	}
}

func requireBDDNativeTransportFrame(ctx context.Context, t *testing.T, transport *support.NativeTUITransport, screen *tui.BDDNativeTransportScreen, width, height int, present, absent string) {
	t.Helper()
	frame, err := awaitBDDNativeTransportFrame(ctx, bddNativeTransportCapture{transport: transport, screen: screen}, func(frame tui.BDDNativeTransportFrame) bool {
		return frame.Width == width && frame.Height == height && frame.Alternate && nativeTransportFrameHas(frame, present, absent)
	})
	if err != nil {
		t.Fatalf("current %dx%d frame %q: %v; rows=%q", width, height, present, err, frame.Rows)
	}
	t.Logf("native current-frame viewport=%dx%d alternate=%t overflow=%d predicate=%q rows=%q", frame.Width, frame.Height, frame.Alternate, frame.OverflowWrites, present, frame.Rows)
}

func TestBDDNativeTransportCurrentPredicateRejectsHistoricalFrame(t *testing.T) {
	screen := tui.NewBDDNativeTransportScreen(40, 16)
	old := "\x1b[?1049h\x1b[HHIPPO | Running and Queue\x1b[2;1HRunning 3 | Queue 1\x1b[3;1HLIVE"
	current := "\x1b[2J\x1b[HHIPPO | Help"
	if err := screen.Feed([]byte(old)); err != nil {
		t.Fatal(err)
	}
	before := screen.Snapshot()
	if !nativeTransportFrameHas(before, "HIPPO | Running and Queue", "HIPPO | Help") {
		t.Fatal("initial current predicate did not see the dashboard")
	}
	if err := screen.Feed([]byte(current)); err != nil {
		t.Fatal(err)
	}
	if nativeTransportFrameHas(screen.Snapshot(), "HIPPO | Running and Queue", "HIPPO | Help") {
		t.Fatal("obsolete dashboard passed current-frame predicate")
	}
	if !strings.Contains(old+current, "HIPPO | Running and Queue") {
		t.Fatal("historical-transcript negative control lost its discriminating old frame")
	}
	before.Rows[0] = "caller mutation"
	if !nativeTransportFrameHas(screen.Snapshot(), "HIPPO | Help", "caller mutation") {
		t.Fatal("snapshot mutation altered the current screen")
	}
}

func TestBDDNativeTransportScreenPreservesEOFBoundsAndStyles(t *testing.T) {
	t.Run("EOF", func(t *testing.T) {
		screen := tui.NewBDDNativeTransportScreen(40, 16)
		if err := screen.Feed([]byte("\x1b[2;")); err != nil {
			t.Fatal(err)
		}
		if err := screen.Finish(); err == nil {
			t.Fatal("incomplete output at EOF was accepted")
		}
	})
	t.Run("bounds", func(t *testing.T) {
		screen := tui.NewBDDNativeTransportScreen(40, 16)
		if err := screen.Feed([]byte("\x1b[1;41HX")); err != nil {
			t.Fatal(err)
		}
		if screen.Snapshot().OverflowWrites != 1 {
			t.Fatal("requested out-of-bounds write disappeared at clipping")
		}
	})
	t.Run("styles", func(t *testing.T) {
		screen := tui.NewBDDNativeTransportScreen(40, 16)
		if err := screen.Feed([]byte("\x1b[1;36mHIPPO\x1b[0m")); err != nil {
			t.Fatal(err)
		}
		if screen.Snapshot().Rows[0] != "HIPPO" {
			t.Fatal("supported styles changed the plain screen content")
		}
	})
}

func TestBDDNativeTransportAwaitToleratesStaleOverflowUntilRepaint(t *testing.T) {
	screen := tui.NewBDDNativeTransportScreen(40, 16)
	if err := screen.Feed([]byte("\x1b[1;41HX")); err != nil {
		t.Fatal(err)
	}
	// A transitional out-of-bounds write must not fail liveness: the await
	// keeps polling until a repaint clears the current viewport, while the
	// sticky history remains for audit.
	observation := &bddNativePublicationObservation{screen: screen}
	calls := 0
	var hook func()
	hook = func() {
		calls++
		if calls < 2 {
			observation.beforeSnapshot = hook
			return
		}
		if err := screen.Feed([]byte("\x1b[2J")); err != nil {
			t.Errorf("repaint feed failed: %v", err)
		}
	}
	observation.beforeSnapshot = hook
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	frame, err := awaitBDDNativeTransportFrame(ctx, observation, func(frame tui.BDDNativeTransportFrame) bool {
		return frame.CurrentOverflowWrites == 0
	})
	if err != nil {
		t.Fatalf("stale overflow failed liveness before repaint: %v; rows=%q", err, frame.Rows)
	}
	if frame.OverflowWrites != 1 {
		t.Fatalf("await concealed sticky overflow history: %+v", frame)
	}
	if calls < 2 {
		t.Fatalf("await did not poll through stale overflow: calls=%d", calls)
	}
}

type bddNativePublicationObservation struct {
	screen         *tui.BDDNativeTransportScreen
	published      error
	beforeSnapshot func()
}

func (observation *bddNativePublicationObservation) OutputError() error { return observation.published }

func (observation *bddNativePublicationObservation) Snapshot() tui.BDDNativeTransportFrame {
	if observation.beforeSnapshot != nil {
		publish := observation.beforeSnapshot
		observation.beforeSnapshot = nil
		publish()
	}
	return observation.screen.Snapshot()
}

func (*bddNativePublicationObservation) Result() support.NativeTUIResult {
	return support.NativeTUIResult{}
}

func TestBDDNativeTransportAwaitRejectsFailurePublishedBeforeSnapshot(t *testing.T) {
	screen := tui.NewBDDNativeTransportScreen(40, 16)
	if err := screen.Feed([]byte("\x1b[?1049h\x1b[HHIPPO | Help")); err != nil {
		t.Fatal(err)
	}
	original := errors.New("original failure published after the nil error read")
	observation := &bddNativePublicationObservation{screen: screen}
	publications := 0
	observation.beforeSnapshot = func() {
		// Run on the exact Snapshot call after the actual await loop read nil.
		// The unchanged fail-closed decoder retains its matching old rows.
		decoderError := screen.Feed([]byte("\x1b[4a"))
		if decoderError == nil {
			t.Fatal("unsupported renderer operation did not fail the real decoder")
		}
		observation.published = errors.Join(original, decoderError)
		publications++
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	nextActions := 0
	frame, err := awaitBDDNativeTransportFrame(ctx, observation, func(frame tui.BDDNativeTransportFrame) bool {
		return nativeTransportFrameHas(frame, "HIPPO | Help", "HIPPO | Running and Queue")
	})
	if err == nil {
		nextActions++
	}
	if !errors.Is(err, original) {
		t.Errorf("matching sticky-failed frame lost the published cause: %v", err)
	}
	if nextActions != 0 {
		t.Errorf("accepted failed frame dispatched %d next actions", nextActions)
	}
	if publications != 1 || !errors.Is(observation.published, original) {
		t.Fatal("publication did not occur at the controlled read/snapshot boundary")
	}
	if !nativeTransportFrameHas(frame, "HIPPO | Help", "HIPPO | Running and Queue") {
		t.Fatal("sticky-failed decoder did not preserve the discriminating matching rows")
	}
}

func TestBDDNativeTransportAwaitRejectsAlreadyPublishedFailure(t *testing.T) {
	screen := tui.NewBDDNativeTransportScreen(40, 16)
	if err := screen.Feed([]byte("\x1b[?1049h\x1b[HHIPPO | Help")); err != nil {
		t.Fatal(err)
	}
	original := errors.New("owned decoder cause published before waiting")
	decoderError := screen.Feed([]byte("\x1b[4a"))
	if decoderError == nil {
		t.Fatal("positive refusal control lost its sticky decoder failure")
	}
	observation := &bddNativePublicationObservation{screen: screen, published: errors.Join(original, decoderError)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	predicates := 0
	_, err := awaitBDDNativeTransportFrame(ctx, observation, func(tui.BDDNativeTransportFrame) bool { predicates++; return true })
	if !errors.Is(err, original) || predicates != 0 {
		t.Fatalf("prepublished failure was not refused before predicate: %v; predicates=%d", err, predicates)
	}
}

type bddNativeSamplingObservation struct {
	bddNativePublicationObservation

	outputEOF bool
}

func (observation *bddNativeSamplingObservation) Result() support.NativeTUIResult {
	return support.NativeTUIResult{OutputEOF: observation.outputEOF}
}

type bddNativeSamplingCase struct {
	name                                       string
	expired, missing, postError, overflow, eof bool
	width                                      int
	delay                                      time.Duration
	published, wantError                       error
	accepted                                   bool
}

func (test bddNativeSamplingCase) observation(t *testing.T, published error) *bddNativeSamplingObservation {
	t.Helper()
	width := test.width
	if width == 0 {
		width = 40
	}
	screen := tui.NewBDDNativeTransportScreen(width, 16)
	content := "\x1b[?1049h\x1b[HHIPPO | Help\r\nEnter c:"
	if test.missing {
		content = "\x1b[?1049h\x1b[HHIPPO | Help\r\nEnter"
	}
	if test.overflow {
		content += "\x1b[1;41HX"
	}
	if err := screen.Feed([]byte(content)); err != nil {
		t.Fatal(err)
	}
	observation := &bddNativeSamplingObservation{bddNativePublicationObservation: bddNativePublicationObservation{screen: screen, published: test.published}, outputEOF: test.eof}
	observation.beforeSnapshot = func() {
		if test.delay != 0 {
			time.Sleep(test.delay)
		}
		if test.postError {
			observation.published = published
		}
	}
	return observation
}

func TestBDDNativeTransportSamplingDeadlineAndRefusals(t *testing.T) {
	published := errors.New("published renderer failure")
	for _, test := range []bddNativeSamplingCase{
		{name: "matching within original500ms", accepted: true},
		{name: "already expired matching", expired: true, wantError: context.DeadlineExceeded},
		{name: "matching snapshot after505ms", delay: 505 * time.Millisecond, wantError: context.DeadlineExceeded},
		{name: "required label never arrives", missing: true, wantError: context.DeadlineExceeded},
		{name: "already published error", published: published, wantError: published},
		{name: "error published at snapshot", postError: true, wantError: published},
		// A persistent overflow is still refused, but only at the deadline:
		// fail-fast cannot tell a transitional write for a retired
		// viewport from a genuinely broken repaint, so the await polls
		// for the healing repaint and names the overflow on expiry.
		{name: "overflow before matching", overflow: true, wantError: context.DeadlineExceeded},
		{name: "EOF before required label", eof: true, missing: true},
		{name: "wrong geometry", width: 39, eof: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			observation := test.observation(t, published)
			ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
			defer cancel()
			if test.expired {
				cancel()
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Millisecond))
				defer cancel()
			}
			predicates := 0
			frame, err := awaitBDDNativeTransportFrame(ctx, observation, func(frame tui.BDDNativeTransportFrame) bool {
				predicates++
				content := strings.Join(frame.Rows, "\n")
				return frame.Width == 40 && frame.Height == 16 && frame.Alternate && frame.CurrentOverflowWrites == 0 && strings.Contains(content, "HIPPO | Help") && strings.Contains(content, "Enter") && strings.Contains(content, "c:")
			})
			if (err == nil) != test.accepted {
				t.Fatalf("accepted=%t want=%t error=%v; frame=%+v", err == nil, test.accepted, err, frame)
			}
			if test.wantError != nil && !errors.Is(err, test.wantError) {
				t.Fatalf("lost refusal identity: %v; want %v", err, test.wantError)
			}
			if (test.expired || test.delay != 0 || errors.Is(test.wantError, published)) && predicates != 0 {
				t.Fatalf("refused publication reached predicate %d times", predicates)
			}
			if test.overflow && (err == nil || !strings.Contains(err.Error(), "wrote outside the viewport")) {
				t.Fatalf("persistent overflow was not named at refusal: %v", err)
			}
		})
	}
}

type bddNativeCallerSampling struct {
	screen       *tui.BDDNativeTransportScreen
	publication  []byte
	published    error
	snapshots    int
	publications int
}

func (observation *bddNativeCallerSampling) OutputError() error { return observation.published }

func (observation *bddNativeCallerSampling) Snapshot() tui.BDDNativeTransportFrame {
	observation.snapshots++
	if observation.snapshots == 2 && observation.publication != nil {
		observation.published = observation.screen.Feed(observation.publication)
		observation.publications++
		observation.publication = nil
	}
	return observation.screen.Snapshot()
}

func (*bddNativeCallerSampling) Result() support.NativeTUIResult { return support.NativeTUIResult{} }

func samplingCallerFrame(t *testing.T, title, content string) *tui.BDDNativeTransportScreen {
	t.Helper()
	screen := tui.NewBDDNativeTransportScreen(40, 16)
	if err := screen.Feed([]byte("\x1b[?1049h\x1b[2J\x1b[H" + title + "\r\n" + content)); err != nil {
		t.Fatal(err)
	}
	return screen
}

func requireSamplingCaller(t *testing.T, help bool) {
	t.Helper()
	title, anchor, required := "HIPPO | Capacity detail", "Disk free:", "Reservation accounting"
	if help {
		title, anchor, required = "HIPPO | Help", "Enter: open selected job detail", "c:"
	}
	observation := &bddNativeCallerSampling{}
	initial := samplingCallerFrame(t, title, "Tab or n\r\nShift-Tab or p\r\nDown or j\r\nUp or k\r\nb:\r\nr refresh\r\n? help\r\nq quit")
	state := &nativeBDDInteraction{fixture: &nativeBDDViewports{width: 40, height: 16}, kind: "sampling-caller", frames: []tui.BDDNativeTransportFrame{initial.Snapshot()}, samplingObservation: observation}
	var batches []int
	var primaryFrame tui.BDDNativeTransportFrame
	total, quit := 0, 0
	advancedWithoutLabel := false
	state.samplingKey = func(ctx context.Context, value []byte) error {
		deadline, bounded := ctx.Deadline()
		if !bounded || ctx.Err() != nil || time.Until(deadline) > 500*time.Millisecond || len(value) > 64 || strings.Trim(string(value), "j") != "" {
			return errors.New("caller changed its original key deadline or byte batch")
		}
		if total == 8 {
			primaryFrame = state.frames[len(state.frames)-1]
			accepted := strings.Join(primaryFrame.Rows, "\n")
			advancedWithoutLabel = !strings.Contains(accepted, required) || observation.publications != 1
			t.Logf("actual caller accepted frame before next keys: required=%q publications=%d frame=%+v", required, observation.publications, primaryFrame)
			observation.publication = nil
		}
		batches = append(batches, len(value))
		total += len(value)
		switch {
		case total == 8:
			observation.screen = samplingCallerFrame(t, title, anchor)
			observation.publication = []byte("\x1b[2J\x1b[H" + title + "\r\n" + anchor + "\r\n" + required)
		case !help && total == 18:
			observation.screen = samplingCallerFrame(t, title, "Waiting request:")
		default:
			end := "process RSS or CPU usage."
			if help {
				end = "Host prefixes"
			}
			observation.screen = samplingCallerFrame(t, title, end)
		}
		return nil
	}
	state.samplingQuit = func() error {
		quit++
		t.Logf("actual help caller quit after accepted frame: required=%q frame=%+v", required, primaryFrame)
		return nil
	}
	var err error
	if help {
		err = state.requireHelp()
	} else {
		err = state.capacityEvidence()
	}
	if advancedWithoutLabel {
		t.Errorf("actual caller advanced to next keys before accepting current %q frame", required)
	}
	if observation.publications != 1 {
		t.Errorf("actual caller accepted former anchor without waiting for current %q", required)
	}
	if err != nil {
		t.Fatalf("actual caller failed: %v", err)
	}
	wantBatches, wantQuit := []int{8, 10, 64, 64, 64, 8}, 0
	if help {
		wantBatches, wantQuit = []int{8, 64, 64, 64, 8}, 1
	}
	if !slices.Equal(batches, wantBatches) || quit != wantQuit {
		t.Fatalf("actual caller changed key batches or quit: keys=%v quit=%d", batches, quit)
	}
	t.Logf("actual caller original key batches=%v quit=%d", batches, quit)
}

func TestBDDNativeSamplingCapacityCallerWaitsForAccounting(t *testing.T) {
	requireSamplingCaller(t, false)
}

func TestBDDNativeSamplingHelpCallerWaitsForCapacityKey(t *testing.T) {
	requireSamplingCaller(t, true)
}
