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
	"slices"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	coordination "github.com/wahidyankf/hippo/internal/domain/coordination"

	guard "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/policy"
)

const (
	schemaVersionExclusive   = 1
	schemaVersionReservation = 2
	schemaVersionAdaptive    = 3
	defaultMaxActiveOwners   = 20
)

// Promotion is a pure owner-promotion configuration.
type Promotion = coordination.Promotion

// Coordination is the validated coordination-domain configuration.
type Coordination = coordination.Configuration

// Result is the application's normalized configuration value.
type Result = application.Configuration

// coordinationMode is the mode a document names for its coordination. The type
// is closed: a schema-2 or schema-3 document may name no mode, an empty one, or
// reservation, so a document naming anything else is refused where it is
// decoded, as an unreadable configuration, rather than carried as text until
// some later comparison.
type coordinationMode uint8

const (
	// coordinationModeDefault is no mode named: the key omitted, or the empty
	// string. Either way the document gets reservation coordination.
	coordinationModeDefault coordinationMode = iota
	// coordinationModeReservation names reservation coordination.
	coordinationModeReservation
)

// reservationModeName is the one mode a document may name, and the mode its
// coordination reports.
const reservationModeName = "reservation"

// UnmarshalText accepts exactly the texts a document has always been able to
// carry here, the empty string and reservation, and refuses every other.
func (mode *coordinationMode) UnmarshalText(text []byte) error {
	switch string(text) {
	case "":
		*mode = coordinationModeDefault
	case reservationModeName:
		*mode = coordinationModeReservation
	default:
		return fmt.Errorf("unsupported coordination mode %q", text)
	}

	return nil
}

type coordinationFile struct {
	Mode                 coordinationMode           `json:"mode,omitempty"`
	MaxCPU               int                        `json:"maxCpu,omitempty"`
	MaxMemoryMiB         int64                      `json:"maxMemoryMiB,omitempty"`
	MaxActiveOwners      int                        `json:"maxActiveOwners,omitempty"`
	BaseActiveOwners     int                        `json:"baseActiveOwners,omitempty"`
	AutomaticOwnerShares map[policy.ProfileName]int `json:"automaticOwnerShares,omitempty"`
	Promotion            *promotionFile             `json:"promotion,omitempty"`
	EmergencyMemoryMiB   int64                      `json:"emergencyAvailableMemoryMiB,omitempty"`
	Tiers                map[string]tierFile        `json:"tiers,omitempty"`
}

type promotionFile struct {
	CompletedRuns             int     `json:"completedRuns"`
	MinimumSources            int     `json:"minimumSources"`
	MinimumAvailableMemoryMiB int64   `json:"minimumAvailableMemoryMiB"`
	MaximumCPUP95Percent      float64 `json:"maximumCpuP95Percent"`
}

type tierFile struct {
	MinimumCPU       int    `json:"minimumCpu"`
	MaximumCPU       int    `json:"maximumCpu"`
	MinimumMemoryMiB int64  `json:"minimumMemoryMiB"`
	MaximumMemoryMiB int64  `json:"maximumMemoryMiB"`
	QueueDeadline    string `json:"queueDeadline"`
}

type profileOverride struct {
	Extends                    policy.ProfileName  `json:"extends,omitempty"`
	Fallback                   *policy.ProfileName `json:"fallback,omitempty"`
	Strict                     *bool               `json:"strict,omitempty"`
	MemoryReservePercent       *float64            `json:"memoryReservePercent,omitempty"`
	MemoryReserveMinMiB        *int64              `json:"memoryReserveMinMiB,omitempty"`
	MemoryReserveMaxMiB        *int64              `json:"memoryReserveMaxMiB,omitempty"`
	NoSwapMemoryReservePercent *float64            `json:"noSwapMemoryReservePercent,omitempty"`
	NoSwapMemoryReserveMinMiB  *int64              `json:"noSwapMemoryReserveMinMiB,omitempty"`
	NoSwapMemoryReserveMaxMiB  *int64              `json:"noSwapMemoryReserveMaxMiB,omitempty"`
	DiskReservePercent         *float64            `json:"diskReservePercent,omitempty"`
	DiskReserveMinMiB          *int64              `json:"diskReserveMinMiB,omitempty"`
	DiskReserveMaxMiB          *int64              `json:"diskReserveMaxMiB,omitempty"`
	MaxConcurrency             *int                `json:"maxConcurrency,omitempty"`
	MaxCPUUtilizationPercent   *float64            `json:"maxCpuUtilizationPercent,omitempty"`
}

type file struct {
	SchemaVersion  int                                    `json:"schemaVersion"`
	DefaultProfile policy.ProfileName                     `json:"defaultProfile,omitempty"`
	Profiles       map[policy.ProfileName]profileOverride `json:"profiles,omitempty"`
	Coordination   *coordinationFile                      `json:"coordination,omitempty"`
}

func exclusiveCoordination() Coordination {
	return Coordination{SchemaVersion: schemaVersionExclusive, Mode: "exclusive"}
}

// reservationCoordination is the coordination a schema-2 file starts from. Its
// owner shares are each built-in profile's lineage default, so the table of 4, 2,
// and 1 lives once, with the lineage that decides it.
func reservationCoordination() Coordination {
	shares := map[policy.ProfileName]int{}
	for name, profile := range policy.BuiltinCatalog().Profiles {
		shares[name] = profile.Lineage.DefaultOwnerShares()
	}

	return Coordination{
		SchemaVersion:   schemaVersionReservation,
		Mode:            reservationModeName,
		MaxActiveOwners: defaultMaxActiveOwners,
		OwnerShares:     shares,
	}
}

func adaptiveCoordination(decoded file, result Coordination, configured *coordinationFile) (Coordination, error) { //nolint:cyclop,gocyclo // Every required adaptive field and bound is validated explicitly.
	if decoded.SchemaVersion != schemaVersionAdaptive {
		return result, nil
	}
	if decoded.Coordination == nil {
		return Coordination{}, errors.New("schema 3 requires adaptive coordination")
	}
	result.SchemaVersion = schemaVersionAdaptive
	if configured.MaxCPU <= 0 || configured.MaxMemoryMiB <= 0 || configured.BaseActiveOwners <= 0 ||
		configured.MaxActiveOwners <= 0 {
		return Coordination{}, errors.New("schema 3 requires positive pool and owner limits")
	}
	if configured.BaseActiveOwners > configured.MaxActiveOwners {
		return Coordination{}, errors.New("baseActiveOwners cannot exceed maxActiveOwners")
	}
	result.BaseActiveOwners = configured.BaseActiveOwners
	if configured.Promotion == nil {
		return Coordination{}, errors.New("schema 3 requires a promotion gate")
	}
	promotion := configured.Promotion
	if promotion.CompletedRuns < 1 || promotion.MinimumSources < 1 ||
		promotion.MinimumSources > promotion.CompletedRuns || promotion.MinimumAvailableMemoryMiB < 256 ||
		promotion.MaximumCPUP95Percent <= 0 || promotion.MaximumCPUP95Percent > 98 {
		return Coordination{}, errors.New("schema 3 promotion gate is invalid")
	}
	minimumAvailable, err := policy.MiBToBytes(promotion.MinimumAvailableMemoryMiB)
	if err != nil {
		return Coordination{}, err
	}
	result.Promotion = Promotion{
		CompletedRuns: promotion.CompletedRuns, MinimumSources: promotion.MinimumSources,
		MinimumAvailableMemoryBytes: minimumAvailable,
		MaximumCPUP95Percent:        promotion.MaximumCPUP95Percent,
	}
	if configured.EmergencyMemoryMiB < 256 {
		return Coordination{}, errors.New("schema 3 emergency memory floor is invalid")
	}
	result.EmergencyAvailableMemoryBytes, err = policy.MiBToBytes(configured.EmergencyMemoryMiB)
	if err != nil {
		return Coordination{}, err
	}
	result.Tiers = make(map[string]guard.ResourceTierPolicy, 3)
	for _, name := range []string{"light", "standard", "heavy"} {
		configuredTier, exists := configured.Tiers[name]
		if !exists {
			return Coordination{}, fmt.Errorf("schema 3 requires %s resource tier", name)
		}
		minimumMemory, conversionError := policy.MiBToBytes(configuredTier.MinimumMemoryMiB)
		if conversionError != nil {
			return Coordination{}, conversionError
		}
		maximumMemory, conversionError := policy.MiBToBytes(configuredTier.MaximumMemoryMiB)
		if conversionError != nil {
			return Coordination{}, conversionError
		}
		deadline, durationError := time.ParseDuration(configuredTier.QueueDeadline)
		if durationError != nil || deadline <= 0 {
			return Coordination{}, fmt.Errorf("schema 3 %s queue deadline is invalid", name)
		}
		tier := guard.ResourceTierPolicy{
			Minimum:       guard.ReservationVector{CPU: configuredTier.MinimumCPU, MemoryBytes: minimumMemory},
			Maximum:       guard.ReservationVector{CPU: configuredTier.MaximumCPU, MemoryBytes: maximumMemory},
			QueueDeadline: deadline,
		}
		if tier.Minimum.CPU < guard.MinimumReservationCPU ||
			tier.Minimum.MemoryBytes < guard.MinimumReservationMemoryBytes ||
			tier.Maximum.CPU < tier.Minimum.CPU || tier.Maximum.MemoryBytes < tier.Minimum.MemoryBytes ||
			tier.Maximum.CPU > result.MaxCPU || tier.Maximum.MemoryBytes > result.MaxMemoryBytes {
			return Coordination{}, fmt.Errorf("schema 3 %s resource tier is outside the pool", name)
		}
		result.Tiers[name] = tier
	}
	if len(configured.Tiers) != len(result.Tiers) {
		return Coordination{}, errors.New("schema 3 contains an unknown resource tier")
	}

	return result, nil
}

func buildCoordination(decoded file, catalog policy.Catalog) (Coordination, error) { //nolint:cyclop,gocognit // Validation enumerates schema compatibility and profile inheritance invariants.
	if decoded.SchemaVersion == schemaVersionExclusive {
		if decoded.Coordination != nil {
			return Coordination{}, errors.New("schema 1 does not support reservation coordination")
		}

		return exclusiveCoordination(), nil
	}
	if decoded.SchemaVersion != schemaVersionReservation && decoded.SchemaVersion != schemaVersionAdaptive {
		return Coordination{}, fmt.Errorf("unsupported configuration schema %d", decoded.SchemaVersion)
	}

	result := reservationCoordination()
	configured := decoded.Coordination
	if configured == nil {
		configured = &coordinationFile{}
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
	visiting := map[policy.ProfileName]bool{}
	var inheritShares func(policy.ProfileName) (int, error)
	inheritShares = func(name policy.ProfileName) (int, error) {
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

	return adaptiveCoordination(decoded, result, configured)
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

func apply(name policy.ProfileName, base policy.Profile, override profileOverride) (policy.Profile, error) {
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
	if decoded.SchemaVersion != schemaVersionExclusive && decoded.SchemaVersion != schemaVersionReservation &&
		decoded.SchemaVersion != schemaVersionAdaptive {
		return policy.Catalog{}, fmt.Errorf("unsupported configuration schema %d", decoded.SchemaVersion)
	}

	catalog := policy.BuiltinCatalog()
	resolved := map[policy.ProfileName]bool{}
	visiting := map[policy.ProfileName]bool{}
	var resolve func(policy.ProfileName) (policy.Profile, error)

	resolve = func(name policy.ProfileName) (policy.Profile, error) {
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

	names := make([]policy.ProfileName, 0, len(decoded.Profiles))
	for name := range decoded.Profiles {
		names = append(names, name)
	}

	slices.Sort(names)

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
		seen := map[policy.ProfileName]bool{}
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
