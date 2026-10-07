// Package coordination owns pure reservation accounting and ledger transitions.
package coordination

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
)

var tokenPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ErrCoordinationProtocolMismatch is valid state whose protocol cannot be joined.
var ErrCoordinationProtocolMismatch = errors.New("shared coordination protocol mismatch")

// ProtocolMismatch names a valid incompatible persisted epoch.
func ProtocolMismatch(reason string) error {
	return fmt.Errorf("%w: %s; drain or upgrade the incompatible client before retrying", ErrCoordinationProtocolMismatch, reason)
}

const (
	// MinimumReservationCPU is the immutable per-owner CPU floor.
	MinimumReservationCPU = 1
	// MinimumReservationMemoryBytes is the immutable per-owner memory floor.
	MinimumReservationMemoryBytes = 256 * policy.MiB
	// LedgerSchemaVersion is the strict persisted reservation protocol.
	LedgerSchemaVersion = 2
	// MaximumOwners is the immutable shared-root owner ceiling.
	MaximumOwners = 20
)

// ErrReservationReplan identifies a request that can never fit the configured safe budget.
var ErrReservationReplan = errors.New("reservation requires replanning")

// ErrReservationDeferred identifies bounded FIFO exhaustion. The guard returns
// it as the internal capacity-deferral status, which callers see as exit 124
// naming hippo.limit.capacity-deferred.
var ErrReservationDeferred = errors.New("reservation capacity remained exhausted")

// ReservationWaitStatus describes one bounded queue heartbeat.
type ReservationWaitStatus struct {
	RunID     string
	Position  int
	Remaining time.Duration
}

// ReservationPolicy configures shared vector admission without naming a consumer repository.
type ReservationPolicy struct {
	Enabled         bool
	MaxCPU          int
	MaxMemoryBytes  int64
	MaxActiveOwners int
	OwnerShares     map[policy.ProfileName]int
	Tiers           map[string]ResourceTierPolicy
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
	Minimum   ReservationVector `json:"minimum,omitzero"`
	Maximum   ReservationVector `json:"maximum,omitzero"`
	Tier      string            `json:"tier,omitempty"`
}

// ResourceTierPolicy bounds one launch-time allocation and its queue wait.
type ResourceTierPolicy struct {
	Minimum       ReservationVector
	Maximum       ReservationVector
	QueueDeadline time.Duration
}

// DefaultResourceTiers returns the schema-3 light, standard, and heavy contract.
func DefaultResourceTiers() map[string]ResourceTierPolicy {
	return map[string]ResourceTierPolicy{
		"light": {
			Minimum:       ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
			Maximum:       ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB},
			QueueDeadline: 30 * time.Minute,
		},
		"standard": {
			Minimum:       ReservationVector{CPU: 2, MemoryBytes: 3 * policy.GiB},
			Maximum:       ReservationVector{CPU: 4, MemoryBytes: 6 * policy.GiB},
			QueueDeadline: 90 * time.Minute,
		},
		"heavy": {
			Minimum:       ReservationVector{CPU: 4, MemoryBytes: 8 * policy.GiB},
			Maximum:       ReservationVector{CPU: 8, MemoryBytes: 16 * policy.GiB},
			QueueDeadline: 4 * time.Hour,
		},
	}
}

// ShedCause is why the reservation ledger marks an owner for shedding, which the
// owner's own guard reads to stop its child. The zero value is ShedCauseNone: the
// owner is not marked. The ledger is shared state that every HIPPO version reads,
// so a cause is the integer v0.8.4 wrote for it and nothing else decodes.
type ShedCause uint8

const (
	// ShedCauseNone is the zero value: no shed.
	ShedCauseNone ShedCause = iota
	// ShedCauseStorage is a shed because the disk floor was crossed.
	ShedCauseStorage
	// ShedCausePressure is a shed under host pressure other than storage.
	ShedCausePressure
)

// Reason is the reason the owner's guard stops with when it sheds for the cause:
// storage keeps its own, and any other pressure is a shed, never the capacity
// deferral whose integer the ledger shares. A value that is no member has none.
func (cause ShedCause) Reason() policy.Reason {
	switch cause {
	case ShedCauseStorage:
		return policy.ReasonStorageBlocked
	case ShedCausePressure:
		return policy.ReasonPressureShed
	case ShedCauseNone:
		// Nothing is shed, so there is nothing to stop for.
	}

	return policy.ReasonNone
}

// LedgerCode is the integer v0.8.4 wrote for the cause in the ledger's
// sheddingExitCode: 73 for storage, and 75 for any other pressure, the integer
// the capacity deferral also carried. This switch is the only table of them.
func (cause ShedCause) LedgerCode() (int, error) {
	switch cause {
	case ShedCauseNone:
		return 0, nil
	case ShedCauseStorage:
		return 73, nil
	case ShedCausePressure:
		return 75, nil
	}

	return 0, fmt.Errorf("shed cause %d is not one the ledger records", uint8(cause))
}

// MarshalJSON writes the integer the ledger records for the cause, and refuses
// a value that is no member.
func (cause ShedCause) MarshalJSON() ([]byte, error) {
	code, err := cause.LedgerCode()
	if err != nil {
		return nil, err
	}

	return json.Marshal(code)
}

// UnmarshalJSON reads 0, 73, or 75 and refuses every other value, so a ledger
// that records a cause this version does not know fails closed before anything
// is admitted or mutated. It asks every possible cause for its integer, so a
// member added to the constants is read back without a second table.
func (cause *ShedCause) UnmarshalJSON(data []byte) error {
	var code int
	if err := json.Unmarshal(data, &code); err != nil {
		return fmt.Errorf("a shedding code is an integer: %w", err)
	}
	for raw := uint8(0); ; raw++ {
		if recorded, err := ShedCause(raw).LedgerCode(); err == nil && recorded == code {
			*cause = ShedCause(raw)

			return nil
		}
		if raw == math.MaxUint8 {
			break
		}
	}

	return fmt.Errorf("sheddingExitCode %d is neither 73 nor 75", code)
}

// ReservationOwner is a privacy-safe shared owner record. PID is diagnostic only;
// ownership liveness is proven by an advisory identity lock rather than PID equality.
type ReservationOwner struct {
	Token          string             `json:"token"`
	PID            int                `json:"pid"`
	Class          policy.TaskClass   `json:"class"`
	Profile        policy.ProfileName `json:"profile"`
	Requested      ReservationVector  `json:"requested"`
	Allocated      ReservationVector  `json:"allocated"`
	Sequence       uint64             `json:"sequence"`
	ConfigHash     string             `json:"configHash,omitempty"`
	ProcessGroup   int                `json:"processGroup,omitempty"`
	Shedding       bool               `json:"shedding,omitempty"`
	SheddingCause  ShedCause          `json:"sheddingExitCode,omitempty"`
	MaxOwners      int                `json:"maxActiveOwners"`
	PeakOwners     int                `json:"peakOwnerCount,omitempty"`
	IdentityDevice uint64             `json:"identityDevice,omitempty"`
	IdentityInode  uint64             `json:"identityInode,omitempty"`
}

// Waiter is one strict FIFO identity and its allocation request.
type Waiter struct {
	Token          string             `json:"token"`
	PID            int                `json:"pid"`
	Class          policy.TaskClass   `json:"class"`
	Profile        policy.ProfileName `json:"profile"`
	Requested      ReservationVector  `json:"requested"`
	Sequence       uint64             `json:"sequence"`
	ConfigHash     string             `json:"configHash,omitempty"`
	MaxOwners      int                `json:"maxActiveOwners"`
	IdentityDevice uint64             `json:"identityDevice,omitempty"`
	IdentityInode  uint64             `json:"identityInode,omitempty"`
}

// Ledger is the portable serialized state of one coordination epoch.
type Ledger struct {
	SchemaVersion int                `json:"schemaVersion"`
	Capacity      ReservationVector  `json:"capacity"`
	NextSequence  uint64             `json:"nextSequence"`
	Owners        []ReservationOwner `json:"owners"`
	Waiters       []Waiter           `json:"waiters"`
}

// ReservationTotals is the schema-stable coordination view exposed by status.
type ReservationTotals struct {
	SchemaVersion int                `json:"schemaVersion"`
	Mode          string             `json:"mode"`
	Capacity      ReservationVector  `json:"capacity"`
	Allocated     ReservationVector  `json:"allocated"`
	Waiting       ReservationVector  `json:"waiting"`
	ActiveOwners  int                `json:"activeOwners"`
	WaitingOwners int                `json:"waitingOwners"`
	Ephemeral     int                `json:"ephemeral"`
	Service       int                `json:"service"`
	Transactional int                `json:"transactional"`
	Owners        []ReservationEntry `json:"owners,omitempty"`
	Waiters       []ReservationEntry `json:"waiters,omitempty"`
	LegacyEntries int                `json:"legacyEntries,omitempty"`
	// AbandonedProcessGroups names payloads still running under a guard that died.
	AbandonedProcessGroups []int `json:"abandonedProcessGroups,omitempty"`
}

// CeilDivide computes positive allocation rounding without overflow.
func CeilDivide(value int64, divisor int) int64 {
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
		shares = resolution.Lineage.DefaultOwnerShares()
	}
	automatic := ReservationVector{
		CPU:         max(MinimumReservationCPU, int(CeilDivide(int64(cpuCapacity), shares))),
		MemoryBytes: max(MinimumReservationMemoryBytes, CeilDivide(memoryCapacity, shares)),
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

// PlanTierReservation prepares a minimum-safe request that may burst once, at
// admission, to the largest vector still available within the selected tier.
func PlanTierReservation(
	sample policy.Sample,
	resolution policy.Resolution,
	settings ReservationPolicy,
	tier string,
	explicitCPU int,
	explicitMemoryBytes int64,
) (ReservationPlan, time.Duration, error) {
	configured, exists := settings.Tiers[tier]
	if !exists {
		return ReservationPlan{}, 0, fmt.Errorf("%w: unknown resource tier %q", ErrReservationReplan, tier)
	}
	base, err := PlanReservation(sample, resolution, settings, 0, 0)
	if err != nil {
		return ReservationPlan{}, 0, err
	}
	minimum, maximum := configured.Minimum, configured.Maximum
	if explicitCPU < 0 || explicitMemoryBytes < 0 {
		return ReservationPlan{}, 0, fmt.Errorf("%w: explicit reservations must be nonnegative", ErrReservationReplan)
	}
	if explicitCPU != 0 {
		if explicitCPU < minimum.CPU || explicitCPU > maximum.CPU {
			return ReservationPlan{}, 0, fmt.Errorf("%w: explicit CPU is outside the %s tier", ErrReservationReplan, tier)
		}
		minimum.CPU, maximum.CPU = explicitCPU, explicitCPU
	}
	if explicitMemoryBytes != 0 {
		if explicitMemoryBytes < minimum.MemoryBytes || explicitMemoryBytes > maximum.MemoryBytes {
			return ReservationPlan{}, 0, fmt.Errorf("%w: explicit memory is outside the %s tier", ErrReservationReplan, tier)
		}
		minimum.MemoryBytes, maximum.MemoryBytes = explicitMemoryBytes, explicitMemoryBytes
	}
	maximum.CPU = min(maximum.CPU, base.Capacity.CPU)
	maximum.MemoryBytes = min(maximum.MemoryBytes, base.Capacity.MemoryBytes)
	if configured.QueueDeadline <= 0 || minimum.CPU < MinimumReservationCPU ||
		minimum.MemoryBytes < MinimumReservationMemoryBytes || maximum.CPU < minimum.CPU ||
		maximum.MemoryBytes < minimum.MemoryBytes || maximum.CPU > base.Capacity.CPU ||
		maximum.MemoryBytes > base.Capacity.MemoryBytes {
		return ReservationPlan{}, 0, fmt.Errorf("%w: %s tier exceeds safe host capacity", ErrReservationReplan, tier)
	}

	return ReservationPlan{
		Capacity: base.Capacity, Requested: minimum, Allocated: minimum,
		Minimum: minimum, Maximum: maximum, Tier: tier,
	}, configured.QueueDeadline, nil
}

// ValidReservationClass admits only supported coordination owner classes.
func ValidReservationClass(class policy.TaskClass) bool {
	switch class {
	case policy.TaskEphemeral, policy.TaskService, policy.TaskTransactional:
		return true
	case policy.TaskRelease:
		return false
	default:
		return false
	}
}

// ValidateReservationVector checks a persisted vector against immutable bounds.
func ValidateReservationVector(name string, vector, capacity ReservationVector) error {
	if vector.CPU < MinimumReservationCPU || vector.MemoryBytes < MinimumReservationMemoryBytes {
		return fmt.Errorf("reservation ledger %s is below immutable floors", name)
	}
	if vector.CPU > capacity.CPU || vector.MemoryBytes > capacity.MemoryBytes {
		return fmt.Errorf("reservation ledger %s exceeds capacity", name)
	}

	return nil
}

// ValidateLedger checks every strict persisted coordination invariant.
func ValidateLedger(ledger Ledger) error { //nolint:cyclop,gocognit,gocyclo // Validation intentionally enumerates every persisted trust-boundary invariant.
	if ledger.SchemaVersion != LedgerSchemaVersion {
		if ledger.SchemaVersion > 0 {
			return ProtocolMismatch(fmt.Sprintf("unsupported reservation ledger schema %d", ledger.SchemaVersion))
		}

		return errors.New("reservation ledger schema must be positive")
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
		if !tokenPattern.MatchString(owner.Token) || tokens[owner.Token] {
			return errors.New("reservation ledger owner token is invalid or duplicated")
		}
		if owner.PID <= 0 {
			return errors.New("reservation ledger owner PID is invalid")
		}
		if !ValidReservationClass(owner.Class) {
			return errors.New("reservation ledger owner class is invalid")
		}
		if owner.Profile == "" {
			return errors.New("reservation ledger owner profile is invalid")
		}
		if owner.MaxOwners < 1 || owner.MaxOwners > MaximumOwners {
			return errors.New("reservation ledger owner maximum is invalid")
		}
		if (owner.IdentityDevice == 0) != (owner.IdentityInode == 0) {
			return errors.New("reservation ledger owner identity metadata is invalid")
		}
		if owner.PeakOwners < 0 || owner.PeakOwners > MaximumOwners {
			return errors.New("reservation ledger owner peak is invalid")
		}
		if owner.Sequence == 0 || owner.Sequence > ledger.NextSequence || sequences[owner.Sequence] ||
			index > 0 && owner.Sequence <= lastOwnerSequence {
			return errors.New("reservation ledger owner sequence is invalid")
		}
		if owner.ProcessGroup < 0 || owner.Shedding && owner.ProcessGroup == 0 ||
			owner.Shedding && owner.SheddingCause == ShedCauseNone || !owner.Shedding && owner.SheddingCause != ShedCauseNone {
			return errors.New("reservation ledger owner process state is invalid")
		}
		if err := ValidateReservationVector("owner request", owner.Requested, ledger.Capacity); err != nil {
			return err
		}
		if err := ValidateReservationVector("owner allocation", owner.Allocated, ledger.Capacity); err != nil {
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
		if !tokenPattern.MatchString(waiter.Token) || tokens[waiter.Token] {
			return errors.New("reservation ledger waiter token is invalid or duplicated")
		}
		if waiter.PID <= 0 {
			return errors.New("reservation ledger waiter PID is invalid")
		}
		if !ValidReservationClass(waiter.Class) {
			return errors.New("reservation ledger waiter class is invalid")
		}
		if waiter.Profile == "" {
			return errors.New("reservation ledger waiter profile is invalid")
		}
		if waiter.MaxOwners < 1 || waiter.MaxOwners > MaximumOwners {
			return errors.New("reservation ledger waiter maximum is invalid")
		}
		if (waiter.IdentityDevice == 0) != (waiter.IdentityInode == 0) {
			return errors.New("reservation ledger waiter identity metadata is invalid")
		}
		if waiter.Sequence == 0 || waiter.Sequence > ledger.NextSequence || sequences[waiter.Sequence] ||
			waiter.Sequence <= lastWaiterSequence {
			return errors.New("reservation ledger waiter sequence is invalid or not FIFO")
		}
		if err := ValidateReservationVector("waiter request", waiter.Requested, ledger.Capacity); err != nil {
			return err
		}
		tokens[waiter.Token], sequences[waiter.Sequence], lastWaiterSequence = true, true, waiter.Sequence
	}

	return nil
}

// CheckedAddVector adds both dimensions after checking integer bounds.
func CheckedAddVector(total *ReservationVector, addition ReservationVector) error {
	if addition.CPU < 0 || addition.MemoryBytes < 0 || total.CPU > int(^uint(0)>>1)-addition.CPU ||
		total.MemoryBytes > int64(^uint64(0)>>1)-addition.MemoryBytes {
		return errors.New("reservation vector aggregate exceeds representable totals")
	}
	total.CPU += addition.CPU
	total.MemoryBytes += addition.MemoryBytes

	return nil
}

// SumOwners totals all live allocations with checked arithmetic.
func SumOwners(owners []ReservationOwner) (ReservationVector, error) {
	var total ReservationVector
	for _, owner := range owners {
		if err := CheckedAddVector(&total, owner.Allocated); err != nil {
			return ReservationVector{}, err
		}
	}

	return total, nil
}

// VectorFits checks both dimensions without overflowing addition.
func VectorFits(used, requested, capacity ReservationVector) bool {
	if used.CPU < 0 || used.MemoryBytes < 0 || requested.CPU < 0 || requested.MemoryBytes < 0 ||
		capacity.CPU < 0 || capacity.MemoryBytes < 0 || used.CPU > capacity.CPU || used.MemoryBytes > capacity.MemoryBytes ||
		requested.CPU > capacity.CPU || requested.MemoryBytes > capacity.MemoryBytes {
		return false
	}

	return used.CPU <= capacity.CPU-requested.CPU &&
		used.MemoryBytes <= capacity.MemoryBytes-requested.MemoryBytes
}

// PlanBounds resolves the fixed or tier-bounded request.
func PlanBounds(plan ReservationPlan) (ReservationVector, ReservationVector, error) {
	minimum, maximum := plan.Minimum, plan.Maximum
	if minimum == (ReservationVector{}) {
		minimum = plan.Requested
	}
	if maximum == (ReservationVector{}) {
		maximum = plan.Requested
	}
	if minimum.CPU < MinimumReservationCPU || minimum.MemoryBytes < MinimumReservationMemoryBytes ||
		maximum.CPU < minimum.CPU || maximum.MemoryBytes < minimum.MemoryBytes ||
		maximum.CPU > plan.Capacity.CPU || maximum.MemoryBytes > plan.Capacity.MemoryBytes {
		return ReservationVector{}, ReservationVector{}, ErrReservationReplan
	}

	return minimum, maximum, nil
}

// LargestAvailableAllocation chooses the greatest vector fitting the available pool.
func LargestAvailableAllocation(used, minimum, maximum, capacity ReservationVector) (ReservationVector, bool) {
	if !VectorFits(used, minimum, capacity) {
		return ReservationVector{}, false
	}
	available := ReservationVector{
		CPU:         capacity.CPU - used.CPU,
		MemoryBytes: capacity.MemoryBytes - used.MemoryBytes,
	}

	return ReservationVector{
		CPU:         min(maximum.CPU, available.CPU),
		MemoryBytes: min(maximum.MemoryBytes, available.MemoryBytes),
	}, true
}

// NormalizedOwnerLimit clamps an unspecified or oversized owner limit.
func NormalizedOwnerLimit(limit int) int {
	if limit <= 0 || limit > MaximumOwners {
		return MaximumOwners
	}

	return limit
}

// SharedOwnerLimit selects the conservative live owner and waiter minimum.
func SharedOwnerLimit(ledger Ledger, requested int) int {
	limit := NormalizedOwnerLimit(requested)
	for _, owner := range ledger.Owners {
		limit = min(limit, owner.MaxOwners)
	}
	for _, waiter := range ledger.Waiters {
		limit = min(limit, waiter.MaxOwners)
	}

	return limit
}

// UpdateCapacity changes the pool only when current owners still fit.
func UpdateCapacity(ledger *Ledger, capacity ReservationVector) bool {
	if ledger.Capacity.CPU == 0 || len(ledger.Owners) == 0 && len(ledger.Waiters) == 0 {
		ledger.Capacity = capacity

		return true
	}
	next := ReservationVector{
		CPU:         min(ledger.Capacity.CPU, capacity.CPU),
		MemoryBytes: min(ledger.Capacity.MemoryBytes, capacity.MemoryBytes),
	}
	used, err := SumOwners(ledger.Owners)
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

// ReservationMetadata is privacy-safe context supplied before queue registration.
type ReservationMetadata struct {
	Source string
	Tags   map[string]string
	Tier   string
}

// ReservationEntry is one privacy-safe live queue or owner row.
type ReservationEntry struct {
	RunID        string             `json:"runId"`
	State        string             `json:"state"`
	Position     int                `json:"position,omitempty"`
	Class        policy.TaskClass   `json:"class"`
	Profile      policy.ProfileName `json:"profile"`
	Source       string             `json:"source,omitempty"`
	Tags         map[string]string  `json:"tags,omitempty"`
	Tier         string             `json:"tier,omitempty"`
	Requested    ReservationVector  `json:"requested"`
	Allocated    ReservationVector  `json:"allocated,omitzero"`
	Minimum      ReservationVector  `json:"minimum,omitzero"`
	Maximum      ReservationVector  `json:"maximum,omitzero"`
	RegisteredAt string             `json:"registeredAt,omitempty"`
	Deadline     string             `json:"deadline,omitempty"`
	Legacy       bool               `json:"legacy,omitempty"`
}

// LeaseMetadata contains only the portable allocation and queue duration of an owner.
type LeaseMetadata struct {
	Requested, Allocation ReservationVector
	WaitDuration          time.Duration
}
