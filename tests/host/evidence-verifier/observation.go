package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
)

func attachObservation(options cliOptions, document *evidenceDocument, root *os.Root) error {
	index := len(document.Steps) - 1
	if index < 0 || document.Steps[index].Outcome != "captured" {
		return errors.New("observation needs the pending native capture")
	}
	data, err := boundedFile(options.observation)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var observed recordedStep
	if err := decoder.Decode(&observed); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("observation has trailing JSON data")
	}
	captured := document.Steps[index]
	if observed.ID != captured.ID || observed.Input != captured.Input || observed.Width != captured.Width ||
		observed.Height != captured.Height || observed.Expected != captured.Expected ||
		observed.Capture != captured.Capture || observed.BeforeCapture != captured.BeforeCapture ||
		observed.InputReceipt != captured.InputReceipt || observed.OperatorEvent != captured.OperatorEvent {
		return errors.New("observation differs from the actual capture-bound action")
	}
	before, err := readArtifact(root.FS(), captured.BeforeCapture)
	if err != nil {
		return err
	}
	if len(before) == 0 {
		return errors.New("before-action capture is empty")
	}
	plan := sequence(document.Host)
	if index >= len(plan) {
		return errors.New("observation exceeds ordered protocol")
	}
	if err := verifyStep(observed, plan[index], root.FS()); err != nil {
		return err
	}
	if err := verifyNativeArtifacts(*document, observed, root.FS()); err != nil {
		return err
	}
	if err := verifyAttachedSharedState(index, *document, observed, root.FS()); err != nil {
		return err
	}
	document.Steps[index] = observed
	return nil
}

func verifyAttachedSharedState(index int, document evidenceDocument, observed recordedStep, artifacts fs.FS) error {
	if index >= 51 {
		return nil
	}
	if err := verifyStateTransition(observed, document.Fixtures); err != nil {
		return err
	}
	if index%17 == 0 {
		return nil
	}
	if !sameApplicationState(observed.Before, document.Steps[index-1].After) {
		return errors.New("observation state differs from preceding inspected action")
	}
	return verifyAgeContinuity(document.Steps[index-1], observed, artifacts)
}
