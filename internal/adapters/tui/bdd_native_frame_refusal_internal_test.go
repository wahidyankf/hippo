package tui

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// Refusals must retain the original error object; a wrapper alone is insufficient.
func bddNativeSameRefusal(actual, original error) bool {
	return errors.Is(actual, original) && reflect.ValueOf(actual).Equal(reflect.ValueOf(original))
}

func TestBDDNativeRefusalIdentityRejectsWrappedReplacement(t *testing.T) {
	original := errors.New("first refusal")
	if !bddNativeSameRefusal(original, original) {
		t.Fatal("original refusal identity rejected")
	}
	if bddNativeSameRefusal(fmt.Errorf("replacement: %w", original), original) {
		t.Fatal("wrapped replacement accepted as original refusal")
	}
}

func TestBDDNativeFrameRejectsPrivateModeSubparameters(t *testing.T) {
	screen := newBDDNativeFrameDecoder(8, 2)
	feedBDDNativeFrames(t, screen, "SAFE")
	requireBDDNativeFrameRows(t, screen.snapshot(), "SAFE", "")
	requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 4)
	first := screen.feed([]byte("\x1b[?1049:7h"))
	if first == nil {
		t.Error("unsupported private-mode subparameters were accepted")
	}
	frame := screen.snapshot()
	if frame.Alternate {
		t.Error("unsupported private-mode syntax switched screens before refusal")
	}
	requireBDDNativeFrameRows(t, frame, "SAFE", "")
	requireBDDNativeFrameCursor(t, frame, 0, 4)
	if err := screen.finish(); err == nil || !bddNativeSameRefusal(err, first) {
		t.Errorf("unsupported private mode did not retain its refusal at EOF: first=%v EOF=%v", first, err)
	}
	if err := screen.feed([]byte("AFTER")); err == nil || !bddNativeSameRefusal(err, first) {
		t.Errorf("later input did not retain the private-mode refusal: first=%v later=%v", first, err)
	}
	frame = screen.snapshot()
	if frame.Alternate {
		t.Error("later input made the refused private mode a valid alternate screen")
	}
	requireBDDNativeFrameRows(t, frame, "SAFE", "")
	requireBDDNativeFrameCursor(t, frame, 0, 4)
	if len(frame.Overflow) != 0 {
		t.Errorf("refused input fabricated printable overflow evidence: %+v", frame.Overflow)
	}
}

func TestBDDNativeFrameAcceptsSemicolonPrivateModes(t *testing.T) {
	screen := newBDDNativeFrameDecoder(6, 2)
	feedBDDNativeFrames(t, screen, "HOME", "\x1b[?1049h\x1b[?7l")
	if !screen.snapshot().Alternate {
		t.Fatal("supported setup did not select the alternate screen")
	}
	// The alternate screen already exists with wrapping off; processing both
	// independent semicolon modes must reenable wrapping without recreating it.
	feedBDDNativeFrames(t, screen, "\x1b[?1049;7hABCDEFZ")
	frame := screen.snapshot()
	if !frame.Alternate {
		t.Error("supported semicolon modes lost the alternate screen")
	}
	requireBDDNativeFrameRows(t, frame, "ABCDEF", "Z")
	requireBDDNativeFrameCursor(t, frame, 1, 1)
	if len(frame.Overflow) != 1 || frame.Overflow[0] != (bddNativeFrameOverflow{Row: 0, Column: 6, Cells: 1, Text: "Z", Width: 6, Height: 2}) {
		t.Errorf("semicolon-mode wrapped write intent=%+v, want Z at (0,6)", frame.Overflow)
	}
	feedBDDNativeFrames(t, screen, "\x1b[?1049l")
	frame = screen.snapshot()
	if frame.Alternate {
		t.Error("supported alternate-screen exit failed")
	}
	requireBDDNativeFrameRows(t, frame, "HOME", "")
	requireBDDNativeFrameCursor(t, frame, 0, 4)
	if err := screen.finish(); err != nil {
		t.Fatalf("supported semicolon modes failed at EOF: %v", err)
	}
}

func TestBDDNativeFrameRejectsOversizedOSCCommand(t *testing.T) {
	screen := newBDDNativeFrameDecoder(8, 2)
	feedBDDNativeFrames(t, screen, "SAFE")
	first := screen.feed([]byte("\x1b]18446744073709551616;payload\x07"))
	if first == nil {
		t.Error("oversized OSC command was accepted as a supported operation")
	}
	frame := screen.snapshot()
	requireBDDNativeFrameRows(t, frame, "SAFE", "")
	requireBDDNativeFrameCursor(t, frame, 0, 4)
	if frame.Alternate {
		t.Error("refused OSC command changed the selected screen")
	}
	if err := screen.finish(); err == nil || !bddNativeSameRefusal(err, first) {
		t.Errorf("oversized OSC refusal was not sticky at EOF: first=%v EOF=%v", first, err)
	}
	if err := screen.feed([]byte("AFTER")); err == nil || !bddNativeSameRefusal(err, first) {
		t.Errorf("later input did not retain the OSC refusal: first=%v later=%v", first, err)
	}
	frame = screen.snapshot()
	requireBDDNativeFrameRows(t, frame, "SAFE", "")
	requireBDDNativeFrameCursor(t, frame, 0, 4)
	if len(frame.Overflow) != 0 {
		t.Errorf("refused OSC input fabricated printable overflow evidence: %+v", frame.Overflow)
	}
}

func TestBDDNativeFrameAcceptsSplitOSCNumericPayload(t *testing.T) {
	for _, test := range []struct {
		name   string
		chunks []string
	}{
		{
			name:   "title with long numeric payload",
			chunks: []string{"\x1b", "]", "0", ";", "title-", "18446744073709551616", "\x07"},
		},
		{
			name:   "split color command and payload",
			chunks: []string{"\x1b]", "1", "0", ";", "rgb:", "1234", "/", "5678", "/9abc", "\x1b", "\\"},
		},
		{
			name:   "split supported three-digit reset command",
			chunks: []string{"\x1b]", "1", "1", "0", ";", "\x07"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := newBDDNativeFrameDecoder(8, 2)
			feedBDDNativeFrames(t, screen, "SAFE")
			for _, chunk := range test.chunks {
				feedBDDNativeFrames(t, screen, chunk)
				requireBDDNativeFrameRows(t, screen.snapshot(), "SAFE", "")
				requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 4)
			}
			feedBDDNativeFrames(t, screen, "Z")
			frame := screen.snapshot()
			requireBDDNativeFrameRows(t, frame, "SAFEZ", "")
			requireBDDNativeFrameCursor(t, frame, 0, 5)
			if frame.Alternate || len(frame.Overflow) != 0 {
				t.Errorf("nonvisible OSC changed screen or invented overflow: %+v", frame)
			}
			if err := screen.finish(); err != nil {
				t.Fatalf("supported split OSC failed at EOF: %v", err)
			}
		})
	}
}

// The renderer's DECST8C may arrive in fragments and must retain the real tab grid.
func TestBDDNativeFrameAcceptsSplitDefaultTabStops(t *testing.T) {
	for _, test := range []struct {
		name   string
		chunks []string
	}{
		{"whole", []string{"\x1b[?5W"}},
		{"fragmented", []string{"\x1b", "[", "?", "5", "W"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := newBDDNativeFrameDecoder(24, 2)
			feedBDDNativeFrames(t, screen, "SAFE")
			for _, chunk := range test.chunks {
				feedBDDNativeFrames(t, screen, chunk)
				requireBDDNativeFrameRows(t, screen.snapshot(), "SAFE", "")
				requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 4)
			}
			feedBDDNativeFrames(t, screen, "\tA\tB")
			frame := screen.snapshot()
			requireBDDNativeFrameRows(t, frame, "SAFE    A       B", "")
			requireBDDNativeFrameCursor(t, frame, 0, 17)
			if frame.Alternate || len(frame.Overflow) != 0 {
				t.Errorf("default tab reset changed buffer or invented overflow: %+v", frame)
			}
			if err := screen.finish(); err != nil {
				t.Fatalf("split default tab reset failed at EOF: %v", err)
			}
		})
	}
}

func TestBDDNativeFrameRejectsUnsupportedTabStopOperations(t *testing.T) {
	for _, sequence := range []string{
		"\x1b[?W", "\x1b[?4W", "\x1b[?5;0W", "\x1b[?5:0W", "\x1b[?5 W", "\x1b[5W",
	} {
		t.Run(sequence, func(t *testing.T) {
			screen := newBDDNativeFrameDecoder(24, 2)
			feedBDDNativeFrames(t, screen, "SAFE")
			first := screen.feed([]byte(sequence))
			if first == nil {
				t.Fatal("unsupported tab-stop operation accepted")
			}
			if err := screen.feed([]byte("\tAFTER")); !bddNativeSameRefusal(err, first) {
				t.Errorf("later input lost first refusal: first=%v later=%v", first, err)
			}
			if err := screen.finish(); !bddNativeSameRefusal(err, first) {
				t.Errorf("EOF lost first refusal: first=%v EOF=%v", first, err)
			}
			frame := screen.snapshot()
			requireBDDNativeFrameRows(t, frame, "SAFE", "")
			requireBDDNativeFrameCursor(t, frame, 0, 4)
			if frame.Alternate || len(frame.Overflow) != 0 {
				t.Errorf("refused tab syntax changed buffer or invented overflow: %+v", frame)
			}
		})
	}
}
