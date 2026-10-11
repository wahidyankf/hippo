package main

import "testing"

func TestNativeEventsRetainActualConfiguredPaneNavigationSequence(t *testing.T) {
	document, current, artifacts := completeFixture("wezterm")
	replaceOperatorEvent(t, document, artifacts, "Cmd-B,e,f,h", "operator-key-event")
	if err := Verify(documentReader(t, document), current, artifacts); err != nil {
		t.Fatalf("complete recorded configured pane sequence rejected: %v", err)
	}
}
