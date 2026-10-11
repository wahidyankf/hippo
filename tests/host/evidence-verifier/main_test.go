package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestRecorderMatchingCaptureDeadline(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprintf("capture-after-deadline-%t", late), func(t *testing.T) {
			directory := t.TempDir()
			for _, name := range []string{"captures", "events"} {
				if err := os.Mkdir(filepath.Join(directory, name), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			before, after := "Running run-A\n", "Queue wait-A\n"
			beforePath, receiptPath := filepath.Join(directory, "before.txt"), filepath.Join(directory, "input.txt")
			document := evidenceDocument{Host: hostTmux, Target: hostTarget{Socket: "hippo-tui-fixture", Pane: "%1", OwnerToken: strings.Repeat("a", 32)}, Candidate: recordedCandidate{BinarySHA256: digest("candidate")}}
			receipt := fmt.Sprintf("origin=host-native-input\nhost=tmux\nstep=40x16-tab\ninput=tab\npane=%%1\nownerToken=%s\nviewport=40x16\nbinarySHA256=%s\ncommand=controlled input receipt\nstatus=0\nbeforeCaptureSHA256=%s\ncaptureSHA256=%s\n", document.Target.OwnerToken, document.Candidate.BinarySHA256, digest(before), digest(after))
			for name, content := range map[string]string{beforePath: before, receiptPath: receipt} {
				if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			limit := time.Second
			if late {
				limit = time.Millisecond
			}
			options := cliOptions{evidence: directory, step: "40x16-tab", expect: "Queue wait-A", beforeCapture: beforePath, inputReceipt: receiptPath, wait: limit}
			calls := 0
			wait := func(ctx context.Context, supplied time.Duration, host string, target hostTarget, step recordedStep) ([]byte, error) {
				if supplied != limit || host != document.Host || target != document.Target {
					t.Fatal("original wait limit or owned capture target changed")
				}
				bounded, cancel := context.WithTimeout(ctx, supplied)
				defer cancel()
				return pollCapture(bounded, host, target, step, func(actual context.Context, _ string, _ hostTarget) ([]byte, error) {
					calls++
					if actual != bounded {
						t.Fatal("capture lost the original bounded context")
					}
					if late {
						<-actual.Done()
						if actual.Err() != context.DeadlineExceeded {
							t.Fatal("late matching capture did not cross the original deadline")
						}
					}
					return []byte(after), nil
				})
			}
			err := driveStep(context.Background(), options, &document, wait)
			if calls != 1 || len(document.Steps) != 1 {
				t.Fatalf("actual capture/action path was not executed exactly once: calls%d steps%d", calls, len(document.Steps))
			}
			step := document.Steps[0]
			afterPath := filepath.Join(directory, "captures", "40x16-tab.txt")
			if late {
				if err == nil || !strings.Contains(err.Error(), "timed out") {
					t.Errorf("matching capture after original deadline was accepted: %v", err)
				}
				if step.Outcome != "failed" || step.Capture != (artifact{}) {
					t.Errorf("late action or accepted capture was retained incorrectly: %+v", step)
				}
				if _, err := os.Stat(afterPath); !os.IsNotExist(err) {
					t.Errorf("accepted aftercapture was published after deadline: %v", err)
				}
				return
			}
			if err != nil || step.Outcome != "captured" || step.Capture.SHA256 != digest(after) {
				t.Fatalf("matching in-time capture was refused: %v %+v", err, step)
			}
			data, err := os.ReadFile(afterPath)
			if err != nil || string(data) != after {
				t.Fatalf("in-time aftercapture bytes were not retained: %v %q", err, data)
			}
		})
	}
}

func TestEvidenceAcceptsCompleteRecordedSequences(t *testing.T) {
	for _, host := range []string{"tmux", "neovim", "wezterm"} {
		t.Run(host, func(t *testing.T) {
			document, current, artifacts := completeFixture(host)
			if err := Verify(documentReader(t, document), current, artifacts); err != nil {
				t.Fatalf("complete synthetic recorded sequence rejected: %v", err)
			}
		})
	}
}

func TestEvidenceRejectsStaleOrIncompleteCandidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any, *Candidate)
	}{
		{"wrong revision", func(doc map[string]any, _ *Candidate) { candidate(doc)["revision"] = strings.Repeat("b", 40) }},
		{"wrong binary digest", func(doc map[string]any, _ *Candidate) {
			candidate(doc)["binarySHA256"] = digest("other binary")
		}},
		{"wrong embedded commit", func(doc map[string]any, _ *Candidate) {
			candidate(doc)["binaryCommit"] = strings.Repeat("b", 40)
		}},
		{"wrong embedded version", func(doc map[string]any, _ *Candidate) { candidate(doc)["binaryVersion"] = "v0.8.5" }},
		{"changed dirty source", func(_ map[string]any, current *Candidate) {
			current.DirtyPaths["internal/adapters/tui/model.go"] = digest("changed source")
		}},
		{"new dirty path", func(_ map[string]any, current *Candidate) {
			current.DirtyPaths["internal/bootstrap/bootstrap.go"] = digest("new change")
		}},
		{"deleted dirty path", func(_ map[string]any, current *Candidate) {
			delete(current.DirtyPaths, "internal/adapters/tui/model.go")
		}},
		{"changed effective config", func(_ map[string]any, current *Candidate) {
			current.ConfigPaths["host.conf"] = digest("changed config")
		}},
		{"missing config fingerprint", func(doc map[string]any, _ *Candidate) { candidate(doc)["configPaths"] = map[string]string{} }},
		{"missing command", func(doc map[string]any, _ *Candidate) { delete(doc, "command") }},
		{"wrong host", func(doc map[string]any, _ *Candidate) { doc["host"] = "wezterm" }},
		{"missing timestamp", func(doc map[string]any, _ *Candidate) { delete(doc, "recordedAt") }},
		{"future timestamp", func(doc map[string]any, current *Candidate) {
			doc["recordedAt"] = current.Now.Add(time.Second).Format(time.RFC3339Nano)
		}},
		{"unsupported schema", func(doc map[string]any, _ *Candidate) { doc["schemaVersion"] = 2 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			document, current, artifacts := completeFixture("tmux")
			test.mutate(document, &current)
			assertRejected(t, document, current, artifacts)
		})
	}
}

func TestEvidenceRejectsMissingReorderedOrFailedActions(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing shared action", func(doc map[string]any) { actions := steps(doc); doc["steps"] = append(actions[:3], actions[4:]...) }},
		{"reordered shared action", func(doc map[string]any) { actions := steps(doc); actions[0], actions[1] = actions[1], actions[0] }},
		{"duplicate shared action", func(doc map[string]any) { actions := steps(doc); doc["steps"] = append(actions, actions[0]) }},
		{"wrong actual input", func(doc map[string]any) { steps(doc)[0]["input"] = "x" }},
		{"failed action", func(doc map[string]any) { steps(doc)[0]["outcome"] = "failed" }},
		{"timed out action", func(doc map[string]any) { steps(doc)[0]["outcome"] = "timeout" }},
		{"skipped action", func(doc map[string]any) { steps(doc)[0]["outcome"] = "skipped" }},
		{"different expected and actual", func(doc map[string]any) { steps(doc)[0]["actual"] = "Running" }},
		{"missing viewport", func(doc map[string]any) { delete(steps(doc)[0], "width") }},
		{"missing too-small recovery", func(doc map[string]any) { removeStep(doc, "resize-40x16-recovery") }},
		{"missing busy resize", func(doc map[string]any) { removeStep(doc, "resize-pending-refresh") }},
		{"selection not moved", func(doc map[string]any) {
			stepNamed(doc, "40x16-down")["after"] = stepNamed(doc, "40x16-down")["before"]
		}},
		{"refresh count unchanged", func(doc map[string]any) { stepNamed(doc, "40x16-r")["after"] = stepNamed(doc, "40x16-r")["before"] }},
		{"refresh age not reset", func(doc map[string]any) { after(stepNamed(doc, "40x16-r"))["sampleAgeSeconds"] = 10 }},
		{"back selection lost", func(doc map[string]any) { after(stepNamed(doc, "40x16-detail-b"))["selectedID"] = "run-other" }},
		{"broken state continuity", func(doc map[string]any) { before(steps(doc)[1])["selectedID"] = "run-other" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			document, current, artifacts := completeFixture("tmux")
			test.mutate(document)
			assertRejected(t, document, current, artifacts)
		})
	}
}

func TestEvidenceRejectsInventedOrOverflowingCaptures(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any, fstest.MapFS)
	}{
		{"missing raw capture", func(doc map[string]any, artifacts fstest.MapFS) { delete(artifacts, capturePath(steps(doc)[0])) }},
		{"invented reference", func(doc map[string]any, _ fstest.MapFS) { capture(steps(doc)[0])["path"] = "captures/invented.txt" }},
		{"capture digest mismatch", func(doc map[string]any, artifacts fstest.MapFS) {
			artifacts[capturePath(steps(doc)[0])].Data = []byte("fabricated replacement")
		}},
		{"empty capture", func(doc map[string]any, artifacts fstest.MapFS) { replaceCapture(steps(doc)[0], artifacts, "") }},
		{"capture omits actual predicate", func(doc map[string]any, artifacts fstest.MapFS) {
			replaceCapture(steps(doc)[0], artifacts, "Running\n")
		}},
		{"ASCII cell overflow", func(doc map[string]any, artifacts fstest.MapFS) {
			replaceCapture(steps(doc)[0], artifacts, "Queue\n"+strings.Repeat("x", 41))
		}},
		{"wide Unicode cell overflow", func(doc map[string]any, artifacts fstest.MapFS) {
			replaceCapture(steps(doc)[0], artifacts, "Queue\n"+strings.Repeat("\u754c", 21))
		}},
		{"height overflow", func(doc map[string]any, artifacts fstest.MapFS) {
			replaceCapture(steps(doc)[0], artifacts, "Queue\n"+strings.Repeat("row\n", 16))
		}},
		{"capture traversal", func(doc map[string]any, _ fstest.MapFS) { capture(steps(doc)[0])["path"] = "../outside.txt" }},
		{"terminal control injection", func(doc map[string]any, artifacts fstest.MapFS) {
			replaceCapture(steps(doc)[0], artifacts, "Queue\n\x1b]52;c;injected\x07")
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			document, current, artifacts := completeFixture("tmux")
			test.mutate(document, artifacts)
			assertRejected(t, document, current, artifacts)
		})
	}
}

func TestEvidenceAcceptsCellBoundariesWithoutCountingUTF8Bytes(t *testing.T) {
	for _, content := range []string{"Queue\n" + strings.Repeat("\u754c", 20), "Queue\n" + strings.Repeat("e\u0301", 40)} {
		document, current, artifacts := completeFixture("tmux")
		replaceCapture(steps(document)[0], artifacts, content)
		if err := Verify(documentReader(t, document), current, artifacts); err != nil {
			t.Fatalf("40-cell content rejected: %v", err)
		}
	}
}

func TestEvidenceRejectsFalseRestorationAndIncompleteExitPaths(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing help quit", func(doc map[string]any) { removeStep(doc, "quit-help") }},
		{"missing Ctrl-C", func(doc map[string]any) { removeStep(doc, "exit-ctrl-c") }},
		{"missing SIGTERM", func(doc map[string]any) { removeStep(doc, "exit-sigterm") }},
		{"missing startup failure", func(doc map[string]any) { removeStep(doc, "exit-startup-failure") }},
		{"missing read failure", func(doc map[string]any) { removeStep(doc, "exit-read-failure") }},
		{"wrong exit status", func(doc map[string]any) { stepNamed(doc, "exit-ctrl-c")["actualExit"] = 0 }},
		{"false terminal restoration", func(doc map[string]any) { stepNamed(doc, "quit-dashboard")["modesAfter"] = "different termios" }},
		{"missing original modes", func(doc map[string]any) { delete(stepNamed(doc, "quit-dashboard"), "modesBefore") }},
		{"foreground group not restored", func(doc map[string]any) { stepNamed(doc, "quit-dashboard")["foregroundAfter"] = 99 }},
		{"host unusable", func(doc map[string]any) { stepNamed(doc, "quit-dashboard")["hostUsable"] = false }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			document, current, artifacts := completeFixture("tmux")
			test.mutate(document)
			assertRejected(t, document, current, artifacts)
		})
	}
}

func TestEvidenceRequiresActualHostNavigationAndOperatorEvents(t *testing.T) {
	cases := []struct {
		name   string
		host   string
		mutate func(map[string]any, fstest.MapFS)
	}{
		{"tmux missing prefix action", "tmux", func(doc map[string]any, _ fstest.MapFS) { removeStep(doc, "tmux-prefix-action") }},
		{"tmux root key no navigation", "tmux", func(doc map[string]any, _ fstest.MapFS) { stepNamed(doc, "tmux-ctrl-h")["paneAfter"] = "task" }},
		{"tmux missing return", "tmux", func(doc map[string]any, _ fstest.MapFS) { removeStep(doc, "tmux-return-task") }},
		{"Neovim double Escape stays terminal", "neovim", func(doc map[string]any, _ fstest.MapFS) {
			stepNamed(doc, "neovim-double-escape")["editorModeAfter"] = "terminal-input"
		}},
		{"Neovim input not resumed", "neovim", func(doc map[string]any, _ fstest.MapFS) { removeStep(doc, "neovim-resume-input") }},
		{"WezTerm missing leader navigation", "wezterm", func(doc map[string]any, _ fstest.MapFS) { removeStep(doc, "wezterm-cmd-b-navigation") }},
		{"WezTerm availability is not input", "wezterm", func(doc map[string]any, artifacts fstest.MapFS) {
			replaceOperatorEvent(t, doc, artifacts, "CLI available", "cli-availability")
		}},
		{"WezTerm success checkbox is not input", "wezterm", func(doc map[string]any, artifacts fstest.MapFS) {
			replaceOperatorEvent(t, doc, artifacts, "success=true", "manual-checkbox")
		}},
		{"WezTerm missing operator artifact", "wezterm", func(doc map[string]any, artifacts fstest.MapFS) {
			delete(artifacts, eventPath(stepNamed(doc, "wezterm-cmd-b-navigation")))
		}},
		{"WezTerm mismatched operator artifact", "wezterm", func(doc map[string]any, artifacts fstest.MapFS) {
			artifacts[eventPath(stepNamed(doc, "wezterm-cmd-b-navigation"))].Data = []byte("another event")
		}},
		{"WezTerm no active pane change", "wezterm", func(doc map[string]any, _ fstest.MapFS) {
			stepNamed(doc, "wezterm-cmd-b-navigation")["paneAfter"] = "task"
		}},
		{"WezTerm missing Shift-Enter", "wezterm", func(doc map[string]any, _ fstest.MapFS) { removeStep(doc, "wezterm-shift-enter") }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			document, current, artifacts := completeFixture(test.host)
			test.mutate(document, artifacts)
			assertRejected(t, document, current, artifacts)
		})
	}
}

func TestEvidenceRejectsMalformedDocumentsAndUnreadableArtifacts(t *testing.T) {
	document, current, artifacts := completeFixture("tmux")
	for _, raw := range []string{"{", "null", "{}", "[]", "{\"schemaVersion\":1}{}"} {
		t.Run(raw, func(t *testing.T) {
			if err := Verify(strings.NewReader(raw), current, artifacts); err == nil {
				t.Fatal("malformed or incomplete document accepted")
			}
		})
	}
	t.Run("artifact read error", func(t *testing.T) {
		artifacts[capturePath(steps(document)[0])] = &fstest.MapFile{Mode: fs.ModeDir | 0o700}
		if err := Verify(documentReader(t, document), current, artifacts); err == nil {
			t.Fatal("unreadable captures accepted")
		}
	})
}

func completeFixture(host string) (map[string]any, Candidate, fstest.MapFS) {
	now := time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC)
	current := Candidate{Host: host, Revision: strings.Repeat("a", 40), BinarySHA256: digest("candidate binary"), BinaryVersion: "v0.0.0-test", BinaryCommit: strings.Repeat("a", 40), Now: now, DirtyPaths: map[string]string{"internal/adapters/tui/model.go": digest("source")}, ConfigPaths: map[string]string{"host.conf": digest("effective config")}}
	doc := map[string]any{"schemaVersion": 1, "host": host, "recordedAt": now.Add(-time.Second).Format(time.RFC3339Nano), "command": []string{"/task/hippo", "tui", "--interval", "1s"}, "candidate": map[string]any{"revision": current.Revision, "binarySHA256": current.BinarySHA256, "binaryVersion": current.BinaryVersion, "binaryCommit": current.BinaryCommit, "dirtyPaths": map[string]string{"internal/adapters/tui/model.go": digest("source")}, "configPaths": map[string]string{"host.conf": digest("effective config")}}}
	artifacts := fstest.MapFS{}
	actions := make([]map[string]any, 0, 80)
	for _, size := range [][2]int{{40, 16}, {60, 20}, {100, 30}} {
		actions = append(actions, sharedSequence(size[0], size[1], artifacts)...)
	}
	for _, action := range []struct{ id, input, text string }{{"resize-39x15", "resize", "Terminal too small"}, {"resize-40x16-recovery", "resize", "Running"}, {"resize-pending-refresh", "resize", "Running pending refresh"}} {
		width, height := 40, 16
		if action.id == "resize-39x15" {
			width, height = 39, 15
		}
		entry := capturedStep(action.id, action.input, action.text, width, height, artifacts)
		entry["before"], entry["after"] = state("dashboard", "running", "run-A", 0, 1, 0), state("dashboard", "running", "run-A", 0, 1, 0)
		if action.id == "resize-pending-refresh" {
			entry["refreshPending"] = true
		}
		actions = append(actions, entry)
	}
	for _, action := range []struct {
		id, input string
		code      int
	}{{"quit-dashboard", "q", 0}, {"quit-detail", "q", 0}, {"quit-help", "q", 0}, {"exit-ctrl-c", "Ctrl-C", 130}, {"exit-sigterm", "SIGTERM", 143}, {"exit-startup-failure", "startup-failure", 125}, {"exit-read-failure", "read-failure", 125}} {
		entry := capturedStep(action.id, action.input, "host ready", 40, 16, artifacts)
		entry["expectedExit"], entry["actualExit"] = action.code, action.code
		entry["modesBefore"], entry["modesAfter"] = "iflag=100 lflag=200", "iflag=100 lflag=200"
		entry["foregroundBefore"], entry["foregroundAfter"], entry["hostUsable"] = 42, 42, true
		actions = append(actions, entry)
	}
	actions = append(actions, hostSequence(host, artifacts)...)
	doc["fixtures"] = map[string][]string{"running": {"run-A", "run-B"}, "queue": {"wait-A", "wait-B"}}
	doc["steps"] = actions
	return doc, current, artifacts
}

func sharedSequence(width, height int, artifacts fstest.MapFS) []map[string]any {
	inputs := []string{"tab", "shift-tab", "n", "p", "down", "up", "j", "k", "enter", "detail-j", "detail-k", "detail-b", "c", "capacity-b", "r", "?", "help-b"}
	view, section, selected, scroll, refresh, age := "dashboard", "running", "run-A", 0, 0, 10
	actions := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		prior := state(view, section, selected, scroll, refresh, age)
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
		entry := capturedStep(fmt.Sprintf("%dx%d-%s", width, height, input), key, text, width, height, artifacts)
		entry["before"], entry["after"] = prior, state(view, section, selected, scroll, refresh, age)
		actions = append(actions, entry)
	}
	return actions
}

func hostSequence(host string, artifacts fstest.MapFS) []map[string]any {
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
		entry := capturedStep(host+"-"+input, input, "host action observed", 40, 16, artifacts)
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
			entry["operatorEvent"] = artifactReference("events/cmd-b.txt", event, artifacts)
		}
		entries = append(entries, entry)
	}
	return entries
}

func capturedStep(id, input, text string, width, height int, artifacts fstest.MapFS) map[string]any {
	return map[string]any{"id": id, "input": input, "width": width, "height": height, "expected": text, "actual": text, "outcome": "passed", "capture": artifactReference("captures/"+id+".txt", "HIPPO\n"+text+"\n", artifacts)}
}

func artifactReference(path, content string, artifacts fstest.MapFS) map[string]any {
	artifacts[path] = &fstest.MapFile{Data: []byte(content), Mode: 0o600}
	return map[string]any{"path": path, "sha256": digest(content)}
}

func state(view, section, selected string, scroll, refresh, age int) map[string]any {
	return map[string]any{"view": view, "section": section, "selectedID": selected, "scroll": scroll, "refreshCount": refresh, "sampleAgeSeconds": age}
}

func candidate(doc map[string]any) map[string]any {
	value, _ := doc["candidate"].(map[string]any)
	return value
}

func steps(doc map[string]any) []map[string]any {
	value, _ := doc["steps"].([]map[string]any)
	return value
}

func capture(entry map[string]any) map[string]any {
	value, _ := entry["capture"].(map[string]any)
	return value
}

func capturePath(entry map[string]any) string {
	value, _ := capture(entry)["path"].(string)
	return value
}

func eventPath(entry map[string]any) string {
	event, _ := entry["operatorEvent"].(map[string]any)
	value, _ := event["path"].(string)
	return value
}

func before(entry map[string]any) map[string]any {
	value, _ := entry["before"].(map[string]any)
	return value
}

func after(entry map[string]any) map[string]any {
	value, _ := entry["after"].(map[string]any)
	return value
}

func stepNamed(doc map[string]any, id string) map[string]any {
	for _, entry := range steps(doc) {
		if entry["id"] == id {
			return entry
		}
	}
	return nil
}

func removeStep(doc map[string]any, id string) {
	filtered := make([]map[string]any, 0, len(steps(doc)))
	for _, entry := range steps(doc) {
		if entry["id"] != id {
			filtered = append(filtered, entry)
		}
	}
	doc["steps"] = filtered
}

func replaceCapture(entry map[string]any, artifacts fstest.MapFS, content string) {
	entry["capture"] = artifactReference(capturePath(entry), content, artifacts)
}

func replaceOperatorEvent(t *testing.T, doc map[string]any, artifacts fstest.MapFS, input, origin string) {
	t.Helper()
	entry := stepNamed(doc, "wezterm-cmd-b-navigation")
	if entry == nil {
		t.Fatal("missing fixture step wezterm-cmd-b-navigation")
	}
	content := "origin=" + origin + "\nhost=wezterm\nstep=wezterm-cmd-b-navigation\ninput=" + input + "\npaneBefore=task\npaneAfter=adjacent\n"
	entry["operatorEvent"] = artifactReference(eventPath(entry), content, artifacts)
}

func digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func documentReader(t *testing.T, document map[string]any) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return bytes.NewReader(data)
}

func assertRejected(t *testing.T, document map[string]any, current Candidate, artifacts fs.FS) {
	t.Helper()
	if err := Verify(documentReader(t, document), current, artifacts); err == nil {
		t.Fatal("invalid evidence accepted without an error")
	}
}
