package tui_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/wahidyankf/hippo/tests/support"
)

func (fixture *publicBDDObservation) viewportHandler(pattern string) any {
	handlers := map[string]any{
		`^a populated TUI at (\d+) columns and (\d+) rows$`:                                             fixture.preparePublicViewport,
		`^its current view is rendered$`:                                                                fixture.renderPublicCurrent,
		`^every line fits (\d+) terminal cells and the complete view fits (\d+) rows$`:                  fixture.requirePublicBounds,
		`^the display uses the (minimum-size|compact|wide) layout$`:                                     fixture.requirePublicLayout,
		`^a selected live job at 40 columns and 16 rows$`:                                               fixture.preparePublicSmallSelected,
		`^the terminal shrinks to 39 by 15 cells and returns to 40 by 16 cells$`:                        fixture.shrinkPublicRecover,
		`^the fitted minimum-size message gives way to the compact view with the same selected run ID$`: fixture.requirePublicSizeRecovery,
		`^a TUI row and detail containing (an overlong unbroken value|wide Unicode characters|combining Unicode characters|ANSI escapes and control bytes)$`: fixture.preparePublicText,
		`^they are rendered at 40 columns and 16 rows$`:                                                             fixture.renderPublicText,
		`^controls are not displayed and each row and detail fits its terminal-cell and row budget$`:                fixture.requirePublicSafeText,
		`^one thousand running and queued jobs with a selected live run ID$`:                                        fixture.preparePublicLarge,
		`^the operator navigates and repeatedly resizes between compact and wide views$`:                            fixture.navigatePublicLarge,
		`^the selected run ID stays visible and each list shows its bounded position and count$`:                    fixture.requirePublicLarge,
		`^three visible jobs with the middle run ID selected$`:                                                      fixture.preparePublicMiddle,
		`^a refresh removes that selected job$`:                                                                     fixture.removePublicSelected,
		`^the nearest surviving row is selected by its prior position and remains visible$`:                         fixture.requirePublicNearest,
		`^job detail is open for a live run ID$`:                                                                    fixture.preparePublicLiveDetail,
		`^a refresh removes that run ID$`:                                                                           fixture.removePublicDetail,
		`^detail reports job no longer active and back returns to a usable section$`:                                fixture.requirePublicMissingDetail,
		`^a TUI whose active job panel is empty$`:                                                                   fixture.preparePublicEmpty,
		`^the operator requests job detail$`:                                                                        fixture.requestPublicDetail,
		`^the explicit empty panel remains usable with no selected job$`:                                            fixture.requirePublicEmpty,
		`^a job allocated two CPUs and one GiB with host CPU utilization at twenty percent$`:                        fixture.preparePublicOwner,
		`^job and capacity details are inspected$`:                                                                  fixture.inspectPublicResources,
		`^reservation allocation and machine utilization have distinct labels without claiming process CPU or RSS$`: fixture.requirePublicResources,
	}
	return handlers[pattern]
}

func (fixture *publicBDDObservation) preparePublicViewport(ctx context.Context, width, height int) (context.Context, error) {
	ctx, err := fixture.preparePublicPopulated(ctx)
	if err != nil {
		return ctx, err
	}
	return ctx, fixture.resizeInteractive(ctx, width, height)
}

func (fixture *publicBDDObservation) renderPublicCurrent(ctx context.Context) error {
	_, err := fixture.await(ctx, nil, nil)
	return err
}

func (fixture *publicBDDObservation) requirePublicBounds(_ context.Context, width, height int) error {
	frame := fixture.screen.Snapshot()
	if frame.Width != width || frame.Height != height || frame.OverflowWrites != 0 || len(frame.Rows) > height {
		return errors.New("actual public native frame violates original viewport dimensions/overflow bounds")
	}
	for index, row := range frame.Rows {
		if lipgloss.Width(row) > width {
			return fmt.Errorf("public frame row %d exceeds %d terminal cells", index, width)
		}
	}
	return nil
}

func (fixture *publicBDDObservation) requirePublicLayout(ctx context.Context, layout string) error {
	var parts []string
	switch layout {
	case "minimum-size":
		switch {
		case fixture.width < 6:
			parts = []string{"q"}
		case fixture.height == 1:
			parts = []string{"q ? help"}
		default:
			parts = []string{fmt.Sprintf("%dx%d", fixture.width, fixture.height), "q ? help"}
		}
	case "compact":
		parts = []string{"Running 3 | Queue 1 [Running]", "Running 1/3"}
	case "wide":
		parts = []string{"Running 1/3", "Queue 1/1"}
	default:
		return errors.New("unknown original viewport layout")
	}
	if _, err := fixture.await(ctx, parts, nil); err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicSmallSelected(ctx context.Context) (context.Context, error) {
	ctx, err := fixture.preparePublicMiddle(ctx)
	if err != nil {
		return ctx, err
	}
	return ctx, fixture.resizePublicRepaint(ctx, 40, 16)
}

func (fixture *publicBDDObservation) shrinkPublicRecover(ctx context.Context) error {
	if err := fixture.resizePublicRepaint(ctx, 39, 15); err != nil {
		return err
	}
	if _, err := fixture.awaitRepaint(ctx, []string{"Terminal too small"}, nil, 0); err != nil {
		return err
	}
	return fixture.resizePublicRepaint(ctx, 40, 16)
}

func (fixture *publicBDDObservation) requirePublicSizeRecovery(ctx context.Context) error {
	if _, err := fixture.awaitRepaint(ctx, []string{"Running 2/3"}, []string{"Terminal too small"}, 0); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "Enter"); err != nil {
		return err
	}
	if _, err := fixture.awaitRepaint(ctx, []string{fixture.family.selected}, nil, 0); err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicText(ctx context.Context, kind string) (context.Context, error) {
	// These original synthetic rows retain their reviewed E2E exemption;
	// refuse private model substitution before acquiring a public fixture.
	switch kind {
	case "wide Unicode characters", "combining Unicode characters", "ANSI escapes and control bytes":
		return ctx, errors.New("TUI e2e requires actual native/public scenario transport; private model fallback refused")
	}
	ctx, err := fixture.prepareCase(ctx, "one", "")
	if err != nil {
		return ctx, err
	}
	if err := fixture.ledger.StageOwnedText(kind); err != nil {
		return ctx, err
	}
	fixture.family = &publicBDDInteractive{view: "dashboard", selected: fixture.ledger.Expected().Owners[0].RunID, textKind: kind}
	return ctx, fixture.startInteractive(ctx, 40, 16, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false)
}

func (fixture *publicBDDObservation) renderPublicText(ctx context.Context) error {
	if err := fixture.live(ctx); err != nil {
		return err
	}
	if err := fixture.requirePublicBounds(ctx, 40, 16); err != nil {
		return err
	}
	if err := fixture.openPublicView(ctx, "job detail"); err != nil {
		return err
	}
	for range 8 {
		if err := fixture.requirePublicBounds(ctx, 40, 16); err != nil {
			return err
		}
		if err := fixture.sendInteractive(ctx, "j"); err != nil {
			return err
		}
		if _, err := fixture.await(ctx, []string{publicViewTitle("job detail")}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (fixture *publicBDDObservation) requirePublicSafeText(ctx context.Context) error {
	var observedText strings.Builder
	for _, frame := range fixture.frames {
		observedText.WriteString(strings.Join(frame.Rows, "\n"))
		if frame.OverflowWrites != 0 || len(frame.Rows) > frame.Height {
			return errors.New("untrusted text overflowed actual public viewport")
		}
		for _, row := range frame.Rows {
			if lipgloss.Width(row) > frame.Width || strings.IndexFunc(row, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
				return errors.New("untrusted public row retained controls or exceeded terminal-cell budget")
			}
		}
	}
	switch fixture.family.textKind {
	case "an overlong unbroken value":
		if !strings.Contains(observedText.String(), strings.Repeat("a", 16)) {
			return errors.New("actual row/detail never exposed the overlong input")
		}
	case "wide Unicode characters":
		if !strings.Contains(observedText.String(), "\u754c") {
			return errors.New("actual row/detail never exposed wide text")
		}
	case "combining Unicode characters":
		if !strings.Contains(observedText.String(), "e\u0301") {
			return errors.New("actual row/detail never exposed combining text")
		}
	case "ANSI escapes and control bytes":
		if strings.Contains(observedText.String(), "unchecked") {
			return errors.New("actual optional-label reader accepted unsafe control payload")
		}
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicLarge(ctx context.Context) (context.Context, error) {
	ctx, err := fixture.prepareCase(ctx, "one", "")
	if err != nil {
		return ctx, err
	}
	if err := fixture.ledger.PrepareLargeLedger(ctx); err != nil {
		return ctx, err
	}
	fixture.family = &publicBDDInteractive{view: "dashboard", selected: fixture.ledger.Expected().Owners[0].RunID}
	if err := fixture.startInteractive(ctx, 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false); err != nil {
		return ctx, err
	}
	return ctx, fixture.live(ctx)
}

func (fixture *publicBDDObservation) navigatePublicLarge(ctx context.Context) error {
	for index := 1; index <= 12; index++ {
		if err := fixture.sendInteractive(ctx, "j"); err != nil {
			return err
		}
		if _, err := fixture.awaitRepaint(ctx, []string{fmt.Sprintf("Running %d/1000", index+1)}, nil, 0); err != nil {
			return err
		}
	}
	fixture.family.selected = fixture.ledger.Expected().Owners[12].RunID
	if err := fixture.sendInteractive(ctx, "n"); err != nil {
		return err
	}
	if _, err := fixture.awaitRepaint(ctx, []string{"Queue 1/1000"}, nil, 0); err != nil {
		return err
	}
	for index := 1; index <= 12; index++ {
		if err := fixture.sendInteractive(ctx, "j"); err != nil {
			return err
		}
		if _, err := fixture.awaitRepaint(ctx, []string{fmt.Sprintf("Queue %d/1000", index+1)}, nil, 0); err != nil {
			return err
		}
	}
	for range 3 {
		if err := fixture.resizePublicRepaint(ctx, 40, 16); err != nil {
			return err
		}
		if _, err := fixture.awaitRepaint(ctx, []string{"Queue 13/1000"}, nil, 0); err != nil {
			return err
		}
		if err := fixture.resizePublicRepaint(ctx, 120, 32); err != nil {
			return err
		}
		if _, err := fixture.awaitRepaint(ctx, []string{"Running 13/1000", "Queue 13/1000"}, nil, 0); err != nil {
			return err
		}
	}
	return nil
}

func (fixture *publicBDDObservation) requirePublicLarge(ctx context.Context) error {
	if err := fixture.sendInteractive(ctx, "Enter"); err != nil {
		return err
	}
	if _, err := fixture.awaitRepaint(ctx, []string{fixture.ledger.Expected().Waiters[12].RunID}, nil, 0); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "b"); err != nil {
		return err
	}
	if _, err := fixture.awaitRepaint(ctx, []string{"Queue 13/1000", "Running 13/1000"}, nil, 0); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "p"); err != nil {
		return err
	}
	if _, err := fixture.awaitRepaint(ctx, []string{"#" + fixture.family.selected[:8]}, nil, 0); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "Enter"); err != nil {
		return err
	}
	if _, err := fixture.awaitRepaint(ctx, []string{fixture.family.selected}, nil, 0); err != nil {
		return err
	}
	frame := fixture.screen.Snapshot()
	if frame.CurrentOverflowWrites != 0 || len(frame.Rows) > frame.Height {
		return errors.New("current large-list repaint violates viewport bounds")
	}
	for _, row := range frame.Rows {
		if lipgloss.Width(row) > frame.Width {
			return errors.New("current large-list row exceeds terminal-cell budget")
		}
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) removePublicSelected(ctx context.Context) error {
	if err := fixture.ledger.RetireOwnedRunning(ctx, fixture.family.selected); err != nil {
		return err
	}
	if err := fixture.holdRefresh(ctx, support.PublicTUIHostReply{CPUPercent: 20}); err != nil {
		return err
	}
	return fixture.releaseRefresh(ctx)
}

func (fixture *publicBDDObservation) requirePublicNearest(ctx context.Context) error {
	if _, err := fixture.await(ctx, []string{"Running 2/2"}, nil); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "Enter"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{fixture.ledger.Expected().Owners[1].RunID}, []string{fixture.family.selected}); err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicLiveDetail(ctx context.Context) (context.Context, error) {
	ctx, err := fixture.preparePublicMiddle(ctx)
	if err != nil {
		return ctx, err
	}
	return ctx, fixture.openPublicView(ctx, "job detail")
}

func (fixture *publicBDDObservation) removePublicDetail(ctx context.Context) error {
	return fixture.removePublicSelected(ctx)
}

func (fixture *publicBDDObservation) requirePublicMissingDetail(ctx context.Context) error {
	if _, err := fixture.await(ctx, []string{"job no longer active"}, nil); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "b"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{publicViewTitle("dashboard"), "Running 2/2"}, nil); err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) preparePublicEmpty(ctx context.Context) (context.Context, error) {
	return fixture.prepareInteractive(ctx, "empty", 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false)
}

func (fixture *publicBDDObservation) requestPublicDetail(ctx context.Context) error {
	return fixture.sendInteractive(ctx, "Enter")
}

func (fixture *publicBDDObservation) requirePublicEmpty(ctx context.Context) error {
	if _, err := fixture.await(ctx, []string{publicViewTitle("dashboard"), "No running jobs", "Running 0/0"}, []string{publicViewTitle("job detail")}); err != nil {
		return err
	}
	return fixture.finish(ctx)
}

func (fixture *publicBDDObservation) inspectPublicResources(ctx context.Context) error {
	if err := fixture.openPublicView(ctx, "job detail"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{"Allocated", "2CPU", "1.0GiB"}, nil); err != nil {
		return err
	}
	if err := fixture.sendInteractive(ctx, "b"); err != nil {
		return err
	}
	if _, err := fixture.await(ctx, []string{publicViewTitle("dashboard")}, nil); err != nil {
		return err
	}
	return fixture.openPublicView(ctx, "capacity detail")
}

func (fixture *publicBDDObservation) requirePublicResources(ctx context.Context) error {
	if _, err := fixture.await(ctx, []string{"Host CPU utilization: 20%", "Reservation"}, []string{"Process CPU", "RSS"}); err != nil {
		return err
	}
	return fixture.finish(ctx)
}

// Only these original resize journeys use current-repaint evidence. The real
// kernel/decoder/SIGWINCH order and the500ms action allowance are unchanged.
func (fixture *publicBDDObservation) resizePublicRepaint(ctx context.Context, width, height int) error {
	action, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stop()
	before := fixture.screen.Snapshot()
	if err := fixture.transport.Resize(action, width, height); err != nil {
		return err
	}
	fixture.width, fixture.height = width, height
	parts := []string{"HIPPO |", "q quit"}
	if width < 40 || height < 16 {
		parts = []string{fmt.Sprintf("%dx%d", width, height), "q ? help"}
	}
	_, err := fixture.awaitRepaint(action, parts, nil, before.FullClearSequence)
	return err
}
