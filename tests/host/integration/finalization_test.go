package host_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// This is a synthetic complete format fixture; it authenticates no real native host.
func TestRecorderFinalizesCompleteCapturedFormatAndRejectsChangedConfig(t *testing.T) {
	verifier := compiledVerifier(t)
	fixture, source, binary, config, fixtures, tools, owner, _, _ := recordingFixture(t)
	evidence := filepath.Join(fixture, "proof")
	prepareRecording(t, verifier, fixture, source, binary, config, fixtures, tools, owner, evidence)
	document := readProtocolDocument(t, evidence)
	candidate, valid := document["candidate"].(map[string]any)
	if !valid {
		t.Fatal("prepared candidate missing")
	}
	artifacts := fstest.MapFS{}
	entries := make([]map[string]any, 0, 68)
	for _, size := range [][2]int{{40, 16}, {60, 20}, {100, 30}} {
		entries = append(entries, finalizationSharedSequence(size[0], size[1], artifacts)...)
	}
	for _, resize := range []struct {
		id            string
		width, height int
	}{{"resize-39x15", 39, 15}, {"resize-40x16-recovery", 40, 16}, {"resize-pending-refresh", 40, 16}} {
		entry := finalizationCapturedStep(resize.id, "resize", "Running", resize.width, resize.height, artifacts)
		entry["before"], entry["after"] = finalizationState("dashboard", "running", "run-A", 0, 1, 0), finalizationState("dashboard", "running", "run-A", 0, 1, 0)
		if resize.id == "resize-pending-refresh" {
			entry["refreshPending"] = true
		}
		entries = append(entries, entry)
	}
	for _, exit := range []struct {
		id, input string
		code      int
	}{{"quit-dashboard", "q", 0}, {"quit-detail", "q", 0}, {"quit-help", "q", 0}, {"exit-ctrl-c", "Ctrl-C", 130}, {"exit-sigterm", "SIGTERM", 143}, {"exit-startup-failure", "startup-failure", 125}, {"exit-read-failure", "read-failure", 125}} {
		entry := finalizationCapturedStep(exit.id, exit.input, "host ready", 40, 16, artifacts)
		entry["expectedExit"], entry["actualExit"] = exit.code, exit.code
		entry["modesBefore"], entry["modesAfter"] = "iflag=100 lflag=200", "iflag=100 lflag=200"
		entry["foregroundBefore"], entry["foregroundAfter"], entry["hostUsable"] = 42, 42, true
		entries = append(entries, entry)
	}
	entries = append(entries, finalizationHostSequence("tmux", artifacts)...)
	// Logical fixture clocks model ordered capture windows, not real host time.
	clock := int64(time.Second)
	for index, entry := range entries {
		stepID := fmt.Sprint(entry["id"])
		prior, valid := entry["beforeCapture"].(map[string]any)
		if !valid {
			prior = finalizationArtifactReference("captures/"+stepID+"-before.txt", "synthetic native before capture\n", artifacts)
			entry["beforeCapture"] = prior
		}
		after, valid := entry["capture"].(map[string]any)
		if !valid {
			t.Fatal("fixture capture missing")
		}
		event := fmt.Sprintf("origin=host-native-input\nhost=tmux\nstep=%s\ninput=%v\npane=%%1\nownerToken=%s\nviewport=%vx%v\nbinarySHA256=%v\ncommand=synthetic native stand-in\nstatus=0\nbeforeCaptureSHA256=%v\ncaptureSHA256=%v\n", stepID, entry["input"], owner, entry["width"], entry["height"], candidate["binarySHA256"], prior["sha256"], after["sha256"])
		lines := []string{strings.TrimSuffix(event, "\n")}
		afterEnd := clock + int64(300*time.Millisecond)
		if index < 51 && entry["input"] == "r" {
			// Completed refresh models the real two-reading readiness boundary.
			afterEnd = clock + int64(1300*time.Millisecond)
		}
		lines = append(lines,
			fmt.Sprintf("beforeCaptureStartMonotonicNS=%d", clock),
			fmt.Sprintf("beforeCaptureEndMonotonicNS=%d", clock+int64(100*time.Millisecond)),
			fmt.Sprintf("captureStartMonotonicNS=%d", clock+int64(200*time.Millisecond)),
			fmt.Sprintf("captureEndMonotonicNS=%d", afterEnd))
		clock = afterEnd + int64(100*time.Millisecond)
		for _, name := range []string{"actualExit", "modesBefore", "modesAfter", "foregroundBefore", "foregroundAfter", "hostUsable", "paneBefore", "paneAfter", "paneReturn"} {
			if value, present := entry[name]; present {
				lines = append(lines, fmt.Sprintf("%s=%v", name, value))
			}
		}
		entry["inputReceipt"] = finalizationArtifactReference("events/"+stepID+"-input.txt", strings.Join(lines, "\n")+"\n", artifacts)
	}
	document["steps"] = entries
	for name, file := range artifacts {
		if err := os.WriteFile(filepath.Join(evidence, filepath.FromSlash(name)), file.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeProtocolJSON(t, filepath.Join(evidence, "evidence.json"), document)
	args := []string{"--host", "tmux", "--binary", binary, "--evidence", evidence}
	for _, extra := range [][]string{nil, {"--record", "--finalize"}} {
		output, err := invokeRecorder(verifier, fixture, tools, append(args, extra...)...)
		if err != nil {
			t.Fatalf("complete synthetic format finalization rejected: %v %q", err, output)
		}
	}
	if err := os.WriteFile(config, []byte("changed effective config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := invokeRecorder(verifier, fixture, tools, append(args, "--record", "--finalize")...)
	if err == nil || !strings.Contains(string(output), "configuration changed") {
		t.Fatalf("changed effective config accepted: %v %q", err, output)
	}
}

// Format fixtures are kept equivalent to the verifier's independently frozen contracts.
func finalizationSharedSequence(width, height int, artifacts fstest.MapFS) []map[string]any {
	inputs := []string{"tab", "shift-tab", "n", "p", "down", "up", "j", "k", "enter", "detail-j", "detail-k", "detail-b", "c", "capacity-b", "r", "?", "help-b"}
	view, section, selected, scroll, refresh, age := "dashboard", "running", "run-A", 0, 0, 10
	actions := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		prior := finalizationState(view, section, selected, scroll, refresh, age)
		key, text := input, "Running"
		switch input {
		case "tab", "n":
			section, selected, text = "queue", "wait-A", "Queue"
		case "shift-tab", "p":
			section, selected = "running", "run-A"
		case "down", "j":
			selected = "run-B"
		case "up", "k":
			selected = "run-A"
		case "enter":
			view, text = "detail", "run-A"
		case "detail-j":
			key, scroll, text = "j", 1, "run-A"
		case "detail-k":
			key, scroll, text = "k", 0, "run-A"
		case "detail-b", "capacity-b", "help-b":
			key, view = "b", "dashboard"
		case "c":
			view, text = "capacity", "Capacity"
		case "r":
			refresh, age = refresh+1, 0
		case "?":
			view, text = "help", "Help"
		}
		entry := finalizationCapturedStep(fmt.Sprintf("%dx%d-%s", width, height, input), key, text, width, height, artifacts)
		entry["before"], entry["after"] = prior, finalizationState(view, section, selected, scroll, refresh, age)
		entry["beforeCapture"] = finalizationArtifactReference(fmt.Sprintf("captures/%dx%d-%s-before.txt", width, height, input), fmt.Sprintf("HIPPO\n%v\nLIVE | age %vs\n", prior["view"], prior["sampleAgeSeconds"]), artifacts)
		entry["capture"] = finalizationArtifactReference(fmt.Sprintf("captures/%dx%d-%s.txt", width, height, input), fmt.Sprintf("HIPPO\n%s\nLIVE | age %ds\n", text, age), artifacts)
		actions = append(actions, entry)
	}
	return actions
}

func finalizationHostSequence(host string, artifacts fstest.MapFS) []map[string]any {
	var inputs []string
	switch host {
	case "tmux":
		inputs = []string{"ctrl-h", "ctrl-l", "ctrl-j", "ctrl-k", "ctrl-backslash", "prefix-action", "return-task"}
	case "neovim":
		inputs = []string{"double-escape", "resume-input", "n", "p", "editor-usable"}
	case "wezterm":
		inputs = []string{"cmd-b-navigation", "return-task", "shift-enter", "q-accessible"}
	}
	entries := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		entry := finalizationCapturedStep(host+"-"+input, input, "host action observed", 40, 16, artifacts)
		entry["paneBefore"], entry["paneAfter"], entry["paneReturn"] = "task", "adjacent", "task"
		if input == "return-task" {
			entry["paneBefore"], entry["paneAfter"] = "adjacent", "task"
		}
		if host == "neovim" {
			entry["editorModeBefore"], entry["editorModeAfter"] = "terminal-input", "normal"
			if input == "resume-input" {
				entry["editorModeBefore"], entry["editorModeAfter"] = "normal", "terminal-input"
			}
			if input == "n" || input == "p" {
				entry["editorModeBefore"], entry["editorModeAfter"] = "terminal-input", "terminal-input"
				entry["sectionBefore"], entry["sectionAfter"] = "running", "queue"
				if input == "p" {
					entry["sectionBefore"], entry["sectionAfter"] = "queue", "running"
				}
			}
		}
		if input == "cmd-b-navigation" {
			event := "origin=operator-key-event\nhost=wezterm\nstep=wezterm-cmd-b-navigation\ninput=Cmd-B,h\npaneBefore=task\npaneAfter=adjacent\n"
			entry["operatorEvent"] = finalizationArtifactReference("events/cmd-b.txt", event, artifacts)
		}
		entries = append(entries, entry)
	}
	return entries
}

func finalizationCapturedStep(id, input, text string, width, height int, artifacts fstest.MapFS) map[string]any {
	return map[string]any{"id": id, "input": input, "width": width, "height": height, "expected": text, "actual": text, "outcome": "passed", "capture": finalizationArtifactReference("captures/"+id+".txt", "HIPPO\n"+text+"\n", artifacts)}
}

func finalizationArtifactReference(path, content string, artifacts fstest.MapFS) map[string]any {
	artifacts[path] = &fstest.MapFile{Data: []byte(content), Mode: 0o600}
	return map[string]any{"path": path, "sha256": finalizationDigest(content)}
}

func finalizationState(view, section, selected string, scroll, refresh, age int) map[string]any {
	return map[string]any{"view": view, "section": section, "selectedID": selected, "scroll": scroll, "refreshCount": refresh, "sampleAgeSeconds": age}
}

func finalizationDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
