package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"
)

func continueRecording(ctx context.Context, options cliOptions) error {
	document, root, err := openDocument(options.evidence)
	if err != nil {
		return err
	}
	defer func() { reportClose(root.Close()) }()
	if err := validateTarget(document.Host, document.Target); err != nil {
		return err
	}
	if document.Host != options.host || len(document.Command) < 2 || document.Command[0] != options.binary {
		return errors.New("requested host or binary differs from prepared target")
	}
	current, err := observeCandidate(ctx, options.host, options.binary, document.SourceRoot, document.ConfigRoots)
	if err != nil {
		return err
	}
	if err := verifyCandidate(document, current); err != nil {
		return err
	}
	switch {
	case options.finalize:
		encoded, err := json.Marshal(document)
		if err != nil {
			return err
		}
		return Verify(bytes.NewReader(encoded), current, root.FS())
	case options.observation != "":
		if err := attachObservation(options, &document, root); err != nil {
			return err
		}
	case options.step != "":
		if err := driveStep(ctx, options, &document, waitCapture); err != nil {
			return errors.Join(err, publishDocument(options.evidence, document))
		}
	default:
		return errors.New("select one recording stage")
	}
	return publishDocument(options.evidence, document)
}

func reportClose(err error) {
	if err != nil {
		// A close failure is reported without replacing the already recorded stage result.
		if _, outputError := fmt.Fprintln(os.Stderr, "host-evidence: close evidence:", err); outputError != nil {
			return
		}
	}
}

type captureWait func(context.Context, time.Duration, string, hostTarget, recordedStep) ([]byte, error)

func driveStep(ctx context.Context, options cliOptions, document *evidenceDocument, wait captureWait) error {
	plan := sequence(document.Host)
	index := len(document.Steps)
	if index >= len(plan) || plan[index].id != options.step {
		return errors.New("requested step is not the next ordered action")
	}
	if index > 0 && document.Steps[index-1].Outcome != outcomePassed {
		return errors.New("inspect and attach the preceding capture before another action")
	}
	if options.expect == "" {
		return errors.New("--expect must name the actual visible predicate to await")
	}
	planned := plan[index]
	step := recordedStep{ID: planned.id, Input: planned.input, Width: planned.width, Height: planned.height, Expected: options.expect, Outcome: "captured", ExpectedExit: planned.exit}
	step, err := captureStep(ctx, options, document, planned, step, wait)
	if err != nil {
		step.Outcome = "failed"
	}
	document.Steps = append(document.Steps, step)
	return err
}

func captureStep(ctx context.Context, options cliOptions, document *evidenceDocument, planned plannedStep, step recordedStep, wait captureWait) (recordedStep, error) {
	before, err := sourceCapture(ctx, options.beforeCapture, document.Host, document.Target)
	if err != nil {
		return step, err
	}
	step.BeforeCapture, err = saveArtifact(options.evidence, "captures/"+planned.id+"-before.txt", before)
	if err != nil {
		return step, err
	}
	if options.inputReceipt == "" {
		err = driveNativeInput(ctx, document.Host, document.Target, planned)
	}
	if err != nil {
		return step, err
	}
	var after []byte
	if options.afterCapture != "" {
		after, err = boundedFile(options.afterCapture)
		if err == nil && !strings.Contains(string(after), step.Expected) {
			err = errors.New("external native capture omits actual visible predicate")
		}
		if err == nil {
			err = verifyCaptureCells(string(after), step.Width, step.Height)
		}
	} else {
		after, err = wait(ctx, options.wait, document.Host, document.Target, step)
	}
	if err != nil {
		return step, err
	}
	step.Capture, err = saveArtifact(options.evidence, "captures/"+planned.id+".txt", after)
	if err != nil {
		return step, err
	}
	if options.inputReceipt != "" {
		data, err := boundedFile(options.inputReceipt)
		if err != nil {
			return step, err
		}
		if err := verifyInputReceipt(data, *document, step); err != nil {
			return step, err
		}
		step.InputReceipt, err = saveArtifact(options.evidence, "events/"+step.ID+"-input.txt", data)
		if err != nil {
			return step, err
		}
	}
	if options.operatorEvents != "" {
		data, err := boundedFile(options.operatorEvents)
		if err != nil {
			return step, err
		}
		step.OperatorEvent, err = saveArtifact(options.evidence, "events/"+planned.id+"-operator.txt", data)
		if err != nil {
			return step, err
		}
	}
	return step, nil
}

func sourceCapture(ctx context.Context, file, host string, target hostTarget) ([]byte, error) {
	if file != "" {
		return boundedFile(file)
	}
	return captureTarget(ctx, host, target)
}

func waitCapture(ctx context.Context, limit time.Duration, host string, target hostTarget, step recordedStep) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	return pollCapture(bounded, host, target, step, captureTarget)
}

func pollCapture(bounded context.Context, host string, target hostTarget, step recordedStep, capture func(context.Context, string, hostTarget) ([]byte, error)) ([]byte, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		capture, err := capture(bounded, host, target)
		if err != nil {
			if errors.Is(bounded.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("visible predicate wait timed out; action is not passed: %w", err)
			}
			return nil, err
		}
		if strings.Contains(string(capture), step.Expected) {
			if err := verifyCaptureCells(string(capture), step.Width, step.Height); err != nil {
				return nil, err
			}
			if err := bounded.Err(); err != nil {
				return nil, fmt.Errorf("visible predicate wait timed out; action is not passed: %w", err)
			}
			return capture, nil
		}
		select {
		case <-bounded.Done():
			return nil, errors.New("visible predicate wait timed out; action is not passed")
		case <-ticker.C:
		}
	}
}

func boundedFile(name string) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	data, readError := io.ReadAll(io.LimitReader(file, maximumArtifactBytes+1))
	if closeError := file.Close(); readError != nil || closeError != nil {
		return nil, errors.Join(readError, closeError)
	}
	if len(data) > maximumArtifactBytes {
		return nil, errors.New("artifact exceeds size limit")
	}
	return data, nil
}

func saveArtifact(directory, name string, data []byte) (artifact, error) {
	if len(data) > maximumArtifactBytes || !fs.ValidPath(name) {
		return artifact{}, errors.New("artifact exceeds size limit")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return artifact{}, err
	}
	writeError := root.WriteFile(name, data, 0o600)
	if closeError := root.Close(); writeError != nil || closeError != nil {
		return artifact{}, errors.Join(writeError, closeError)
	}
	sum := sha256.Sum256(data)
	return artifact{Path: name, SHA256: hex.EncodeToString(sum[:])}, nil
}
