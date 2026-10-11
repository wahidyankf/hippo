package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestRecordedNativeArtifactsRemainHashBound(t *testing.T) {
	for _, changed := range []string{"before capture", "native input receipt"} {
		t.Run(changed, func(t *testing.T) {
			document, current, artifacts := completeFixture("tmux")
			owner := strings.Repeat("a", 32)
			document["target"] = map[string]string{"pane": "%1", "socket": "hippo-tui-fixture", "ownerToken": owner}
			// Unit-native receipts use explicit valid clocks and displayed ages for
			// all shared stages, so corruption cannot fail on missing age evidence.
			for index, action := range steps(document)[:51] {
				actionID, validID := action["id"].(string)
				if !validID {
					t.Fatal("fixture action lacks its canonical id")
				}
				beforeName, receiptName := "captures/before-"+actionID+".txt", "events/"+actionID+"-input.txt"
				if index == 0 {
					beforeName, receiptName = "captures/before-tab.txt", "events/tab-input.txt"
				}
				prior := artifactReference(beforeName, fmt.Sprintf("HIPPO\nRunning\nLIVE | age %vs\n", before(action)["sampleAgeSeconds"]), artifacts)
				action["beforeCapture"] = prior
				action["capture"] = artifactReference(capturePath(action), fmt.Sprintf("HIPPO\n%v\nLIVE | age %vs\n", action["actual"], after(action)["sampleAgeSeconds"]), artifacts)
				clock := int64(index+1) * 1000000000
				event := fmt.Sprintf("origin=host-native-input\nhost=tmux\nstep=%s\ninput=%v\npane=%%1\nownerToken=%s\nviewport=%vx%v\nbinarySHA256=%v\ncommand=unit-native-input\nstatus=0\nbeforeCaptureSHA256=%v\ncaptureSHA256=%v\nbeforeCaptureStartMonotonicNS=%d\nbeforeCaptureEndMonotonicNS=%d\ncaptureStartMonotonicNS=%d\ncaptureEndMonotonicNS=%d\n", actionID, action["input"], owner, action["width"], action["height"], candidate(document)["binarySHA256"], prior["sha256"], capture(action)["sha256"], clock, clock+100000000, clock+200000000, clock+300000000)
				action["inputReceipt"] = artifactReference(receiptName, event, artifacts)
			}
			if err := Verify(documentReader(t, document), current, artifacts); err != nil {
				t.Fatalf("complete capture-bound native fixture rejected: %v", err)
			}
			name := "captures/before-tab.txt"
			if changed == "native input receipt" {
				name = "events/tab-input.txt"
			}
			artifacts[name].Data = []byte("changed actual native artifact\n")
			if err := Verify(documentReader(t, document), current, artifacts); err == nil || !strings.Contains(err.Error(), "artifact digest differs from captured bytes") {
				t.Fatalf("changed retained native artifact did not fail its digest binding: %v", err)
			}
		})
	}
}
