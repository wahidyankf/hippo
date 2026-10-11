package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The real file boundary, including symlink resolution, must be fail closed.
func TestBDDNativeObserverRootRejectsEscapingInput(t *testing.T) {
	directory := nativeObserverInputTestDirectory(t)
	outside := filepath.Join(t.TempDir(), "outside.json")
	input := BDDNativeObserverInput{Sequence: 1, Snapshot: BDDNativeObserverSnapshot()}
	input.Now = *input.Snapshot.SampledAt
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "input.json")); err != nil {
		t.Fatal(err)
	}
	observer, err := newBDDNativeControlledObserver(t.Context(), directory)
	if err == nil {
		if closeError := observer.close(); closeError != nil {
			t.Error(closeError)
		}
		t.Fatal("native input escaped the owned root through a symlink")
	}
}

func TestBDDNativeObserverInputRoundTripsOrdinarySnapshot(t *testing.T) {
	directory := nativeObserverInputTestDirectory(t)
	input := BDDNativeObserverInput{Sequence: 1, Snapshot: BDDNativeObserverSnapshot()}
	input.Now = *input.Snapshot.SampledAt
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "input.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	observer, err := newBDDNativeControlledObserver(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	if closeError := observer.close(); closeError != nil {
		t.Fatal(closeError)
	}
	actual := observer.input
	if actual.Sequence != input.Sequence || !actual.Now.Equal(input.Now) || actual.Snapshot.SampledAt == nil || !actual.Snapshot.SampledAt.Equal(*input.Snapshot.SampledAt) || len(actual.Snapshot.Running) != len(input.Snapshot.Running) {
		t.Fatal("ordinary snapshot input changed across the real file boundary")
	}
	for index, row := range actual.Snapshot.Running {
		expected := input.Snapshot.Running[index].Entry
		if row.Entry.RunID != expected.RunID || row.Entry.Class != expected.Class || row.Entry.Allocated != expected.Allocated {
			t.Fatal("native input lost the complete owner identity/class/allocation")
		}
	}
}

func nativeObserverInputTestDirectory(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	t.Setenv("TMPDIR", parent)
	// Match the real helper's owned-child protocol beneath testing's cleaned root.
	directory := filepath.Join(parent, "hippo-native-input-fixture")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}
