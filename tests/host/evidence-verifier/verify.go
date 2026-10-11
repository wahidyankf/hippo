package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

const (
	maximumDocumentBytes = 8 << 20
	maximumArtifactBytes = 2 << 20
)

func verifyDocument(reader io.Reader, current Candidate, artifacts fs.FS) error {
	document, err := decodeEvidence(reader)
	if err != nil {
		return err
	}
	return verifyRecordedDocument(document, current, artifacts)
}

func decodeEvidence(reader io.Reader) (evidenceDocument, error) {
	var document evidenceDocument
	data, err := io.ReadAll(io.LimitReader(reader, maximumDocumentBytes+1))
	if err != nil {
		return document, fmt.Errorf("read evidence: %w", err)
	}
	if len(data) > maximumDocumentBytes {
		return document, errors.New("evidence exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return document, fmt.Errorf("decode evidence: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return document, errors.New("evidence has trailing JSON data")
	}
	return document, nil
}

func verifyRecordedDocument(document evidenceDocument, current Candidate, artifacts fs.FS) error {
	if err := verifyCandidate(document, current); err != nil {
		return err
	}
	if err := verifyFixtures(document.Fixtures); err != nil {
		return err
	}
	plan := sequence(document.Host)
	if len(document.Steps) != len(plan) {
		return fmt.Errorf("expected %d ordered steps, got %d", len(plan), len(document.Steps))
	}
	for index, step := range document.Steps {
		if err := verifyNativeArtifacts(document, step, artifacts); err != nil {
			return fmt.Errorf("step %d %q: %w", index+1, step.ID, err)
		}
		if err := verifyStep(step, plan[index], artifacts); err != nil {
			return fmt.Errorf("step %d %q: %w", index+1, step.ID, err)
		}
	}
	return verifySharedState(document.Steps, document.Fixtures, artifacts)
}

func verifyCandidate(document evidenceDocument, current Candidate) error {
	if document.SchemaVersion != 1 || document.Host != current.Host || !slices.Contains([]string{hostTmux, hostNeovim, hostWezterm}, document.Host) {
		return errors.New("unsupported schema or mismatched host")
	}
	if document.RecordedAt.IsZero() || current.Now.IsZero() || document.RecordedAt.After(current.Now) {
		return errors.New("invalid evidence timestamp")
	}
	if len(document.Command) < 2 || !filepath.IsAbs(document.Command[0]) || document.Command[1] != "tui" {
		return errors.New("missing absolute TUI command")
	}
	recorded := document.Candidate
	if !hexadecimal(recorded.Revision, 40) || recorded.Revision != current.Revision ||
		!hexadecimal(recorded.BinarySHA256, 64) || recorded.BinarySHA256 != current.BinarySHA256 ||
		!hexadecimal(recorded.BinaryCommit, 40) || recorded.BinaryCommit != current.BinaryCommit ||
		recorded.BinaryVersion == "" || recorded.BinaryVersion != current.BinaryVersion {
		return errors.New("source or binary identity changed")
	}
	if !maps.Equal(recorded.DirtyPaths, current.DirtyPaths) || len(recorded.ConfigPaths) == 0 ||
		!maps.Equal(recorded.ConfigPaths, current.ConfigPaths) {
		return errors.New("dirty source or effective configuration changed")
	}
	for _, fingerprints := range []map[string]string{recorded.DirtyPaths, recorded.ConfigPaths} {
		for name, digest := range fingerprints {
			if name == "" || !hexadecimal(digest, 64) {
				return errors.New("missing content fingerprint")
			}
		}
	}
	return nil
}

func hexadecimal(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}

func verifyFixtures(fixtures map[string][]string) error {
	seen := map[string]bool{}
	for _, section := range []string{"running", "queue"} {
		if len(fixtures[section]) < 2 {
			return fmt.Errorf("%s needs two known fixture rows", section)
		}
		for _, identity := range fixtures[section] {
			if identity == "" || seen[identity] {
				return errors.New("fixture identities are empty or duplicated")
			}
			seen[identity] = true
		}
	}
	return nil
}

func verifyStep(step recordedStep, expected plannedStep, artifacts fs.FS) error {
	if step.ID != expected.id || step.Input != expected.input || step.Width != expected.width || step.Height != expected.height {
		return errors.New("ordered action or viewport does not match protocol")
	}
	if step.Outcome != outcomePassed || step.Expected == "" || step.Actual != step.Expected {
		return errors.New("expected outcome was not observed")
	}
	capture, err := readArtifact(artifacts, step.Capture)
	if err != nil {
		return err
	}
	if len(capture) == 0 || !strings.Contains(string(capture), step.Actual) {
		return errors.New("capture does not contain the observed predicate")
	}
	if err := verifyCaptureCells(string(capture), step.Width, step.Height); err != nil {
		return err
	}
	if strings.HasPrefix(step.ID, "quit-") || strings.HasPrefix(step.ID, "exit-") {
		return verifyExit(step, expected.exit)
	}
	if strings.HasPrefix(step.ID, "tmux-") || strings.HasPrefix(step.ID, "neovim-") || strings.HasPrefix(step.ID, "wezterm-") {
		return verifyHostAction(step, artifacts)
	}
	if step.ID == pendingResizeID && !step.RefreshPending {
		return errors.New("pending-refresh resize did not run while busy")
	}
	return nil
}

func readArtifact(artifacts fs.FS, reference artifact) ([]byte, error) {
	if !fs.ValidPath(reference.Path) || !hexadecimal(reference.SHA256, 64) {
		return nil, errors.New("invalid artifact reference")
	}
	data, err := fs.ReadFile(artifacts, reference.Path)
	if err != nil {
		return nil, fmt.Errorf("read artifact %q: %w", reference.Path, err)
	}
	if len(data) > maximumArtifactBytes {
		return nil, errors.New("artifact exceeds size limit")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != reference.SHA256 {
		return nil, errors.New("artifact digest differs from captured bytes")
	}
	return data, nil
}

func verifyExit(step recordedStep, expected int) error {
	if step.ExpectedExit != expected || step.ActualExit != expected || step.ModesBefore == "" ||
		step.ModesAfter != step.ModesBefore || step.ForegroundBefore <= 0 ||
		step.ForegroundAfter != step.ForegroundBefore || !step.HostUsable {
		return errors.New("exit result or terminal/host restoration differs")
	}
	return nil
}

func verifyHostAction(step recordedStep, artifacts fs.FS) error {
	if strings.HasPrefix(step.ID, "neovim-") {
		return verifyEditorAction(step)
	}
	if strings.HasSuffix(step.ID, "return-task") {
		if step.PaneBefore == "" || step.PaneAfter == "" || step.PaneBefore == step.PaneAfter {
			return errors.New("host did not return to task pane")
		}
		return nil
	}
	if strings.HasPrefix(step.ID, "tmux-") || step.ID == "wezterm-cmd-b-navigation" {
		if step.PaneBefore == "" || step.PaneAfter == "" || step.PaneBefore == step.PaneAfter || step.PaneReturn != step.PaneBefore {
			return errors.New("host navigation or return was not observed")
		}
	}
	if step.ID == "wezterm-cmd-b-navigation" {
		return verifyOperatorEvent(step, artifacts)
	}
	return nil
}

func verifyEditorAction(step recordedStep) error {
	switch step.ID {
	case "neovim-double-escape", "neovim-editor-usable":
		if step.EditorModeBefore != terminalInputMode || step.EditorModeAfter != "normal" {
			return errors.New("editor did not regain normal-mode control")
		}
	case "neovim-resume-input":
		if step.EditorModeBefore != "normal" || step.EditorModeAfter != terminalInputMode {
			return errors.New("editor did not resume terminal input")
		}
	case "neovim-n", "neovim-p":
		if step.EditorModeAfter != terminalInputMode || step.SectionBefore == step.SectionAfter {
			return errors.New("TUI section did not change after editor input resumed")
		}
	}
	return nil
}

func verifyOperatorEvent(step recordedStep, artifacts fs.FS) error {
	data, err := readArtifact(artifacts, step.OperatorEvent)
	if err != nil {
		return err
	}
	fields := strings.Split(string(data), "\n")
	if !slices.Contains(fields, "input=Cmd-B,h") && !slices.Contains(fields, "input=Cmd-B,e,f,h") {
		return errors.New("native leader action lacks matching configured input sequence")
	}
	for _, field := range []string{"origin=operator-key-event", "host=wezterm", "step=" + step.ID, "paneBefore=" + step.PaneBefore, "paneAfter=" + step.PaneAfter} {
		if !slices.Contains(fields, field) {
			return errors.New("native leader action lacks matching actual operator event")
		}
	}
	return nil
}
