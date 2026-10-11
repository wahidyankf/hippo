package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

func driveNativeInput(ctx context.Context, host string, target hostTarget, step plannedStep) error {
	if externalInputRequired(host, step.id) {
		return errors.New("this host action or induced condition requires an actual --input-receipt")
	}
	if _, err := runCommand(ctx, "", hostTmux, "-L", target.Socket, "resize-window", "-t", target.Pane, "-x", strconv.Itoa(step.width), "-y", strconv.Itoa(step.height)); err != nil {
		return err
	}
	if err := verifyNativeViewport(ctx, host, target, step); err != nil {
		return err
	}
	if step.input == inputResize {
		return nil
	}
	keys := map[string]string{
		inputTab: tmuxTab, inputShiftTab: tmuxBackTab, inputDown: tmuxDown, "up": tmuxUp, inputEnter: tmuxEnter, "Ctrl-C": tmuxInterrupt,
		"n": "n", "p": "p", "j": "j", "k": "k", "b": "b", "c": "c", "r": "r", "?": "?", "q": "q",
		"double-escape": "<Esc><Esc>", "resume-input": "i", "editor-usable": "<Esc><Esc>",
	}
	key, ok := keys[step.input]
	if !ok {
		return errors.New("native driver has no input binding")
	}
	if host == hostNeovim {
		switch key {
		case tmuxTab, tmuxBackTab, tmuxDown, tmuxUp, tmuxEnter, tmuxInterrupt:
			key = map[string]string{tmuxTab: "<Tab>", tmuxBackTab: "<S-Tab>", tmuxDown: "<Down>", tmuxUp: "<Up>", tmuxEnter: "<CR>", tmuxInterrupt: "<C-c>"}[key]
		}
		encoded, err := json.Marshal(key)
		if err != nil {
			return err
		}
		_, err = runCommand(ctx, "", "nvim", "--server", target.EditorServer, "--remote-expr", "nvim_input("+string(encoded)+")")
		return err
	}
	_, err := runCommand(ctx, "", hostTmux, "-L", target.Socket, "send-keys", "-t", target.Pane, key)
	return err
}

func verifyNativeViewport(ctx context.Context, host string, target hostTarget, step plannedStep) error {
	var actual []byte
	var err error
	if host == hostNeovim {
		actual, err = runCommand(ctx, "", "nvim", "--server", target.EditorServer, "--remote-expr", "printf('%dx%d', nvim_win_get_width(0), nvim_win_get_height(0))")
	} else {
		actual, err = runCommand(ctx, "", hostTmux, "-L", target.Socket, "display-message", "-p", "-t", target.Pane, "#{pane_width}x#{pane_height}")
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(actual)) != strconv.Itoa(step.width)+"x"+strconv.Itoa(step.height) {
		return errors.New("actual native viewport differs from requested terminal cells; use root native resize/capture receipt")
	}
	return nil
}
