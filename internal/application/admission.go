package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
)

// Acquisition distinguishes ownership from a completed bounded deferral.
type Acquisition struct {
	Lease    *Lease
	Deferred bool
}

// Acquire obtains ownership using application retries around atomic runtime attempts.
func (services RunServices) Acquire(ctx context.Context, config RunConfig) (Acquisition, error) {
	if config.ReservationPolicy.Enabled && config.Now == nil {
		config.Now = services.Clock.Now
	}
	admission, err := services.Coordination.BeginAdmission(ctx, config)
	if err != nil {
		return Acquisition{}, err
	}
	defer admission.Close()
	if admission.Admitted != nil {
		return Acquisition{Lease: admission.Admitted}, nil
	}
	now := services.Clock.Now
	interval := time.Second
	pause := (func(time.Duration))(nil)
	if config.ReservationPolicy.Enabled {
		now = config.Now
		interval = 10 * time.Millisecond
		pause = config.Sleep
	}
	deadline := now().Add(config.Policy.LeaseWait)
	var lastHeartbeat time.Time
	for {
		current := now()
		result, attemptError := admission.Attempt(ctx, max(deadline.Sub(current), 0), current)
		if attemptError != nil {
			return Acquisition{}, attemptError
		}
		if !result.Waiting {
			return Acquisition{Lease: result.Lease}, nil
		}
		current = now()
		if config.Policy.LeaseWait == 0 || !current.Before(deadline) {
			return services.admissionDeadline(config, admission, current)
		}
		lastHeartbeat = admissionHeartbeat(config, result.Status, current, lastHeartbeat)
		duration := min(interval, max(deadline.Sub(current), 0))
		waitError := services.admissionWait(ctx, config, duration, pause)
		if waitError != nil {
			return Acquisition{}, services.admissionCancelled(config, admission, waitError, now())
		}
	}
}

func (services RunServices) admissionDeadline(config RunConfig, admission Admission, current time.Time) (Acquisition, error) {
	if !config.ReservationPolicy.Enabled {
		return Acquisition{Deferred: true}, nil
	}
	if err := admission.Receipt("admission-deadline", current); err != nil {
		return Acquisition{}, services.refusedEvidenceWrite("writing the never-started receipt", err)
	}
	return Acquisition{}, coordination.ErrReservationDeferred
}

func admissionHeartbeat(config RunConfig, status coordination.ReservationWaitStatus, current, last time.Time) time.Time {
	if !config.ReservationPolicy.Enabled || (!last.IsZero() && current.Sub(last) < 30*time.Second) {
		return last
	}
	switch {
	case config.AdmissionHeartbeat != nil:
		config.AdmissionHeartbeat(status)
	case config.Stderr != nil:
		_, _ = fmt.Fprintf(config.Stderr, "HIPPO queued run=%s position=%d remaining=%s.\n", status.RunID, status.Position, status.Remaining.Round(time.Second))
	default:
		return last
	}
	return current
}

func (services RunServices) admissionWait(ctx context.Context, config RunConfig, duration time.Duration, pause func(time.Duration)) error {
	if config.AdmissionPause != nil {
		return config.AdmissionPause(ctx, duration)
	}
	return services.Clock.Wait(ctx, duration, pause)
}

func (services RunServices) admissionCancelled(config RunConfig, admission Admission, cause error, current time.Time) error {
	if !config.ReservationPolicy.Enabled {
		return cause
	}
	if err := admission.Receipt(evidence.OutcomeAdmissionCancelled.String(), current); err != nil {
		return errors.Join(cause, services.refusedEvidenceWrite("writing the never-started receipt", err))
	}
	return cause
}

func (services RunServices) acquire(ctx context.Context, config RunConfig) (*Lease, error) {
	result, err := services.Acquire(ctx, config)
	return result.Lease, err
}
