package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runRecording(ctx context.Context, options cliOptions, stdout, stderr io.Writer) int {
	var err error
	if options.prepare || options.step == "" && options.observation == "" && !options.finalize {
		err = prepareEvidence(ctx, options)
	} else {
		err = continueRecording(ctx, options)
	}
	if err != nil {
		if _, outputError := fmt.Fprintln(stderr, "host-evidence:", err); outputError != nil {
			return 125
		}
		return 125
	}
	if _, err := fmt.Fprintln(stdout, "recording stage saved; trusted native input/captures and visual inspection remain required"); err != nil {
		return 125
	}
	return 0
}

func prepareEvidence(ctx context.Context, options cliOptions) error {
	if _, err := os.Lstat(options.evidence); !os.IsNotExist(err) {
		return errors.New("evidence directory must be fresh and task-owned")
	}
	if err := validateTarget(options.host, options.target); err != nil {
		return err
	}
	current, err := observeCandidate(ctx, options.host, options.binary, options.sourceRoot, options.configRoots)
	if err != nil {
		return err
	}
	fixtureData, err := os.ReadFile(options.fixtures)
	if err != nil {
		return fmt.Errorf("read actual fixture identities: %w", err)
	}
	var fixtures map[string][]string
	if err := json.Unmarshal(fixtureData, &fixtures); err != nil {
		return err
	}
	if err := verifyFixtures(fixtures); err != nil {
		return err
	}
	capture, err := captureTarget(ctx, options.host, options.target)
	if err != nil {
		return err
	}
	if !strings.Contains(string(capture), "HIPPO_HOST_EVIDENCE_OWNER:"+options.target.OwnerToken) {
		return errors.New("target has no matching actual task ownership marker")
	}
	document := evidenceDocument{
		SchemaVersion: 1, Host: options.host, RecordedAt: time.Now().UTC(), Command: []string{options.binary, "tui", "--interval", "60s", "--color", "never"},
		Candidate: recordedCandidate{Revision: current.Revision, DirtyPaths: current.DirtyPaths, ConfigPaths: current.ConfigPaths, BinarySHA256: current.BinarySHA256, BinaryVersion: current.BinaryVersion, BinaryCommit: current.BinaryCommit},
		Fixtures:  fixtures, Steps: []recordedStep{}, SourceRoot: options.sourceRoot, ConfigRoots: []string(options.configRoots), Target: options.target,
	}
	if err := os.Mkdir(options.evidence, 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(options.evidence, "captures"), 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(options.evidence, "events"), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(options.evidence, "ownership-capture.txt"), capture, 0o600); err != nil {
		return err
	}
	return publishDocument(options.evidence, document)
}

func validateTarget(host string, target hostTarget) error {
	if !hexadecimal(target.OwnerToken, 32) || target.Pane == "" {
		return errors.New("explicit task pane and 32-hex ownership token are required")
	}
	if host == hostTmux || host == hostNeovim {
		if !strings.HasPrefix(target.Socket, "hippo-tui-") {
			return errors.New("host must use an isolated hippo-tui- socket")
		}
		if !strings.HasPrefix(target.Pane, "%") {
			return errors.New("tmux task pane must be an explicit pane ID")
		}
	}
	if host == hostNeovim && !filepath.IsAbs(target.EditorServer) {
		return errors.New("task editor requires its explicit absolute RPC socket")
	}
	return nil
}

func captureTarget(ctx context.Context, host string, target hostTarget) ([]byte, error) {
	if host == hostWezterm {
		return runCommand(ctx, "", hostWezterm, "cli", "get-text", "--pane-id", target.Pane)
	}
	return runCommand(ctx, "", hostTmux, "-L", target.Socket, "capture-pane", "-p", "-t", target.Pane)
}

func publishDocument(directory string, document evidenceDocument) error {
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".evidence-*.json")
	if err != nil {
		return err
	}
	name := file.Name()
	if _, writeError := file.Write(append(encoded, '\n')); writeError != nil {
		return errors.Join(writeError, file.Close(), os.Remove(name))
	}
	if err := file.Close(); err != nil {
		return errors.Join(err, os.Remove(name))
	}
	if err := os.Rename(name, filepath.Join(directory, "evidence.json")); err != nil {
		return errors.Join(err, os.Remove(name))
	}
	return nil
}
