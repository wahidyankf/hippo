package tui

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/wahidyankf/hippo/internal/status"
)

func newBodyViewport() viewport.Model {
	body := viewport.New()
	body.KeyMap.PageUp.SetEnabled(false)
	body.KeyMap.PageDown.SetEnabled(false)
	body.KeyMap.HalfPageUp.SetEnabled(false)
	body.KeyMap.HalfPageDown.SetEnabled(false)
	body.KeyMap.Left.SetEnabled(false)
	body.KeyMap.Right.SetEnabled(false)
	body.MouseWheelEnabled = false
	return body
}

func (state *model) handleKey(message tea.KeyPressMsg) tea.Cmd {
	stroke := message.Keystroke()
	switch stroke {
	case "ctrl+c":
		state.interruption = status.Interruption{Signal: 2, Name: "interrupt"}
		return state.quit()
	case "q":
		return state.quit()
	case "r":
		state.requestObservation()
	case "?":
		state.openHelp()
	case "b":
		state.back()
	default:
		state.navigate(message, stroke)
	}
	return nil
}

func (state *model) quit() tea.Cmd {
	state.quitting = true
	if state.effects.cancel != nil {
		state.effects.cancel()
	}
	return tea.Quit
}

func (state *model) navigate(message tea.KeyPressMsg, stroke string) {
	if state.view != dashboardView {
		if stroke == "j" || stroke == "down" || stroke == "k" || stroke == "up" {
			state.body, _ = state.body.Update(message)
		}
		return
	}
	switch stroke {
	case "tab", "shift+tab", "n", "p":
		if state.section == runningSection {
			state.section = queueSection
		} else {
			state.section = runningSection
		}
	case "down", "j":
		state.moveSelection(1)
	case "up", "k":
		state.moveSelection(-1)
	case "enter":
		if selected := state.selected[state.section].runID; selected != "" {
			state.detailID = selected
			state.openView(jobView)
		}
	case "c":
		state.openView(capacityView)
	}
}

func (state *model) openView(view viewKind) {
	state.view = view
	state.body.GotoTop()
	state.syncBody()
}

func (state *model) openHelp() {
	if state.view == helpView {
		return
	}
	state.previousView, state.previousOffset = state.view, state.body.YOffset()
	state.openView(helpView)
}

func (state *model) back() {
	if state.view == helpView {
		state.view = state.previousView
		state.syncBody()
		state.body.SetYOffset(state.previousOffset)
		return
	}
	state.view = dashboardView
}
