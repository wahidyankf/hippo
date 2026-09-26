// Package identity validates privacy-safe labels attached to Hippo runs.
package identity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// SchemaVersion is the current run-identity document schema.
	SchemaVersion = 1
	// MaximumTags bounds identity cardinality in shared state.
	MaximumTags = 8
	// MaximumJSONBytes bounds the complete encoded identity.
	MaximumJSONBytes = 512
)

var (
	sourcePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	keyPattern    = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,31}$`)
	valuePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._:@+-]{0,63}$`)
)

// Value is the schema-stable, privacy-safe label set for one run.
type Value struct {
	SchemaVersion int               `json:"schemaVersion"`
	Source        string            `json:"source"`
	Tags          map[string]string `json:"tags,omitempty"`
}

func consumeUniqueJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}

	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, keyError := decoder.Token()
			if keyError != nil {
				return keyError
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("identity object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate identity field %q", key)
			}
			seen[key] = true
			if err = consumeUniqueJSON(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err = consumeUniqueJSON(decoder); err != nil {
				return err
			}
		}
	}

	_, err = decoder.Token()

	return err
}

func decode(data []byte) (Value, error) {
	unique := json.NewDecoder(bytes.NewReader(data))
	if err := consumeUniqueJSON(unique); err != nil {
		return Value{}, err
	}
	if _, err := unique.Token(); !errors.Is(err, io.EOF) {
		return Value{}, errors.New("identity must contain one JSON value")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value Value
	if err := decoder.Decode(&value); err != nil {
		return Value{}, err
	}

	return value, nil
}

func parseTag(raw string) (string, string, error) {
	key, value, found := strings.Cut(raw, "=")
	if !found || key == "" || value == "" {
		return "", "", errors.New("tag must use key=value")
	}
	if !keyPattern.MatchString(key) {
		return "", "", fmt.Errorf("tag key %q is invalid", key)
	}
	if !valuePattern.MatchString(value) || strings.Contains(value, "..") {
		return "", "", fmt.Errorf("tag value for %q is invalid or path-like", key)
	}

	return key, value, nil
}

// ParseTags validates repeatable key=value filters with last-value-wins semantics.
func ParseTags(raw []string) (map[string]string, error) {
	tags := map[string]string{}
	for _, entry := range raw {
		key, value, err := parseTag(entry)
		if err != nil {
			return nil, err
		}
		tags[key] = value
	}
	if len(tags) > MaximumTags {
		return nil, fmt.Errorf("identity permits at most %d tags", MaximumTags)
	}

	return tags, nil
}

// Validate rejects labels that could reveal paths or grow shared records without bound.
func Validate(value Value) error {
	if value.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported identity schema %d", value.SchemaVersion)
	}
	if value.Source == "" {
		return errors.New("identity source is required")
	}
	if !sourcePattern.MatchString(value.Source) || strings.Contains(value.Source, "..") {
		return fmt.Errorf("identity source %q is invalid or path-like", value.Source)
	}
	if len(value.Tags) > MaximumTags {
		return fmt.Errorf("identity permits at most %d tags", MaximumTags)
	}
	for key, tagValue := range value.Tags {
		if !keyPattern.MatchString(key) {
			return fmt.Errorf("tag key %q is invalid", key)
		}
		if !valuePattern.MatchString(tagValue) || strings.Contains(tagValue, "..") {
			return fmt.Errorf("tag value for %q is invalid or path-like", key)
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(encoded) > MaximumJSONBytes {
		return fmt.Errorf("identity exceeds %d encoded bytes", MaximumJSONBytes)
	}

	return nil
}

// ValidateOverrides checks invocation overrides without reading any file, so
// a caller can reject its own mistyped flags before it does any work.
func ValidateOverrides(sourceOverride string, tagOverrides []string) error {
	if sourceOverride != "" && (!sourcePattern.MatchString(sourceOverride) || strings.Contains(sourceOverride, "..")) {
		return fmt.Errorf("identity source %q is invalid or path-like", sourceOverride)
	}
	_, err := ParseTags(tagOverrides)

	return err
}

// Load reads an optional schema-1 file, then applies invocation overrides.
func Load(path, sourceOverride string, tagOverrides []string) (Value, error) {
	value := Value{SchemaVersion: SchemaVersion, Tags: map[string]string{}}
	data, err := os.ReadFile(filepath.Clean(path))
	if err == nil {
		value, err = decode(data)
		if err != nil {
			return Value{}, fmt.Errorf("decode identity: %w", err)
		}
		if value.Tags == nil {
			value.Tags = map[string]string{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Value{}, err
	}

	if sourceOverride != "" {
		value.Source = sourceOverride
	}
	overrides, err := ParseTags(tagOverrides)
	if err != nil {
		return Value{}, err
	}
	maps.Copy(value.Tags, overrides)
	if err = Validate(value); err != nil {
		return Value{}, err
	}

	return value, nil
}

// Path resolves an explicit environment path, then discovers a worktree-local
// identity while walking upward. A machine default is the final fallback.
func Path(environment map[string]string, workingDirectory string) string {
	if path := environment["HIPPO_IDENTITY"]; path != "" {
		return path
	}
	if workingDirectory == "" {
		workingDirectory, _ = os.Getwd()
	}
	current, err := filepath.Abs(workingDirectory)
	if err == nil {
		for {
			candidate := filepath.Join(current, "hippo.identity.json")
			if _, statError := os.Stat(candidate); statError == nil {
				return candidate
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	if path := environment["HIPPO_DEFAULT_IDENTITY"]; path != "" {
		return path
	}

	return filepath.Join(workingDirectory, "hippo.identity.json")
}
