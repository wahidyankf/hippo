package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/policy"
)

// BDDNativeTransportFrame crosses the test package boundary as ordinary values.
type BDDNativeTransportFrame struct {
	Rows                  []string
	Width, Height         int
	Alternate             bool
	OverflowWrites        int
	OverflowEvidence      string
	CurrentOverflowWrites int
	FullClearSequence     uint64
}

// BDDNativeTransportScreen wraps the existing fail-closed decoder unchanged.
type BDDNativeTransportScreen struct{ decoder *bddNativeFrameDecoder }

// NewBDDNativeTransportScreen creates a TUI-local screen for raw native output.
func NewBDDNativeTransportScreen(width, height int) *BDDNativeTransportScreen {
	return &BDDNativeTransportScreen{decoder: newBDDNativeFrameDecoder(width, height)}
}

// Feed consumes a single ordered producer's raw PTY bytes.
func (screen *BDDNativeTransportScreen) Feed(chunk []byte) error {
	return screen.decoder.feed(chunk)
}

// Resize applies the transport's ordered native resize to the local decoder.
func (screen *BDDNativeTransportScreen) Resize(width, height int) {
	screen.decoder.resize(width, height)
}

// Finish rejects incomplete sequences at actual output EOF.
func (screen *BDDNativeTransportScreen) Finish() error {
	return screen.decoder.finish()
}

// Snapshot owns its row values after the decoder releases its lock.
func (screen *BDDNativeTransportScreen) Snapshot() BDDNativeTransportFrame {
	frame := screen.decoder.snapshot()
	var evidence strings.Builder
	for index, write := range frame.Overflow[:min(8, len(frame.Overflow))] {
		fmt.Fprintf(&evidence, "write[%d] cell=%d,%d cells=%d viewport=%dx%d bytes=%x; ", index, write.Row, write.Column, write.Cells, write.Width, write.Height, []byte(write.Text))
	}
	return BDDNativeTransportFrame{Rows: frame.Rows, Width: frame.Width, Height: frame.Height, Alternate: frame.Alternate, OverflowWrites: len(frame.Overflow), OverflowEvidence: evidence.String(), CurrentOverflowWrites: len(frame.CurrentOverflow), FullClearSequence: frame.FullClearSequence}
}

// RunBDDNativeTeaHelper runs the private session with genuine Tea and terminal
// capture/restoration. Readiness originates in the first request processed by
// the real event loop; Bubble Tea initializes its input reader before that loop.
// The supplied snapshot is a controlled observer input, never rendered output.
func RunBDDNativeTeaHelper(ctx context.Context, input, output *os.File, ready func() error) error {
	if directory := os.Getenv("HIPPO_TEST_NATIVE_TUI_INPUT_DIR"); directory != "" {
		return runBDDNativeControlledTea(ctx, input, output, ready, directory)
	}
	terminal, err := captureRunnerTerminal(input, output)
	if err != nil {
		return err
	}
	var once sync.Once
	var readyError error
	observe := func(context.Context) (application.ObservationSnapshot, int, error) {
		once.Do(func() { readyError = ready() })
		if readyError != nil {
			return application.ObservationSnapshot{}, 0, readyError
		}
		snapshot := bddSnapshot()
		sampledAt := time.Now()
		snapshot.SampledAt = &sampledAt
		return snapshot, 0, nil
	}
	options := Options{Interval: time.Hour, Styling: true, Input: input, Output: output, Environment: os.Environ()}
	dependencies := runnerDependencies{
		now: time.Now,
		newTimer: func(duration time.Duration) runnerTimer {
			timer := time.NewTimer(duration)
			return runnerTimer{events: timer.C, reset: timer.Reset, stop: timer.Stop}
		},
		newProgram: func(programContext context.Context, state *model, options Options) runnerProgram {
			program := tea.NewProgram(state, tea.WithContext(programContext), tea.WithInput(options.Input), tea.WithOutput(options.Output), tea.WithEnvironment(options.Environment), tea.WithoutSignalHandler())
			return runnerProgram{run: program.Run, send: program.Send, kill: program.Kill}
		},
		restore: terminal.restore,
	}
	_, runError := runSession(ctx, observe, options, dependencies)
	return errors.Join(runError, readyError)
}

// BDDNativeObserverInput contains observer/clock inputs, never rendered output.
// These values are consumed only by the fixed Integration test helper.
type BDDNativeObserverInput struct {
	Sequence     uint64                          `json:"Sequence"`
	Now          time.Time                       `json:"Now"`
	Snapshot     application.ObservationSnapshot `json:"Snapshot"`
	Blocked      bool                            `json:"Blocked"`
	Failed       bool                            `json:"Failed"`
	LateOnCancel bool                            `json:"LateOnCancel"`
}

// BDDNativeObserverEvent records real worker calls and scheduler timer effects.
type BDDNativeObserverEvent struct {
	Kind     string    `json:"Kind"`
	Request  int       `json:"Request"`
	Now      time.Time `json:"Now"`
	Failed   bool      `json:"Failed"`
	Sequence uint64    `json:"Sequence"`
}

// BDDNativeObserverSnapshot supplies controlled observer data, never a model.
func BDDNativeObserverSnapshot() application.ObservationSnapshot {
	snapshot := bddSnapshot()
	// The ordinary input crosses JSON's strict task-class decoder. Private Unit
	// fixtures omit this irrelevant admission field; native inputs carry a member.
	for index := range snapshot.Running {
		snapshot.Running[index].Entry.Class = policy.TaskEphemeral
	}
	for index := range snapshot.Queue {
		snapshot.Queue[index].Entry.Class = policy.TaskEphemeral
	}
	return snapshot
}

type bddNativeControlledObserver struct {
	mutex         sync.Mutex
	input         BDDNativeObserverInput
	journal       *os.File
	root          *os.Root
	requests      int
	timerActive   bool
	timerDeadline time.Time
	ticks         chan time.Time
	pollDone      chan struct{}
	stop          context.CancelFunc
	failure       error
	journalBytes  int
	programDone   chan struct{}
}

// openBDDNativeObserverRoot binds the private fixture to a retained directory.
// Parent-relative opens and identity checks reject a replaced or symlinked root.
func openBDDNativeObserverRoot(directory string) (*os.Root, error) {
	relative, err := filepath.Rel(os.TempDir(), directory)
	if err != nil || !filepath.IsAbs(directory) || !filepath.IsLocal(relative) || filepath.Dir(relative) != "." || !strings.HasPrefix(relative, "hippo-native-input-") {
		return nil, errors.New("native observer input is outside its owned fixture")
	}
	parent, err := os.OpenRoot(os.TempDir())
	if err != nil {
		return nil, err
	}
	info, err := parent.Lstat(relative)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return nil, errors.Join(err, errors.New("native observer input directory is not private"), parent.Close())
	}
	root, err := parent.OpenRoot(relative)
	if err != nil {
		return nil, errors.Join(err, parent.Close())
	}
	bound, err := root.Stat(".")
	if err != nil || !os.SameFile(info, bound) || !bound.IsDir() || bound.Mode().Perm() != 0o700 {
		return nil, errors.Join(err, errors.New("native observer input directory identity changed"), root.Close(), parent.Close())
	}
	stat, ok := bound.Sys().(*syscall.Stat_t)
	if !ok || int64(stat.Uid) != int64(os.Getuid()) {
		return nil, errors.Join(errors.New("native observer input directory has a foreign owner"), root.Close(), parent.Close())
	}
	if err := parent.Close(); err != nil {
		return nil, errors.Join(err, root.Close())
	}
	return root, nil
}

func readBDDNativeObserverInput(root *os.Root) (BDDNativeObserverInput, error) {
	file, err := root.Open("input.json")
	if err != nil {
		return BDDNativeObserverInput{}, err
	}
	input, decodeError := decodeBDDNativeObserverInput(file)
	return input, errors.Join(decodeError, file.Close())
}

func decodeBDDNativeObserverInput(file *os.File) (BDDNativeObserverInput, error) {
	var input BDDNativeObserverInput
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
		return BDDNativeObserverInput{}, errors.New("native observer input is not a bounded regular file")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 2*1024*1024+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return input, errors.New("native observer input has extra or oversized data")
	}
	if input.Sequence == 0 || input.Now.IsZero() {
		return input, errors.New("native observer input lacks sequence or clock")
	}
	return input, nil
}

func newBDDNativeControlledObserver(ctx context.Context, directory string) (*bddNativeControlledObserver, error) {
	root, err := openBDDNativeObserverRoot(directory)
	if err != nil {
		return nil, err
	}
	input, err := readBDDNativeObserverInput(root)
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	journal, err := root.OpenFile("events.jsonl", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	owned, stop := context.WithCancel(ctx)
	state := &bddNativeControlledObserver{input: input, journal: journal, root: root, ticks: make(chan time.Time, 1), pollDone: make(chan struct{}), programDone: make(chan struct{}), stop: stop}
	go state.poll(owned)
	return state, nil
}

// All journal writes hold mutex, preserving the actual request/completion order.
func (state *bddNativeControlledObserver) record(kind string, failed bool) {
	data, err := json.Marshal(BDDNativeObserverEvent{Kind: kind, Request: state.requests, Now: state.input.Now, Failed: failed, Sequence: state.input.Sequence})
	if err != nil {
		state.failure = errors.Join(state.failure, err)
		return
	}
	data = append(data, '\n')
	if state.journalBytes+len(data) > 1024*1024 {
		state.failure = errors.Join(state.failure, errors.New("native observer journal exceeds its bound"))
		return
	}
	n, err := state.journal.Write(data)
	state.journalBytes += n
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	state.failure = errors.Join(state.failure, err)
}

func (state *bddNativeControlledObserver) poll(ctx context.Context) {
	defer close(state.pollDone)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		input, err := readBDDNativeObserverInput(state.root)
		state.mutex.Lock()
		switch {
		case err != nil:
			state.failure = errors.Join(state.failure, err)
		case input.Sequence < state.input.Sequence || input.Now.Before(state.input.Now):
			state.failure = errors.Join(state.failure, errors.New("native observer input moved backwards"))
		default:
			changed := input.Sequence != state.input.Sequence
			state.input = input
			if changed {
				state.record("input applied", false)
			}
			if state.timerActive && !input.Now.Before(state.timerDeadline) {
				state.timerActive = false
				select {
				case state.ticks <- input.Now:
				default:
					state.failure = errors.Join(state.failure, errors.New("native clock tick could not be delivered"))
				}
			}
		}
		state.mutex.Unlock()
	}
}

func (state *bddNativeControlledObserver) now() time.Time {
	state.mutex.Lock()
	defer state.mutex.Unlock()
	return state.input.Now
}

func (state *bddNativeControlledObserver) newTimer(_ time.Duration) runnerTimer {
	return runnerTimer{events: state.ticks, reset: func(duration time.Duration) bool {
		state.mutex.Lock()
		defer state.mutex.Unlock()
		wasActive := state.timerActive
		state.timerActive, state.timerDeadline = true, state.input.Now.Add(duration)
		state.record("timer reset", false)
		return wasActive
	}, stop: func() bool {
		state.mutex.Lock()
		defer state.mutex.Unlock()
		wasActive := state.timerActive
		state.timerActive = false
		return wasActive
	}}
}

func (state *bddNativeControlledObserver) observe(ctx context.Context) (application.ObservationSnapshot, int, error) {
	state.mutex.Lock()
	state.requests++
	state.record("request", false)
	state.mutex.Unlock()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		state.mutex.Lock()
		input, failure := state.input, state.failure
		if !input.Blocked || failure != nil {
			state.record("complete", input.Failed || failure != nil)
			state.mutex.Unlock()
			if input.Failed {
				return application.ObservationSnapshot{}, 0, errors.New("/private/native collector failure")
			}
			return input.Snapshot, 0, failure
		}
		state.mutex.Unlock()
		select {
		case <-ctx.Done():
			state.mutex.Lock()
			state.record("cancelled", true)
			late := state.input.LateOnCancel
			snapshot := state.input.Snapshot
			state.mutex.Unlock()
			if late {
				<-state.programDone
				state.mutex.Lock()
				state.record("late returned", false)
				state.mutex.Unlock()
				return snapshot, 0, nil
			}
			return application.ObservationSnapshot{}, 0, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (state *bddNativeControlledObserver) close() error {
	state.stop()
	<-state.pollDone
	state.mutex.Lock()
	defer state.mutex.Unlock()
	state.record("control joined", state.failure != nil)
	return errors.Join(state.failure, state.journal.Close(), state.root.Close())
}

func runBDDNativeControlledTea(ctx context.Context, input, output *os.File, ready func() error, directory string) error {
	terminal, err := captureRunnerTerminal(input, output)
	if err != nil {
		return err
	}
	observer, err := newBDDNativeControlledObserver(ctx, directory)
	if err != nil {
		return err
	}
	var once sync.Once
	var readyError error
	observe := func(ctx context.Context) (application.ObservationSnapshot, int, error) {
		once.Do(func() { readyError = ready() })
		if readyError != nil {
			return application.ObservationSnapshot{}, 0, readyError
		}
		return observer.observe(ctx)
	}
	options := Options{Interval: 5 * time.Second, Styling: false, Input: input, Output: output, Environment: os.Environ()}
	dependencies := runnerDependencies{
		now: observer.now, newTimer: observer.newTimer,
		newProgram: func(programContext context.Context, state *model, options Options) runnerProgram {
			program := tea.NewProgram(state, tea.WithContext(programContext), tea.WithInput(options.Input), tea.WithOutput(options.Output), tea.WithEnvironment(options.Environment), tea.WithoutSignalHandler())
			return runnerProgram{run: func() (tea.Model, error) {
				result, err := program.Run()
				observer.mutex.Lock()
				observer.record("program returned", err != nil)
				observer.mutex.Unlock()
				close(observer.programDone)
				return result, err
			}, send: program.Send, kill: program.Kill}
		}, restore: terminal.restore,
	}
	_, runError := runSession(ctx, observe, options, dependencies)
	return errors.Join(runError, readyError, observer.close())
}
