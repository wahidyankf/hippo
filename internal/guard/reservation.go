package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
	"golang.org/x/sys/unix"
)

const (
	// MinimumReservationCPU is the immutable per-owner CPU floor.
	MinimumReservationCPU = 1
	// MinimumReservationMemoryBytes is the immutable per-owner memory floor.
	MinimumReservationMemoryBytes  = 256 * policy.MiB
	reservationLedgerSchemaVersion = 2
	maximumReservationOwners       = 20
)

// ErrReservationReplan identifies a request that can never fit the configured safe budget.
var ErrReservationReplan = errors.New("reservation requires replanning")

// ErrReservationDeferred identifies bounded FIFO exhaustion that maps to exit 75.
var ErrReservationDeferred = errors.New("reservation capacity remained exhausted")

// ReservationPolicy configures shared vector admission without naming a consumer repository.
type ReservationPolicy struct {
	Enabled         bool
	MaxCPU          int
	MaxMemoryBytes  int64
	MaxActiveOwners int
	OwnerShares     map[string]int
}

// ReservationVector is an atomic CPU-and-memory allocation.
type ReservationVector struct {
	CPU         int   `json:"cpu"`
	MemoryBytes int64 `json:"memoryBytes"`
}

// ReservationPlan fixes one request and the safe shared capacity used to validate it.
type ReservationPlan struct {
	Capacity  ReservationVector `json:"capacity"`
	Requested ReservationVector `json:"requested"`
	Allocated ReservationVector `json:"allocated"`
}

// ReservationOwner is a privacy-safe shared owner record. PID is diagnostic only;
// ownership liveness is proven by an advisory identity lock rather than PID equality.
type ReservationOwner struct {
	Token          string            `json:"token"`
	PID            int               `json:"pid"`
	Class          policy.TaskClass  `json:"class"`
	Profile        string            `json:"profile"`
	Requested      ReservationVector `json:"requested"`
	Allocated      ReservationVector `json:"allocated"`
	Sequence       uint64            `json:"sequence"`
	ConfigHash     string            `json:"configHash,omitempty"`
	ProcessGroup   int               `json:"processGroup,omitempty"`
	Shedding       bool              `json:"shedding,omitempty"`
	SheddingExit   int               `json:"sheddingExitCode,omitempty"`
	MaxOwners      int               `json:"maxActiveOwners"`
	PeakOwners     int               `json:"peakOwnerCount,omitempty"`
	IdentityDevice uint64            `json:"identityDevice,omitempty"`
	IdentityInode  uint64            `json:"identityInode,omitempty"`
}

type reservationWaiter struct {
	Token          string            `json:"token"`
	PID            int               `json:"pid"`
	Class          policy.TaskClass  `json:"class"`
	Profile        string            `json:"profile"`
	Requested      ReservationVector `json:"requested"`
	Sequence       uint64            `json:"sequence"`
	ConfigHash     string            `json:"configHash,omitempty"`
	MaxOwners      int               `json:"maxActiveOwners"`
	IdentityDevice uint64            `json:"identityDevice,omitempty"`
	IdentityInode  uint64            `json:"identityInode,omitempty"`
}

type reservationLedger struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Capacity      ReservationVector   `json:"capacity"`
	NextSequence  uint64              `json:"nextSequence"`
	Owners        []ReservationOwner  `json:"owners"`
	Waiters       []reservationWaiter `json:"waiters"`
}

// ReservationTotals is the schema-stable coordination view exposed by status.
type ReservationTotals struct {
	SchemaVersion int               `json:"schemaVersion"`
	Mode          string            `json:"mode"`
	Capacity      ReservationVector `json:"capacity"`
	Allocated     ReservationVector `json:"allocated"`
	Waiting       ReservationVector `json:"waiting"`
	ActiveOwners  int               `json:"activeOwners"`
	WaitingOwners int               `json:"waitingOwners"`
	Ephemeral     int               `json:"ephemeral"`
	Service       int               `json:"service"`
	Transactional int               `json:"transactional"`
}

func ceilDivide(value int64, divisor int) int64 {
	quotient, remainder := value/int64(divisor), value%int64(divisor)
	if remainder != 0 {
		quotient++
	}

	return quotient
}

// PlanReservation calculates an automatic fair share and applies explicit vector overrides.
func PlanReservation(
	sample policy.Sample,
	resolution policy.Resolution,
	settings ReservationPolicy,
	explicitCPU int,
	explicitMemoryBytes int64,
) (ReservationPlan, error) {
	if explicitCPU < 0 || explicitMemoryBytes < 0 {
		return ReservationPlan{}, fmt.Errorf("%w: explicit reservations must be nonnegative", ErrReservationReplan)
	}

	cpuCapacity := max(MinimumReservationCPU, sample.AvailableParallelism-1)
	if settings.MaxCPU > 0 {
		cpuCapacity = min(cpuCapacity, settings.MaxCPU)
	}
	memoryLimit := sample.EffectiveMemoryLimitBytes
	if memoryLimit <= 0 {
		memoryLimit = sample.PhysicalMemoryBytes
	}
	memoryCapacity := memoryLimit - resolution.MemoryReserve
	if settings.MaxMemoryBytes > 0 {
		memoryCapacity = min(memoryCapacity, settings.MaxMemoryBytes)
	}
	if memoryCapacity < MinimumReservationMemoryBytes {
		return ReservationPlan{}, fmt.Errorf("%w: safe memory capacity is below 256 MiB", ErrReservationReplan)
	}

	shares := settings.OwnerShares[resolution.ResolvedProfile]
	if shares == 0 {
		switch resolution.ResolvedProfile {
		case "balanced":
			shares = 4
		case "constrained":
			shares = 2
		default:
			shares = 1
		}
	}
	automatic := ReservationVector{
		CPU:         max(MinimumReservationCPU, int(ceilDivide(int64(cpuCapacity), shares))),
		MemoryBytes: max(MinimumReservationMemoryBytes, ceilDivide(memoryCapacity, shares)),
	}
	requested := automatic
	if explicitCPU != 0 {
		requested.CPU = explicitCPU
	}
	if explicitMemoryBytes != 0 {
		requested.MemoryBytes = explicitMemoryBytes
	}
	if requested.CPU < MinimumReservationCPU || requested.MemoryBytes < MinimumReservationMemoryBytes {
		return ReservationPlan{}, fmt.Errorf("%w: reservations require at least one CPU and 256 MiB", ErrReservationReplan)
	}

	capacity := ReservationVector{CPU: cpuCapacity, MemoryBytes: memoryCapacity}
	if requested.CPU > capacity.CPU || requested.MemoryBytes > capacity.MemoryBytes {
		return ReservationPlan{}, fmt.Errorf("%w: requested vector exceeds safe host capacity", ErrReservationReplan)
	}

	return ReservationPlan{Capacity: capacity, Requested: requested, Allocated: requested}, nil
}

func reservationLedgerPath(root string) string {
	return filepath.Join(root, "reservations.json")
}

func reservationIdentityPath(root, value string) (string, error) {
	if !sessionTokenPattern.MatchString(value) {
		return "", errors.New("invalid reservation identity token")
	}

	return filepath.Join(root, "reservation-identities", value+".lock"), nil
}

func reservationIdentityAnchorPath(root, value string) (string, error) {
	path, err := reservationIdentityPath(root, value)
	if err != nil {
		return "", err
	}

	return strings.TrimSuffix(path, ".lock") + ".anchor", nil
}

func openReservationIdentity(root, value string) (*os.File, error) {
	path, err := reservationIdentityPath(root, value)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}

	identity, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(identity.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = identity.Close()
		_ = os.Remove(path)

		return nil, err
	}
	anchor, err := reservationIdentityAnchorPath(root, value)
	if err != nil || os.Link(path, anchor) != nil {
		_ = identity.Close()
		_ = os.Remove(path)
		_ = os.Remove(anchor)
		if err != nil {
			return nil, err
		}

		return nil, errors.New("reservation identity anchor could not be created")
	}
	return identity, nil
}

func reservationIdentityAlive(root, value string, expectedDevice, expectedInode uint64) (bool, error) { //nolint:gocognit // Identity validation deliberately distinguishes stale, live, replaced, and unverifiable kernel state.
	path, err := reservationIdentityPath(root, value)
	if err != nil {
		return false, err
	}
	anchor, err := reservationIdentityAnchorPath(root, value)
	if err != nil {
		return false, err
	}
	var identity *os.File
	var observedDevice, observedInode uint64
	for _, candidate := range []string{path, anchor} {
		opened, openError := os.OpenFile(candidate, os.O_RDWR, 0o600)
		if errors.Is(openError, os.ErrNotExist) {
			continue
		}
		if openError != nil {
			return false, errors.New("reservation identity state is unverifiable")
		}
		var info unix.Stat_t
		if statError := unix.Fstat(int(opened.Fd()), &info); statError != nil {
			_ = opened.Close()

			return false, errors.New("reservation identity state is unverifiable")
		}
		device, deviceError := filesystemIdentifier(info.Dev)
		inode, inodeError := filesystemIdentifier(info.Ino)
		if deviceError != nil || inodeError != nil {
			_ = opened.Close()

			return false, errors.New("reservation identity state is unverifiable")
		}
		if expectedDevice != 0 || expectedInode != 0 {
			if device != expectedDevice || inode != expectedInode {
				_ = opened.Close()
				continue
			}
		} else if identity != nil && (device != observedDevice || inode != observedInode) {
			_ = opened.Close()
			_ = identity.Close()

			return false, errors.New("reservation identity state is unverifiable")
		}
		if identity == nil {
			identity, observedDevice, observedInode = opened, device, inode
		} else {
			_ = opened.Close()
		}
	}
	if identity == nil {
		if expectedDevice != 0 || expectedInode != 0 {
			return false, errors.New("reservation identity state is unverifiable")
		}

		return false, nil
	}
	defer func() { _ = identity.Close() }()
	err = unix.Flock(int(identity.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return true, nil
	}
	if err != nil {
		return false, errors.New("reservation identity state is unverifiable")
	}
	unlockError := unix.Flock(int(identity.Fd()), unix.LOCK_UN)

	return false, unlockError
}

func reservationIdentityMetadata(identity *os.File) (uint64, uint64, error) {
	if identity == nil {
		return 0, 0, errors.New("reservation identity is missing")
	}
	var info unix.Stat_t
	if err := unix.Fstat(int(identity.Fd()), &info); err != nil {
		return 0, 0, err
	}

	device, err := filesystemIdentifier(info.Dev)
	if err != nil {
		return 0, 0, err
	}
	inode, err := filesystemIdentifier(info.Ino)
	if err != nil {
		return 0, 0, err
	}

	return device, inode, nil
}

func ensureReservationIdentityLocked(identity *os.File) error {
	if identity == nil {
		return errors.New("reservation identity is missing")
	}
	if err := unix.Flock(int(identity.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("reservation identity ownership could not be retained")
	}

	return nil
}

func consumeUniqueReservationJSON(decoder *json.Decoder) error {
	tokenValue, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, compound := tokenValue.(json.Delim)
	if !compound {
		return nil
	}

	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyValue, keyError := decoder.Token()
			if keyError != nil {
				return keyError
			}
			key, valid := keyValue.(string)
			if !valid {
				return errors.New("reservation ledger object key must be a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate reservation ledger field %q", key)
			}
			seen[key] = true
			if valueError := consumeUniqueReservationJSON(decoder); valueError != nil {
				return valueError
			}
		}
	case '[':
		for decoder.More() {
			if itemError := consumeUniqueReservationJSON(decoder); itemError != nil {
				return itemError
			}
		}
	}
	_, err = decoder.Token()

	return err
}

func validReservationClass(class policy.TaskClass) bool {
	return class == policy.TaskEphemeral || class == policy.TaskService || class == policy.TaskTransactional
}

func validateReservationVector(name string, vector, capacity ReservationVector) error {
	if vector.CPU < MinimumReservationCPU || vector.MemoryBytes < MinimumReservationMemoryBytes {
		return fmt.Errorf("reservation ledger %s is below immutable floors", name)
	}
	if vector.CPU > capacity.CPU || vector.MemoryBytes > capacity.MemoryBytes {
		return fmt.Errorf("reservation ledger %s exceeds capacity", name)
	}

	return nil
}

func validateReservationLedger(ledger reservationLedger) error { //nolint:cyclop,gocognit,gocyclo // Validation intentionally enumerates every persisted trust-boundary invariant.
	if ledger.SchemaVersion != reservationLedgerSchemaVersion {
		return fmt.Errorf("unsupported reservation ledger schema %d", ledger.SchemaVersion)
	}
	if ledger.Capacity.CPU < 0 || ledger.Capacity.MemoryBytes < 0 {
		return errors.New("reservation ledger capacity must be nonnegative")
	}
	if len(ledger.Owners)+len(ledger.Waiters) > 0 &&
		(ledger.Capacity.CPU < MinimumReservationCPU || ledger.Capacity.MemoryBytes < MinimumReservationMemoryBytes) {
		return errors.New("reservation ledger active capacity is below immutable floors")
	}

	tokens := map[string]bool{}
	sequences := map[uint64]bool{}
	used := ReservationVector{}
	var lastOwnerSequence uint64
	for index, owner := range ledger.Owners {
		if !sessionTokenPattern.MatchString(owner.Token) || tokens[owner.Token] {
			return errors.New("reservation ledger owner token is invalid or duplicated")
		}
		if owner.PID <= 0 {
			return errors.New("reservation ledger owner PID is invalid")
		}
		if !validReservationClass(owner.Class) {
			return errors.New("reservation ledger owner class is invalid")
		}
		if owner.Profile == "" {
			return errors.New("reservation ledger owner profile is invalid")
		}
		if owner.MaxOwners < 1 || owner.MaxOwners > maximumReservationOwners {
			return errors.New("reservation ledger owner maximum is invalid")
		}
		if (owner.IdentityDevice == 0) != (owner.IdentityInode == 0) {
			return errors.New("reservation ledger owner identity metadata is invalid")
		}
		if owner.PeakOwners < 0 || owner.PeakOwners > maximumReservationOwners {
			return errors.New("reservation ledger owner peak is invalid")
		}
		if owner.Sequence == 0 || owner.Sequence > ledger.NextSequence || sequences[owner.Sequence] ||
			index > 0 && owner.Sequence <= lastOwnerSequence {
			return errors.New("reservation ledger owner sequence is invalid")
		}
		if owner.ProcessGroup < 0 || owner.Shedding && owner.ProcessGroup == 0 ||
			owner.Shedding && !validSheddingExitCode(owner.SheddingExit) || !owner.Shedding && owner.SheddingExit != 0 {
			return errors.New("reservation ledger owner process state is invalid")
		}
		if err := validateReservationVector("owner request", owner.Requested, ledger.Capacity); err != nil {
			return err
		}
		if err := validateReservationVector("owner allocation", owner.Allocated, ledger.Capacity); err != nil {
			return err
		}
		if owner.Requested != owner.Allocated {
			return errors.New("reservation ledger owner allocation differs from its immutable request")
		}
		if used.CPU > ledger.Capacity.CPU-owner.Allocated.CPU ||
			used.MemoryBytes > ledger.Capacity.MemoryBytes-owner.Allocated.MemoryBytes {
			return errors.New("reservation ledger allocated totals exceed capacity")
		}
		used.CPU += owner.Allocated.CPU
		used.MemoryBytes += owner.Allocated.MemoryBytes
		tokens[owner.Token], sequences[owner.Sequence], lastOwnerSequence = true, true, owner.Sequence
	}

	lastWaiterSequence := lastOwnerSequence
	for _, waiter := range ledger.Waiters {
		if !sessionTokenPattern.MatchString(waiter.Token) || tokens[waiter.Token] {
			return errors.New("reservation ledger waiter token is invalid or duplicated")
		}
		if waiter.PID <= 0 {
			return errors.New("reservation ledger waiter PID is invalid")
		}
		if !validReservationClass(waiter.Class) {
			return errors.New("reservation ledger waiter class is invalid")
		}
		if waiter.Profile == "" {
			return errors.New("reservation ledger waiter profile is invalid")
		}
		if waiter.MaxOwners < 1 || waiter.MaxOwners > maximumReservationOwners {
			return errors.New("reservation ledger waiter maximum is invalid")
		}
		if (waiter.IdentityDevice == 0) != (waiter.IdentityInode == 0) {
			return errors.New("reservation ledger waiter identity metadata is invalid")
		}
		if waiter.Sequence == 0 || waiter.Sequence > ledger.NextSequence || sequences[waiter.Sequence] ||
			waiter.Sequence <= lastWaiterSequence {
			return errors.New("reservation ledger waiter sequence is invalid or not FIFO")
		}
		if err := validateReservationVector("waiter request", waiter.Requested, ledger.Capacity); err != nil {
			return err
		}
		tokens[waiter.Token], sequences[waiter.Sequence], lastWaiterSequence = true, true, waiter.Sequence
	}

	return nil
}

func initializeMissingReservationLedger(root string) (reservationLedger, error) {
	directory := filepath.Join(root, "reservation-identities")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return reservationLedger{SchemaVersion: reservationLedgerSchemaVersion}, nil
	}
	if err != nil {
		return reservationLedger{}, errors.New("reservation identity state is unverifiable")
	}
	const (
		identityLockEntry = 1 << iota
		identityAnchorEntry
		completeIdentityEntries = identityLockEntry | identityAnchorEntry
	)
	identities := make(map[string]int, len(entries)/2)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			return reservationLedger{}, errors.New("reservation accounting is missing with unverifiable identity state")
		}
		extension := filepath.Ext(name)
		value := strings.TrimSuffix(name, extension)
		if !sessionTokenPattern.MatchString(value) {
			return reservationLedger{}, errors.New("reservation accounting is missing with unverifiable identity state")
		}
		switch extension {
		case ".lock":
			identities[value] |= identityLockEntry
		case ".anchor":
			identities[value] |= identityAnchorEntry
		default:
			return reservationLedger{}, errors.New("reservation accounting is missing with unverifiable identity state")
		}
	}
	tokens := make([]string, 0, len(identities))
	for value, parts := range identities {
		if parts != completeIdentityEntries {
			return reservationLedger{}, errors.New("reservation accounting is missing with unverifiable identity state")
		}
		tokens = append(tokens, value)
	}
	slices.Sort(tokens)
	for _, value := range tokens {
		alive, aliveError := reservationIdentityAlive(root, value, 0, 0)
		if aliveError != nil {
			return reservationLedger{}, aliveError
		}
		if alive {
			return reservationLedger{}, errors.New("reservation accounting is missing while an owner identity remains live")
		}
		if removeError := removeReservationIdentity(root, value); removeError != nil {
			return reservationLedger{}, errors.New("reservation stale identity could not be reconciled")
		}
	}

	return reservationLedger{SchemaVersion: reservationLedgerSchemaVersion}, nil
}

func readReservationLedger(root string) (reservationLedger, error) {
	data, err := os.ReadFile(reservationLedgerPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return initializeMissingReservationLedger(root)
	}
	if err != nil {
		return reservationLedger{}, err
	}

	uniqueDecoder := json.NewDecoder(bytes.NewReader(data))
	if err = consumeUniqueReservationJSON(uniqueDecoder); err != nil {
		return reservationLedger{}, fmt.Errorf("decode reservation ledger: %w", err)
	}
	if _, err = uniqueDecoder.Token(); !errors.Is(err, io.EOF) {
		return reservationLedger{}, errors.New("reservation ledger must contain one JSON value")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var ledger reservationLedger
	if err = decoder.Decode(&ledger); err != nil {
		return reservationLedger{}, fmt.Errorf("decode reservation ledger: %w", err)
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return reservationLedger{}, errors.New("reservation ledger must contain one JSON value")
	}
	if err = validateReservationLedger(ledger); err != nil {
		return reservationLedger{}, err
	}

	return ledger, nil
}

func writeReservationLedger(root string, ledger reservationLedger) (returnError error) {
	temporary, err := os.CreateTemp(root, ".reservations-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		returnError = errors.Join(returnError, temporary.Close())
		if removeError := os.Remove(temporaryPath); !errors.Is(removeError, os.ErrNotExist) {
			returnError = errors.Join(returnError, removeError)
		}
	}()

	if err = temporary.Chmod(0o600); err != nil {
		return err
	}
	if err = json.NewEncoder(temporary).Encode(ledger); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}

	return os.Rename(temporaryPath, reservationLedgerPath(root))
}

func removeReservationIdentity(root, value string) error {
	path, err := reservationIdentityPath(root, value)
	if err != nil {
		return err
	}
	anchor, err := reservationIdentityAnchorPath(root, value)
	if err != nil {
		return err
	}
	var returnError error
	for _, candidate := range []string{path, anchor} {
		if removeError := os.Remove(candidate); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
			returnError = errors.Join(returnError, removeError)
		}
	}

	return returnError
}

func reconcileReservationLedger(root string, ledger *reservationLedger) error {
	staleTokens := make([]string, 0)
	owners := ledger.Owners[:0]
	for _, owner := range ledger.Owners {
		alive, err := reservationIdentityAlive(root, owner.Token, owner.IdentityDevice, owner.IdentityInode)
		if err != nil {
			return err
		}
		if alive {
			owners = append(owners, owner)

			continue
		}
		staleTokens = append(staleTokens, owner.Token)
	}
	ledger.Owners = owners
	waiters := ledger.Waiters[:0]
	for _, waiter := range ledger.Waiters {
		alive, err := reservationIdentityAlive(root, waiter.Token, waiter.IdentityDevice, waiter.IdentityInode)
		if err != nil {
			return err
		}
		if alive {
			waiters = append(waiters, waiter)

			continue
		}
		staleTokens = append(staleTokens, waiter.Token)
	}
	ledger.Waiters = waiters
	if len(staleTokens) == 0 {
		return nil
	}
	// Commit accounting retirement before removing the corresponding identity
	// paths. A failed ledger write must leave the old identity evidence intact;
	// otherwise a later reader would see a live record pointing at missing state.
	if err := writeReservationLedger(root, *ledger); err != nil {
		return errors.New("reservation stale accounting could not be reconciled")
	}
	for _, value := range staleTokens {
		if err := removeReservationIdentity(root, value); err != nil {
			return errors.New("reservation stale identity could not be reconciled")
		}
	}

	return nil
}

func checkedAddReservationVector(total *ReservationVector, addition ReservationVector) error {
	if addition.CPU < 0 || addition.MemoryBytes < 0 || total.CPU > int(^uint(0)>>1)-addition.CPU ||
		total.MemoryBytes > int64(^uint64(0)>>1)-addition.MemoryBytes {
		return errors.New("reservation vector aggregate exceeds representable totals")
	}
	total.CPU += addition.CPU
	total.MemoryBytes += addition.MemoryBytes

	return nil
}

func sumReservationOwners(owners []ReservationOwner) (ReservationVector, error) {
	var total ReservationVector
	for _, owner := range owners {
		if err := checkedAddReservationVector(&total, owner.Allocated); err != nil {
			return ReservationVector{}, err
		}
	}

	return total, nil
}

func vectorFits(used, requested, capacity ReservationVector) bool {
	if used.CPU < 0 || used.MemoryBytes < 0 || requested.CPU < 0 || requested.MemoryBytes < 0 ||
		capacity.CPU < 0 || capacity.MemoryBytes < 0 || used.CPU > capacity.CPU || used.MemoryBytes > capacity.MemoryBytes ||
		requested.CPU > capacity.CPU || requested.MemoryBytes > capacity.MemoryBytes {
		return false
	}

	return used.CPU <= capacity.CPU-requested.CPU &&
		used.MemoryBytes <= capacity.MemoryBytes-requested.MemoryBytes
}

func normalizedOwnerLimit(limit int) int {
	if limit <= 0 || limit > maximumReservationOwners {
		return maximumReservationOwners
	}

	return limit
}

func sharedOwnerLimit(ledger reservationLedger, requested int) int {
	limit := normalizedOwnerLimit(requested)
	for _, owner := range ledger.Owners {
		limit = min(limit, owner.MaxOwners)
	}
	for _, waiter := range ledger.Waiters {
		limit = min(limit, waiter.MaxOwners)
	}

	return limit
}

func ensureReservationCoordination(root string) error {
	marker, present, err := readCoordinationMarker(root)
	if err != nil {
		return err
	}
	if present && marker.Mode == coordinationModeReservation {
		return nil
	}
	if err = pruneSessionRecords(root); err != nil {
		return err
	}
	heavyPath := filepath.Join(root, "heavy.lock")
	_, heavyError := os.Stat(heavyPath)
	heavyActive := heavyError == nil && heavyLeaseHeld(heavyPath)
	if heavyError != nil && !errors.Is(heavyError, os.ErrNotExist) {
		return heavyError
	}
	liveSessions, sessionError := hasLiveSessionRecords(root)
	if sessionError != nil {
		return sessionError
	}
	if liveSessions || heavyActive {
		return fmt.Errorf("%w: exclusive mode has a live or unverifiable owner", errCoordinationDeferred)
	}
	if err = os.RemoveAll(heavyPath); err != nil {
		return compatibilityStateDeferred("stale compatibility heavy state cannot be removed")
	}

	return writeCoordinationMarker(root, coordinationMarker{
		SchemaVersion: coordinationSchemaVersion,
		Mode:          coordinationModeReservation,
	})
}

func updateLedgerCapacity(ledger *reservationLedger, capacity ReservationVector) bool {
	if ledger.Capacity.CPU == 0 || len(ledger.Owners) == 0 && len(ledger.Waiters) == 0 {
		ledger.Capacity = capacity

		return true
	}
	next := ReservationVector{
		CPU:         min(ledger.Capacity.CPU, capacity.CPU),
		MemoryBytes: min(ledger.Capacity.MemoryBytes, capacity.MemoryBytes),
	}
	used, err := sumReservationOwners(ledger.Owners)
	if err != nil {
		return false
	}
	if used.CPU > next.CPU || used.MemoryBytes > next.MemoryBytes {
		return false
	}
	for _, waiter := range ledger.Waiters {
		if waiter.Requested.CPU > next.CPU || waiter.Requested.MemoryBytes > next.MemoryBytes {
			return false
		}
	}
	ledger.Capacity = next

	return true
}

func inheritedReservation(root, candidate string, ledger reservationLedger) (*Session, error) {
	if candidate == "" {
		return nil, nil //nolint:nilnil // Absence of an inheritance candidate is an expected non-error state.
	}
	for _, owner := range ledger.Owners {
		if owner.Token == candidate {
			alive, err := reservationIdentityAlive(root, candidate, owner.IdentityDevice, owner.IdentityInode)
			if err != nil || !alive {
				return nil, err
			}
			return &Session{
				Inherited:  true,
				Token:      candidate,
				Allocation: owner.Allocated,
				Requested:  owner.Requested,
			}, nil
		}
	}

	return nil, nil //nolint:nilnil // A live candidate not present in this ledger is an expected non-error state.
}

func waitForReservationRetry(ctx context.Context, deadline time.Time) error {
	delay := min(coordinationPollInterval, time.Until(deadline))
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// AcquireReservation atomically joins the FIFO queue and acquires both vector dimensions.
func AcquireReservation( //nolint:cyclop,funlen,gocognit,gocyclo,maintidx // One transaction owns FIFO enqueue, reconcile, admission, and bounded cleanup.
	ctx context.Context,
	root, inheritedToken string,
	class policy.TaskClass,
	profile, configHash string,
	plan ReservationPlan,
	maxActiveOwners int,
	wait time.Duration,
) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if wait < 0 {
		return nil, errors.New("reservation wait must be nonnegative")
	}
	if plan.Requested.CPU < MinimumReservationCPU || plan.Requested.MemoryBytes < MinimumReservationMemoryBytes ||
		plan.Requested.CPU > plan.Capacity.CPU || plan.Requested.MemoryBytes > plan.Capacity.MemoryBytes {
		return nil, ErrReservationReplan
	}
	maxActiveOwners = normalizedOwnerLimit(maxActiveOwners)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}

	if inheritedToken != "" {
		coordinationLock, err := acquireCoordinationLock(ctx, root, wait)
		if err != nil {
			return nil, err
		}
		ledger, readError := readReservationLedger(root)
		if readError == nil {
			readError = reconcileReservationLedger(root, &ledger)
		}
		session, inheritError := inheritedReservation(root, inheritedToken, ledger)
		err = errors.Join(readError, inheritError, releaseCoordinationLock(coordinationLock))
		if err != nil {
			return nil, err
		}
		if session != nil {
			return session, nil
		}
	}

	value, err := token()
	if err != nil {
		return nil, err
	}
	var identity *os.File
	queued := false
	defer func() { //nolint:contextcheck // Waiter cleanup runs after caller cancellation and deliberately uses its own bounded context.
		if queued {
			if cleanupError := removeReservationWaiterAfterCancellation(root, value); cleanupError != nil && identity != nil {
				retainReservationWaiterUntilCleanup(root, value, identity)
				identity = nil
			}
		}
		if identity != nil {
			_ = unix.Flock(int(identity.Fd()), unix.LOCK_UN)
			_ = identity.Close()
			_ = removeReservationIdentity(root, value)
		}
	}()

	deadline := time.Now().Add(wait)
	started := time.Now()
	for {
		remaining := max(time.Until(deadline), 0)
		coordinationLock, lockError := acquireCoordinationLock(ctx, root, remaining)
		if lockError != nil {
			return nil, lockError
		}
		if lockError = ensureReservationCoordination(root); lockError != nil {
			_ = releaseCoordinationLock(coordinationLock)

			return nil, lockError
		}
		ledger, readError := readReservationLedger(root)
		if readError != nil {
			_ = releaseCoordinationLock(coordinationLock)

			return nil, readError
		}
		if reconcileError := reconcileReservationLedger(root, &ledger); reconcileError != nil {
			_ = releaseCoordinationLock(coordinationLock)

			return nil, reconcileError
		}
		if identity == nil {
			identity, lockError = openReservationIdentity(root, value)
			if lockError != nil {
				_ = releaseCoordinationLock(coordinationLock)

				return nil, lockError
			}
		}
		identityDevice, identityInode, identityError := reservationIdentityMetadata(identity)
		if identityError != nil {
			_ = releaseCoordinationLock(coordinationLock)

			return nil, identityError
		}
		if !updateLedgerCapacity(&ledger, plan.Capacity) {
			_ = releaseCoordinationLock(coordinationLock)

			return nil, ErrReservationDeferred
		}
		if !queued {
			if ledger.NextSequence == ^uint64(0) {
				if len(ledger.Owners) != 0 || len(ledger.Waiters) != 0 {
					_ = releaseCoordinationLock(coordinationLock)

					return nil, ErrReservationDeferred
				}
				ledger.NextSequence = 0
			}
			ledger.NextSequence++
			ledger.Waiters = append(ledger.Waiters, reservationWaiter{
				Token: value, PID: os.Getpid(), Class: class, Profile: profile,
				Requested: plan.Requested, Sequence: ledger.NextSequence, ConfigHash: configHash,
				MaxOwners: maxActiveOwners, IdentityDevice: identityDevice, IdentityInode: identityInode,
			})
			queued = true
		}

		admitted := false
		var sequence uint64
		used, sumError := sumReservationOwners(ledger.Owners)
		if sumError != nil {
			_ = releaseCoordinationLock(coordinationLock)

			return nil, sumError
		}
		if len(ledger.Waiters) > 0 && ledger.Waiters[0].Token == value &&
			len(ledger.Owners) < sharedOwnerLimit(ledger, maxActiveOwners) && vectorFits(used, plan.Requested, ledger.Capacity) {
			admittedWaiter := ledger.Waiters[0]
			sequence = admittedWaiter.Sequence
			ledger.Waiters = ledger.Waiters[1:]
			activeOwners := len(ledger.Owners) + 1
			for index := range ledger.Owners {
				ledger.Owners[index].PeakOwners = max(ledger.Owners[index].PeakOwners, activeOwners)
			}
			ledger.Owners = append(ledger.Owners, ReservationOwner{
				Token: value, PID: os.Getpid(), Class: class, Profile: profile,
				Requested: plan.Requested, Allocated: plan.Requested,
				Sequence: sequence, ConfigHash: configHash, MaxOwners: maxActiveOwners, PeakOwners: activeOwners,
				IdentityDevice: admittedWaiter.IdentityDevice, IdentityInode: admittedWaiter.IdentityInode,
			})
			admitted = true
		}
		writeError := writeReservationLedger(root, ledger)
		releaseError := releaseCoordinationLock(coordinationLock)
		if err = errors.Join(writeError, releaseError); err != nil {
			return nil, err
		}
		if admitted {
			queued = false
			session := &Session{
				Token: value, Allocation: plan.Requested, Requested: plan.Requested,
				WaitDuration: time.Since(started), identityLock: identity,
			}
			identity = nil

			return session, nil
		}
		if wait == 0 || !time.Now().Before(deadline) {
			return nil, ErrReservationDeferred
		}
		if err = waitForReservationRetry(ctx, deadline); err != nil {
			return nil, err
		}
	}
}

func retainReservationWaiterUntilCleanup(root, value string, identity *os.File) {
	if identity == nil {
		return
	}
	_ = ensureReservationIdentityLocked(identity)
	go func() {
		for {
			if err := ensureReservationIdentityLocked(identity); err != nil {
				time.Sleep(coordinationPollInterval)

				continue
			}
			if err := removeReservationWaiterAfterCancellation(root, value); err == nil {
				_ = identity.Close()
				_ = removeReservationIdentity(root, value)

				return
			}
			time.Sleep(coordinationPollInterval)
		}
	}()
}

func removeReservationWaiterAfterCancellation(root, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), coordinationLifecycleWait)
	defer cancel()

	return removeReservationWaiter(ctx, root, value)
}

func removeReservationWaiter(ctx context.Context, root, value string) (returnError error) {
	lock, err := acquireCoordinationLock(ctx, root, coordinationLifecycleWait)
	if err != nil {
		return err
	}
	defer func() { returnError = errors.Join(returnError, releaseCoordinationLock(lock)) }()

	ledger, err := readReservationLedger(root)
	if err != nil {
		return err
	}
	ledger.Waiters = slices.DeleteFunc(ledger.Waiters, func(waiter reservationWaiter) bool { return waiter.Token == value })

	return writeReservationLedger(root, ledger)
}

// ActivateReservation records the supervised child process group without changing allocation.
func ActivateReservation(root string, session *Session, processGroup int) (returnError error) {
	if session == nil || session.Inherited || processGroup <= 0 {
		return nil
	}
	lock, err := lockCoordinationForRelease(root)
	if err != nil {
		return err
	}
	defer func() { returnError = errors.Join(returnError, releaseCoordinationLock(lock)) }()

	ledger, err := readReservationLedger(root)
	if err != nil {
		return err
	}
	for index := range ledger.Owners {
		if ledger.Owners[index].Token == session.Token {
			ledger.Owners[index].ProcessGroup = processGroup

			return writeReservationLedger(root, ledger)
		}
	}

	return errors.New("reservation owner disappeared before child activation")
}

func releaseReservationIdentity(root string, session *Session) error {
	if session == nil || session.identityLock == nil {
		return nil
	}
	err := session.identityLock.Close()
	session.identityLock = nil

	return errors.Join(err, removeReservationIdentity(root, session.Token))
}

func abandonReservationIdentity(session *Session) error {
	if session == nil || session.identityLock == nil {
		return nil
	}
	err := session.identityLock.Close()
	session.identityLock = nil

	return err
}

// retainReleaseForReconciliation keeps an unreleasable owner mark alive so a
// peer completes the release, and reports shared-lock contention as a deferred
// cleanup rather than a caller failure.
func retainReleaseForReconciliation(root string, session *Session, lockError error) error {
	if retainError := retainReservationOwnerUntilRelease(root, session); retainError != nil {
		return errors.Join(lockError, retainError)
	}
	if errors.Is(lockError, errCoordinationDeferred) {
		return fmt.Errorf("%w: %w", ErrCoordinationCleanupDeferred, lockError)
	}

	return lockError
}

// ReleaseReservation removes exactly one token-authenticated owner and its liveness lock.
func ReleaseReservation(root string, session *Session) error {
	return releaseReservation(root, session, true)
}

func releaseReservation(root string, session *Session, retainOnFailure bool) (returnError error) {
	if session == nil || session.Inherited {
		return nil
	}
	lock, err := lockCoordinationForRelease(root)
	if err != nil {
		if !retainOnFailure || session.identityLock == nil {
			return err
		}

		return retainReleaseForReconciliation(root, session, err)
	}
	defer func() { returnError = errors.Join(returnError, releaseCoordinationLock(lock)) }()

	ledger, err := readReservationLedger(root)
	if err != nil {
		return err
	}
	found := false
	ledger.Owners = slices.DeleteFunc(ledger.Owners, func(owner ReservationOwner) bool {
		if owner.Token == session.Token {
			found = true

			return true
		}

		return false
	})
	if !found {
		return errors.New("refusing to release a reservation owned by another process")
	}
	if err = writeReservationLedger(root, ledger); err != nil {
		return err
	}
	returnError = errors.Join(returnError, releaseReservationIdentity(root, session))
	if len(ledger.Owners) == 0 && len(ledger.Waiters) == 0 {
		returnError = errors.Join(returnError, os.Remove(reservationLedgerPath(root)))
		if marker, present, markerError := readCoordinationMarker(root); markerError != nil {
			returnError = errors.Join(returnError, markerError)
		} else if present && marker.Mode == coordinationModeReservation {
			removeError := os.Remove(coordinationMarkerPath(root))
			if !errors.Is(removeError, os.ErrNotExist) {
				returnError = errors.Join(returnError, removeError)
			}
		}
	}

	return returnError
}

func retainReservationOwnerUntilRelease(root string, session *Session) error {
	retained := &Session{Token: session.Token, identityLock: session.identityLock}
	session.identityLock = nil
	lockError := ensureReservationIdentityLocked(retained.identityLock)
	go func() {
		for retained.identityLock != nil {
			if err := ensureReservationIdentityLocked(retained.identityLock); err != nil {
				time.Sleep(coordinationPollInterval)

				continue
			}
			if err := releaseReservation(root, retained, false); err == nil || retained.identityLock == nil {
				return
			}
			time.Sleep(coordinationPollInterval)
		}
	}()

	return lockError
}

// ReservationStatus reads a reconciled, privacy-safe shared-root summary.
func ReservationStatus(ctx context.Context, root string) (ReservationTotals, error) {
	totals := ReservationTotals{SchemaVersion: 4, Mode: "exclusive"}
	marker, present, err := readCoordinationMarker(root)
	if err != nil || !present || marker.Mode != coordinationModeReservation {
		return totals, err
	}
	totals.Mode = "reservation"
	lock, err := acquireCoordinationLock(ctx, root, coordinationObservationWait)
	if err != nil {
		return ReservationTotals{}, err
	}
	defer releaseCoordinationLock(lock) //nolint:errcheck // Read-only status already has its result.

	ledger, err := readReservationLedger(root)
	if err != nil {
		return ReservationTotals{}, err
	}
	if err = reconcileReservationLedger(root, &ledger); err != nil {
		return ReservationTotals{}, err
	}
	totals.Capacity = ledger.Capacity
	totals.ActiveOwners = len(ledger.Owners)
	totals.WaitingOwners = len(ledger.Waiters)
	for _, owner := range ledger.Owners {
		if err = checkedAddReservationVector(&totals.Allocated, owner.Allocated); err != nil {
			return ReservationTotals{}, err
		}
		switch owner.Class {
		case policy.TaskEphemeral:
			totals.Ephemeral++
		case policy.TaskService:
			totals.Service++
		case policy.TaskTransactional:
			totals.Transactional++
		case policy.TaskRelease:
			// Release workloads use the separate strict release guard.
		}
	}
	for _, waiter := range ledger.Waiters {
		if err = checkedAddReservationVector(&totals.Waiting, waiter.Requested); err != nil {
			return ReservationTotals{}, err
		}
	}

	return totals, nil
}

// ReservationOwnerPeak returns the event-complete peak active-owner count
// retained for one live token without exposing any peer identity.
func ReservationOwnerPeak(ctx context.Context, root string, session *Session) (int, error) {
	if session == nil || session.Inherited {
		return 0, nil
	}
	lock, err := acquireCoordinationLock(ctx, root, coordinationLifecycleWait)
	if err != nil {
		return 0, err
	}
	defer releaseCoordinationLock(lock) //nolint:errcheck // Read-only peak lookup already returns its result.
	ledger, err := readReservationLedger(root)
	if err != nil {
		return 0, err
	}
	if err = reconcileReservationLedger(root, &ledger); err != nil {
		return 0, err
	}
	for _, owner := range ledger.Owners {
		if owner.Token == session.Token {
			return max(owner.PeakOwners, 1), nil
		}
	}

	return 0, errors.New("reservation owner disappeared during peak observation")
}

func reservationVictimPresent(ctx context.Context, root string, victim ReservationOwner) (bool, error) {
	lock, err := acquireCoordinationLock(ctx, root, coordinationLifecycleWait)
	if err != nil {
		return false, err
	}
	finish := func(present bool, outcome error) (bool, error) {
		return present, errors.Join(outcome, releaseCoordinationLock(lock))
	}

	ledger, err := readReservationLedger(root)
	if err != nil {
		return finish(false, err)
	}
	if err = reconcileReservationLedger(root, &ledger); err != nil {
		return finish(false, err)
	}
	for _, owner := range ledger.Owners {
		if owner.Token != victim.Token {
			continue
		}
		if owner.ProcessGroup != victim.ProcessGroup || !owner.Shedding || owner.SheddingExit != victim.SheddingExit {
			return finish(false, errors.New("selected reservation victim ownership changed during shedding"))
		}

		return finish(true, nil)
	}

	return finish(false, nil)
}

func validSheddingExitCode(code int) bool {
	return code == StorageBlockedExitCode || code == CapacityDeferredExitCode
}

// ReservationSheddingSelection returns the privacy-safe exit selected for this owner.
// Only the owning guard uses this mark to stop and reap its own supervised child.
func ReservationSheddingSelection(root string, session *Session) (bool, int, error) {
	if session == nil || session.Inherited {
		return false, 0, nil
	}
	lock, err := lockCoordinationForRelease(root)
	if err != nil {
		return false, 0, err
	}
	finish := func(selected bool, exitCode int, outcome error) (bool, int, error) {
		return selected, exitCode, errors.Join(outcome, releaseCoordinationLock(lock))
	}

	ledger, err := readReservationLedger(root)
	if err != nil {
		return finish(false, 0, err)
	}
	if err = reconcileReservationLedger(root, &ledger); err != nil {
		return finish(false, 0, err)
	}
	for _, owner := range ledger.Owners {
		if owner.Token == session.Token {
			return finish(owner.Shedding, owner.SheddingExit, nil)
		}
	}

	return finish(false, 0, errors.New("reservation owner disappeared during supervision"))
}

// SelectPressureVictim atomically selects at most one newest revocable owner.
func SelectPressureVictim(root string, exitCode int) (ReservationOwner, bool, error) {
	if !validSheddingExitCode(exitCode) {
		return ReservationOwner{}, false, errors.New("reservation shedding exit code is invalid")
	}
	lock, err := acquireCoordinationLock(context.Background(), root, coordinationSelectionWait)
	if err != nil {
		return ReservationOwner{}, false, err
	}
	defer releaseCoordinationLock(lock) //nolint:errcheck // A selection error is returned from ledger mutation.

	ledger, err := readReservationLedger(root)
	if err != nil {
		return ReservationOwner{}, false, err
	}
	if err = reconcileReservationLedger(root, &ledger); err != nil {
		return ReservationOwner{}, false, err
	}
	if slices.ContainsFunc(ledger.Owners, func(owner ReservationOwner) bool { return owner.Shedding }) {
		return ReservationOwner{}, false, nil
	}
	selected := -1
	for _, class := range []policy.TaskClass{policy.TaskEphemeral, policy.TaskService} {
		for index := range ledger.Owners {
			owner := ledger.Owners[index]
			if owner.Class == class && !owner.Shedding && owner.ProcessGroup > 0 &&
				(selected < 0 || owner.Sequence > ledger.Owners[selected].Sequence) {
				selected = index
			}
		}
		if selected >= 0 {
			break
		}
	}
	if selected < 0 {
		return ReservationOwner{}, false, nil
	}
	ledger.Owners[selected].Shedding = true
	ledger.Owners[selected].SheddingExit = exitCode
	if err = writeReservationLedger(root, ledger); err != nil {
		return ReservationOwner{}, false, err
	}

	return ledger.Owners[selected], true, nil
}
