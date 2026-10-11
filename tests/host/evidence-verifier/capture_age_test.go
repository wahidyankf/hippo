package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// This calls the actual staged attach function, so a missing continuity call
// cannot hide behind checks performed only by the full final verifier.
func TestStagedObservationChecksAgeBeforeAdvancing(t *testing.T) {
	for _, test := range []struct {
		name       string
		age        int
		priorStart int64
		accepted   bool
	}{
		{"legal tick", 1, 1000000000, true},
		{"unbounded age jump between stages", 10, 1000000000, false},
		{"reversed stage clocks", 0, 2000000000, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			root, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := root.Close(); err != nil {
					t.Error(err)
				}
			})
			write := func(name, text string) artifact {
				if err := root.WriteFile(name, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
				return artifact{Path: name, SHA256: digest(text)}
			}
			target := hostTarget{Pane: "%1", OwnerToken: strings.Repeat("b", 32)}
			binarySHA := strings.Repeat("a", 64)
			makeStep := func(actionID, input, text string, age int, start int64) recordedStep {
				beforeText := fmt.Sprintf("HIPPO\nQueue\nLIVE | age %ds\n", age)
				afterText := fmt.Sprintf("HIPPO\n%s\nLIVE | age %ds\n", text, age)
				beforeRef := write(actionID+"-before.txt", beforeText)
				afterRef := write(actionID+"-after.txt", afterText)
				receipt := fmt.Sprintf("origin=host-native-input\nhost=neovim\nstep=%s\ninput=%s\npane=%%1\nownerToken=%s\nviewport=40x16\nbinarySHA256=%s\nbeforeCaptureSHA256=%s\ncaptureSHA256=%s\ncommand=synthetic-unit-input\nstatus=0\nbeforeCaptureStartMonotonicNS=%d\nbeforeCaptureEndMonotonicNS=%d\ncaptureStartMonotonicNS=%d\ncaptureEndMonotonicNS=%d\n", actionID, input, target.OwnerToken, binarySHA, beforeRef.SHA256, afterRef.SHA256, start, start+100000000, start+200000000, start+300000000)
				return recordedStep{
					ID: actionID, Input: input, Width: 40, Height: 16, Expected: text, Actual: text, Outcome: outcomePassed,
					BeforeCapture: beforeRef, Capture: afterRef, InputReceipt: write(actionID+"-receipt.txt", receipt),
				}
			}
			prior := makeStep("40x16-tab", "tab", "Queue", 0, test.priorStart)
			prior.After = viewState{View: "dashboard", Section: "queue", SelectedID: "wait-A"}
			current := makeStep("40x16-shift-tab", "shift-tab", "Running", test.age, 1400000000)
			current.Before = viewState{View: "dashboard", Section: "queue", SelectedID: "wait-A", SampleAgeSeconds: test.age}
			current.After = viewState{View: "dashboard", Section: "running", SelectedID: "run-A", SampleAgeSeconds: test.age}
			data, err := json.Marshal(current)
			if err != nil {
				t.Fatal(err)
			}
			if err := root.WriteFile("inspection.json", data, 0o600); err != nil {
				t.Fatal(err)
			}
			pending := current
			pending.Outcome = "captured"
			document := evidenceDocument{
				Host: hostNeovim, SourceRoot: "/synthetic-unit-source", Target: target,
				Candidate: recordedCandidate{BinarySHA256: binarySHA},
				Fixtures:  map[string][]string{"running": {"run-A", "run-B"}, "queue": {"wait-A", "wait-B"}},
				Steps:     []recordedStep{prior, pending},
			}
			err = attachObservation(cliOptions{observation: filepath.Join(directory, "inspection.json")}, &document, root)
			if (err == nil) != test.accepted {
				t.Fatalf("staged age accepted=%v: %v", err == nil, err)
			}
			if !test.accepted && document.Steps[1].Outcome != "captured" {
				t.Fatal("failed staged age advanced protocol")
			}
		})
	}
}

// Source-only regression cases; Go execution awaits the sole build/test lease.
func TestVisibleClockAgeHasFiniteElapsedBound(t *testing.T) {
	for _, test := range []struct {
		name          string
		before, after int
		elapsed       time.Duration
		accepted      bool
	}{
		{"same displayed second", 3, 3, 100 * time.Millisecond, true},
		{"real one-second display tick", 3, 4, 100 * time.Millisecond, true},
		{"elapsed stages between captures", 3, 13, 10 * time.Second, true},
		{"unbounded jump", 3, 10, 100 * time.Millisecond, false},
		{"unsolicited reset", 3, 0, 100 * time.Millisecond, false},
		{"negative age", -1, 0, time.Second, false},
		{"reversed clock", 3, 4, -time.Second, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := verifyAgeAdvance(test.before, test.after, 100*time.Second, 100*time.Second+test.elapsed)
			if (err == nil) != test.accepted {
				t.Fatalf("observed age bound accepted=%v: %v", err == nil, err)
			}
		})
	}
}

func TestCaptureAgeRequiresTheActualDisplayedAge(t *testing.T) {
	for _, test := range []struct {
		text     string
		age      int
		accepted bool
	}{
		{"HIPPO | Running and Queue\nLIVE | age 4s\n", 4, true},
		{"LIVE | age 1m4s\n", 64, true},
		{"LIVE | refreshing | age 4s\n", 4, true},
		{"LIVE | age 4s\n", 0, false},
		{"LIVE | age unavailable\n", 0, false},
		{"LIVE | age 4s\nSTALE | age 4s\n", 4, false},
	} {
		if err := verifyDisplayedAge([]byte(test.text), test.age); (err == nil) != test.accepted {
			t.Fatalf("actual display %q accepted=%v: %v", test.text, err == nil, err)
		}
	}
}

func TestAgeDoesNotHideApplicationState(t *testing.T) {
	prior := viewState{View: "dashboard", Section: "running", SelectedID: "real-A", Scroll: 0, RefreshCount: 1, SampleAgeSeconds: 3}
	legal := prior
	legal.SampleAgeSeconds = 4
	if !sameApplicationState(prior, legal) {
		t.Fatal("legal display age tick changed application state")
	}
	for _, mutate := range []func(*viewState){
		func(state *viewState) { state.View = "detail" },
		func(state *viewState) { state.Section = "queue" },
		func(state *viewState) { state.SelectedID = "real-B" },
		func(state *viewState) { state.Scroll = 1 },
		func(state *viewState) { state.RefreshCount = 2 },
	} {
		changed := legal
		mutate(&changed)
		if sameApplicationState(prior, changed) {
			t.Fatal("age comparison hid application state change")
		}
	}
}

func TestClockRequiresOrderedBoundedCaptureWindows(t *testing.T) {
	valid := map[string]string{"beforeCaptureStartMonotonicNS": "1000000000", "beforeCaptureEndMonotonicNS": "1100000000", "captureStartMonotonicNS": "1200000000", "captureEndMonotonicNS": "1300000000"}
	if _, err := parseCaptureClock(valid); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"beforeCaptureStartMonotonicNS", "beforeCaptureEndMonotonicNS", "captureStartMonotonicNS", "captureEndMonotonicNS"} {
		bad := maps.Clone(valid)
		bad[key] = "0"
		if _, err := parseCaptureClock(bad); err == nil {
			t.Fatal("missing clock accepted")
		}
	}
	if _, err := parseCaptureClock(map[string]string{"beforeCaptureStartMonotonicNS": "1", "beforeCaptureEndMonotonicNS": "2", "captureStartMonotonicNS": "3", "captureEndMonotonicNS": strings.Repeat("9", 30)}); err == nil {
		t.Fatal("overflow clock accepted")
	}
}

func TestCaptureClockClassifiesOnlyCanonicalCompletedRefresh(t *testing.T) {
	for _, test := range []struct {
		name     string
		step     recordedStep
		elapsed  time.Duration
		accepted bool
	}{
		{"40x16 refresh after one second", recordedStep{ID: "40x16-r", Input: "r", Width: 40, Height: 16}, 1200 * time.Millisecond, true},
		{"60x20 refresh at readiness limit", recordedStep{ID: "60x20-r", Input: "r", Width: 60, Height: 20}, 3 * time.Second, true},
		{"100x30 refresh after readiness limit", recordedStep{ID: "100x30-r", Input: "r", Width: 100, Height: 30}, 3*time.Second + time.Nanosecond, false},
		{"ordinary navigation at action limit", recordedStep{ID: "40x16-tab", Input: "tab", Width: 40, Height: 16}, 500 * time.Millisecond, true},
		{"ordinary navigation after action limit", recordedStep{ID: "40x16-tab", Input: "tab", Width: 40, Height: 16}, 500*time.Millisecond + time.Nanosecond, false},
		{"resize remains an input action", recordedStep{ID: "resize-pending-refresh", Input: "resize", Width: 40, Height: 16}, time.Second, false},
		{"suffix is not a canonical refresh", recordedStep{ID: "unexpected-r", Input: "r", Width: 40, Height: 16}, time.Second, false},
		{"canonical id with wrong input", recordedStep{ID: "40x16-r", Input: "n", Width: 40, Height: 16}, time.Second, false},
		{"canonical id with wrong viewport", recordedStep{ID: "40x16-r", Input: "r", Width: 60, Height: 20}, time.Second, false},
		{"input alone is not refresh identity", recordedStep{ID: "40x16-tab", Input: "r", Width: 40, Height: 16}, time.Second, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fields := map[string]string{
				"beforeCaptureStartMonotonicNS": "1000000000",
				"beforeCaptureEndMonotonicNS":   "1100000000",
				"captureStartMonotonicNS":       "1200000000",
				"captureEndMonotonicNS":         strconv.FormatInt(int64(1100*time.Millisecond+test.elapsed), 10),
			}
			text := captureClockFields(fields)
			test.step.InputReceipt = artifact{Path: "receipt.txt", SHA256: digest(text)}
			artifacts := fstest.MapFS{"receipt.txt": &fstest.MapFile{Data: []byte(text)}}
			_, err := readCaptureClock(test.step, artifacts)
			if (err == nil) != test.accepted {
				t.Fatalf("clock accepted=%v: %v", err == nil, err)
			}
		})
	}
}

func captureClockFields(fields map[string]string) string {
	parts := make([]string, 0, 4)
	for _, key := range []string{"beforeCaptureStartMonotonicNS", "beforeCaptureEndMonotonicNS", "captureStartMonotonicNS", "captureEndMonotonicNS"} {
		parts = append(parts, key+"="+fields[key]+"\n")
	}
	return strings.Join(parts, "")
}

// This is format/verification evidence only. The fixture records no real host
// execution or pending acknowledgement; responsiveness has separate runtime checks.
func completedRefreshFixture(t *testing.T, host string, elapsed time.Duration) (map[string]any, Candidate, fstest.MapFS) {
	t.Helper()
	document, current, artifacts := completeFixture(host)
	owner := strings.Repeat("a", 32)
	document["target"] = map[string]string{"pane": "%1", "socket": "hippo-tui-fixture", "ownerToken": owner}
	clock := int64(10 * time.Second)
	for _, action := range steps(document)[:51] {
		actionID, validID := action["id"].(string)
		if !validID {
			t.Fatal("fixture action lacks its canonical id")
		}
		beforeRef := artifactReference("captures/"+actionID+"-before.txt", fmt.Sprintf("HIPPO\nRunning\nLIVE | age %vs\n", before(action)["sampleAgeSeconds"]), artifacts)
		action["beforeCapture"] = beforeRef
		action["capture"] = artifactReference(capturePath(action), fmt.Sprintf("HIPPO\n%v\nLIVE | age %vs\n", action["actual"], after(action)["sampleAgeSeconds"]), artifacts)
		transition := 200 * time.Millisecond
		if actionID == "40x16-r" {
			transition = elapsed
		}
		beforeEnd := clock + int64(100*time.Millisecond)
		afterEnd := beforeEnd + int64(transition)
		text := fmt.Sprintf("origin=host-native-input\nhost=%s\nstep=%s\ninput=%v\npane=%%1\nownerToken=%s\nviewport=%vx%v\nbinarySHA256=%s\nbeforeCaptureSHA256=%v\ncaptureSHA256=%v\ncommand=synthetic-verification-input\nstatus=0\nbeforeCaptureStartMonotonicNS=%d\nbeforeCaptureEndMonotonicNS=%d\ncaptureStartMonotonicNS=%d\ncaptureEndMonotonicNS=%d\n", host, actionID, action["input"], owner, action["width"], action["height"], current.BinarySHA256, beforeRef["sha256"], capture(action)["sha256"], clock, beforeEnd, beforeEnd+int64(10*time.Millisecond), afterEnd)
		action["inputReceipt"] = artifactReference("events/"+actionID+"-input.txt", text, artifacts)
		clock = afterEnd + int64(100*time.Millisecond)
	}
	return document, current, artifacts
}

func replaceRefreshClockField(t *testing.T, document map[string]any, artifacts fstest.MapFS, id, field, replacement string) {
	t.Helper()
	action := stepNamed(document, id)
	if action == nil {
		t.Fatalf("fixture action %s is absent", id)
	}
	reference, validReference := action["inputReceipt"].(map[string]any)
	if !validReference {
		t.Fatal("fixture input receipt is absent")
	}
	path, validPath := reference["path"].(string)
	if !validPath {
		t.Fatal("fixture input receipt path is absent")
	}
	file, presentFile := artifacts[path]
	if !presentFile || file == nil {
		t.Fatalf("fixture input receipt file %s is absent", path)
	}
	fields, err := receiptFields(file.Data)
	if err != nil {
		t.Fatal(err)
	}
	old, presentField := fields[field]
	if !presentField {
		t.Fatalf("fixture receipt field %s is absent", field)
	}
	text := strings.Replace(string(file.Data), field+"="+old+"\n", field+"="+replacement+"\n", 1)
	action["inputReceipt"] = artifactReference(path, text, artifacts)
}

type completedRefreshCase struct {
	name     string
	elapsed  time.Duration
	mutate   func(*testing.T, map[string]any, fstest.MapFS)
	accepted bool
}

func completedRefreshCases() []completedRefreshCase {
	return []completedRefreshCase{
		{"one-second observation", 1200 * time.Millisecond, nil, true},
		{"at three-second readiness bound", 3 * time.Second, nil, true},
		{"after readiness bound", 3*time.Second + time.Nanosecond, nil, false},
		{"preceding navigation still has 500 ms limit", time.Second, func(t *testing.T, doc map[string]any, artifacts fstest.MapFS) {
			t.Helper()
			ref, ok := stepNamed(doc, "40x16-capacity-b")["inputReceipt"].(map[string]any)
			if !ok {
				t.Fatal("receipt absent")
			}
			path, ok := ref["path"].(string)
			if !ok {
				t.Fatal("receipt path absent")
			}
			fields, err := receiptFields(artifacts[path].Data)
			if err != nil {
				t.Fatal(err)
			}
			end, err := strconv.ParseInt(fields["captureEndMonotonicNS"], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			replaceRefreshClockField(t, doc, artifacts, "40x16-capacity-b", "captureEndMonotonicNS", strconv.FormatInt(end+int64(400*time.Millisecond), 10))
		}, false},
		{"before clock reversed", time.Second, func(t *testing.T, doc map[string]any, artifacts fstest.MapFS) {
			t.Helper()
			replaceRefreshClockField(t, doc, artifacts, "40x16-r", "beforeCaptureStartMonotonicNS", "999999999999")
		}, false},
		{"after capture starts in its future", time.Second, func(t *testing.T, doc map[string]any, artifacts fstest.MapFS) {
			t.Helper()
			replaceRefreshClockField(t, doc, artifacts, "40x16-r", "captureStartMonotonicNS", "999999999999")
		}, false},
		{"after capture overlaps before", time.Second, func(t *testing.T, doc map[string]any, artifacts fstest.MapFS) {
			t.Helper()
			replaceRefreshClockField(t, doc, artifacts, "40x16-r", "captureStartMonotonicNS", "1")
		}, false},
		{"missing finite clock", time.Second, func(t *testing.T, doc map[string]any, artifacts fstest.MapFS) {
			t.Helper()
			replaceRefreshClockField(t, doc, artifacts, "40x16-r", "captureEndMonotonicNS", "0")
		}, false},
		{"overflow clock", time.Second, func(t *testing.T, doc map[string]any, artifacts fstest.MapFS) {
			t.Helper()
			replaceRefreshClockField(t, doc, artifacts, "40x16-r", "captureEndMonotonicNS", strings.Repeat("9", 30))
		}, false},
		{"displayed age differs from inspection", time.Second, func(t *testing.T, doc map[string]any, artifacts fstest.MapFS) {
			t.Helper()
			action := stepNamed(doc, "40x16-r")
			ref := artifactReference(capturePath(action), "HIPPO\nRunning\nLIVE | age 1s\n", artifacts)
			action["capture"] = ref
			digest, ok := ref["sha256"].(string)
			if !ok {
				t.Fatal("capture digest absent")
			}
			replaceRefreshClockField(t, doc, artifacts, "40x16-r", "captureSHA256", digest)
		}, false},
		{"wrong actual input correlation", time.Second, func(t *testing.T, doc map[string]any, artifacts fstest.MapFS) {
			t.Helper()
			replaceRefreshClockField(t, doc, artifacts, "40x16-r", "input", "n")
		}, false},
		{"altered raw input bytes", time.Second, func(t *testing.T, doc map[string]any, artifacts fstest.MapFS) {
			t.Helper()
			ref, ok := stepNamed(doc, "40x16-r")["inputReceipt"].(map[string]any)
			if !ok {
				t.Fatal("receipt absent")
			}
			path, ok := ref["path"].(string)
			if !ok {
				t.Fatal("receipt path absent")
			}
			artifacts[path].Data = []byte("altered raw receipt")
		}, false},
	}
}

func TestCompletedRefreshStagedAttachAndFinalVerifierAgree(t *testing.T) {
	for _, host := range []string{hostTmux, hostWezterm} {
		for _, test := range completedRefreshCases() {
			t.Run(host+"/"+test.name, func(t *testing.T) {
				runCompletedRefreshStagedCase(t, host, test)
			})
		}
	}
}

func runCompletedRefreshStagedCase(t *testing.T, host string, test completedRefreshCase) {
	t.Helper()
	document, current, artifacts := completedRefreshFixture(t, host, test.elapsed)
	if test.mutate != nil {
		test.mutate(t, document, artifacts)
	}
	finalErr := Verify(documentReader(t, document), current, artifacts)
	if (finalErr == nil) != test.accepted {
		t.Fatalf("final accepted=%v: %v", finalErr == nil, finalErr)
	}
	decoded, err := decodeEvidence(documentReader(t, document))
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	for path, file := range artifacts {
		if err := root.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := root.WriteFile(path, file.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	observed := decoded.Steps[14]
	data, err := json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("inspection.json", data, 0o600); err != nil {
		t.Fatal(err)
	}
	decoded.Steps = decoded.Steps[:15]
	decoded.Steps[14].Outcome = "captured"
	stagedErr := attachObservation(cliOptions{observation: filepath.Join(root.Name(), "inspection.json")}, &decoded, root)
	if (stagedErr == nil) != test.accepted {
		t.Fatalf("staged accepted=%v: %v", stagedErr == nil, stagedErr)
	}
	if !test.accepted && decoded.Steps[14].Outcome != "captured" {
		t.Fatal("rejected staged action advanced")
	}
}

func TestCompletedRefreshContinuityRemainsStrict(t *testing.T) {
	for _, test := range []struct {
		name    string
		reverse bool
	}{{"following help retains legal refresh clock", false}, {"following help cannot overlap completed refresh", true}} {
		t.Run(test.name, func(t *testing.T) {
			document, current, artifacts := completedRefreshFixture(t, hostWezterm, 1200*time.Millisecond)
			if test.reverse {
				replaceRefreshClockField(t, document, artifacts, "40x16-?", "beforeCaptureStartMonotonicNS", "1")
			}
			decoded, err := decodeEvidence(documentReader(t, document))
			if err != nil {
				t.Fatal(err)
			}
			err = verifyAgeContinuity(decoded.Steps[14], decoded.Steps[15], artifacts)
			if (err == nil) == test.reverse {
				t.Fatalf("continuity accepted=%v: %v", err == nil, err)
			}
			err = Verify(documentReader(t, document), current, artifacts)
			if (err == nil) == test.reverse {
				t.Fatalf("final continuity accepted=%v: %v", err == nil, err)
			}
		})
	}
}
