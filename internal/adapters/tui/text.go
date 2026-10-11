package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
)

const unavailableText = "unavailable"

// safeText parses terminal sequences before normalizing UTF-8 or controls.
// Encoded C1 introducers are equivalent to their raw terminal bytes.
func safeText(value string) string {
	value = strings.NewReplacer(
		"\u0090", "\x90", "\u0098", "\x98", "\u009b", "\x9b", "\u009c", "\x9c",
		"\u009d", "\x9d", "\u009e", "\x9e", "\u009f", "\x9f",
	).Replace(value)
	value = strings.ToValidUTF8(ansi.Strip(value), "")
	return strings.Map(func(character rune) rune {
		switch character {
		case '\r', '\n', '\t':
			return ' '
		default:
			if unicode.IsControl(character) {
				return -1
			}
			return character
		}
	}, value)
}

func fitText(value string, width int) string {
	if width <= 0 {
		return ""
	}
	value = safeText(value)
	return ansi.Truncate(value, width, strings.Repeat(".", min(3, width)))
}

func wrapText(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.Wrap(safeText(value), width, " ")
}

func bytesText(value int64) string {
	if value < 0 {
		return unavailableText
	}
	for _, unit := range []struct {
		name string
		size int64
	}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}} {
		if value >= unit.size {
			return fmt.Sprintf("%.1f%s", float64(value)/float64(unit.size), unit.name)
		}
	}
	return fmt.Sprintf("%dB", value)
}

func optionalBytes(value *int64) string {
	if value == nil {
		return unavailableText
	}
	return bytesText(*value)
}

func vectorText(value coordination.ReservationVector, known bool) string {
	if !known {
		return unavailableText
	}
	return fmt.Sprintf("%dCPU %s", value.CPU, bytesText(value.MemoryBytes))
}

func positiveText(value int) string {
	if value <= 0 {
		return unavailableText
	}
	return strconv.Itoa(value)
}

func nonemptyText(value string) string {
	if value == "" {
		return unavailableText
	}
	return safeText(value)
}
