package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// Engine keeps atomic coordination identity and launcher ownership together.
type Engine struct {
	hooks     RunConfig
	leases    map[string]*Session
	ports     map[string]*PortLease
	workloads map[string]*ownedWorkload
	sequence  uint64
}

type ownedWorkload struct {
	lifetime   *supervisedLifetime
	stopExited chan error
	cancel     chan struct{}
	joined     chan struct{}
	cancelOnce sync.Once
	leaseID    string
}

func (workload *ownedWorkload) closeBridge() {
	workload.cancelOnce.Do(func() { close(workload.cancel) })
	<-workload.joined
}

// NewEngine creates an isolated owner for one guarded-work invocation.
func NewEngine() *Engine {
	return &Engine{leases: map[string]*Session{}, ports: map[string]*PortLease{}, workloads: map[string]*ownedWorkload{}}
}

// Input translates the portable portion of a runtime fixture configuration.
func Input(config RunConfig) application.RunConfig {
	return application.RunConfig{
		Command:                       config.Command,
		Arguments:                     config.Arguments,
		TaskClass:                     config.TaskClass,
		WorkingDirectory:              config.WorkingDirectory,
		Environment:                   config.Environment,
		EvidenceRoot:                  config.EvidenceRoot,
		DiskPath:                      config.DiskPath,
		LeasePort:                     config.LeasePort,
		LeaseMinimum:                  config.LeaseMinimum,
		LeaseMaximum:                  config.LeaseMaximum,
		LeaseOwner:                    config.LeaseOwner,
		ConcurrencyEnvironment:        config.ConcurrencyEnvironment,
		PortLeaseRoot:                 config.PortLeaseRoot,
		Collector:                     config.Collector,
		Policy:                        config.Policy,
		Resolution:                    config.Resolution,
		ReservationPolicy:             config.ReservationPolicy,
		ReservationPlan:               config.ReservationPlan,
		ReservationMetadata:           config.ReservationMetadata,
		EmergencyAvailableMemoryBytes: config.EmergencyAvailableMemoryBytes,
		EvidenceLimits:                config.EvidenceLimits,
		ConfigHash:                    config.ConfigHash,
		Now:                           config.Now,
		ObserveChildStatus:            config.ObserveChildStatus,
		RetirementConfirmation:        config.RetirementConfirmation,
		Sleep:                         config.Sleep,
		ChildStdin:                    config.ChildStdin,
		ChildStdout:                   config.ChildStdout,
		ChildStderr:                   config.ChildStderr,
		Stderr:                        config.Stderr,
	}
}

func (engine *Engine) configured(config application.RunConfig) RunConfig {
	result := RunConfig{
		Command:                       config.Command,
		Arguments:                     config.Arguments,
		TaskClass:                     config.TaskClass,
		WorkingDirectory:              config.WorkingDirectory,
		Environment:                   config.Environment,
		EvidenceRoot:                  config.EvidenceRoot,
		DiskPath:                      config.DiskPath,
		LeasePort:                     config.LeasePort,
		LeaseMinimum:                  config.LeaseMinimum,
		LeaseMaximum:                  config.LeaseMaximum,
		LeaseOwner:                    config.LeaseOwner,
		ConcurrencyEnvironment:        config.ConcurrencyEnvironment,
		PortLeaseRoot:                 config.PortLeaseRoot,
		Collector:                     config.Collector,
		Policy:                        config.Policy,
		Resolution:                    config.Resolution,
		ReservationPolicy:             config.ReservationPolicy,
		ReservationPlan:               config.ReservationPlan,
		ReservationMetadata:           config.ReservationMetadata,
		EmergencyAvailableMemoryBytes: config.EmergencyAvailableMemoryBytes,
		EvidenceLimits:                config.EvidenceLimits,
		ConfigHash:                    config.ConfigHash,
		Now:                           config.Now,
		ObserveChildStatus:            config.ObserveChildStatus,
		RetirementConfirmation:        config.RetirementConfirmation,
		Sleep:                         config.Sleep,
		ChildStdin:                    config.ChildStdin,
		ChildStdout:                   config.ChildStdout,
		ChildStderr:                   config.ChildStderr,
		Stderr:                        config.Stderr,
	}
	result.startLifetime = engine.hooks.startLifetime
	result.stopLifetime = engine.hooks.stopLifetime
	return result
}

// Normalize supplies OS streams/environment at the runtime edge and serializes shared writers.
func (engine *Engine) Normalize(config application.RunConfig) application.RunConfig {
	if config.Stderr == nil {
		config.Stderr = os.Stderr
	}
	if config.ChildStdin == nil {
		config.ChildStdin = os.Stdin
	}
	if config.ChildStdout == nil {
		config.ChildStdout = os.Stdout
	}
	if config.ChildStderr == nil {
		config.ChildStderr = os.Stderr
	}
	if config.Environment == nil {
		config.Environment = os.Environ()
	}
	if config.PortLeaseRoot == "" {
		config.PortLeaseRoot = filepath.Join(os.TempDir(), "hippo-port-leases")
	}
	current := engine.configured(config)
	serializeWriters(&current)
	config.Stderr = current.Stderr
	config.ChildStdout = current.ChildStdout
	config.ChildStderr = current.ChildStderr
	return config
}

// ValidateCommand checks executable availability without starting a payload.
func (*Engine) ValidateCommand(command string) *status.Failure {
	return checkCommandIsRunnable(command)
}

// Release drops only this invocation's positively retired ownership.
func (engine *Engine) Release(config application.RunConfig, lease *application.Lease) error {
	if lease == nil {
		return nil
	}
	session := engine.leases[lease.Token]
	for id, workload := range engine.workloads {
		if workload.leaseID == lease.Token {
			workload.closeBridge()
			delete(engine.workloads, id)
		}
	}
	delete(engine.leases, lease.Token)
	if config.ReservationPolicy.Enabled {
		return ReleaseReservation(config.EvidenceRoot, session)
	}
	return ReleaseSession(config.EvidenceRoot, session)
}

// Abandon closes the guard's descriptor while retaining unconfirmed accounting.
func (engine *Engine) Abandon(lease *application.Lease) error {
	if lease == nil {
		return nil
	}
	for id, workload := range engine.workloads {
		if workload.leaseID == lease.Token {
			workload.closeBridge()
			delete(engine.workloads, id)
		}
	}
	session := engine.leases[lease.Token]
	delete(engine.leases, lease.Token)
	return abandonReservationIdentity(session)
}

// DescribeHeavy reports the active compatibility lease safely.
func (*Engine) DescribeHeavy(root string) string { return DescribeHeavyLease(root) }

// ReleasePort releases the owned port identity after confirmed retirement.
func (engine *Engine) ReleasePort(config application.RunConfig, port *application.PortLease) error {
	if port == nil {
		return nil
	}
	lease := engine.ports[port.ID]
	delete(engine.ports, port.ID)
	return ReleasePortLease(config.PortLeaseRoot, lease)
}

// AbandonPort retains an unconfirmed port identity for kernel reconciliation.
func (engine *Engine) AbandonPort(port *application.PortLease) error {
	if port == nil {
		return nil
	}
	lease := engine.ports[port.ID]
	delete(engine.ports, port.ID)
	return abandonPortLeaseIdentity(lease)
}

// Start performs launcher handshakes and controlling-terminal transfer atomically.
func (engine *Engine) Start(ctx context.Context, config application.RunConfig, lease *application.Lease, port *application.PortLease) (*application.Workload, error) {
	session := engine.leases[lease.Token]
	if session == nil {
		return nil, errors.New("owned session is unavailable")
	}
	var portLease *PortLease
	if port != nil {
		portLease = engine.ports[port.ID]
	}
	lifetime, err := launchConfiguredLifetime(ctx, engine.configured(config), session, portLease)
	if lifetime == nil {
		return nil, err
	}
	engine.sequence++
	workloadID := fmt.Sprintf("%s-%d", lease.Token, engine.sequence)
	done := make(chan application.ChildCompletion, 1)
	entry := &ownedWorkload{lifetime: lifetime, stopExited: make(chan error, 1), cancel: make(chan struct{}), joined: make(chan struct{}), leaseID: lease.Token}
	engine.workloads[workloadID] = entry
	go func() {
		defer close(entry.joined)
		select {
		case waitError := <-lifetime.exited:
			entry.stopExited <- waitError
			done <- application.ChildCompletion{ExitCode: waitStatusCode(waitError), Failed: waitError != nil}
		case <-entry.cancel:
		}
	}()
	return &application.Workload{ID: workloadID, Done: done}, err
}

// Stop terminates and reaps only the owned workload and translates its child result.
func (engine *Engine) Stop(config application.RunConfig, workload *application.Workload) (application.ChildCompletion, error) {
	entry := engine.workloads[workload.ID]
	if entry == nil {
		return application.ChildCompletion{}, errors.New("owned workload is unavailable")
	}
	lifetime := *entry.lifetime
	lifetime.exited = entry.stopExited
	waitError, stopError := stopConfiguredLifetime(engine.configured(config), &lifetime)
	if !errors.Is(stopError, errChildRetirementUnconfirmed) {
		entry.closeBridge()
	}
	return application.ChildCompletion{ExitCode: waitStatusCode(waitError), Failed: waitError != nil}, stopError
}

// Activate records the owned child group under the same coordination identity.
func (engine *Engine) Activate(config application.RunConfig, lease *application.Lease, workload *application.Workload) error {
	entry := engine.workloads[workload.ID]
	if entry == nil {
		return errors.New("owned workload is unavailable")
	}
	return ActivateReservation(config.EvidenceRoot, engine.leases[lease.Token], entry.lifetime.processGroup)
}

// Snapshot reconciles shared coordination using the existing serialized path.
func (*Engine) Snapshot(ctx context.Context, root, mode string) (coordination.ReservationTotals, error) {
	totals := coordination.ReservationTotals{SchemaVersion: 5, Mode: mode}
	if root == "" {
		return totals, nil
	}
	active, present, err := ActiveCoordinationMode(root)
	if err != nil {
		return totals, refusedEvidenceWrite("reading coordination evidence", err)
	}
	if !present {
		return totals, nil
	}
	if active != "reservation" {
		snapshot, snapshotError := ExclusiveStatus(ctx, root)
		return snapshot, refusedEvidenceWrite("reading coordination evidence", snapshotError)
	}
	snapshot, snapshotError := ReservationStatus(ctx, root)
	return snapshot, refusedEvidenceWrite("reading coordination evidence", snapshotError)
}

// OwnerPeak observes the owner's aggregate peak under bounded coordination.
func (engine *Engine) OwnerPeak(ctx context.Context, root string, lease *application.Lease) (int, error) {
	return ReservationOwnerPeak(ctx, root, engine.leases[lease.Token])
}

// SheddingSelection observes the owner's own mark before host sampling.
func (engine *Engine) SheddingSelection(root string, lease *application.Lease) (bool, coordination.ShedCause, error) {
	return ReservationSheddingSelection(root, engine.leases[lease.Token])
}

// SelectVictim atomically applies one pure victim election under coordination.
func (*Engine) SelectVictim(root string, cause coordination.ShedCause, emergency bool) (coordination.ReservationOwner, bool, error) {
	if emergency {
		return SelectEmergencyPressureVictim(root, cause)
	}
	return SelectPressureVictim(root, cause)
}

// VictimPresent observes a remote victim once and never signals it.
func (*Engine) VictimPresent(ctx context.Context, root string, victim coordination.ReservationOwner) (bool, error) {
	return reservationVictimPresent(ctx, root, victim)
}

// Cleanup applies the existing private evidence retention policy.
func (*Engine) Cleanup(root string, now time.Time) error { return evidence.Cleanup(root, now) }

// Open opens one bounded writer; the application owns its lifecycle and finalization.
func (*Engine) Open(config application.RunConfig) (application.RunSink, error) {
	writer, err := NewEvidenceWriter(config.EvidenceRoot, EvidenceIdentifier("development-"+string(config.TaskClass), config.Now(), os.Getpid()), config.EvidenceLimits)
	if err != nil {
		return application.RunSink{}, err
	}
	return application.RunSink{
		RunID: writer.RunID(), SetIdentity: writer.SetIdentity, SetContext: writer.SetContext,
		SetReservationContext: func(lease *application.Lease, peak int, outcome evidence.BudgetOutcome) {
			writer.SetReservationContext(&Session{Requested: lease.Requested, Allocation: lease.Allocation, WaitDuration: lease.WaitDuration}, peak, outcome)
		},
		ObserveReservationOwners: writer.ObserveReservationOwners, Append: writer.Append,
		Finalize: func(class policy.TaskClass, outcome evidence.Outcome, health int) error {
			_, err := writer.Finalize(class, outcome, health)
			return err
		},
	}, nil
}

// Receipt records a never-started or safety outcome without exposing persistence.
func (*Engine) Receipt(root, runID, state, reason string, metadata coordination.ReservationMetadata, class policy.TaskClass, now time.Time) error {
	return writeSafetyReceipt(root, runID, state, reason, metadata, class, now)
}

// WriteRefused identifies refused filesystem evidence writes at the adapter boundary.
func (*Engine) WriteRefused(err error) bool { return evidence.WriteRefused(err) }

// RunClock supplies real time and the existing deterministic pause seam.
type RunClock struct{}

// Now returns the current time.
func (RunClock) Now() time.Time { return time.Now() }

// Wait waits cancellably, preserving explicit fixture pauses.
func (RunClock) Wait(ctx context.Context, duration time.Duration, pause func(time.Duration)) error {
	return waitForContext(ctx, duration, pause)
}

// Ticker creates an owned sampling timer.
func (RunClock) Ticker(interval time.Duration) application.RunTicker {
	ticker := time.NewTicker(interval)
	return application.RunTicker{Ticks: ticker.C, Stop: ticker.Stop}
}

// BeginPort validates a port request before exposing atomic attempts.
func (engine *Engine) BeginPort(config application.RunConfig) (application.PortAdmission, error) {
	if err := ValidatePortLeaseRequest(config.LeasePort, config.LeaseOwner, config.LeaseMinimum, config.LeaseMaximum); err != nil {
		return application.PortAdmission{}, err
	}
	if err := os.MkdirAll(config.PortLeaseRoot, 0o700); err != nil {
		return application.PortAdmission{}, err
	}
	return application.PortAdmission{Attempt: func() (application.PortResult, error) {
		lease, retry, err := attemptPortLease(config.PortLeaseRoot, config.LeasePort, config.LeaseOwner)
		if lease == nil {
			return application.PortResult{Retry: retry}, err
		}
		engine.ports[lease.Token] = lease
		return application.PortResult{Lease: &application.PortLease{ID: lease.Token}}, err
	}}, nil
}

// TakePort transfers a positively acquired runtime port identity to an outer owner.
func (engine *Engine) TakePort(lease *application.PortLease) *PortLease {
	if lease == nil {
		return nil
	}
	port := engine.ports[lease.ID]
	delete(engine.ports, lease.ID)
	return port
}
