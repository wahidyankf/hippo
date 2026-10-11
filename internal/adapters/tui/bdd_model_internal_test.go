package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// BDDTestDriver owns private model effects and scenario observations in tests only.
type BDDTestDriver struct {
	adapter                      contract.Adapter
	state                        *model
	trace                        *modelTrace
	beforeSelection              [2]rowSelection
	beforeSection                section
	beforeView                   viewKind
	beforeOffset, beforeRequests int
	frames                       []string
	textCase                     string
	initialAllocation            coordination.ReservationVector
	initialID                    string
	runner                       *bddRunnerState
	guardFactory                 func(context.Context) (func() error, func() error, error)
	guardProbe, guardFinish      func() error
	observationFactory           func(context.Context, string) (application.ObservationSnapshot, error)
	nativeStepFactory            func(string) any
	nativeClose                  func()
}

var _ contract.Driver = (*BDDTestDriver)(nil)

func (driver *BDDTestDriver) Reset() {
	driver.Close()
	adapter, factory, observation := driver.adapter, driver.guardFactory, driver.observationFactory
	nativeFactory, nativeClose := driver.nativeStepFactory, driver.nativeClose
	*driver = BDDTestDriver{adapter: adapter, guardFactory: factory, observationFactory: observation, nativeStepFactory: nativeFactory, nativeClose: nativeClose}
}

func (driver *BDDTestDriver) Close() {
	if driver.nativeClose != nil {
		driver.nativeClose()
	}
	if driver.runner != nil && driver.runner.cancel != nil {
		driver.runner.cancel(nil)
	}
	if driver.guardFinish != nil {
		_ = driver.guardFinish()
		driver.guardFinish = nil
		driver.guardProbe = nil
	}
}

func bddStep(pattern string, handler any) contract.StepBinding {
	return contract.StepBinding{Pattern: pattern, Handler: handler}
}

func (driver *BDDTestDriver) Bindings() []contract.StepBinding {
	runnerBindings := driver.runnerBindings()
	bindings := make([]contract.StepBinding, 0, 69+len(runnerBindings))
	bindings = append(bindings, []contract.StepBinding{
		bddStep(`^a TUI whose previous observation has just completed$`, driver.prepareCompleted),
		bddStep(`^five seconds elapse after that completion$`, driver.elapseRefresh),
		bddStep(`^exactly one new observation starts and no observation starts before that deadline$`, driver.requireDeadline),
		bddStep(`^one blocked TUI observation$`, driver.prepareBlocked),
		bddStep(`^refresh is requested three times before that observation completes$`, driver.requestThreeRefreshes),
		bddStep(`^no second observation overlaps it and exactly one immediate follow-up starts after completion$`, driver.requireCoalescing),
		bddStep(`^one blocked TUI observation and a selected running job$`, driver.prepareBusySelection),
		bddStep(`^the operator switches section and resizes to 40 by 16 cells$`, driver.navigateBusy),
		bddStep(`^the section and compact layout change before the observation is released$`, driver.requireBusyNavigation),
		bddStep(`^a TUI without a successful observation$`, driver.prepareInitial),
		bddStep(`^its first observation fails$`, driver.failObservation),
		bddStep(`^the display shows unavailable data and a safe error summary with refresh still accessible$`, driver.requireInitialFailure),
		bddStep(`^a successful TUI snapshot with one owner and known allocated resources$`, driver.prepareSuccessfulOwner),
		bddStep(`^the next observation fails$`, driver.failNextObservation),
		bddStep(`^the same owner and allocation remain visible with STALE sample age and a safe error summary$`, driver.requireStale),
		bddStep(`^a stale TUI snapshot after an observation failure$`, driver.prepareStale),
		bddStep(`^a later observation succeeds$`, driver.recoverObservation),
		bddStep(`^the new snapshot replaces the old data and STALE and ERROR markers disappear$`, driver.requireRecovery),
		bddStep(`^a successful observation whose second host reading was measured ten seconds ago$`, driver.prepareAged),
		bddStep(`^one lightweight age tick elapses before the next refresh deadline$`, driver.elapseAgeTick),
		bddStep(`^the displayed sample age advances and the host collector is not called again$`, driver.requireAgeTick),
		bddStep(`^a successful observation whose second host measurement time is (absent|malformed)$`, driver.prepareUnknownAge),
		bddStep(`^the TUI displays its sample age$`, driver.renderCurrent),
		bddStep(`^sample age is unavailable instead of using refresh completion time$`, driver.requireUnknownAge),
		bddStep(`^the operator quits and the collector receives cancellation$`, driver.quitBusyRunner),
		bddStep(`^the TUI exits after its observation and timing work finish and no late result updates its display$`, driver.requireBusyRunnerJoined),
		bddStep(`^known zero reservation demand and unavailable optional CPU memory swap and disk readings$`, driver.prepareMissingResources),
		bddStep(`^capacity detail is displayed$`, driver.openCapacity),
		bddStep(`^reservation demand says zero and the missing host readings say unavailable$`, driver.requireMissingResources),
		bddStep(`^a job allocated two CPUs and one GiB with host CPU utilization at twenty percent$`, driver.prepareSeparateResources),
		bddStep(`^job and capacity details are inspected$`, driver.inspectResources),
		bddStep(`^reservation allocation and machine utilization have distinct labels without claiming process CPU or RSS$`, driver.requireSeparateResources),
		bddStep(`^a populated TUI at (\d+) columns and (\d+) rows$`, driver.prepareViewport),
		bddStep(`^its current view is rendered$`, driver.renderCurrent),
		bddStep(`^every line fits (\d+) terminal cells and the complete view fits (\d+) rows$`, driver.requireBounds),
		bddStep(`^the display uses the (minimum-size|compact|wide) layout$`, driver.requireLayout),
		bddStep(`^a selected live job at 40 columns and 16 rows$`, driver.prepareSmallSelection),
		bddStep(`^the terminal shrinks to 39 by 15 cells and returns to 40 by 16 cells$`, driver.shrinkAndRecover),
		bddStep(`^the fitted minimum-size message gives way to the compact view with the same selected run ID$`, driver.requireSizeRecovery),
		bddStep(`^a TUI row and detail containing (an overlong unbroken value|wide Unicode characters|combining Unicode characters|ANSI escapes and control bytes)$`, driver.prepareUntrustedText),
		bddStep(`^they are rendered at 40 columns and 16 rows$`, driver.renderUntrustedText),
		bddStep(`^controls are not displayed and each row and detail fits its terminal-cell and row budget$`, driver.requireSafeFrames),
		bddStep(`^one thousand running and queued jobs with a selected live run ID$`, driver.prepareLargeLists),
		bddStep(`^the operator navigates and repeatedly resizes between compact and wide views$`, driver.navigateLargeLists),
		bddStep(`^the selected run ID stays visible and each list shows its bounded position and count$`, driver.requireLargeLists),
		bddStep(`^three visible jobs with the middle run ID selected$`, driver.prepareMiddleSelection),
		bddStep(`^a refresh removes that selected job$`, driver.removeSelected),
		bddStep(`^the nearest surviving row is selected by its prior position and remains visible$`, driver.requireNearestSelection),
		bddStep(`^job detail is open for a live run ID$`, driver.prepareJobDetail),
		bddStep(`^a refresh removes that run ID$`, driver.removeDetail),
		bddStep(`^detail reports job no longer active and back returns to a usable section$`, driver.requireMissingDetail),
		bddStep(`^a TUI whose active job panel is empty$`, driver.prepareEmpty),
		bddStep(`^the operator requests job detail$`, driver.requestJobDetail),
		bddStep(`^the explicit empty panel remains usable with no selected job$`, driver.requireEmpty),
		bddStep(`^a populated TUI with the middle running job selected$`, driver.prepareMiddleSelection),
		bddStep(`^the operator presses (Tab|n|Shift-Tab|p|Down|j|Up|k|Enter|c|\?) on the dashboard$`, driver.pressDashboard),
		bddStep(`^the TUI performs (.+)$`, driver.requireAction),
		bddStep(`^the TUI (job detail|capacity detail|help) is open with scrollable content$`, driver.prepareScrollable),
		bddStep(`^the operator presses (Down|j|Up|k|b|r|x|Ctrl-Space|Ctrl-h|Ctrl-j|Ctrl-k|Ctrl-l|Ctrl-backslash|Escape)$`, driver.pressKey),
		bddStep(`^the TUI (dashboard|job detail|capacity detail|help|minimum-size) is open$`, driver.prepareNamedView),
		bddStep(`^one refresh is requested without leaving that view$`, driver.requireRefreshInView),
		bddStep(`^a populated TUI$`, driver.preparePopulated),
		bddStep(`^keyboard help is opened$`, driver.openHelp),
		bddStep(`^every section selection detail capacity back refresh help and quit action has a documented usable key$`, driver.requireHelp),
		bddStep(`^a populated TUI with a selected run ID$`, driver.preparePopulated),
		bddStep(`^no job action or TUI quit occurs and selection is unchanged$`, driver.requireIgnoredKey),
		bddStep(`^color is disabled and the TUI has selected waiting stale and error states$`, driver.prepareNoColor),
		bddStep(`^the dashboard and details are rendered$`, driver.renderAllViews),
		bddStep(`^text markers distinguish those states without ANSI styling$`, driver.requireNoColor),
	}...)
	bindings = append(bindings, runnerBindings...)
	if driver.adapter.Name != contract.Unit && driver.nativeStepFactory != nil {
		for index := range bindings {
			if handler := driver.nativeStepFactory(bindings[index].Pattern); handler != nil {
				bindings[index].Handler = handler
			}
		}
	}
	return bindings
}

func (driver *BDDTestDriver) requireUnit() error {
	if driver.adapter.Name != contract.Unit {
		return fmt.Errorf("TUI %s requires actual native/public scenario transport; private model fallback refused", driver.adapter.Name)
	}
	return nil
}

func bddSnapshot() application.ObservationSnapshot {
	measured := modelContractTime(100)
	vector := coordination.ReservationVector{CPU: 2, MemoryBytes: policy.GiB}
	rows := make([]application.ObservationRow, 3)
	for i := range rows {
		rows[i] = application.ObservationRow{Entry: coordination.ReservationEntry{RunID: fmt.Sprintf("%032x", i+1), Source: fmt.Sprintf("run-%d", i), Allocated: vector, Requested: vector, Tags: map[string]string{"long": strings.Repeat("longword", 160)}}, AllocatedKnown: true, RequestedKnown: true}
	}
	return application.ObservationSnapshot{SampledAt: &measured, Running: rows, Queue: []application.ObservationRow{{Entry: coordination.ReservationEntry{RunID: strings.Repeat("b", 32), Source: "queued-job", Position: 3, Requested: vector}, RequestedKnown: true}}, CapacityKnown: true, UsageKnown: true, QueueRegistered: true, Status: application.StatusView{Sample: policy.Sample{CPUUtilizationPercent: new(20.0), AvailableMemoryBytes: new(4 * policy.GiB), AvailableParallelism: 8, DiskFreeBytes: new(12 * policy.GiB)}, Coordination: coordination.ReservationTotals{Mode: "reservation", ActiveOwners: 3, WaitingOwners: 1, Capacity: coordination.ReservationVector{CPU: 8, MemoryBytes: 8 * policy.GiB}, Allocated: vector, Waiting: vector}}}
}

func (driver *BDDTestDriver) prepareInitial() error {
	if err := driver.requireUnit(); err != nil {
		return err
	}
	driver.state, driver.trace = newModelContract()
	driver.state.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	driver.state.Update(startMsg{now: modelContractTime(99)})
	return nil
}

func (driver *BDDTestDriver) preparePopulated() error {
	if err := driver.prepareInitial(); err != nil {
		return err
	}
	modelComplete(driver.state, 1, 110, bddSnapshot(), nil)
	driver.remember()
	return nil
}

func (driver *BDDTestDriver) remember() {
	driver.beforeSelection = driver.state.selected
	driver.beforeSection = driver.state.section
	driver.beforeView = driver.state.view
	driver.beforeOffset = driver.state.body.YOffset()
	driver.beforeRequests = len(driver.trace.requests)
}

func (driver *BDDTestDriver) prepareCompleted() error { return driver.preparePopulated() }
func (driver *BDDTestDriver) elapseRefresh() error {
	state := driver.state
	if state == nil {
		return errors.New("refresh has no model")
	}
	state.Update(clockMsg{now: modelContractTime(114), generation: 1, refreshDue: true})
	if len(driver.trace.requests) != 1 {
		return errors.New("observation started before five-second deadline")
	}
	state.Update(clockMsg{now: modelContractTime(115), generation: 1, refreshDue: true})
	return nil
}

func (driver *BDDTestDriver) requireDeadline() error {
	if len(driver.trace.requests) != 2 || len(driver.trace.schedules) != 1 || !driver.trace.schedules[0].deadline.Equal(modelContractTime(115)) {
		return errors.New("refresh deadline/count differs from completion plus five seconds")
	}
	return nil
}
func (driver *BDDTestDriver) prepareBlocked() error { return driver.prepareInitial() }
func (driver *BDDTestDriver) requestThreeRefreshes() error {
	for range 3 {
		driver.state.Update(modelKey('r'))
	}
	return nil
}

func (driver *BDDTestDriver) requireCoalescing() error {
	if len(driver.trace.requests) != 1 || !driver.state.busy || !driver.state.pending {
		return errors.New("blocked observation overlapped or lost pending refresh")
	}
	modelComplete(driver.state, 1, 110, bddSnapshot(), nil)
	if len(driver.trace.requests) != 2 || !driver.state.busy || driver.state.pending || len(driver.trace.schedules) != 0 {
		return errors.New("completion did not consume exactly one immediate follow-up")
	}
	return nil
}

func (driver *BDDTestDriver) prepareBusySelection() error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	driver.state.Update(modelKey('r'))
	return nil
}

func (driver *BDDTestDriver) navigateBusy() error {
	driver.state.Update(modelKey('n'))
	driver.state.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	return nil
}

func (driver *BDDTestDriver) requireBusyNavigation() error {
	if !driver.state.busy || driver.state.section != queueSection || driver.state.width != 40 || driver.state.height != 16 || len(driver.trace.requests) != 2 {
		return errors.New("navigation/resize blocked on outstanding observation")
	}
	return driver.requireBounds(40, 16)
}

func (driver *BDDTestDriver) failObservation() error {
	modelComplete(driver.state, driver.state.generation, 111, application.ObservationSnapshot{}, errors.New("/private/unsafe\x1b[31m"))
	return nil
}

func (driver *BDDTestDriver) failNextObservation() error {
	driver.state.Update(modelKey('r'))
	return driver.failObservation()
}

func (driver *BDDTestDriver) requireInitialFailure() error {
	content := driver.state.View().Content
	if driver.state.hasSnapshot || driver.state.stale || !strings.Contains(content, "No observation") || !strings.Contains(content, "unavailable") || !strings.Contains(content, "ERROR") || strings.Contains(content, "/private/") {
		return errors.New("initial failure invented accounting or leaked raw error")
	}
	before := len(driver.trace.requests)
	driver.state.Update(modelKey('r'))
	if len(driver.trace.requests) != before+1 {
		return errors.New("refresh inaccessible after initial failure")
	}
	return nil
}

func (driver *BDDTestDriver) prepareSuccessfulOwner() error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	driver.state.snapshot.Running = driver.state.snapshot.Running[:1]
	driver.initialID = driver.state.snapshot.Running[0].Entry.RunID
	driver.initialAllocation = driver.state.snapshot.Running[0].Entry.Allocated
	return nil
}

func (driver *BDDTestDriver) requireStale() error {
	state := driver.state
	if !state.hasSnapshot || !state.stale || len(state.snapshot.Running) != 1 || state.snapshot.Running[0].Entry.RunID != driver.initialID || state.snapshot.Running[0].Entry.Allocated != driver.initialAllocation {
		return errors.New("failed refresh changed last-good owner/allocation")
	}
	return driver.requireText(state.View().Content, "STALE", "ERROR", "age", "Observation unavailable")
}

func (driver *BDDTestDriver) prepareStale() error {
	if err := driver.prepareSuccessfulOwner(); err != nil {
		return err
	}
	return driver.failNextObservation()
}

func (driver *BDDTestDriver) recoverObservation() error {
	driver.state.Update(modelKey('r'))
	snapshot := bddSnapshot()
	snapshot.Running[0].Entry.RunID = "replacement-owner"
	modelComplete(driver.state, driver.state.generation, 112, snapshot, nil)
	return nil
}

func (driver *BDDTestDriver) requireRecovery() error {
	state := driver.state
	if state.stale || state.errorSummary != "" || state.snapshot.Running[0].Entry.RunID != "replacement-owner" || strings.Contains(state.View().Content, "STALE") || strings.Contains(state.View().Content, "ERROR") {
		return errors.New("successful observation did not replace stale data/markers")
	}
	return nil
}
func (driver *BDDTestDriver) prepareAged() error { return driver.preparePopulated() }
func (driver *BDDTestDriver) elapseAgeTick() error {
	driver.state.Update(clockMsg{now: modelContractTime(111), generation: 1})
	return nil
}

func (driver *BDDTestDriver) requireAgeTick() error {
	age, known := driver.state.sampleAge()
	if !known || age != 11*time.Second || len(driver.trace.requests) != 1 {
		return errors.New("age tick recollected or used wrong measurement")
	}
	return driver.requireText(driver.state.View().Content, "11s")
}

func (driver *BDDTestDriver) prepareUnknownAge(value string) error {
	if value != "absent" && value != "malformed" {
		return fmt.Errorf("unknown time case %q", value)
	}
	if driver.observationFactory == nil {
		return errors.New("unknown age has no real application observation factory")
	}
	if err := driver.prepareInitial(); err != nil {
		return err
	}
	snapshot, err := driver.observationFactory(context.Background(), value)
	if err != nil {
		return err
	}
	modelComplete(driver.state, 1, 110, snapshot, nil)
	return nil
}

func (driver *BDDTestDriver) requireUnknownAge() error {
	age, known := driver.state.sampleAge()
	if known || age != 0 {
		return errors.New("unknown sample time replaced with completion time")
	}
	return driver.requireText(driver.state.View().Content, "age unavailable")
}

func (driver *BDDTestDriver) prepareMissingResources() error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	driver.state.snapshot.Status.Sample = policy.Sample{}
	driver.state.snapshot.Status.Coordination.Allocated = coordination.ReservationVector{}
	driver.state.snapshot.Status.Coordination.Waiting = coordination.ReservationVector{}
	return nil
}

func (driver *BDDTestDriver) openCapacity() error {
	driver.state.Update(modelKey('c'))
	return driver.renderCurrent()
}

func (driver *BDDTestDriver) requireMissingResources() error {
	lines := strings.Join(driver.state.bodyLines(), "\n")
	return driver.requireText(lines, "Host CPU utilization: unavailable", "Available RAM: unavailable", "Swap used: unavailable", "Disk free: unavailable", "Waiting request: 0CPU", "Allocated: 0CPU")
}
func (driver *BDDTestDriver) prepareSeparateResources() error { return driver.preparePopulated() }
func (driver *BDDTestDriver) inspectResources() error {
	driver.state.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	driver.frames = append(driver.frames, strings.Join(driver.state.bodyLines(), "\n"))
	driver.state.Update(modelKey('b'))
	driver.state.Update(modelKey('c'))
	driver.frames = append(driver.frames, strings.Join(driver.state.bodyLines(), "\n"))
	return nil
}

func (driver *BDDTestDriver) requireSeparateResources() error {
	if len(driver.frames) != 2 {
		return errors.New("job/capacity subjects were not inspected")
	}
	if err := driver.requireText(driver.frames[0], "Allocated: 2CPU", "1.0GiB"); err != nil {
		return err
	}
	return driver.requireText(driver.frames[1], "Host CPU utilization: 20%", "Reservation accounting", "not process RSS or CPU usage")
}

func (driver *BDDTestDriver) prepareViewport(width, height int) error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	driver.state.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return nil
}

func (driver *BDDTestDriver) renderCurrent() error {
	if driver.state == nil {
		return errors.New("no prepared model for rendering")
	}
	driver.frames = append(driver.frames, driver.state.View().Content)
	return nil
}

func (driver *BDDTestDriver) requireBounds(width, height int) error {
	frame := driver.state.View()
	if !frame.AltScreen {
		return errors.New("frame lost terminal ownership metadata")
	}
	lines := strings.Split(frame.Content, "\n")
	if len(lines) > height {
		return fmt.Errorf("view rows %d exceeds %d", len(lines), height)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > width {
			return fmt.Errorf("view width %d exceeds %d", lipgloss.Width(line), width)
		}
	}
	return nil
}

func (driver *BDDTestDriver) requireLayout(layout string) error {
	content := driver.state.View().Content
	switch layout {
	case "minimum-size":
		return driver.requireText(content, "Terminal too small", "40x16")
	case "compact":
		if strings.Contains(content, "queued-job") {
			return errors.New("compact layout rendered inactive queue rows")
		}
		return driver.requireText(content, "[Running]")
	case "wide":
		return driver.requireText(content, "queued-job", "Running", "Queue")
	}
	return fmt.Errorf("unknown layout %q", layout)
}

func (driver *BDDTestDriver) prepareSmallSelection() error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	driver.state.Update(modelKey('j'))
	driver.remember()
	return nil
}

func (driver *BDDTestDriver) shrinkAndRecover() error {
	driver.state.Update(tea.WindowSizeMsg{Width: 39, Height: 15})
	if err := driver.requireBounds(39, 15); err != nil {
		return err
	}
	if err := driver.requireLayout("minimum-size"); err != nil {
		return err
	}
	driver.state.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	return nil
}

func (driver *BDDTestDriver) requireSizeRecovery() error {
	if driver.state.selected != driver.beforeSelection {
		return errors.New("size recovery changed complete live selection")
	}
	if err := driver.requireBounds(40, 16); err != nil {
		return err
	}
	return driver.requireLayout("compact")
}

func (driver *BDDTestDriver) prepareUntrustedText(value string) error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	text := map[string]string{"an overlong unbroken value": strings.Repeat("unbroken", 160), "wide Unicode characters": strings.Repeat("\u754c", 80), "combining Unicode characters": strings.Repeat("e\u0301", 80), "ANSI escapes and control bytes": "safe\x1b[31mred\x1b]52;c;private-payload\x07\r\t"}[value]
	if text == "" {
		return fmt.Errorf("unknown text %q", value)
	}
	driver.textCase = value
	driver.state.snapshot.Running[0].Entry.Source = text
	driver.state.snapshot.Running[0].Entry.Tags = map[string]string{"text": text}
	return nil
}

func (driver *BDDTestDriver) renderUntrustedText() error {
	if err := driver.renderCurrent(); err != nil {
		return err
	}
	driver.state.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for range 200 {
		if err := driver.renderCurrent(); err != nil {
			return err
		}
		driver.state.Update(modelKey('j'))
	}
	return nil
}

func (driver *BDDTestDriver) requireSafeFrames() error {
	if len(driver.frames) < 2 {
		return errors.New("row/detail were not rendered")
	}
	for _, frame := range driver.frames {
		if strings.ContainsAny(frame, "\x1b\x07\r\t") || strings.Contains(frame, "private-payload") {
			return errors.New("unsafe terminal payload escaped rendered frame")
		}
		lines := strings.Split(frame, "\n")
		if len(lines) > 16 {
			return errors.New("unsafe detail escaped row budget")
		}
		for _, line := range lines {
			if lipgloss.Width(line) > 40 {
				return errors.New("unsafe detail escaped cell budget")
			}
		}
	}
	return nil
}

func (driver *BDDTestDriver) prepareLargeLists() error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	snapshot := bddSnapshot()
	snapshot.Running, snapshot.Queue = nil, nil
	for i := range 1000 {
		snapshot.Running = append(snapshot.Running, application.ObservationRow{Entry: coordination.ReservationEntry{RunID: fmt.Sprintf("%032x", i+1), Source: fmt.Sprintf("run-%04d", i)}})
		snapshot.Queue = append(snapshot.Queue, application.ObservationRow{Entry: coordination.ReservationEntry{RunID: fmt.Sprintf("%032x", i+1001), Source: fmt.Sprintf("queue-%04d", i), Position: i + 1}})
	}
	driver.state.Update(modelKey('r'))
	modelComplete(driver.state, 2, 112, snapshot, nil)
	for range 777 {
		driver.state.Update(modelKey('j'))
	}
	driver.remember()
	return nil
}

func (driver *BDDTestDriver) navigateLargeLists() error {
	for _, size := range [][2]int{{40, 16}, {100, 30}, {60, 20}, {80, 24}} {
		driver.state.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if driver.state.selected[runningSection].runID != driver.beforeSelection[runningSection].runID {
			return errors.New("resize lost selected full ID")
		}
		if err := driver.requireBounds(size[0], size[1]); err != nil {
			return err
		}
		if err := driver.requireText(driver.state.View().Content, "run-0777", "1000", ">"); err != nil {
			return err
		}
	}
	driver.state.Update(modelKey('n'))
	driver.state.Update(modelKey('j'))
	return nil
}

func (driver *BDDTestDriver) requireLargeLists() error {
	return driver.requireText(driver.state.View().Content, "queue-0001", "1000", ">")
}

func (driver *BDDTestDriver) prepareMiddleSelection() error { return driver.prepareSmallSelection() }

func (driver *BDDTestDriver) removeSelected() error {
	snapshot := driver.state.snapshot
	snapshot.Running = append(snapshot.Running[:1:1], snapshot.Running[2:]...)
	driver.state.Update(modelKey('r'))
	modelComplete(driver.state, driver.state.generation, 112, snapshot, nil)
	return nil
}

func (driver *BDDTestDriver) requireNearestSelection() error {
	selected := driver.state.selected[runningSection]
	if selected.index != 1 || selected.runID != fmt.Sprintf("%032x", 3) {
		return errors.New("removed middle selection did not move to nearest surviving prior index")
	}
	return driver.requireText(driver.state.View().Content, "run-2", ">")
}

func (driver *BDDTestDriver) prepareJobDetail() error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	driver.state.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return nil
}

func (driver *BDDTestDriver) removeDetail() error {
	snapshot := driver.state.snapshot
	snapshot.Running = nil
	driver.state.Update(modelKey('r'))
	modelComplete(driver.state, driver.state.generation, 112, snapshot, nil)
	return nil
}

func (driver *BDDTestDriver) requireMissingDetail() error {
	if err := driver.requireText(driver.state.View().Content, "job no longer active"); err != nil {
		return err
	}
	driver.state.Update(modelKey('b'))
	if driver.state.view != dashboardView {
		return errors.New("missing detail traps back")
	}
	return driver.requireText(driver.state.View().Content, "No running jobs")
}

func (driver *BDDTestDriver) prepareEmpty() error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	driver.state.snapshot.Running = nil
	driver.state.reconcileSelection(runningSection, nil)
	return nil
}

func (driver *BDDTestDriver) requestJobDetail() error {
	driver.state.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return nil
}

func (driver *BDDTestDriver) requireEmpty() error {
	if driver.state.view != dashboardView || driver.state.selected[runningSection].runID != "" {
		return errors.New("empty panel fabricated selected detail")
	}
	return driver.requireText(driver.state.View().Content, "No running jobs")
}

func bddKey(value string) (tea.KeyPressMsg, error) {
	keys := map[string]tea.KeyPressMsg{"Tab": {Code: tea.KeyTab}, "Shift-Tab": {Code: tea.KeyTab, Mod: tea.ModShift}, "Down": {Code: tea.KeyDown}, "Up": {Code: tea.KeyUp}, "Enter": {Code: tea.KeyEnter}, "Ctrl-Space": {Code: tea.KeySpace, Mod: tea.ModCtrl}, "Ctrl-backslash": {Code: '\\', Mod: tea.ModCtrl}, "Escape": {Code: tea.KeyEscape}}
	if key, ok := keys[value]; ok {
		return key, nil
	}
	if strings.HasPrefix(value, "Ctrl-") && len(value) == 6 {
		return tea.KeyPressMsg{Code: rune(value[5]), Mod: tea.ModCtrl}, nil
	}
	if len(value) == 1 {
		return modelKey(rune(value[0])), nil
	}
	return tea.KeyPressMsg{}, fmt.Errorf("unknown canonical key %q", value)
}
func (driver *BDDTestDriver) pressDashboard(value string) error { return driver.pressKey(value) }
func (driver *BDDTestDriver) pressKey(value string) error {
	key, err := bddKey(value)
	if err != nil {
		return err
	}
	driver.remember()
	driver.state.Update(key)
	return nil
}

func (driver *BDDTestDriver) requireAction(action string) error {
	state := driver.state
	switch action {
	case "select the next section", "select the previous section":
		if state.section == driver.beforeSection {
			return errors.New("section key did not navigate")
		}
	case "select the next job":
		if state.selected[state.section].index != driver.beforeSelection[state.section].index+1 {
			return errors.New("next job key selected wrong row")
		}
	case "select the previous job":
		if state.selected[state.section].index != driver.beforeSelection[state.section].index-1 {
			return errors.New("previous job key selected wrong row")
		}
	case "open the selected job detail":
		if state.view != jobView || state.detailID != driver.beforeSelection[state.section].runID {
			return errors.New("Enter opened wrong job identity")
		}
	case "open capacity detail":
		if state.view != capacityView {
			return errors.New("capacity key reached wrong view")
		}
	case "open keyboard help":
		if state.view != helpView {
			return errors.New("help key reached wrong view")
		}
	case "scroll one row down":
		if state.body.YOffset() != driver.beforeOffset+1 {
			return errors.New("scroll down did not advance one row")
		}
	case "scroll one row up":
		if state.body.YOffset() != driver.beforeOffset-1 {
			return errors.New("scroll up did not retreat one row")
		}
	case "restore the previous section and selection", "restore the previous view and selection":
		if state.view != dashboardView || state.section != driver.beforeSection || state.selected != driver.beforeSelection {
			return errors.New("back lost previous destination or selection")
		}
	default:
		return fmt.Errorf("unknown canonical action %q", action)
	}
	return driver.requireBounds(state.width, state.height)
}

func (driver *BDDTestDriver) prepareScrollable(view string) error {
	if err := driver.prepareNamedView(view); err != nil {
		return err
	}
	driver.state.Update(modelKey('j'))
	driver.state.Update(modelKey('j'))
	driver.remember()
	return nil
}

func (driver *BDDTestDriver) prepareNamedView(view string) error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	switch view {
	case "dashboard":
	case "job detail":
		driver.state.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "capacity detail":
		driver.state.Update(modelKey('c'))
	case "help":
		driver.state.Update(modelKey('?'))
	case "minimum-size":
		driver.state.Update(tea.WindowSizeMsg{Width: 39, Height: 15})
	default:
		return fmt.Errorf("unknown canonical view %q", view)
	}
	driver.remember()
	return nil
}

func (driver *BDDTestDriver) requireRefreshInView() error {
	if driver.state.view != driver.beforeView || len(driver.trace.requests) != driver.beforeRequests+1 {
		return errors.New("refresh left view or requested wrong count")
	}
	return nil
}
func (driver *BDDTestDriver) openHelp() error { driver.state.Update(modelKey('?')); return nil }
func (driver *BDDTestDriver) requireHelp() error {
	content := strings.Join(driver.state.bodyLines(), "\n")
	return driver.requireText(content, "Tab or n", "Shift-Tab or p", "Down or j", "Up or k", "Enter", "c:", "b:", "r refresh", "? help", "q quit")
}

func (driver *BDDTestDriver) requireIgnoredKey() error {
	if driver.state.quitting || driver.state.view != driver.beforeView || driver.state.section != driver.beforeSection || driver.state.selected != driver.beforeSelection || len(driver.trace.requests) != driver.beforeRequests {
		return errors.New("unrelated host key captured product action")
	}
	return nil
}

func (driver *BDDTestDriver) prepareNoColor() error {
	if err := driver.preparePopulated(); err != nil {
		return err
	}
	driver.state.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return driver.failNextObservation()
}

func (driver *BDDTestDriver) renderAllViews() error {
	if err := driver.renderCurrent(); err != nil {
		return err
	}
	driver.state.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return driver.renderCurrent()
}

func (driver *BDDTestDriver) requireNoColor() error {
	if len(driver.frames) != 2 {
		return errors.New("dashboard/detail not rendered")
	}
	for _, frame := range driver.frames {
		if strings.Contains(frame, "\x1b") {
			return errors.New("NO_COLOR retained ANSI styling")
		}
	}
	return driver.requireText(driver.frames[0], "ACTIVE", "WAIT", "STALE", "ERROR", ">")
}

func (*BDDTestDriver) requireText(content string, words ...string) error {
	for _, word := range words {
		if !strings.Contains(content, word) {
			return fmt.Errorf("actual rendered content omits %q: %q", word, content)
		}
	}
	return nil
}
