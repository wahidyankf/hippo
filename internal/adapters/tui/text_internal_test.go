package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

func TestTextRemovesTerminalPayloadsBeforeNormalizingControls(t *testing.T) {
	cases := []struct {
		name, input, want string
	}{
		{"CSI and OSC", "safe\x1b[31mred\x1b[0m\x1b]52;c;private\x07end", "saferedend"},
		{"raw C1", "safe\x9b31mred\x9d52;c;private\x9cend", "saferedend"},
		{"encoded C1", "safe\u009b31mred\u009d52;c;private\u009cend", "saferedend"},
		{"DCS", "safe\x1bPprivate\x1b\\end", "safeend"},
		{"controls and invalid UTF8", "a\r\n\t\x00\x7f\u0085\xffb", "a   b"},
		{"combining and wide", "\u754ce\u0301", "\u754ce\u0301"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := safeText(test.input); got != test.want {
				t.Errorf("sanitized text=%q want=%q", got, test.want)
			}
		})
	}
}

func TestTextTruncatesInCellsWithFittedASCIIMarker(t *testing.T) {
	for width := range 16 {
		for _, input := range []string{strings.Repeat("a", 40), strings.Repeat("\u754c", 20), strings.Repeat("e\u0301", 20), "\x1b]52;c;private\x07" + strings.Repeat("wide", 10)} {
			got := fitText(input, width)
			if lipgloss.Width(got) > width || !utf8.ValidString(got) || strings.Contains(got, "private") || strings.Contains(got, "\x1b") {
				t.Errorf("unsafe/truncated frame at width=%d: %q", width, got)
			}
			if width > 0 && !strings.HasSuffix(got, strings.Repeat(".", min(3, width))) {
				t.Errorf("missing fitted marker at width=%d: %q", width, got)
			}
		}
	}
	if fitText("unchanged", 20) != "unchanged" || fitText("ignored", -1) != "" {
		t.Error("truncation changed fitting text or produced negative-budget output")
	}
}

func TestTextWrapsLongValuesWithinCellsWithoutTerminalPayloads(t *testing.T) {
	for _, width := range []int{40, 60, 80} {
		input := "Tag: \x1b]52;c;private\x07" + strings.Repeat("\u754ce\u0301word", 40)
		got := wrapText(input, width)
		if strings.Contains(got, "private") || strings.ContainsAny(got, "\x1b\r\t") {
			t.Error("wrapped detail preserved terminal payload")
		}
		for line := range strings.SplitSeq(got, "\n") {
			if lipgloss.Width(line) > width {
				t.Errorf("wrapped line exceeds %d cells: %q", width, line)
			}
		}
	}
	if wrapText("ignored", 0) != "" {
		t.Error("zero-width detail emitted content")
	}
}

func TestTextStylingRetainsPlainStatesAndCellBudgets(t *testing.T) {
	state, _ := newViewsContract(t, 80, 24)
	plain := state.View().Content
	state.styling = true
	styled := state.View().Content
	if !strings.Contains(styled, "\x1b") {
		t.Error("enabled styling produced no semantic style")
	}
	if lipgloss.Height(plain) != lipgloss.Height(styled) {
		t.Error("styling changed row budget")
	}
	for line := range strings.SplitSeq(styled, "\n") {
		if lipgloss.Width(line) > 80 {
			t.Error("styling changed cell budget")
		}
	}
	if got := safeText(styled); got != strings.ReplaceAll(plain, "\n", " ") {
		t.Error("styling changed the visible state text")
	}
}
