package tui

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBDDNativeFramesSplitUTF8AndCSI(t *testing.T) {
	screen := newBDDNativeFrameDecoder(8, 3)
	feedBDDNativeFrames(t, screen, "\x1b[2J\x1b[HA")
	feedBDDNativeFrames(t, screen, "\xe7")
	requireBDDNativeFrameRows(t, screen.snapshot(), "A", "", "")
	requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 1)
	feedBDDNativeFrames(t, screen, "\x95", "\x8c", "e", "\xcc", "\x81")
	frame := screen.snapshot()
	requireBDDNativeFrameRows(t, frame, "A\u754ce\u0301", "", "")
	requireBDDNativeFrameCursor(t, frame, 0, 4)
	if !utf8.ValidString(strings.Join(frame.Rows, "\n")) {
		t.Error("split UTF-8 produced an invalid current screen")
	}
	feedBDDNativeFrames(t, screen, "\x1b[2;")
	requireBDDNativeFrameRows(t, screen.snapshot(), "A\u754ce\u0301", "", "")
	requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 4)
	feedBDDNativeFrames(t, screen, "5", "HZ", "\x1b[1;", "4HQ")
	requireBDDNativeFrameRows(t, screen.snapshot(), "A\u754cQ", "    Z", "")
	requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 4)
	if len(screen.snapshot().Overflow) != 0 {
		t.Error("fitting split characters or cursor movement reported overflow")
	}
}

func TestBDDNativeFramesCursorEraseAndRepaint(t *testing.T) {
	screen := newBDDNativeFrameDecoder(10, 3)
	feedBDDNativeFrames(t, screen, "ABCDEFG\x1b[2;1HQUEUE-OLD\x1b[3;1HOBSOLETE")
	requireBDDNativeFrameRows(t, screen.snapshot(), "ABCDEFG", "QUEUE-OLD", "OBSOLETE")
	feedBDDNativeFrames(t, screen, "\x1b[2A\r\x1b[2Cxy\x1b[0K")
	requireBDDNativeFrameRows(t, screen.snapshot(), "ABxy", "QUEUE-OLD", "OBSOLETE")
	requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 4)
	feedBDDNativeFrames(t, screen, "\x1b[2;1HNEW\x1b[K\x1b[3;1H\x1b[2K")
	requireBDDNativeFrameRows(t, screen.snapshot(), "ABxy", "NEW", "")
	feedBDDNativeFrames(t, screen, "\x1b[2J")
	requireBDDNativeFrameRows(t, screen.snapshot(), "", "", "")
	requireBDDNativeFrameCursor(t, screen.snapshot(), 2, 0)
	feedBDDNativeFrames(t, screen, "\x1b[HREADY")
	requireBDDNativeFrameRows(t, screen.snapshot(), "READY", "", "")
}

func TestBDDNativeFramesAlternateScreenRestoresPrimary(t *testing.T) {
	screen := newBDDNativeFrameDecoder(12, 3)
	feedBDDNativeFrames(t, screen, "shell")
	feedBDDNativeFrames(t, screen, "\x1b[?1049", "h")
	frame := screen.snapshot()
	if !frame.Alternate {
		t.Error("alternate-screen entry was not observed")
	}
	requireBDDNativeFrameRows(t, frame, "", "", "")
	requireBDDNativeFrameCursor(t, frame, 0, 0)
	feedBDDNativeFrames(t, screen, "ALT\x1b[2;1HDETAIL")
	requireBDDNativeFrameRows(t, screen.snapshot(), "ALT", "DETAIL", "")
	feedBDDNativeFrames(t, screen, "\x1b[?1049l")
	frame = screen.snapshot()
	if frame.Alternate {
		t.Error("alternate-screen exit retained the alternate screen")
	}
	requireBDDNativeFrameRows(t, frame, "shell", "", "")
	requireBDDNativeFrameCursor(t, frame, 0, 5)
	feedBDDNativeFrames(t, screen, "\x1b[?1049hSECOND")
	requireBDDNativeFrameRows(t, screen.snapshot(), "SECOND", "", "")
	if !screen.snapshot().Alternate {
		t.Error("second alternate-screen entry was not observed")
	}
}

func TestBDDNativeFramesResizeDiscardsObsoleteCells(t *testing.T) {
	screen := newBDDNativeFrameDecoder(10, 4)
	feedBDDNativeFrames(t, screen, "HEAD\x1b[1;10HX\x1b[4;1HOBSOLETE")
	requireBDDNativeFrameRows(t, screen.snapshot(), "HEAD     X", "", "", "OBSOLETE")
	screen.resize(6, 2)
	frame := screen.snapshot()
	if frame.Width != 6 || frame.Height != 2 {
		t.Errorf("resized viewport=%dx%d, want 6x2", frame.Width, frame.Height)
	}
	requireBDDNativeFrameRows(t, frame, "HEAD", "")
	feedBDDNativeFrames(t, screen, "\x1b[2J\x1b[HMIN\x1b[2;1HOK")
	requireBDDNativeFrameRows(t, screen.snapshot(), "MIN", "OK")
	screen.resize(10, 4)
	requireBDDNativeFrameRows(t, screen.snapshot(), "MIN", "OK", "", "")
	feedBDDNativeFrames(t, screen, "\x1b[HCOMPACT\x1b[K")
	requireBDDNativeFrameRows(t, screen.snapshot(), "COMPACT", "OK", "", "")
	if len(screen.snapshot().Overflow) != 0 {
		t.Error("resize or fitting repaint invented an out-of-bounds write")
	}
}

func TestBDDNativeFramesCurrentScreenRejectsHistoricalMatch(t *testing.T) {
	screen := newBDDNativeFrameDecoder(12, 2)
	feedBDDNativeFrames(t, screen, "OLD-ID")
	old := screen.snapshot()
	feedBDDNativeFrames(t, screen, "\x1b[2J\x1b[HNEW-ID")
	current := screen.snapshot()
	requireBDDNativeFrameRows(t, old, "OLD-ID", "")
	requireBDDNativeFrameRows(t, current, "NEW-ID", "")
	if strings.Contains(strings.Join(current.Rows, "\n"), "OLD-ID") {
		t.Error("current-screen predicate accepted a cleared historical job")
	}
	// A late repaint really changes the visible screen; seeing NEW-ID earlier
	// cannot satisfy a predicate on this later frame.
	feedBDDNativeFrames(t, screen, "\x1b[2J\x1b[HOLD-ID")
	late := screen.snapshot()
	requireBDDNativeFrameRows(t, late, "OLD-ID", "")
	if strings.Contains(strings.Join(late.Rows, "\n"), "NEW-ID") {
		t.Error("late old repaint was concealed by a historical new-frame match")
	}
	requireBDDNativeFrameRows(t, current, "NEW-ID", "")
}

func TestBDDNativeFramesDetectWritesBeforeClipping(t *testing.T) {
	for _, test := range []struct {
		name, stream string
		want         bddNativeFrameOverflow
		wantRows     []string
		wantRow      int
		wantColumn   int
	}{
		{
			name: "ASCII beyond right edge", stream: "ABCDEFG",
			want:     bddNativeFrameOverflow{Row: 0, Column: 6, Cells: 1, Text: "G", Width: 6, Height: 2},
			wantRows: []string{"ABCDEF", "G"}, wantRow: 1, wantColumn: 1,
		},
		{
			name: "wide character straddles right edge", stream: "\x1b[1;6H\u754c",
			want:     bddNativeFrameOverflow{Row: 0, Column: 5, Cells: 2, Text: "\u754c", Width: 6, Height: 2},
			wantRows: []string{"", "\u754c"}, wantRow: 1, wantColumn: 2,
		},
		{
			name: "cursor requested below last row", stream: "\x1b[3;1HZ",
			want:     bddNativeFrameOverflow{Row: 2, Column: 0, Cells: 1, Text: "Z", Width: 6, Height: 2},
			wantRows: []string{"", "Z"}, wantRow: 1, wantColumn: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := newBDDNativeFrameDecoder(6, 2)
			feedBDDNativeFrames(t, screen, test.stream)
			frame := screen.snapshot()
			if !slices.Equal(frame.Overflow, []bddNativeFrameOverflow{test.want}) {
				t.Errorf("pre-clipping writes=%+v, want %+v", frame.Overflow, test.want)
			}
			requireBDDNativeFrameRows(t, frame, test.wantRows...)
			requireBDDNativeFrameCursor(t, frame, test.wantRow, test.wantColumn)
			// Clearing the display must not clear the evidence of an earlier bad write.
			feedBDDNativeFrames(t, screen, "\x1b[2J\x1b[HOK")
			if !slices.Equal(screen.snapshot().Overflow, []bddNativeFrameOverflow{test.want}) {
				t.Errorf("repaint discarded or changed overflow evidence: %+v", screen.snapshot().Overflow)
			}
			requireBDDNativeFrameRows(t, screen.snapshot(), "OK", "")
		})
	}
}

func TestBDDNativeFramesExactCellEdgesDoNotOverflow(t *testing.T) {
	screen := newBDDNativeFrameDecoder(6, 2)
	feedBDDNativeFrames(t, screen, "ABCD\u754c\r\n123456")
	frame := screen.snapshot()
	requireBDDNativeFrameRows(t, frame, "ABCD\u754c", "123456")
	if len(frame.Overflow) != 0 {
		t.Errorf("writes ending exactly at the cell edge reported overflow: %+v", frame.Overflow)
	}
	feedBDDNativeFrames(t, screen, "\x1b[1;1He\u0301\x1b[0K")
	requireBDDNativeFrameRows(t, screen.snapshot(), "e\u0301", "123456")
	requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 1)
}

func TestBDDNativeFramesSnapshotsOwnTheirSlices(t *testing.T) {
	screen := newBDDNativeFrameDecoder(6, 2)
	feedBDDNativeFrames(t, screen, "ABCDEFG")
	captured := screen.snapshot()
	requireBDDNativeFrameRows(t, captured, "ABCDEF", "G")
	if len(captured.Overflow) != 1 {
		t.Fatalf("capture lacks its actual overflow attempt: %+v", captured.Overflow)
	}
	captured.Rows[0] = "caller"
	captured.Overflow[0].Text = "caller"
	fresh := screen.snapshot()
	requireBDDNativeFrameRows(t, fresh, "ABCDEF", "G")
	if len(fresh.Overflow) != 1 || fresh.Overflow[0].Text != "G" {
		t.Error("caller mutation changed the decoder's overflow evidence")
	}
	feedBDDNativeFrames(t, screen, "\x1b[2J\x1b[HNEW")
	requireBDDNativeFrameRows(t, fresh, "ABCDEF", "G")
	requireBDDNativeFrameRows(t, screen.snapshot(), "NEW", "")
}

func feedBDDNativeFrames(t *testing.T, screen *bddNativeFrameDecoder, chunks ...string) {
	t.Helper()
	for _, chunk := range chunks {
		if err := screen.feed([]byte(chunk)); err != nil {
			t.Fatalf("consume terminal chunk %q: %v", chunk, err)
		}
	}
}

func requireBDDNativeFrameRows(t *testing.T, frame bddNativeFrameSnapshot, want ...string) {
	t.Helper()
	if !slices.Equal(frame.Rows, want) {
		t.Errorf("current screen rows=%q, want %q", frame.Rows, want)
	}
}

func requireBDDNativeFrameCursor(t *testing.T, frame bddNativeFrameSnapshot, row, column int) {
	t.Helper()
	if frame.CursorRow != row || frame.CursorColumn != column {
		t.Errorf("current cursor=(%d,%d), want (%d,%d)", frame.CursorRow, frame.CursorColumn, row, column)
	}
}

// bddNativeFrameSnapshot contains ordinary values only. Rows retain the viewport
// height, omit right padding, and count cursor positions in zero-based cells.
// A snapshot owns its slices; later output and callers cannot mutate another capture.
type bddNativeFrameSnapshot struct {
	Rows                    []string
	Width, Height           int
	CursorRow, CursorColumn int
	Alternate               bool
	Overflow                []bddNativeFrameOverflow
	CurrentOverflow         []bddNativeFrameOverflow
	FullClearSequence       uint64
}

// Overflow records a requested printable write before terminal clamp, wrap or
// scroll can hide it. Coordinates are zero based; Width/Height are the viewport
// at that write. The record survives repaint and resize. It is not inferred
// from the fixed screen dimensions and does not prohibit an exact-edge write.
type bddNativeFrameOverflow struct {
	Row, Column, Cells int
	Text               string
	Width, Height      int
}

// These transcripts exercise visible renderer changes, not merely accepted CSI finals.
func TestBDDNativeFramesRendererLineShifts(t *testing.T) {
	for _, test := range []struct {
		name, stream string
		rows         []string
		row, column  int
	}{
		{"insert", "\x1b[L", []string{"HEADER", "one\u754c", "", "two", "three", "FOOT"}, 2, 0},
		{"insert two", "\x1b[2L", []string{"HEADER", "one\u754c", "", "", "two", "FOOT"}, 2, 0},
		{"delete", "\x1b[M", []string{"HEADER", "one\u754c", "three", "four", "", "FOOT"}, 2, 0},
		{"delete two", "\x1b[2M", []string{"HEADER", "one\u754c", "four", "", "", "FOOT"}, 2, 0},
		{"scroll up", "\x1b[2S", []string{"HEADER", "three", "four", "", "", "FOOT"}, 2, 2},
		{"scroll down", "\x1b[2T", []string{"HEADER", "", "", "one\u754c", "two", "FOOT"}, 2, 2},
		{"zero count defaults to one", "\x1b[0L", []string{"HEADER", "one\u754c", "", "two", "three", "FOOT"}, 2, 0},
		{"large count clips to region", "\x1b[99999M", []string{"HEADER", "one\u754c", "", "", "", "FOOT"}, 2, 0},
		{"insert outside margins", "\x1b[6;3H\x1b[L", []string{"HEADER", "one\u754c", "two", "three", "four", "FOOT"}, 5, 2},
		{"delete outside margins", "\x1b[1;3H\x1b[M", []string{"HEADER", "one\u754c", "two", "three", "four", "FOOT"}, 0, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := newBDDNativeFrameDecoder(8, 6)
			feedBDDNativeFrames(t, screen, "HEADER\x1b[2;1Hone\u754c\x1b[3;1Htwo\x1b[4;1Hthree\x1b[5;1Hfour\x1b[6;1HFOOT")
			feedBDDNativeFrames(t, screen, "\x1b[2;", "5r\x1b[3;3H")
			for _, value := range []byte(test.stream) {
				feedBDDNativeFrames(t, screen, string([]byte{value}))
			}
			requireBDDNativeFrameRows(t, screen.snapshot(), test.rows...)
			requireBDDNativeFrameCursor(t, screen.snapshot(), test.row, test.column)
			if len(screen.snapshot().Overflow) != 0 {
				t.Error("row shifting invented a printable overflow")
			}
		})
	}
}

func TestBDDNativeFramesIndexUsesActiveMargins(t *testing.T) {
	for _, test := range []struct {
		name, stream string
		rows         []string
		row          int
	}{
		{"LF at bottom", "\x1b[5;1H\n", []string{"HEADER", "two", "three", "four", "", "FOOT"}, 4},
		{"IND at bottom", "\x1b[5;1H\x1bD", []string{"HEADER", "two", "three", "four", "", "FOOT"}, 4},
		{"RI at top", "\x1b[2;1H\x1bM", []string{"HEADER", "", "one\u754c", "two", "three", "FOOT"}, 1},
		{"LF outside bottom", "\x1b[6;1H\n", []string{"HEADER", "one\u754c", "two", "three", "four", "FOOT"}, 5},
		{"IND outside bottom", "\x1b[6;1H\x1bD", []string{"HEADER", "one\u754c", "two", "three", "four", "FOOT"}, 5},
		{"RI outside top", "\x1b[1;1H\x1bM", []string{"HEADER", "one\u754c", "two", "three", "four", "FOOT"}, 0},
		{"LF above region moves", "\x1b[1;1H\n", []string{"HEADER", "one\u754c", "two", "three", "four", "FOOT"}, 1},
		{"RI below region moves", "\x1b[6;1H\x1bM", []string{"HEADER", "one\u754c", "two", "three", "four", "FOOT"}, 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := newBDDNativeFrameDecoder(8, 6)
			feedBDDNativeFrames(t, screen, "HEADER\x1b[2;1Hone\u754c\x1b[3;1Htwo\x1b[4;1Hthree\x1b[5;1Hfour\x1b[6;1HFOOT\x1b[2;5r", test.stream)
			requireBDDNativeFrameRows(t, screen.snapshot(), test.rows...)
			requireBDDNativeFrameCursor(t, screen.snapshot(), test.row, 0)
		})
	}
}

func TestBDDNativeFramesMarginsResetAndBufferIsolation(t *testing.T) {
	t.Run("home and default reset", func(t *testing.T) {
		screen := newBDDNativeFrameDecoder(6, 4)
		feedBDDNativeFrames(t, screen, "A\x1b[2;1HB\x1b[3;1HC\x1b[4;1HD\x1b[2;3r")
		requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 0)
		feedBDDNativeFrames(t, screen, "\x1b[0;0r\x1b[4;1H\n")
		requireBDDNativeFrameRows(t, screen.snapshot(), "B", "C", "D", "")
		feedBDDNativeFrames(t, screen, "\x1b[r")
		requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 0)
	})
	t.Run("resize resets whole height", func(t *testing.T) {
		screen := newBDDNativeFrameDecoder(6, 4)
		feedBDDNativeFrames(t, screen, "A\x1b[2;1HB\x1b[3;1HC\x1b[4;1HD\x1b[2;3r")
		screen.resize(6, 5)
		feedBDDNativeFrames(t, screen, "\x1b[5;1HE\r\n")
		requireBDDNativeFrameRows(t, screen.snapshot(), "B", "C", "D", "E", "")
	})
	t.Run("alternate starts fresh and preserves primary region", func(t *testing.T) {
		screen := newBDDNativeFrameDecoder(6, 4)
		feedBDDNativeFrames(t, screen, "A\x1b[2;1HB\x1b[3;1HC\x1b[4;1HD\x1b[2;3r\x1b[?1049hX\x1b[2;1HY\x1b[3;1HZ\x1b[4;1HW\r\n")
		requireBDDNativeFrameRows(t, screen.snapshot(), "Y", "Z", "W", "")
		feedBDDNativeFrames(t, screen, "\x1b[?1049l\x1b[3;1H\n")
		requireBDDNativeFrameRows(t, screen.snapshot(), "A", "C", "", "D")
		feedBDDNativeFrames(t, screen, "\x1b[?1049h\x1b[4;1HW\r\n")
		requireBDDNativeFrameRows(t, screen.snapshot(), "", "", "W", "")
	})
	t.Run("single row full buffer remains usable", func(t *testing.T) {
		screen := newBDDNativeFrameDecoder(6, 1)
		feedBDDNativeFrames(t, screen, "A\r\nB\r\x1bM")
		requireBDDNativeFrameRows(t, screen.snapshot(), "")
		requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 0)
		// DEC requires top<bottom; do not invent a one-row custom region.
		if err := screen.feed([]byte("\x1b[r")); err == nil {
			t.Error("invalid one-row DECSTBM region accepted")
		}
	})
}

func TestBDDNativeFramesCharacterSplicesRepairWideEdges(t *testing.T) {
	for _, test := range []struct {
		name, stream, row string
		column            int
	}{
		{"insert in trailing half", "\x1b[1;3H\x1b[2@", "A    BCD", 2},
		{"delete trailing half", "\x1b[1;3H\x1b[P", "A BCDE", 2},
		{"move whole wide pair", "\x1b[1;2H\x1b[@", "A \u754cBCDE", 1},
		{"delete whole wide pair", "\x1b[1;2H\x1b[2P", "ABCDE", 1},
		{"insert bounded blank fill", "\x1b[1;3H\x1b[99@", "A", 2},
		{"delete bounded blank fill", "\x1b[1;3H\x1b[99P", "A", 2},
		{"zero count is one", "\x1b[1;2H\x1b[0P", "A BCDE", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := newBDDNativeFrameDecoder(8, 2)
			feedBDDNativeFrames(t, screen, "A\u754cBCDE\x1b[2;1HSENTINEL")
			for _, value := range []byte(test.stream) {
				feedBDDNativeFrames(t, screen, string([]byte{value}))
			}
			requireBDDNativeFrameRows(t, screen.snapshot(), test.row, "SENTINEL")
			requireBDDNativeFrameCursor(t, screen.snapshot(), 0, test.column)
			if len(screen.snapshot().Overflow) != 0 {
				t.Error("cell splice invented a printable overflow")
			}
		})
	}
	for _, test := range []struct{ stream, row string }{
		{"\x1b[1;2H\x1b[3@", "A   \u754c"},
		{"\x1b[1;2H\x1b[4@", "A"},
	} {
		screen := newBDDNativeFrameDecoder(6, 1)
		feedBDDNativeFrames(t, screen, "A\u754cBC", test.stream)
		requireBDDNativeFrameRows(t, screen.snapshot(), test.row)
	}
}

func TestBDDNativeFramesRendererOperationsPreserveOverflow(t *testing.T) {
	screen := newBDDNativeFrameDecoder(4, 2)
	feedBDDNativeFrames(t, screen, "A\x1b[1;3H\x1b[", "2", "b")
	requireBDDNativeFrameRows(t, screen.snapshot(), "A AA", "")
	requireBDDNativeFrameCursor(t, screen.snapshot(), 0, 3)
	feedBDDNativeFrames(t, screen, "\x1b[2b")
	requireBDDNativeFrameRows(t, screen.snapshot(), "A AA", "AA")
	requireBDDNativeFrameCursor(t, screen.snapshot(), 1, 2)
	want := []bddNativeFrameOverflow{{Row: 0, Column: 4, Cells: 1, Text: "A", Width: 4, Height: 2}}
	if !slices.Equal(screen.snapshot().Overflow, want) {
		t.Errorf("REP lost its pre-wrap overflow: %+v", screen.snapshot().Overflow)
	}
	feedBDDNativeFrames(t, screen, "\x1b[S\x1b[H\x1b[@")
	if !slices.Equal(screen.snapshot().Overflow, want) {
		t.Error("visual row/cell shifts cleared prior overflow evidence")
	}
	for _, prefix := range []string{"", "\u754c", "A\u0301"} {
		screen := newBDDNativeFrameDecoder(8, 2)
		feedBDDNativeFrames(t, screen, prefix)
		if err := screen.feed([]byte("\x1b[b")); err == nil {
			t.Errorf("REP accepted unsupported retained glyph %q", prefix)
		}
	}

	// Stationary row/cell operations must not hide a previously requested
	// out-of-bounds cursor position before the next actual printable write.
	for _, final := range []byte{'S', 'T', '@', 'P'} {
		t.Run("pending intention "+string(final), func(t *testing.T) {
			screen := newBDDNativeFrameDecoder(6, 2)
			feedBDDNativeFrames(t, screen, "\x1b[99;99H", "\x1b["+string(final), "A")
			want := []bddNativeFrameOverflow{{Row: 98, Column: 98, Cells: 1, Text: "A", Width: 6, Height: 2}}
			if !slices.Equal(screen.snapshot().Overflow, want) {
				t.Errorf("stationary %c hid pending preclip intention: %+v", final, screen.snapshot().Overflow)
			}
			requireBDDNativeFrameRows(t, screen.snapshot(), "", "     A")
			requireBDDNativeFrameCursor(t, screen.snapshot(), 1, 5)
		})
	}
}

func TestBDDNativeFramesBackwardTabsUseEightColumnGrid(t *testing.T) {
	for _, test := range []struct {
		name, stream string
		column       int
	}{
		{"previous stop", "\x1b[1;18H\x1b[Z", 16},
		{"from stop", "\x1b[1;17H\x1b[Z", 8},
		{"two stops", "\x1b[1;18H\x1b[2Z", 8},
		{"left edge", "\x1b[1;1H\x1b[Z", 0},
		{"large bounded count", "\x1b[1;18H\x1b[99999Z", 0},
		{"zero is default", "\x1b[1;18H\x1b[0Z", 16},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := newBDDNativeFrameDecoder(24, 1)
			feedBDDNativeFrames(t, screen, "SENTINEL", test.stream)
			requireBDDNativeFrameRows(t, screen.snapshot(), "SENTINEL")
			requireBDDNativeFrameCursor(t, screen.snapshot(), 0, test.column)
		})
	}
}

func TestBDDNativeFramesRendererParametersRemainStrict(t *testing.T) {
	streams := make([]string, 0, 38)
	streams = append(streams, "\x1b[3;2r", "\x1b[2;2r", "\x1b[1;5r", "\x1b[1;2;3r", "\x1b[1:2r", "\x1b[?2r")
	for _, final := range []byte{'L', 'M', 'S', 'T', '@', 'P', 'b', 'Z'} {
		streams = append(streams, "\x1b[1;2"+string(final), "\x1b[1:2"+string(final), "\x1b[?1"+string(final), "\x1b[1 "+string(final))
	}
	for _, stream := range streams {
		t.Run(stream, func(t *testing.T) {
			screen := newBDDNativeFrameDecoder(8, 4)
			feedBDDNativeFrames(t, screen, "A\x1b[2;1HB")
			before := screen.snapshot()
			err := screen.feed([]byte(stream))
			if err == nil {
				t.Fatalf("unsupported renderer parameters %q accepted", stream)
			}
			if later := screen.feed([]byte("\x1b[2Jignored")); !bddNativeSameRefusal(later, err) {
				t.Error("renderer refusal was not sticky")
			}
			requireBDDNativeFrameRows(t, screen.snapshot(), before.Rows...)
			requireBDDNativeFrameCursor(t, screen.snapshot(), before.CursorRow, before.CursorColumn)
		})
	}
}

// Neither resize nor a partial clear can conceal a bad repaint. A real full
// repaint replaces current visual damage but retains its complete audit trail.
func TestBDDNativeFramesCurrentRepaintKeepsOverflowHistory(t *testing.T) {
	screen := newBDDNativeFrameDecoder(6, 2)
	feedBDDNativeFrames(t, screen, "\x1b[2J\x1b[HABCDEFG")
	old := screen.snapshot()
	if len(old.Overflow) != 1 || len(old.CurrentOverflow) != 1 || old.FullClearSequence == 0 {
		t.Fatalf("initial overflow evidence missing: %+v", old)
	}
	screen.resize(5, 2)
	resized := screen.snapshot()
	if !slices.Equal(resized.Overflow, old.Overflow) || !slices.Equal(resized.CurrentOverflow, old.CurrentOverflow) || resized.FullClearSequence != old.FullClearSequence {
		t.Fatal("native resize concealed overflow or invented a repaint")
	}
	feedBDDNativeFrames(t, screen, "\x1b[2K\x1b[H")
	if len(screen.snapshot().CurrentOverflow) != 1 {
		t.Fatal("partial erasure concealed latest-repaint overflow")
	}
	feedBDDNativeFrames(t, screen, "\x1b[2", "J\x1b[HMIN")
	current := screen.snapshot()
	requireBDDNativeFrameRows(t, current, "MIN", "")
	if !slices.Equal(current.Overflow, old.Overflow) || len(current.CurrentOverflow) != 0 || current.FullClearSequence <= old.FullClearSequence {
		t.Fatalf("fitting full repaint lost history or retained obsolete current damage: %+v", current)
	}
	feedBDDNativeFrames(t, screen, "\x1b[HABCDEF")
	latest := screen.snapshot()
	if len(latest.CurrentOverflow) != 1 || len(latest.Overflow) != 2 || latest.FullClearSequence != current.FullClearSequence {
		t.Fatalf("new unfitted repaint was concealed: %+v", latest)
	}
	if latest.CurrentOverflow[0].Column != 5 || latest.CurrentOverflow[0].Text != "F" || latest.CurrentOverflow[0].Width != 5 {
		t.Fatalf("latest write-time coordinates changed: %+v", latest.CurrentOverflow)
	}
	if len(old.Overflow) != 1 || len(old.CurrentOverflow) != 1 {
		t.Fatal("later repaint mutated captured evidence")
	}
}
