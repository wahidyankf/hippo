package main

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func verifyCaptureCells(capture string, width, height int) error {
	// Native captures must already be plain text; never sanitize altered proof into a pass.
	if ansi.Strip(capture) != capture || !utf8.ValidString(capture) {
		return errors.New("native capture contains a terminal payload or invalid UTF-8")
	}
	plain := strings.ReplaceAll(capture, "\r\n", "\n")
	for _, character := range plain {
		if unicode.IsControl(character) && character != '\n' {
			return errors.New("native capture contains non-display controls")
		}
	}
	lines := strings.Split(strings.TrimSuffix(plain, "\n"), "\n")
	if width <= 0 || height <= 0 || len(lines) > height {
		return errors.New("capture exceeds terminal row bounds")
	}
	for index, line := range lines {
		if cells := ansi.StringWidth(line); cells > width {
			return fmt.Errorf("capture row %d occupies %d cells beyond width %d", index+1, cells, width)
		}
	}
	return nil
}
