package identity

import (
	"errors"
	"path"
	"strings"
)

// DisplayLabel is immutable, validated operator context for a live run.
// Its zero value means no display label is available.
type DisplayLabel struct{ value string }

// ParseDisplayLabel accepts the same bounded ASCII words as identity tag values.
func ParseDisplayLabel(value string) (DisplayLabel, error) {
	if !valuePattern.MatchString(value) || strings.Contains(value, "..") {
		return DisplayLabel{}, errors.New("display label must contain 1-64 safe ASCII characters and must not be path-like")
	}
	return DisplayLabel{value: value}, nil
}

// ExecutableDisplayLabel derives safe context from the executable basename only.
func ExecutableDisplayLabel(command string) DisplayLabel {
	base := path.Base(strings.ReplaceAll(command, "\\", "/"))
	var label strings.Builder
	for _, value := range base {
		if displayAlphanumeric(value) || strings.ContainsRune(" ._:@+-", value) {
			label.WriteRune(value)
		}
	}
	value := strings.ReplaceAll(label.String(), "..", "")
	value = strings.TrimLeftFunc(value, func(character rune) bool { return !displayAlphanumeric(character) })
	if len(value) > 64 {
		value = value[:64]
	}
	if value == "" {
		value = "command"
	}
	return DisplayLabel{value: value}
}

func displayAlphanumeric(value rune) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

// String returns the validated label, or an empty string for an unavailable label.
func (label DisplayLabel) String() string { return label.value }
