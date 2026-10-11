package host_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecorderDrivesOrderedCaptureAndBindsObservation(t *testing.T) {
	verifier := compiledVerifier(t)
	fixture, source, binary, config, fixtures, tools, owner, _, _ := recordingFixture(t)
	evidence := filepath.Join(fixture, "proof")
	prepareRecording(t, verifier, fixture, source, binary, config, fixtures, tools, owner, evidence)
	if err := os.WriteFile(filepath.Join(fixture, "capture.txt"), []byte("Queue wait-A\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--record", "--host", "tmux", "--binary", binary, "--evidence", evidence}
	output, err := invokeRecorder(verifier, fixture, tools, append(args, "--step", "40x16-tab", "--expect", "Queue wait-A")...)
	if err != nil {
		t.Fatalf("ordered native drive rejected: %v output%q", err, output)
	}
	document := readProtocolDocument(t, evidence)
	steps, validSteps := document["steps"].([]any)
	if !validSteps || len(steps) != 1 {
		t.Fatalf("drive did not retain exactly one pending capture: %v", document)
	}
	step, validStep := steps[0].(map[string]any)
	if !validStep || step["outcome"] == "passed" || step["input"] != "tab" {
		t.Fatalf("drive invented inspected outcome or input: %v", steps)
	}
	argv, err := os.ReadFile(filepath.Join(fixture, "host-argv.bin"))
	if err != nil || !strings.Contains(string(argv), "send-keys\x00-t\x00%1\x00Tab\x00") || !strings.Contains(string(argv), "resize-window\x00-t\x00%1\x00-x\x0040\x00-y\x0016\x00") {
		t.Fatalf("native driver omitted explicit owned target/input/size: %v %q", err, argv)
	}
	step["expected"], step["actual"], step["outcome"] = "Queue wait-A", "Queue wait-A", "passed"
	step["before"] = map[string]any{"view": "dashboard", "section": "running", "selectedID": "run-A"}
	step["after"] = map[string]any{"view": "dashboard", "section": "queue", "selectedID": "wait-A"}
	observation := filepath.Join(fixture, "inspection.json")
	writeProtocolJSON(t, observation, step)
	output, err = invokeRecorder(verifier, fixture, tools, append(args, "--observation", observation)...)
	if err != nil {
		t.Fatalf("capture-bound inspection rejected: %v output%q", err, output)
	}
	document = readProtocolDocument(t, evidence)
	steps, validSteps = document["steps"].([]any)
	if !validSteps || len(steps) != 1 {
		t.Fatal("inspection lost captured action")
	}
	step, validStep = steps[0].(map[string]any)
	if !validStep || step["outcome"] != "passed" {
		t.Fatalf("inspection not attached: %v", steps)
	}
	output, err = invokeRecorder(verifier, fixture, tools, append(args, "--finalize")...)
	if err == nil || !strings.Contains(string(output), "ordered steps") {
		t.Fatalf("incomplete finalization did not reject actual missing steps: %v %q", err, output)
	}
}

func TestRecorderRetainsTimedOutActionAndRejectsAmbiguousStages(t *testing.T) {
	verifier := compiledVerifier(t)
	fixture, source, binary, config, fixtures, tools, owner, _, _ := recordingFixture(t)
	evidence := filepath.Join(fixture, "proof")
	prepareRecording(t, verifier, fixture, source, binary, config, fixtures, tools, owner, evidence)
	args := []string{"--record", "--host", "tmux", "--binary", binary, "--evidence", evidence}
	output, err := invokeRecorder(verifier, fixture, tools, append(args, "--step", "40x16-tab", "--finalize", "--expect", "missing", "--wait", "100ms")...)
	if err == nil || !strings.Contains(string(output), "one recording stage") {
		t.Errorf("ambiguous stages not refused before action: %v %q", err, output)
	}
	output, err = invokeRecorder(verifier, fixture, tools, append(args, "--step", "40x16-tab", "--expect", "missing", "--wait", "100ms")...)
	if err == nil || !strings.Contains(string(output), "timed out") {
		t.Fatalf("bounded missing predicate did not fail: %v %q", err, output)
	}
	document := readProtocolDocument(t, evidence)
	steps, validSteps := document["steps"].([]any)
	if !validSteps || len(steps) != 1 {
		t.Fatalf("timed-out action disappeared from evidence: %v", document)
	}
	step, validStep := steps[0].(map[string]any)
	if !validStep || step["outcome"] != "failed" {
		t.Fatalf("timed-out action not retained as failed: %v", steps)
	}
}

func TestRecorderRejectsUnknownDocumentFieldsBeforeRecording(t *testing.T) {
	verifier := compiledVerifier(t)
	fixture, source, binary, config, fixtures, tools, owner, _, _ := recordingFixture(t)
	evidence := filepath.Join(fixture, "proof")
	prepareRecording(t, verifier, fixture, source, binary, config, fixtures, tools, owner, evidence)
	document := readProtocolDocument(t, evidence)
	document["inventedOutcome"] = "passed"
	writeProtocolJSON(t, filepath.Join(evidence, "evidence.json"), document)
	output, err := invokeRecorder(verifier, fixture, tools, "--record", "--host", "tmux", "--binary", binary, "--evidence", evidence, "--finalize")
	if err == nil || !strings.Contains(string(output), "unknown field") {
		t.Fatalf("unknown evidence field was silently dropped: %v %q", err, output)
	}
}

func TestRecorderNativeViewportMustMatchRequestedSize(t *testing.T) {
	verifier := compiledVerifier(t)
	fixture, source, binary, config, fixtures, tools, owner, _, _ := recordingFixture(t)
	evidence := filepath.Join(fixture, "proof")
	prepareRecording(t, verifier, fixture, source, binary, config, fixtures, tools, owner, evidence)
	host := "#!/usr/bin/env bash\nset -euo pipefail\ncase \"$*\" in\n *display-message*) printf '%s\\n' '39x15' ;;\n *capture-pane*) printf '%s\\n' 'Queue wait-A' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(tools, "tmux"), []byte(host), 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := invokeRecorder(verifier, fixture, tools, "--record", "--host", "tmux", "--binary", binary, "--evidence", evidence, "--step", "40x16-tab", "--expect", "Queue wait-A")
	if err == nil || !strings.Contains(string(output), "actual native viewport") {
		t.Fatalf("different actual pane viewport was accepted: %v %q", err, output)
	}
}

func TestRecorderBindsExternalNativeInputToExactImportedCaptures(t *testing.T) {
	verifier := compiledVerifier(t)
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprintf("changed-capture-digest-%t", changed), func(t *testing.T) {
			assertExternalNativeCapture(t, verifier, changed)
		})
	}
}

func assertExternalNativeCapture(t *testing.T, verifier string, changed bool) {
	t.Helper()
	fixture, source, binary, config, fixtures, tools, owner, _, _ := recordingFixture(t)
	evidence := filepath.Join(fixture, "proof")
	prepareRecording(t, verifier, fixture, source, binary, config, fixtures, tools, owner, evidence)
	document := readProtocolDocument(t, evidence)
	candidate, valid := document["candidate"].(map[string]any)
	if !valid {
		t.Fatal("prepared candidate missing")
	}
	before, after := "Running run-A\n", "Queue wait-A\n"
	beforePath, afterPath := filepath.Join(fixture, "before capture.txt"), filepath.Join(fixture, "after capture.txt")
	for name, content := range map[string]string{beforePath: before, afterPath: after} {
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	captureDigest := finalizationDigest(after)
	if changed {
		captureDigest = strings.Repeat("0", 64)
	}
	receipt := fmt.Sprintf("origin=host-native-input\nhost=tmux\nstep=40x16-tab\ninput=tab\npane=%%1\nownerToken=%s\nviewport=40x16\nbinarySHA256=%v\ncommand=synthetic native input stand-in\nstatus=0\nbeforeCaptureSHA256=%s\ncaptureSHA256=%s\n", owner, candidate["binarySHA256"], finalizationDigest(before), captureDigest)
	eventPath := filepath.Join(fixture, "native receipt.txt")
	if err := os.WriteFile(eventPath, []byte(receipt), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := invokeRecorder(verifier, fixture, tools, "--record", "--host", "tmux", "--binary", binary, "--evidence", evidence, "--step", "40x16-tab", "--expect", "Queue wait-A", "--input-receipt", eventPath, "--before-capture", beforePath, "--after-capture", afterPath)
	if changed {
		if err == nil || !strings.Contains(string(output), "differs") {
			t.Fatalf("altered receipt capture digest accepted: %v %q", err, output)
		}
		return
	}
	if err != nil {
		t.Fatalf("capture-bound external native fixture rejected: %v %q", err, output)
	}
	for suffix, expected := range map[string]string{"-before.txt": before, ".txt": after} {
		data, err := os.ReadFile(filepath.Join(evidence, "captures", "40x16-tab"+suffix))
		if err != nil || string(data) != expected {
			t.Fatalf("native imported capture bytes changed: %v %q", err, data)
		}
	}
}

func readProtocolDocument(t *testing.T, directory string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func writeProtocolJSON(t *testing.T, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
