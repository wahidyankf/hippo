package application

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	evidencedomain "github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// RunConfig describes one guarded child process and its resource policy.
type RunConfig struct {
	Command                               string
	Arguments                             []string
	TaskClass                             policy.TaskClass
	WorkingDirectory                      string
	Environment                           []string
	EvidenceRoot                          string
	DiskPath                              string
	LeasePort, LeaseMinimum, LeaseMaximum int
	LeaseOwner                            string
	ConcurrencyEnvironment                []string
	PortLeaseRoot                         string
	Collector                             policy.Collector
	Policy                                policy.Policy
	Resolution                            policy.Resolution
	ReservationPolicy                     coordination.ReservationPolicy
	ReservationPlan                       coordination.ReservationPlan
	ReservationMetadata                   coordination.ReservationMetadata
	EmergencyAvailableMemoryBytes         int64
	EvidenceLimits                        evidencedomain.Limits
	ConfigHash                            string
	Now                                   func() time.Time
	// ObserveChildStatus, when set, is called with the status a started
	// child produced, and only then. It is how the boundary tells a status
	// the child chose from one hippo chose: hippo reports a reason for its
	// own refusals, and passes a child's status through untouched and
	// unexplained, which is the only honest thing to do with it.
	ObserveChildStatus func(int)
	// RetirementConfirmation, when positive, replaces the default window a
	// forced stop waits for the killed process group to retire. A caller that
	// knows its host can stall longer than the default sets it, so the stop is
	// judged by the group's real exit. Zero keeps the default.
	RetirementConfirmation   time.Duration
	Sleep                    func(time.Duration)
	AdmissionPause           func(context.Context, time.Duration) error
	AdmissionHeartbeat       func(coordination.ReservationWaitStatus)
	AdmissionCleanupWait     time.Duration
	ChildStdin               io.Reader
	ChildStdout, ChildStderr io.Writer
	Stderr                   io.Writer
}

// Lease is an opaque ownership handle and its portable allocation metadata.
type Lease struct {
	Token        string
	Inherited    bool
	Allocation   coordination.ReservationVector
	Requested    coordination.ReservationVector
	WaitDuration time.Duration
}

// PortLease is an opaque runtime-owned port reservation handle.
type PortLease struct{ ID string }

// ChildCompletion is the child result translated by the runtime boundary.
type ChildCompletion struct {
	ExitCode int
	Failed   bool
}

// Workload is an opaque owned workload with one typed completion event.
type Workload struct {
	ID   string
	Done <-chan ChildCompletion
}

// RunTicker is a bounded timer capability owned by one run.
type RunTicker struct {
	Ticks <-chan time.Time
	Stop  func()
}

// AdmissionResult is one atomic enqueue/admit observation.
type AdmissionResult struct {
	Lease   *Lease
	Waiting bool
	Status  coordination.ReservationWaitStatus
}

// Admission keeps runtime identity private while application owns retries and deadlines.
type Admission struct {
	Admitted *Lease
	Attempt  func(context.Context, time.Duration, time.Time) (AdmissionResult, error)
	Receipt  func(string, time.Time) error
	Close    func()
}

// RunCoordination performs atomic coordination mutations and bounded observations.
type RunCoordination interface {
	BeginAdmission(ctx context.Context, config RunConfig) (Admission, error)
	Release(config RunConfig, lease *Lease) error
	Abandon(lease *Lease) error
	DescribeHeavy(root string) string
	Snapshot(ctx context.Context, root, mode string) (coordination.ReservationTotals, error)
	OwnerPeak(ctx context.Context, root string, lease *Lease) (int, error)
	SheddingSelection(root string, lease *Lease) (bool, coordination.ShedCause, error)
	SelectVictim(root string, cause coordination.ShedCause, emergency bool) (coordination.ReservationOwner, bool, error)
	VictimPresent(ctx context.Context, root string, victim coordination.ReservationOwner) (bool, error)
}

// WorkloadRuntime owns executable checks, launcher identity, terminal transfer and group retirement.
type WorkloadRuntime interface {
	Normalize(config RunConfig) RunConfig
	ValidateCommand(command string) *status.Failure
	Start(ctx context.Context, config RunConfig, lease *Lease, port *PortLease) (*Workload, error)
	Stop(config RunConfig, workload *Workload) (ChildCompletion, error)
	Activate(config RunConfig, lease *Lease, workload *Workload) error
}

// PortResult is one atomic service-port acquisition observation.
type PortResult struct {
	Lease *PortLease
	Retry bool
}

// PortAdmission keeps concrete port identity at the runtime boundary.
type PortAdmission struct{ Attempt func() (PortResult, error) }

// PortRuntime performs owned service-port identity effects.
type PortRuntime interface {
	BeginPort(config RunConfig) (PortAdmission, error)
	ReleasePort(config RunConfig, port *PortLease) error
	AbandonPort(port *PortLease) error
}

// RunClock owns time and cancellation-aware waiting.
type RunClock interface {
	Now() time.Time
	Wait(ctx context.Context, duration time.Duration, pause func(time.Duration)) error
	Ticker(interval time.Duration) RunTicker
}

// RunSink owns one evidence lifetime without exposing a file or concrete writer.
type RunSink struct {
	RunID                    string
	SetIdentity              func(coordination.ReservationMetadata)
	SetContext               func(policy.Resolution, string)
	SetReservationContext    func(*Lease, int, evidencedomain.BudgetOutcome)
	ObserveReservationOwners func(int)
	Append                   func(policy.Sample) error
	Finalize                 func(policy.TaskClass, evidencedomain.Outcome, int) error
}

// RunEvidence performs bounded evidence effects and classifies refused writes.
type RunEvidence interface {
	Cleanup(root string, now time.Time) error
	Open(config RunConfig) (RunSink, error)
	Receipt(root, runID, state, reason string, metadata coordination.ReservationMetadata, class policy.TaskClass, now time.Time) error
	WriteRefused(err error) bool
}

// RunServices holds the capabilities required to execute the guarded-work use case.
type RunServices struct {
	Coordination RunCoordination
	Workload     WorkloadRuntime
	Ports        PortRuntime
	Evidence     RunEvidence
	Clock        RunClock
}

// ErrChildRetirementUnconfirmed keeps ownership fail closed after an unconfirmed stop.
var ErrChildRetirementUnconfirmed = errors.New("child process-group retirement remained unconfirmed")
