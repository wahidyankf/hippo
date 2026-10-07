package runtime

import coordination "github.com/wahidyankf/hippo/internal/domain/coordination"

// ReservationPolicy is the coordination-domain value at the runtime boundary.
type ReservationPolicy = coordination.ReservationPolicy

// ReservationVector is the coordination-domain value at the runtime boundary.
type ReservationVector = coordination.ReservationVector

// ReservationPlan is the coordination-domain value at the runtime boundary.
type ReservationPlan = coordination.ReservationPlan

// ResourceTierPolicy is the coordination-domain value at the runtime boundary.
type ResourceTierPolicy = coordination.ResourceTierPolicy

// ReservationWaitStatus is the coordination-domain value at the runtime boundary.
type ReservationWaitStatus = coordination.ReservationWaitStatus

// ShedCause is the coordination-domain value at the runtime boundary.
type ShedCause = coordination.ShedCause

// ReservationOwner is the coordination-domain value at the runtime boundary.
type ReservationOwner = coordination.ReservationOwner

// ReservationTotals is the coordination-domain value at the runtime boundary.
type ReservationTotals = coordination.ReservationTotals

// ReservationMetadata is the coordination-domain value at the runtime boundary.
type ReservationMetadata = coordination.ReservationMetadata

// ReservationEntry is the coordination-domain value at the runtime boundary.
type ReservationEntry = coordination.ReservationEntry

type (
	reservationWaiter = coordination.Waiter
	reservationLedger = coordination.Ledger
)

// Domain constants preserve the runtime boundary wire vocabulary.
const (
	MinimumReservationCPU          = coordination.MinimumReservationCPU
	MinimumReservationMemoryBytes  = coordination.MinimumReservationMemoryBytes
	reservationLedgerSchemaVersion = coordination.LedgerSchemaVersion
	maximumReservationOwners       = coordination.MaximumOwners
	ShedCauseNone                  = coordination.ShedCauseNone
	ShedCauseStorage               = coordination.ShedCauseStorage
	ShedCausePressure              = coordination.ShedCausePressure
)

// Domain errors preserve classified coordination identity at the runtime boundary.
var (
	ErrReservationReplan   = coordination.ErrReservationReplan
	ErrReservationDeferred = coordination.ErrReservationDeferred
)
