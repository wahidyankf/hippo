package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/wahidyankf/hippo/internal/policy"
)

const (
	schemaVersionExclusive   = 1
	schemaVersionReservation = 2
	defaultMaxActiveOwners   = 20
)

// Coordination is the validated, privacy-safe shared-root coordination policy.
type Coordination struct {
	Mode            string
	MaxCPU          int
	MaxMemoryBytes  int64
	MaxActiveOwners int
	OwnerShares     map[string]int
}

// Result contains the validated catalog without retaining local file contents.
type Result struct {
	Catalog      policy.Catalog
	Coordination Coordination
	Hash         string
	Source       string
}

type coordinationFile struct {
	Mode                 string         `json:"mode,omitempty"`
	MaxCPU               int            `json:"maxCpu,omitempty"`
	MaxMemoryMiB         int64          `json:"maxMemoryMiB,omitempty"`
	MaxActiveOwners      int            `json:"maxActiveOwners,omitempty"`
	AutomaticOwnerShares map[string]int `json:"automaticOwnerShares,omitempty"`
}

type profileOverride struct {
	Extends                    string   `json:"extends,omitempty"`
	Fallback                   *string  `json:"fallback,omitempty"`
	Strict                     *bool    `json:"strict,omitempty"`
	MemoryReservePercent       *float64 `json:"memoryReservePercent,omitempty"`
	MemoryReserveMinMiB        *int64   `json:"memoryReserveMinMiB,omitempty"`
	MemoryReserveMaxMiB        *int64   `json:"memoryReserveMaxMiB,omitempty"`
	NoSwapMemoryReservePercent *float64 `json:"noSwapMemoryReservePercent,omitempty"`
	NoSwapMemoryReserveMinMiB  *int64   `json:"noSwapMemoryReserveMinMiB,omitempty"`
	NoSwapMemoryReserveMaxMiB  *int64   `json:"noSwapMemoryReserveMaxMiB,omitempty"`
	DiskReservePercent         *float64 `json:"diskReservePercent,omitempty"`
	DiskReserveMinMiB          *int64   `json:"diskReserveMinMiB,omitempty"`
	DiskReserveMaxMiB          *int64   `json:"diskReserveMaxMiB,omitempty"`
	MaxConcurrency             *int     `json:"maxConcurrency,omitempty"`
	MaxCPUUtilizationPercent   *float64 `json:"maxCpuUtilizationPercent,omitempty"`
}

type file struct {
	SchemaVersion  int                        `json:"schemaVersion"`
	DefaultProfile string                     `json:"defaultProfile,omitempty"`
	Profiles       map[string]profileOverride `json:"profiles,omitempty"`
	Coordination   *coordinationFile          `json:"coordination,omitempty"`
}

func exclusiveCoordination() Coordination {
	return Coordination{Mode: "exclusive"}
}

func reservationCoordination() Coordination {
	return Coordination{
		Mode:            "reservation",
		MaxActiveOwners: defaultMaxActiveOwners,
		OwnerShares: map[string]int{
			"balanced":    4,
			"constrained": 2,
			"minimal":     1,
		},
	}
}

func buildCoordination(decoded file, catalog policy.Catalog) (Coordination, error) { //nolint:cyclop,gocognit // Validation enumerates schema compatibility and profile inheritance invariants.
	if decoded.SchemaVersion == schemaVersionExclusive {
		if decoded.Coordination != nil {
			return Coordination{}, errors.New("schema 1 does not support reservation coordination")
		}

		return exclusiveCoordination(), nil
	}
	if decoded.SchemaVersion != schemaVersionReservation {
		return Coordination{}, fmt.Errorf("unsupported configuration schema %d", decoded.SchemaVersion)
	}

	result := reservationCoordination()
	configured := decoded.Coordination
	if configured == nil {
		configured = &coordinationFile{}
	}
	if configured.Mode != "" && configured.Mode != "reservation" {
		return Coordination{}, fmt.Errorf("unsupported schema 2 coordination mode %q", configured.Mode)
	}
	if configured.MaxCPU < 0 || configured.MaxMemoryMiB < 0 || configured.MaxActiveOwners < 0 {
		return Coordination{}, errors.New("reservation limits must be nonnegative")
	}
	if configured.MaxMemoryMiB > 0 && configured.MaxMemoryMiB < 256 {
		return Coordination{}, errors.New("maximum memory weakens the immutable 256 MiB floor")
	}
	if configured.MaxActiveOwners > defaultMaxActiveOwners {
		return Coordination{}, fmt.Errorf("maxActiveOwners cannot exceed %d", defaultMaxActiveOwners)
	}

	if configured.MaxCPU > 0 {
		result.MaxCPU = configured.MaxCPU
	}
	if configured.MaxMemoryMiB > 0 {
		converted, conversionError := policy.MiBToBytes(configured.MaxMemoryMiB)
		if conversionError != nil {
			return Coordination{}, conversionError
		}
		result.MaxMemoryBytes = converted
	}
	if configured.MaxActiveOwners > 0 {
		result.MaxActiveOwners = configured.MaxActiveOwners
	}
	for name, shares := range configured.AutomaticOwnerShares {
		if _, exists := catalog.Profiles[name]; !exists {
			return Coordination{}, fmt.Errorf("automatic owner shares name unknown profile %q", name)
		}
		if shares < 1 || shares > defaultMaxActiveOwners {
			return Coordination{}, fmt.Errorf("automatic owner shares for %q must be between 1 and %d", name, defaultMaxActiveOwners)
		}
		result.OwnerShares[name] = shares
	}
	visiting := map[string]bool{}
	var inheritShares func(string) (int, error)
	inheritShares = func(name string) (int, error) {
		if shares := result.OwnerShares[name]; shares > 0 {
			return shares, nil
		}
		if visiting[name] {
			return 0, fmt.Errorf("automatic owner shares inheritance cycle at %q", name)
		}
		visiting[name] = true
		override := decoded.Profiles[name]
		if override.Extends == "" {
			return 0, fmt.Errorf("automatic owner shares unavailable for profile %q", name)
		}
		shares, err := inheritShares(override.Extends)
		delete(visiting, name)
		if err == nil {
			result.OwnerShares[name] = shares
		}

		return shares, err
	}
	for name := range catalog.Profiles {
		if _, err := inheritShares(name); err != nil {
			return Coordination{}, err
		}
	}

	return result, nil
}

func consumeJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
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
				return errors.New("JSON object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON field %q", key)
			}

			seen[key] = true
			if valueError := consumeJSON(decoder); valueError != nil {
				return valueError
			}
		}
	case '[':
		for decoder.More() {
			if itemError := consumeJSON(decoder); itemError != nil {
				return itemError
			}
		}
	}

	_, err = decoder.Token()
	return err
}

func decode(data []byte) (file, error) {
	duplicateDecoder := json.NewDecoder(bytes.NewReader(data))
	if err := consumeJSON(duplicateDecoder); err != nil {
		return file{}, err
	}

	if _, err := duplicateDecoder.Token(); !errors.Is(err, io.EOF) {
		return file{}, errors.New("configuration must contain one JSON value")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var decoded file
	if err := decoder.Decode(&decoded); err != nil {
		return file{}, err
	}

	return decoded, nil
}

func setFloat(target, override *float64) {
	if override != nil {
		*target = *override
	}
}

func setBytes(target, override *int64) error {
	if override != nil {
		converted, err := policy.MiBToBytes(*override)
		if err != nil {
			return err
		}
		*target = converted
	}

	return nil
}

func apply(name string, base policy.Profile, override profileOverride) (policy.Profile, error) {
	base.Name = name
	if override.Fallback != nil {
		base.Fallback = *override.Fallback
	}
	if override.Strict != nil {
		base.Strict = *override.Strict
	}

	setFloat(&base.MemoryReservePercent, override.MemoryReservePercent)
	if err := setBytes(&base.MemoryReserveMinBytes, override.MemoryReserveMinMiB); err != nil {
		return policy.Profile{}, err
	}
	if err := setBytes(&base.MemoryReserveMaxBytes, override.MemoryReserveMaxMiB); err != nil {
		return policy.Profile{}, err
	}
	setFloat(&base.NoSwapMemoryReservePercent, override.NoSwapMemoryReservePercent)
	if err := setBytes(&base.NoSwapMemoryReserveMinBytes, override.NoSwapMemoryReserveMinMiB); err != nil {
		return policy.Profile{}, err
	}
	if err := setBytes(&base.NoSwapMemoryReserveMaxBytes, override.NoSwapMemoryReserveMaxMiB); err != nil {
		return policy.Profile{}, err
	}
	setFloat(&base.DiskReservePercent, override.DiskReservePercent)
	if err := setBytes(&base.DiskReserveMinBytes, override.DiskReserveMinMiB); err != nil {
		return policy.Profile{}, err
	}
	if err := setBytes(&base.DiskReserveMaxBytes, override.DiskReserveMaxMiB); err != nil {
		return policy.Profile{}, err
	}

	if override.MaxConcurrency != nil {
		base.MaxConcurrency = *override.MaxConcurrency
	}

	setFloat(&base.MaxCPUUtilizationPercent, override.MaxCPUUtilizationPercent)

	return base, nil
}

func validateProfile(profile policy.Profile) error {
	if profile.MemoryReservePercent <= 0 || profile.NoSwapMemoryReservePercent <= 0 || profile.DiskReservePercent <= 0 {
		return errors.New("reserve percentages must be positive")
	}

	if profile.MemoryReserveMinBytes < 128*policy.MiB ||
		profile.NoSwapMemoryReserveMinBytes < 256*policy.MiB ||
		profile.DiskReserveMinBytes < policy.HardDiskFloorBytes {
		return errors.New("profile weakens immutable safety floors")
	}

	if profile.MemoryReserveMaxBytes < profile.MemoryReserveMinBytes ||
		profile.NoSwapMemoryReserveMaxBytes < profile.NoSwapMemoryReserveMinBytes ||
		profile.DiskReserveMaxBytes < profile.DiskReserveMinBytes {
		return errors.New("profile reserve maximum is below its minimum")
	}

	if profile.MaxConcurrency < 0 || profile.MaxCPUUtilizationPercent <= 0 || profile.MaxCPUUtilizationPercent > 98 {
		return errors.New("profile CPU limits are invalid")
	}

	return nil
}

func buildCatalog(decoded file) (policy.Catalog, error) { //nolint:gocognit // Recursive inheritance and fallback validation deliberately share one graph walk.
	if decoded.SchemaVersion != schemaVersionExclusive && decoded.SchemaVersion != schemaVersionReservation {
		return policy.Catalog{}, fmt.Errorf("unsupported configuration schema %d", decoded.SchemaVersion)
	}

	catalog := policy.BuiltinCatalog()
	resolved := map[string]bool{}
	visiting := map[string]bool{}
	var resolve func(string) (policy.Profile, error)

	resolve = func(name string) (policy.Profile, error) {
		if resolved[name] {
			return catalog.Profiles[name], nil
		}
		if visiting[name] {
			return policy.Profile{}, fmt.Errorf("profile inheritance cycle at %q", name)
		}

		override, configured := decoded.Profiles[name]
		base, builtin := catalog.Profiles[name]
		if !configured {
			if !builtin {
				return policy.Profile{}, fmt.Errorf("unknown profile %q", name)
			}
			resolved[name] = true

			return base, nil
		}

		visiting[name] = true
		if override.Extends != "" {
			parent, err := resolve(override.Extends)
			if err != nil {
				return policy.Profile{}, err
			}
			base = parent
		} else if !builtin {
			return policy.Profile{}, fmt.Errorf("custom profile %q requires extends", name)
		}

		base, err := apply(name, base, override)
		if err != nil {
			return policy.Profile{}, fmt.Errorf("profile %q: %w", name, err)
		}
		if err := validateProfile(base); err != nil {
			return policy.Profile{}, fmt.Errorf("profile %q: %w", name, err)
		}

		delete(visiting, name)
		resolved[name] = true
		catalog.Profiles[name] = base

		return base, nil
	}

	names := make([]string, 0, len(decoded.Profiles))
	for name := range decoded.Profiles {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		if _, err := resolve(name); err != nil {
			return policy.Catalog{}, err
		}
	}

	for name, profile := range catalog.Profiles {
		if profile.Fallback != "" {
			if _, exists := catalog.Profiles[profile.Fallback]; !exists {
				return policy.Catalog{}, fmt.Errorf("profile %q has unknown fallback %q", name, profile.Fallback)
			}
		}
	}

	for name := range catalog.Profiles {
		seen := map[string]bool{}
		current := name
		for current != "" {
			if seen[current] {
				return policy.Catalog{}, fmt.Errorf("profile fallback cycle at %q", current)
			}
			seen[current] = true
			current = catalog.Profiles[current].Fallback
		}
	}

	defaultProfile := decoded.DefaultProfile
	if defaultProfile == "" {
		defaultProfile = catalog.DefaultProfile
	}

	if _, exists := catalog.Profiles[defaultProfile]; !exists {
		return policy.Catalog{}, fmt.Errorf("unknown default profile %q", defaultProfile)
	}

	catalog.DefaultProfile = defaultProfile

	return catalog, nil
}

// Load reads an explicit or optional local configuration.
func Load(path string, explicit bool) (Result, error) {
	if path == "" {
		return Result{Catalog: policy.BuiltinCatalog(), Coordination: exclusiveCoordination()}, nil
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) && !explicit {
		return Result{Catalog: policy.BuiltinCatalog(), Coordination: exclusiveCoordination()}, nil
	}
	if err != nil {
		return Result{}, err
	}

	decoded, err := decode(data)
	if err != nil {
		return Result{}, err
	}

	catalog, err := buildCatalog(decoded)
	if err != nil {
		return Result{}, err
	}
	coordination, err := buildCoordination(decoded, catalog)
	if err != nil {
		return Result{}, err
	}

	hash := sha256.Sum256(data)

	return Result{
		Catalog:      catalog,
		Coordination: coordination,
		Hash:         hex.EncodeToString(hash[:]),
		Source:       "local",
	}, nil
}

// Path resolves CLI, environment, and repository-local configuration precedence.
func Path(cliPath string, environment map[string]string) (string, bool) {
	if cliPath != "" {
		return cliPath, true
	}

	if path := environment["HIPPO_CONFIG"]; path != "" {
		return path, true
	}

	if path := environment["HIPPO_DEFAULT_CONFIG"]; path != "" {
		return path, false
	}

	return "hippo.local.json", false
}
