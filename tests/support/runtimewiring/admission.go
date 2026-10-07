// Package runtimewiring connects test fixtures to application-owned admission
// workflows through the same atomic runtime effects used by production.
package runtimewiring

import (
	"context"
	"time"

	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

// AcquireSession drives application admission for an exclusive or inherited session
// and returns the concrete runtime session for fixture inspection and cleanup.
func AcquireSession(ctx context.Context, root, inherited string, class policy.TaskClass, wait time.Duration) (*runtimeadapter.Session, error) {
	engine := runtimeadapter.NewEngine()
	config := application.RunConfig{EvidenceRoot: root, Environment: []string{"HIPPO_SESSION=" + inherited}, TaskClass: class, Now: time.Now}
	config.Policy.LeaseWait = wait
	lease, err := (application.RunServices{Coordination: engine, Clock: runtimeadapter.RunClock{}, Evidence: engine}).Acquire(ctx, config)
	return engine.TakeSession(lease.Lease), err
}

// AcquireReservation drives application reservation admission with default runtime
// timing and returns the admitted session to the calling fixture.
func AcquireReservation(ctx context.Context, root, inherited string, class policy.TaskClass, profile policy.ProfileName, hash string, plan coordination.ReservationPlan, owners int, wait time.Duration) (*runtimeadapter.Session, error) {
	return AcquireReservationWithOptions(ctx, root, inherited, class, profile, hash, plan, owners, wait, runtimeadapter.ReservationAdmissionOptions{})
}

// AcquireReservationWithOptions wires fixture timing and heartbeat controls into
// application admission while retaining atomic runtime reservation effects.
func AcquireReservationWithOptions(ctx context.Context, root, inherited string, class policy.TaskClass, profile policy.ProfileName, hash string, plan coordination.ReservationPlan, owners int, wait time.Duration, options runtimeadapter.ReservationAdmissionOptions) (*runtimeadapter.Session, error) {
	engine := runtimeadapter.NewEngine()
	config := application.RunConfig{EvidenceRoot: root, Environment: []string{"HIPPO_SESSION=" + inherited}, TaskClass: class, Now: options.Now, ConfigHash: hash, ReservationPlan: plan, ReservationPolicy: coordination.ReservationPolicy{Enabled: true, MaxActiveOwners: owners}, ReservationMetadata: options.Metadata, AdmissionPause: options.Pause, AdmissionHeartbeat: options.Heartbeat, AdmissionCleanupWait: options.CleanupWait}
	if config.Now == nil {
		config.Now = time.Now
	}
	config.Policy.LeaseWait = wait
	config.Resolution.ResolvedProfile = profile
	lease, err := (application.RunServices{Coordination: engine, Clock: runtimeadapter.RunClock{}, Evidence: engine}).Acquire(ctx, config)
	return engine.TakeSession(lease.Lease), err
}

// AcquirePortLease exercises application retry policy through atomic runtime identity effects.
func AcquirePortLease(root string, port int, owner string, minimum, maximum int) (*runtimeadapter.PortLease, error) {
	engine := runtimeadapter.NewEngine()
	lease, err := (application.RunServices{Ports: engine}).AcquirePort(application.RunConfig{PortLeaseRoot: root, LeasePort: port, LeaseOwner: owner, LeaseMinimum: minimum, LeaseMaximum: maximum})
	return engine.TakePort(lease), err
}

// WaitPressureVictimRelease exercises application-owned bounded victim observation.
func WaitPressureVictimRelease(root string, victim coordination.ReservationOwner, timeout time.Duration) error {
	return (application.RunServices{Coordination: runtimeadapter.NewEngine(), Clock: runtimeadapter.RunClock{}}).WaitVictimRelease(context.Background(), root, victim, timeout)
}
