package main

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

func verifyNativeArtifacts(document evidenceDocument, step recordedStep, artifacts fs.FS) error {
	if document.SourceRoot != "" && (step.BeforeCapture.Path == "" || externalInputRequired(document.Host, step.ID) && step.InputReceipt.Path == "") {
		return errors.New("recorded native step lacks before capture or required actual input receipt")
	}
	if step.BeforeCapture.Path != "" {
		before, err := readArtifact(artifacts, step.BeforeCapture)
		if err != nil {
			return err
		}
		if len(before) == 0 {
			return errors.New("native before-action capture is empty")
		}
		// The preceding action may use another protocol viewport; all fit the largest one.
		if err := verifyCaptureCells(string(before), 100, 30); err != nil {
			return err
		}
	}
	if step.InputReceipt.Path != "" {
		data, err := readArtifact(artifacts, step.InputReceipt)
		if err != nil {
			return err
		}
		if err := verifyInputReceipt(data, document, step); err != nil {
			return err
		}
		if strings.HasPrefix(step.ID, fmt.Sprintf("%dx%d-", step.Width, step.Height)) {
			return verifyCaptureAge(step, artifacts)
		}
		return nil
	}
	return nil
}

func externalInputRequired(host, id string) bool {
	return host == hostWezterm || strings.HasPrefix(id, "tmux-") || strings.HasPrefix(id, "neovim-") ||
		strings.HasPrefix(id, "quit-") || strings.HasPrefix(id, "exit-") || id == pendingResizeID
}

func verifyInputReceipt(data []byte, document evidenceDocument, step recordedStep) error {
	fields, err := receiptFields(data)
	if err != nil {
		return err
	}
	for name, expected := range map[string]string{
		"origin": "host-native-input", "host": document.Host, "step": step.ID, "input": step.Input,
		"pane": document.Target.Pane, "ownerToken": document.Target.OwnerToken,
		"viewport": fmt.Sprintf("%dx%d", step.Width, step.Height), "binarySHA256": document.Candidate.BinarySHA256,
		"beforeCaptureSHA256": step.BeforeCapture.SHA256, "captureSHA256": step.Capture.SHA256,
	} {
		if expected == "" || fields[name] != expected {
			return errors.New("external native input receipt differs from candidate/action/captures")
		}
	}
	status, err := strconv.Atoi(fields["status"])
	if err != nil || status < 0 || status > 255 || fields["command"] == "" {
		return errors.New("external native input receipt lacks actual command/status")
	}
	if step.Outcome == outcomePassed {
		return verifyObservedNativeFacts(fields, step)
	}
	return nil
}

func receiptFields(data []byte) (map[string]string, error) {
	fields := map[string]string{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" {
			continue
		}
		name, value, found := strings.Cut(line, "=")
		if !found || name == "" {
			return nil, errors.New("native input receipt has a malformed field")
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, errors.New("native input receipt has duplicate fields")
		}
		fields[name] = value
	}
	return fields, nil
}

func verifyObservedNativeFacts(fields map[string]string, step recordedStep) error {
	facts := map[string]string{}
	switch {
	case strings.HasPrefix(step.ID, "quit-") || strings.HasPrefix(step.ID, "exit-"):
		facts = map[string]string{
			"actualExit": strconv.Itoa(step.ActualExit), "modesBefore": step.ModesBefore, "modesAfter": step.ModesAfter,
			"foregroundBefore": strconv.Itoa(step.ForegroundBefore), "foregroundAfter": strconv.Itoa(step.ForegroundAfter),
			"hostUsable": strconv.FormatBool(step.HostUsable),
		}
	case strings.HasPrefix(step.ID, "tmux-") || strings.HasPrefix(step.ID, "wezterm-"):
		for name, value := range map[string]string{"paneBefore": step.PaneBefore, "paneAfter": step.PaneAfter, "paneReturn": step.PaneReturn} {
			if value != "" {
				facts[name] = value
			}
		}
	case strings.HasPrefix(step.ID, "neovim-"):
		facts = map[string]string{"editorModeBefore": step.EditorModeBefore, "editorModeAfter": step.EditorModeAfter}
	}
	for name, value := range facts {
		if value == "" || fields[name] != value {
			return errors.New("inspection differs from actual native state receipt")
		}
	}
	return nil
}
