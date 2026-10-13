package tui_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/cucumber/godog"
	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// This increment executes original canonical viewport rows. Its result cannot
// replace the complete Integration suite or the public-binary E2E boundary.
func TestTUIBDDNativeViewportCanonical(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.Integration {
		t.Skip("native viewport rows require explicit Integration")
	}
	driver := runNativeCanonicalSelection(t, "@tui-views-01", map[string]int{"Each viewport uses a bounded appropriate layout": 10}, 10, 40)
	requireNativeCanonicalSessions(t, driver)
}

func TestTUIBDDNativeRefreshCanonical(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.Integration {
		t.Skip("native refresh rows require explicit Integration")
	}
	var tags []string
	for index := 1; index <= 9; index++ {
		tags = append(tags, fmt.Sprintf("@tui-refresh-%02d", index))
	}
	names := map[string]int{
		"Default refresh starts five seconds after completion":         1,
		"Repeated refresh requests coalesce while observation is busy": 1,
		"Navigation and resize remain usable during collection":        1,
		"Initial observation failure does not invent empty accounting": 1,
		"Failed refresh preserves the last good snapshot as stale":     1,
		"Successful refresh clears stale and error markers":            1,
		"Sample age advances without collecting again":                 1,
		"Unknown sample time remains unavailable":                      2,
		"Exiting a busy TUI ignores late observations":                 1,
	}
	driver := runNativeCanonicalSelection(t, strings.Join(tags, ","), names, 10, 30)
	requireNativeCanonicalSessions(t, driver)
}

func runNativeCanonicalSelection(t *testing.T, tags string, names map[string]int, wantRows, wantSteps int, paths ...string) *composedBDDDriver {
	t.Helper()
	adapter, err := contract.AdapterByName(contract.Integration)
	if err != nil {
		t.Fatal(err)
	}
	driver := newComposedBDDDriver(adapter)
	defer driver.Close()
	bindings := driver.Bindings()
	if err := contract.VerifyAt(adapter, bindings, remainingCanonicalCorpus); err != nil {
		t.Fatal(err)
	}
	options := contract.SuiteOptionsAt(adapter, remainingCanonicalCorpus)
	options.Tags = tags
	if len(paths) != 0 {
		options.Paths = paths
	}
	features, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	rows, steps := 0, 0
	actualNames := map[string]int{}
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			if _, exists := names[scenario.Name]; !exists {
				t.Fatalf("selected unrelated canonical scenario %q", scenario.Name)
			}
			actualNames[scenario.Name]++
			rows++
			steps += len(scenario.Steps)
		}
	}
	if rows != wantRows || steps != wantSteps || len(actualNames) != len(names) {
		t.Fatalf("canonical native selection has %d rows/%d steps, want %d/%d", rows, steps, wantRows, wantSteps)
	}
	for name, count := range names {
		if actualNames[name] != count {
			t.Fatalf("canonical %q has %d rows, want %d", name, actualNames[name], count)
		}
	}
	var transcript bytes.Buffer
	options.Output, options.TestingT, options.DefaultContext = &transcript, t, t.Context()
	executedRows, passedSteps := 0, 0
	result := godog.TestSuite{
		Name: "hippo-tui-native-increment",
		ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
			scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				driver.Reset()
				executedRows++
				return ctx, nil
			})
			scenarioContext.StepContext().After(func(ctx context.Context, _ *godog.Step, status godog.StepResultStatus, _ error) (context.Context, error) {
				if status == godog.StepPassed {
					passedSteps++
				}
				return ctx, nil
			})
			for _, binding := range bindings {
				scenarioContext.Step(binding.Pattern, binding.Handler)
			}
		},
		Options: options,
	}.Run()
	t.Log(transcript.String())
	if result != 0 || executedRows != wantRows || passedSteps != wantSteps {
		t.Fatalf("canonical native result=%d rows=%d passed steps=%d, want 0/%d/%d", result, executedRows, passedSteps, wantRows, wantSteps)
	}
	return driver
}

func TestTUIBDDNativeNeutralCanonical(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.Integration {
		t.Skip("native neutral rows require explicit Integration")
	}
	names := map[string]int{
		"TUI observation preserves running identities and FIFO demand":          1,
		"TUI filtering retains global accounting and original queue positions":  1,
		"TUI observation uses the current shared status evidence":               1,
		"An idle TUI does not create coordination ownership":                    1,
		"Exclusive monitoring names its unregistered queue limitation":          1,
		"Legacy reservation rows retain known allocation vectors":               1,
		"Completed observations remain unchanged when source values are reused": 1,
		"Annotations join only the complete live job identity":                  1,
		"Unusable annotations leave live accounting intact":                     7,
	}
	var tags []string
	for index := 1; index <= 7; index++ {
		tags = append(tags, fmt.Sprintf("@tui-snapshot-%02d", index))
	}
	tags = append(tags, "@tui-label-01", "@tui-label-02")
	runNativeCanonicalSelection(t, strings.Join(tags, ","), names, 15, 53)
	for _, original := range []struct{ feature, name string }{
		{"public-cli.feature", "Display annotations do not change public status or watch comparison"},
		{"execution.feature", "Mixed versions coordinate without requiring display metadata"},
	} {
		t.Run(original.name, func(t *testing.T) {
			path := nativeCanonicalScenarioPath(t, original.feature, original.name)
			runNativeCanonicalSelection(t, "", map[string]int{original.name: 1}, 1, 4, path)
		})
	}
}

func requireNativeCanonicalSessions(t *testing.T, driver *composedBDDDriver) {
	t.Helper()
	if len(driver.native.completed) != 10 || driver.native.cleanupError != nil {
		t.Fatalf("native viewport sessions completed=%d cleanup=%v, want ten clean sessions", len(driver.native.completed), driver.native.cleanupError)
	}
	for _, receipt := range driver.native.completed {
		if !receipt.Joined() || receipt.ExitCode != 0 || len(receipt.Stderr) != 0 {
			t.Fatalf("native viewport did not naturally quit, reap and drain: %+v", receipt)
		}
		t.Logf("native viewport pid=%d natural-exit=%d joined=%t stdout-bytes=%d", receipt.Identity.PID, receipt.ExitCode, receipt.Joined(), len(receipt.Stdout))
	}
}

// Each original viewport handler observes a genuine renderer. The factory is
// passive until its preparation step; unimplemented routes keep their existing
// fail-closed handlers, and E2E never borrows this controlled observer input.
type nativeBDDViewports struct {
	transport           *support.NativeTUITransport
	screen              *tui.BDDNativeTransportScreen
	cancel              context.CancelFunc
	width               int
	height              int
	completed           []support.NativeTUIResult
	cleanupError        error
	refresh             *nativeBDDRefresh
	lifecycle           *nativeBDDLifecycle
	keyboardPrepared    bool
	keyboardKey         string
	keyboardDetailView  string
	keyboardDetailTitle string
	keyboardDetailRows  [4]string
	interaction         *nativeBDDInteraction
}

func (fixture *nativeBDDViewports) handler(pattern string) any {
	if handler := fixture.interactionHandler(pattern); handler != nil {
		return handler
	}
	switch pattern {
	case `^a populated TUI at (\d+) columns and (\d+) rows$`:
		return fixture.prepare
	case `^its current view is rendered$`:
		return fixture.render
	case `^every line fits (\d+) terminal cells and the complete view fits (\d+) rows$`:
		return fixture.requireBounds
	case `^the display uses the (minimum-size|compact|wide) layout$`:
		return fixture.requireLayout
	case `^a populated TUI with the middle running job selected$`:
		return fixture.prepareKeyboardDashboard
	case `^the operator presses (Tab|n|Shift-Tab|p|Down|j|Up|k|Enter|c|\?) on the dashboard$`:
		return fixture.pressDashboardKeyboard
	case `^the TUI (job detail|capacity detail|help) is open with scrollable content$`:
		return fixture.prepareDetailKeyboard
	case `^the operator presses (Down|j|Up|k|b|r|x|Ctrl-Space|Ctrl-h|Ctrl-j|Ctrl-k|Ctrl-l|Ctrl-backslash|Escape)$`:
		return fixture.pressInteractionOrDetailKeyboard
	case `^the TUI performs (.+)$`:
		return fixture.requireDashboardKeyboardAction
	default:
		if fixture.lifecycle != nil {
			if handler := fixture.lifecycle.handler(pattern); handler != nil {
				return handler
			}
		}
		return fixture.refreshHandler(pattern)
	}
}

func (fixture *nativeBDDViewports) prepareKeyboardDashboard() error {
	if err := fixture.prepare(40, 16); err != nil {
		return err
	}
	if err := fixture.render(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := fixture.transport.SendKey(ctx, []byte("\x1b[B")); err != nil {
		return err
	}
	frame, err := awaitBDDNativeTransportFrame(ctx, bddNativeTransportCapture{transport: fixture.transport, screen: fixture.screen}, func(frame tui.BDDNativeTransportFrame) bool {
		content := strings.Join(frame.Rows, "\n")
		return frame.Alternate && frame.Width == 40 && frame.Height == 16 &&
			strings.Contains(content, "HIPPO | Running and Queue") &&
			strings.Contains(content, "Running 3 | Queue 1 [Running]") &&
			strings.Contains(content, "Running 2/3") &&
			strings.Contains(content, "> ACTIVE unavailable run-1 ")
	})
	if err != nil {
		return fmt.Errorf("native dashboard Down did not select the middle job: %w; rows=%q", err, frame.Rows)
	}
	fixture.keyboardPrepared = true
	return nil
}

func (fixture *nativeBDDViewports) pressDashboardKeyboard(key string) error {
	if !fixture.keyboardPrepared || fixture.transport == nil {
		return errors.New("native dashboard keyboard was not prepared")
	}
	var encoded []byte
	switch key {
	case "Tab":
		encoded = []byte("\t")
	case "Shift-Tab":
		encoded = []byte("\x1b[Z")
	case "Down":
		encoded = []byte("\x1b[B")
	case "Up":
		encoded = []byte("\x1b[A")
	case "Enter":
		encoded = []byte("\r")
	case "n", "p", "j", "k", "c", "?":
		encoded = []byte(key)
	default:
		return fmt.Errorf("unsupported native dashboard key %q", key)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := fixture.transport.SendKey(ctx, encoded); err != nil {
		return err
	}
	fixture.keyboardKey = key
	return nil
}

func (fixture *nativeBDDViewports) requireDashboardKeyboardAction(action string) error {
	if fixture.keyboardDetailView != "" {
		return fixture.requireDetailKeyboardAction(action)
	}
	if !fixture.keyboardPrepared || fixture.keyboardKey == "" || fixture.transport == nil || fixture.screen == nil {
		return errors.New("native dashboard action requires its prepared physical key")
	}
	var wantAction string
	title, position, source := "HIPPO | Running and Queue", "", ""
	switch fixture.keyboardKey {
	case "Tab", "n":
		wantAction, position, source = "select the next section", "Queue 1/1", "queued-job"
	case "Shift-Tab", "p":
		wantAction, position, source = "select the previous section", "Queue 1/1", "queued-job"
	case "Down", "j":
		wantAction, position, source = "select the next job", "Running 3/3", "run-2"
	case "Up", "k":
		wantAction, position, source = "select the previous job", "Running 1/3", "run-0"
	case "Enter":
		wantAction, title = "open the selected job detail", "HIPPO | Job detail"
	case "c":
		wantAction, title = "open capacity detail", "HIPPO | Capacity detail"
	case "?":
		wantAction, title = "open keyboard help", "HIPPO | Help"
	default:
		return fmt.Errorf("unsupported native dashboard key %q", fixture.keyboardKey)
	}
	if action != wantAction {
		return fmt.Errorf("native dashboard key %q cannot assert action %q; want %q", fixture.keyboardKey, action, wantAction)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	frame, err := awaitBDDNativeTransportFrame(ctx, bddNativeTransportCapture{transport: fixture.transport, screen: fixture.screen}, func(frame tui.BDDNativeTransportFrame) bool {
		return fixture.dashboardKeyboardFrameMatches(frame, title, position, source)
	})
	if err != nil {
		return fmt.Errorf("native dashboard key %q did not %s: %w; rows=%q", fixture.keyboardKey, action, err, frame.Rows)
	}
	if len(frame.Rows) > fixture.height || frame.CurrentOverflowWrites != 0 {
		return fmt.Errorf("native dashboard keyboard frame exceeds its row budget: %+v", frame)
	}
	for index, row := range frame.Rows {
		if lipgloss.Width(row) > fixture.width {
			return fmt.Errorf("native dashboard keyboard row %d exceeds %d terminal cells", index, fixture.width)
		}
	}
	return fixture.quit()
}

func (fixture *nativeBDDViewports) dashboardKeyboardFrameMatches(frame tui.BDDNativeTransportFrame, title, position, source string) bool {
	if !frame.Alternate || frame.Width != fixture.width || frame.Height != fixture.height {
		return false
	}
	content := strings.Join(frame.Rows, "\n")
	if !strings.Contains(content, title) {
		return false
	}
	if fixture.keyboardKey == "Enter" {
		return strings.Contains(content, "Run ID: 00000000000000000000000000000002")
	}
	if source == "" {
		return true
	}
	if !strings.Contains(content, position) {
		return false
	}
	for _, row := range frame.Rows {
		if strings.HasPrefix(row, "> ") && strings.Contains(row, " "+source+" ") {
			return true
		}
	}
	return false
}

func (fixture *nativeBDDViewports) prepareDetailKeyboard(view string) error {
	var key, title string
	var rows [4]string
	switch view {
	case "job detail":
		key, title = "Enter", "HIPPO | Job detail"
		rows = [4]string{"Run ID: 00000000000000000000000000000001", "Label: unavailable (", "Source: run-0", "State: Running "}
	case "capacity detail":
		key, title = "c", "HIPPO | Capacity detail"
		rows = [4]string{"Host capacity and pressure", "Host parallelism: 8", "Host CPU utilization: 20%", "Physical RAM: "}
	case "help":
		key, title = "?", "HIPPO | Help"
		rows = [4]string{"Help: keyboard actions", "Available in every view:", "r refresh (one queued request while", "busy)"}
	default:
		return fmt.Errorf("unsupported native scrollable view %q", view)
	}
	if err := fixture.prepare(40, 16); err != nil {
		return err
	}
	if err := fixture.render(); err != nil {
		return err
	}
	if _, err := fixture.awaitDetailKeyboardRow("HIPPO | Running and Queue", "Running 3 | Queue 1 [Running]"); err != nil {
		return err
	}
	fixture.keyboardPrepared = true
	fixture.keyboardDetailView, fixture.keyboardDetailTitle, fixture.keyboardDetailRows = view, title, rows
	if err := fixture.pressDashboardKeyboard(key); err != nil {
		return err
	}
	if _, err := fixture.awaitDetailKeyboardRow(title, rows[0]); err != nil {
		return err
	}
	for _, prefix := range rows[1:3] {
		if err := fixture.pressDashboardKeyboard("j"); err != nil {
			return err
		}
		if _, err := fixture.awaitDetailKeyboardRow(title, prefix); err != nil {
			return err
		}
	}
	fixture.keyboardKey = ""
	return nil
}

func (fixture *nativeBDDViewports) pressDetailKeyboard(key string) error {
	if fixture.keyboardDetailView == "" || !fixture.keyboardPrepared || fixture.transport == nil {
		return errors.New("native detail/help keyboard was not prepared")
	}
	switch key {
	case "Down", "j", "Up", "k":
		return fixture.pressDashboardKeyboard(key)
	case "b":
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if err := fixture.transport.SendKey(ctx, []byte("b")); err != nil {
			return err
		}
		fixture.keyboardKey = key
		return nil
	default:
		return fmt.Errorf("unsupported native detail/help key %q", key)
	}
}

func (fixture *nativeBDDViewports) requireDetailKeyboardAction(action string) error {
	if fixture.keyboardDetailView == "" || !fixture.keyboardPrepared || fixture.keyboardKey == "" {
		return errors.New("native detail/help action requires its prepared physical key")
	}
	var wantAction, prefix string
	title := fixture.keyboardDetailTitle
	switch fixture.keyboardKey {
	case "Down", "j":
		wantAction, prefix = "scroll one row down", fixture.keyboardDetailRows[3]
	case "Up", "k":
		wantAction, prefix = "scroll one row up", fixture.keyboardDetailRows[1]
	case "b":
		if fixture.keyboardDetailView == "help" {
			wantAction = "restore the previous view and selection"
		} else {
			wantAction = "restore the previous section and selection"
		}
		title, prefix = "HIPPO | Running and Queue", "Running 3 | Queue 1 [Running]"
	default:
		return fmt.Errorf("unsupported native detail/help key %q", fixture.keyboardKey)
	}
	if action != wantAction {
		return fmt.Errorf("native detail/help key %q cannot assert action %q; want %q", fixture.keyboardKey, action, wantAction)
	}
	frame, err := fixture.awaitDetailKeyboardRow(title, prefix)
	if err != nil {
		return err
	}
	if len(frame.Rows) > fixture.height || frame.CurrentOverflowWrites != 0 {
		return fmt.Errorf("native detail/help keyboard frame exceeds its row budget: %+v", frame)
	}
	for index, row := range frame.Rows {
		if lipgloss.Width(row) > fixture.width {
			return fmt.Errorf("native detail/help keyboard row %d exceeds %d terminal cells", index, fixture.width)
		}
	}
	return fixture.quit()
}

func (fixture *nativeBDDViewports) awaitDetailKeyboardRow(title, prefix string) (tui.BDDNativeTransportFrame, error) {
	if fixture.transport == nil || fixture.screen == nil {
		return tui.BDDNativeTransportFrame{}, errors.New("native detail/help frame was not prepared")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	frame, err := awaitBDDNativeTransportFrame(ctx, bddNativeTransportCapture{transport: fixture.transport, screen: fixture.screen}, func(frame tui.BDDNativeTransportFrame) bool {
		if !frame.Alternate || frame.Width != 40 || frame.Height != 16 || len(frame.Rows) < 7 {
			return false
		}
		if strings.TrimSpace(frame.Rows[0]) != title || !strings.HasPrefix(frame.Rows[4], prefix) {
			return false
		}
		if title == "HIPPO | Running and Queue" {
			return strings.TrimSpace(frame.Rows[5]) == "Running 1/3" && strings.HasPrefix(frame.Rows[6], "> ACTIVE unavailable run-0 ")
		}
		return true
	})
	if err != nil {
		return frame, fmt.Errorf("native %s key %q did not reach first body row %q: %w; rows=%q", title, fixture.keyboardKey, prefix, err, frame.Rows)
	}
	return frame, nil
}

func (fixture *nativeBDDViewports) prepare(width, height int) error {
	fixture.closeSession()
	if fixture.cleanupError != nil {
		return fixture.cleanupError
	}
	return fixture.start(width, height, os.Environ())
}

func (fixture *nativeBDDViewports) start(width, height int, environment []string) error {
	return fixture.startContext(context.Background(), width, height, environment)
}

func (fixture *nativeBDDViewports) startContext(parent context.Context, width, height int, environment []string) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	fixture.cancel, fixture.width, fixture.height = cancel, width, height
	fixture.screen = tui.NewBDDNativeTransportScreen(width, height)
	transport, err := support.StartNativeTUI(ctx, support.NativeTUIStart{
		Mode: contract.Integration, Executable: os.Args[0], Helper: support.NativeTUITeaHelper,
		Environment: environment, Width: width, Height: height,
		Feed: fixture.screen.Feed, ResizeScreen: fixture.screen.Resize, Finish: fixture.screen.Finish,
	})
	if err != nil {
		cancel()
		return err
	}
	fixture.transport = transport
	if err := transport.AwaitReady(ctx); err != nil {
		cleanupContext, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer stop()
		result, drainError := transport.WaitAndDrain(cleanupContext)
		return errors.Join(err, drainError, fmt.Errorf("native readiness child stdout=%q stderr=%q", result.Stdout, result.Stderr))
	}
	return nil
}

func (fixture *nativeBDDViewports) render() error {
	return fixture.renderContext(context.Background())
}

func (fixture *nativeBDDViewports) renderContext(parent context.Context) error {
	if fixture.transport == nil {
		return errors.New("native viewport was not prepared")
	}
	ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
	defer cancel()
	_, err := awaitBDDNativeTransportFrame(ctx, bddNativeTransportCapture{transport: fixture.transport, screen: fixture.screen}, func(frame tui.BDDNativeTransportFrame) bool {
		if !frame.Alternate || frame.Width != fixture.width || frame.Height != fixture.height {
			return false
		}
		content := strings.Join(frame.Rows, "\n")
		if fixture.width < 40 || fixture.height < 16 {
			return strings.Contains(content, "Terminal too small: need 40x16")
		}
		return strings.Contains(content, "HIPPO | Running and Queue") && strings.Contains(content, "LIVE") && strings.Contains(content, "Running 1/3")
	})
	return err
}

func (fixture *nativeBDDViewports) requireBounds(width, height int) error {
	if err := fixture.render(); err != nil {
		return err
	}
	frame := fixture.screen.Snapshot()
	if frame.Width != width || frame.Height != height || len(frame.Rows) > height || frame.CurrentOverflowWrites != 0 {
		return fmt.Errorf("actual native frame exceeds declared %dx%d budget: %+v", width, height, frame)
	}
	for index, row := range frame.Rows {
		if lipgloss.Width(row) > width {
			return fmt.Errorf("actual native row %d exceeds %d terminal cells", index, width)
		}
	}
	return nil
}

func (fixture *nativeBDDViewports) requireLayout(layout string) error {
	if err := fixture.render(); err != nil {
		return err
	}
	content := strings.Join(fixture.screen.Snapshot().Rows, "\n")
	var matches bool
	switch layout {
	case "minimum-size":
		matches = strings.Contains(content, "Terminal too small: need 40x16") && strings.Contains(content, "q ? help") && !strings.Contains(content, "HIPPO | Running and Queue")
	case "compact":
		matches = strings.Contains(content, "Running 3 | Queue 1 [Running]") && strings.Contains(content, "Running 1/3") && !strings.Contains(content, "Queue 1/1")
	case "wide":
		matches = strings.Contains(content, "Running 1/3") && strings.Contains(content, "Queue 1/1") && !strings.Contains(content, "Running 3 | Queue 1 [Running]")
	}
	if !matches {
		return fmt.Errorf("actual native viewport %dx%d does not show %s layout: %q", fixture.width, fixture.height, layout, content)
	}
	return fixture.quit()
}

func (fixture *nativeBDDViewports) quit() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := fixture.transport.SendKey(ctx, []byte("q")); err != nil {
		return err
	}
	result, err := fixture.transport.WaitAndDrain(ctx)
	if err != nil {
		return err
	}
	if !result.Joined() || result.ExitCode != 0 || len(result.Stderr) != 0 {
		return fmt.Errorf("native viewport quit did not naturally reap and drain: %+v", result)
	}
	fixture.completed = append(fixture.completed, result)
	if fixture.refresh != nil && fixture.refresh.directory != "" {
		if err := fixture.refresh.requireJoinedJournal(); err != nil {
			return err
		}
	}
	fixture.closeSession()
	return fixture.cleanupError
}

func (fixture *nativeBDDViewports) closeSession() {
	fixture.closeSessionContext(context.Background())
}

func (fixture *nativeBDDViewports) closeSessionContext(parent context.Context) {
	if fixture.interaction != nil {
		fixture.interaction.reset()
	}
	fixture.keyboardPrepared = false
	fixture.keyboardKey = ""
	fixture.keyboardDetailView = ""
	fixture.keyboardDetailTitle = ""
	fixture.keyboardDetailRows = [4]string{}
	if fixture.transport != nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
		closeError := fixture.transport.Close(ctx)
		cancel()
		if !fixture.transport.Result().Joined() {
			closeError = errors.Join(closeError, errors.New("native viewport cleanup did not join all owned readers and child"))
		}
		fixture.cleanupError = errors.Join(fixture.cleanupError, closeError)
		fixture.transport = nil
	}
	if fixture.cancel != nil {
		fixture.cancel()
		fixture.cancel = nil
	}
	if fixture.refresh != nil && fixture.refresh.directory != "" {
		fixture.cleanupError = errors.Join(fixture.cleanupError, os.RemoveAll(fixture.refresh.directory))
		fixture.refresh.directory = ""
	}
}

func nativeCanonicalScenarioPath(t *testing.T, filename, name string) string {
	t.Helper()
	adapter, err := contract.AdapterByName(contract.Integration)
	if err != nil {
		t.Fatal(err)
	}
	options := contract.SuiteOptionsAt(adapter, remainingCanonicalCorpus)
	path := filepath.Join(remainingCanonicalCorpus, filename)
	options.Paths = []string{path}
	features, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	var line int64
	for _, feature := range features {
		for _, child := range feature.Feature.Children {
			if child.Scenario != nil && child.Scenario.Name == name {
				if line != 0 {
					t.Fatal("canonical name is ambiguous")
				}
				line = child.Scenario.Location.Line
			}
		}
	}
	if line == 0 {
		t.Fatal("canonical name is absent")
	}
	return fmt.Sprintf("%s:%d", path, line)
}
