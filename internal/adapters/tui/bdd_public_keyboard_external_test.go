package tui_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/tests/support"
)

func (fixture *publicBDDObservation) keyboardHandler(pattern string) any {
	switch pattern {
	case `^a populated TUI with the middle running job selected$`:
		return fixture.preparePublicMiddle
	case `^the operator presses (Tab|n|Shift-Tab|p|Down|j|Up|k|Enter|c|\?) on the dashboard$`:
		return fixture.sendInteractive
	case `^the TUI performs (.+)$`:
		return fixture.requirePublicAction
	case `^the TUI (job detail|capacity detail|help) is open with scrollable content$`:
		return fixture.preparePublicScrollable
	case `^the operator presses (Down|j|Up|k|b|r|x|Ctrl-Space|Ctrl-h|Ctrl-j|Ctrl-k|Ctrl-l|Ctrl-backslash|Escape)$`:
		return fixture.pressPublicViewKey
	case `^the TUI (dashboard|job detail|capacity detail|help|minimum-size) is open$`:
		return fixture.preparePublicNamedView
	case `^one refresh is requested without leaving that view$`:
		return fixture.refreshPublicView
	case `^a populated TUI$`, `^a populated TUI with a selected run ID$`:
		return fixture.preparePublicPopulated
	case `^keyboard help is opened$`:
		return fixture.openPublicHelp
	case `^every section selection detail capacity back refresh help and quit action has a documented usable key$`:
		return fixture.requirePublicHelp
	case `^no job action or TUI quit occurs and selection is unchanged$`:
		return fixture.requirePublicIgnored
	case `^color is disabled and the TUI has selected waiting stale and error states$`:
		return fixture.preparePublicNoColor
	case `^the dashboard and details are rendered$`:
		return fixture.renderPublicAllViews
	case `^text markers distinguish those states without ANSI styling$`:
		return fixture.requirePublicNoColor
	default:
		return nil
	}
}

func (fixture *publicBDDObservation) preparePublicPopulated(ctx context.Context) (context.Context, error) {
	return fixture.prepareInteractive(ctx, "three", 40, 16, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false)
}

func (fixture *publicBDDObservation) selectPublicMiddle(ctx context.Context) error {
	owners := fixture.ledger.Expected().Owners
	if len(owners) != 3 {
		return errors.New("middle selection requires three genuinely owned running rows")
	}
	if err := fixture.sendInteractive(ctx, "j"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{"Running 2/3"}, nil); err != nil {
		return err
	}
	fixture.family.selected = owners[1].RunID
	return nil
}

func (fixture *publicBDDObservation) preparePublicMiddle(ctx context.Context) (context.Context, error) {
	ctx, err := fixture.preparePublicPopulated(ctx)
	if err != nil {
		return ctx, err
	}
	return ctx, fixture.selectPublicMiddle(ctx)
}

func publicViewTitle(view string) string {
	switch view {
	case "dashboard":
		return "HIPPO | Running and Queue"
	case "job detail":
		return "HIPPO | Job detail"
	case "capacity detail":
		return "HIPPO | Capacity detail"
	case "help":
		return "HIPPO | Help"
	case "minimum-size":
		return "Terminal too small"
	}
	return ""
}

func (fixture *publicBDDObservation) openPublicView(ctx context.Context, view string) error {
	var key string
	switch view {
	case "dashboard":
		return nil
	case "job detail":
		key = "Enter"
	case "capacity detail":
		key = "c"
	case "help":
		key = "?"
	case "minimum-size":
		fixture.family.view = view
		return fixture.resizeInteractive(ctx, 39, 15)
	default:
		return fmt.Errorf("unknown original public view %q", view)
	}
	fixture.family.previousView = fixture.family.view
	if err := fixture.sendInteractive(ctx, key); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{publicViewTitle(view)}, nil); err != nil {
		return err
	}
	fixture.family.view = view
	return nil
}

func (fixture *publicBDDObservation) preparePublicNamedView(ctx context.Context, view string) (context.Context, error) {
	ctx, err := fixture.preparePublicMiddle(ctx)
	if err != nil {
		return ctx, err
	}
	return ctx, fixture.openPublicView(ctx, view)
}

func (fixture *publicBDDObservation) preparePublicScrollable(ctx context.Context, view string) (context.Context, error) {
	ctx, err := fixture.prepareInteractive(ctx, "three", 40, 16, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false)
	if err != nil {
		return ctx, err
	}
	fixture.family.selected = fixture.ledger.Expected().Owners[0].RunID
	if err := fixture.openPublicView(ctx, view); err != nil {
		return ctx, err
	}
	for offset := 1; offset <= 2; offset++ {
		if err := fixture.sendInteractive(ctx, "j"); err != nil {
			return ctx, err
		}
		if err := fixture.awaitPublicBody(ctx, publicDetailRows(view, fixture.family.selected)[offset]); err != nil {
			return ctx, err
		}
	}
	fixture.family.key = ""
	return ctx, nil
}

func publicDetailRows(view, id string) [4]string {
	switch view {
	case "job detail":
		return [4]string{"Run ID: " + id, "Label: unavailable (", "Source: owner", "State: Running "}
	case "capacity detail":
		return [4]string{"Host capacity and pressure", fmt.Sprintf("Host parallelism: %d", runtime.NumCPU()), "Host CPU utilization: 20%", "Physical RAM: "}
	case "help":
		return [4]string{"Help: keyboard actions", "Available in every view:", "r refresh (one queued request while", "busy)"}
	}
	return [4]string{}
}

func (fixture *publicBDDObservation) awaitPublicBody(ctx context.Context, prefix string) error {
	bound, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		frame := fixture.screen.Snapshot()
		if err := fixture.transport.OutputError(); err != nil {
			return err
		}
		if frame.OverflowWrites != 0 {
			return errors.New("detail scroll overflow")
		}
		if len(frame.Rows) > 4 && strings.HasPrefix(frame.Rows[4], prefix) {
			fixture.frames = append(fixture.frames, frame)
			return nil
		}
		select {
		case <-bound.Done():
			return fmt.Errorf("public detail first body row did not become %q: %w; rows=%q", prefix, bound.Err(), frame.Rows)
		case <-ticker.C:
		}
	}
}

func (fixture *publicBDDObservation) requirePublicAction(ctx context.Context, action string) error {
	state := fixture.family
	if state.view != "dashboard" {
		return fixture.requirePublicDetailAction(ctx, action)
	}
	var want string
	var parts []string
	switch state.key {
	case "Tab", "n":
		want = "select the next section"
		parts = []string{"[Queue]", "Queue 1/1"}
	case "Shift-Tab", "p":
		want = "select the previous section"
		parts = []string{"[Queue]", "Queue 1/1"}
	case "Down", "j":
		want = "select the next job"
		parts = []string{"Running 3/3"}
	case "Up", "k":
		want = "select the previous job"
		parts = []string{"Running 1/3"}
	case "Enter":
		want = "open the selected job detail"
		parts = []string{publicViewTitle("job detail"), state.selected}
	case "c":
		want = "open capacity detail"
		parts = []string{publicViewTitle("capacity detail")}
	case "?":
		want = "open keyboard help"
		parts = []string{publicViewTitle("help")}
	default:
		return errors.New("dashboard action lacks original key")
	}
	if action != want {
		return fmt.Errorf("original key %q expected action %q, got %q", state.key, want, action)
	}
	if _, err := fixture.await(ctx, parts, nil); err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) pressPublicViewKey(ctx context.Context, key string) error {
	if key == "r" {
		// Arm before the original When key, not afterward with a second r.
		fixture.family.beforeTrace = fixture.probe.Trace()
		if err := fixture.armRefresh(support.PublicTUIHostReply{CPUPercent: 20}); err != nil {
			return err
		}
	}
	return fixture.sendInteractive(ctx, key)
}

func (fixture *publicBDDObservation) refreshPublicView(ctx context.Context) error {
	before := fixture.family.view
	bound, stop := context.WithTimeout(ctx, 3*time.Second)
	defer stop()
	if err := fixture.probe.WaitForReading(bound, fixture.family.gate); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{publicViewTitle(before)}, nil); err != nil {
		return err
	}
	if err := fixture.releaseRefresh(ctx); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{publicViewTitle(before)}, nil); err != nil {
		return err
	}
	if fixture.probe.Trace().Readings != fixture.family.beforeTrace.Readings+2 {
		return errors.New("view refresh did not complete exactly one genuine follow-up observation")
	}
	if before == "job detail" {
		if _, err := fixture.await(ctx, []string{fixture.family.selected}, nil); err != nil {
			return err
		}
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) openPublicHelp(ctx context.Context) error {
	return fixture.openPublicView(ctx, "help")
}

func (fixture *publicBDDObservation) requirePublicHelp(ctx context.Context) error {
	remaining := map[string]bool{"Tab or n": true, "Shift-Tab or p": true, "Down or j": true, "Up or k": true, "Enter": true, "c: open": true, "b back": true, "r refresh": true, "? help": true, "q quit": true}
	for steps := 0; steps < 64 && len(remaining) > 0; steps++ {
		text := strings.Join(fixture.screen.Snapshot().Rows, "\n")
		for token := range remaining {
			if strings.Contains(text, token) {
				delete(remaining, token)
			}
		}
		if len(remaining) == 0 {
			break
		}
		before := fixture.screen.Snapshot()
		if err := fixture.sendInteractive(ctx, "j"); err != nil {
			return err
		}
		bound, stop := context.WithTimeout(ctx, 500*time.Millisecond)
		ticker := time.NewTicker(time.Millisecond)
		changed := false
		for !changed {
			next := fixture.screen.Snapshot()
			changed = strings.Join(next.Rows, "\n") != strings.Join(before.Rows, "\n")
			if changed {
				fixture.frames = append(fixture.frames, next)
				break
			}
			select {
			case <-bound.Done():
				ticker.Stop()
				stop()
				return errors.New("public help stopped scrolling before every original action was visible")
			case <-ticker.C:
			}
		}
		ticker.Stop()
		stop()
	}
	if len(remaining) != 0 {
		return fmt.Errorf("actual public help did not expose original actions: %v", remaining)
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) requirePublicIgnored(ctx context.Context) error {
	if fixture.transport.Result().Reaped {
		return errors.New("ignored input quit selected candidate")
	}
	// Two real section transitions provide an ordered input barrier. An accidental
	// detail action would consume n/p differently and cannot satisfy this dashboard oracle.
	if err := fixture.sendInteractive(ctx, "n"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{"[Queue]"}, nil); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "p"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{"[Running]", "Running 1/3"}, nil); err != nil {
		return err
	}
	if fixture.probe.Trace().Readings != 2 {
		return errors.New("ignored key initiated a collector observation")
	}
	if err := fixture.sendInteractive(ctx, "Enter"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{fixture.ledger.Expected().Owners[0].RunID}, nil); err != nil {
		return err
	}
	if err := fixture.ledger.VerifyLiveness(ctx); err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicNoColor(ctx context.Context) (context.Context, error) {
	ctx, err := fixture.preparePublicMiddle(ctx)
	if err != nil {
		return ctx, err
	}
	return ctx, fixture.failPublicNext(ctx)
}

func (fixture *publicBDDObservation) renderPublicAllViews(ctx context.Context) error {
	if _, err := fixture.await(ctx, []string{"STALE", "ERROR", "[Running]", ">"}, nil); err != nil {
		return err
	}
	// Compact layout displays one real panel at a time. Inspect its Queue tab
	// and genuine waiting detail before restoring the selected Running tab.
	if err := fixture.sendInteractive(ctx, "n"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{"[Queue]", "WAIT", ">", "STALE", "ERROR"}, nil); err != nil {
		return err
	}
	if err := fixture.openPublicView(ctx, "job detail"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{fixture.ledger.Expected().Waiters[0].RunID, "State: Queue waiting", "STALE", "ERROR"}, nil); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "b"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{"[Queue]"}, nil); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "p"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{"[Running]", "Running 2/3", "STALE", "ERROR"}, nil); err != nil {
		return err
	}
	if err := fixture.openPublicView(ctx, "job detail"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{fixture.family.selected, "STALE", "ERROR"}, nil); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "b"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{"HIPPO | Running and Queue"}, nil); err != nil {
		return err
	}
	return fixture.openPublicView(ctx, "capacity detail")
}

func (fixture *publicBDDObservation) requirePublicNoColor(ctx context.Context) error {
	if err := fixture.finish(ctx); err != nil {
		return err
	}
	result := fixture.transport.Result()
	if regexp.MustCompile(`\x1b\[[0-9;:]*m`).Match(result.Stdout) {
		return errors.New("NO_COLOR actual joined raw terminal stream contains ANSI styling")
	}
	return fixture.ledger.VerifyLiveness(ctx)
}

func (fixture *publicBDDObservation) requirePublicDetailAction(ctx context.Context, action string) error {
	state := fixture.family
	var want, prefix string
	switch state.key {
	case "Down", "j":
		want = "scroll one row down"
		prefix = publicDetailRows(state.view, state.selected)[3]
	case "Up", "k":
		want = "scroll one row up"
		prefix = publicDetailRows(state.view, state.selected)[1]
	case "b":
		want = "restore the previous section and selection"
		if state.view == "help" {
			want = "restore the previous view and selection"
		}
		prefix = "Running 3 | Queue 1 [Running]"
	default:
		return errors.New("detail action lacks its original physical key")
	}
	if action != want {
		return fmt.Errorf("key %s does not assert %q", state.key, action)
	}
	if err := fixture.awaitPublicBody(ctx, prefix); err != nil {
		return err
	}
	if state.key == "b" {
		if _, err := fixture.await(ctx, []string{"Running 1/3"}, nil); err != nil {
			return err
		}
	}
	return fixture.finish(ctx)
}
