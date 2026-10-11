package main

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

func sequence(host string) []plannedStep {
	actions := make([]plannedStep, 0, 80)
	for _, size := range [][2]int{{40, 16}, {60, 20}, {100, 30}} {
		for _, action := range []struct{ name, input string }{
			{inputTab, inputTab},
			{inputShiftTab, inputShiftTab},
			{"n", "n"},
			{"p", "p"},
			{inputDown, inputDown},
			{"up", "up"},
			{"j", "j"},
			{"k", "k"},
			{inputEnter, inputEnter},
			{"detail-j", "j"},
			{"detail-k", "k"},
			{"detail-b", "b"},
			{"c", "c"},
			{"capacity-b", "b"},
			{"r", "r"},
			{"?", "?"},
			{"help-b", "b"},
		} {
			actions = append(actions, plannedStep{id: fmt.Sprintf("%dx%d-%s", size[0], size[1], action.name), input: action.input, width: size[0], height: size[1]})
		}
	}
	actions = append(actions, []plannedStep{
		{id: "resize-39x15", input: inputResize, width: 39, height: 15},
		{id: "resize-40x16-recovery", input: inputResize, width: 40, height: 16},
		{id: pendingResizeID, input: inputResize, width: 40, height: 16},
	}...)
	for _, exit := range []plannedStep{
		{id: "quit-dashboard", input: "q"},
		{id: "quit-detail", input: "q"},
		{id: "quit-help", input: "q"},
		{id: "exit-ctrl-c", input: "Ctrl-C", exit: 130},
		{id: "exit-sigterm", input: "SIGTERM", exit: 143},
		{id: "exit-startup-failure", input: "startup-failure", exit: 125},
		{id: "exit-read-failure", input: "read-failure", exit: 125},
	} {
		exit.width, exit.height = 40, 16
		actions = append(actions, exit)
	}
	var hostInputs []string
	switch host {
	case hostTmux:
		hostInputs = []string{"ctrl-h", "ctrl-l", "ctrl-j", "ctrl-k", "ctrl-backslash", "prefix-action", "return-task"}
	case hostNeovim:
		hostInputs = []string{"double-escape", "resume-input", "n", "p", "editor-usable"}
	case hostWezterm:
		hostInputs = []string{"cmd-b-navigation", "return-task", "shift-enter", "q-accessible"}
	}
	for _, input := range hostInputs {
		actions = append(actions, plannedStep{id: host + "-" + input, input: input, width: 40, height: 16})
	}
	return actions
}

func verifySharedState(steps []recordedStep, fixtures map[string][]string, artifacts fs.FS) error {
	for offset := 0; offset < 51; offset += 17 {
		for index, step := range steps[offset : offset+17] {
			if index > 0 && !sameApplicationState(step.Before, steps[offset+index-1].After) {
				return errors.New("shared action state continuity differs")
			}
			if index > 0 {
				if err := verifyAgeContinuity(steps[offset+index-1], step, artifacts); err != nil {
					return err
				}
			}
			if err := verifyStateTransition(step, fixtures); err != nil {
				return fmt.Errorf("%s: %w", step.ID, err)
			}
		}
	}
	for _, step := range steps[51:54] {
		if step.After.SelectedID != steps[50].After.SelectedID || step.After.Section != steps[50].After.Section {
			return errors.New("resize lost selection or section")
		}
	}
	return nil
}

func verifyStateTransition(step recordedStep, fixtures map[string][]string) error {
	prior, actual := step.Before, step.After
	running, queue := fixtures["running"], fixtures["queue"]
	if len(running) < 2 || len(queue) < 2 {
		return errors.New("observed running or queue fixture rows are missing")
	}
	if len(fixtures[prior.Section]) < 2 || prior.SelectedID == "" {
		return errors.New("observed section or selection is missing")
	}
	expected := prior
	parts := strings.SplitN(step.ID, "-", 2)
	action := parts[1]
	switch action {
	case inputTab, "n":
		expected.Section, expected.SelectedID = "queue", queue[0]
	case inputShiftTab, "p":
		expected.Section, expected.SelectedID = "running", running[0]
	case inputDown, "j":
		expected.SelectedID = fixtures[prior.Section][1]
	case "up", "k":
		expected.SelectedID = fixtures[prior.Section][0]
	case inputEnter:
		expected.View = "detail"
	case "detail-j":
		expected.Scroll++
	case "detail-k":
		expected.Scroll--
	case "detail-b", "capacity-b", "help-b":
		expected.View = "dashboard"
	case "c":
		expected.View = "capacity"
	case "r":
		expected.RefreshCount++
		expected.SampleAgeSeconds = 0
	case "?":
		expected.View = "help"
	}
	if !sameApplicationState(expected, actual) || actual.Scroll < 0 || actual.SampleAgeSeconds < 0 ||
		(action == "r" && actual.SampleAgeSeconds != 0) ||
		(action != "r" && step.InputReceipt.Path == "" && prior.SampleAgeSeconds != actual.SampleAgeSeconds) {
		return errors.New("observed view transition differs from actual input")
	}
	return nil
}

// Time-dependent age has its separate capture-bound check. Every user/application
// state field remains in the exact equality comparison.
func sameApplicationState(left, right viewState) bool {
	left.SampleAgeSeconds, right.SampleAgeSeconds = 0, 0
	return left == right
}
